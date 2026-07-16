package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestMediaWorkerStartStopIsIdempotent(t *testing.T) {
	worker := newIdleMediaWorkerForLifecycleTest(t)
	require.NoError(t, worker.Start())
	require.NoError(t, worker.Start())
	worker.Stop()
	worker.Stop()
}

func TestMediaWorkerStartStopIsConcurrentSafe(t *testing.T) {
	worker := newIdleMediaWorkerForLifecycleTest(t)

	var starts sync.WaitGroup
	for range 16 {
		starts.Add(1)
		go func() {
			defer starts.Done()
			require.NoError(t, worker.Start())
		}()
	}
	starts.Wait()

	var stops sync.WaitGroup
	for range 16 {
		stops.Add(1)
		go func() {
			defer stops.Done()
			worker.Stop()
		}()
	}
	stops.Wait()
}

func TestMediaWorkerStartEnsureGroupsFailureDoesNotLaunchLoops(t *testing.T) {
	worker := newIdleMediaWorkerForLifecycleTest(t)
	queue := &lifecycleMediaQueue{ensureErr: errors.New("redis unavailable")}
	worker.deps.Queue = queue

	require.ErrorContains(t, worker.Start(), "ensure media worker consumer groups")
	time.Sleep(20 * time.Millisecond)
	require.Zero(t, queue.receiveCalls.Load())
	worker.lifecycleMu.Lock()
	require.False(t, worker.started)
	worker.lifecycleMu.Unlock()

	queue.ensureErr = nil
	require.NoError(t, worker.Start())
	worker.Stop()
}

func TestMediaWorkerStartRestartsConsumerAfterPanic(t *testing.T) {
	worker := newIdleMediaWorkerForLifecycleTest(t)
	queue := &lifecycleMediaQueue{}
	queue.panicReceive.Store(true)
	worker.deps.Queue = queue

	require.NoError(t, worker.Start())
	t.Cleanup(worker.Stop)
	require.Eventually(t, func() bool {
		return queue.receiveCalls.Load() >= 2
	}, time.Second, 10*time.Millisecond)
}

func TestMediaWorkerStartRestartsRecoveryAfterPanic(t *testing.T) {
	worker := newIdleMediaWorkerForLifecycleTest(t)
	repo := &lifecycleMediaTaskRepository{workerTaskRepository: worker.deps.Tasks.(*workerTaskRepository)}
	repo.panicRecover.Store(true)
	worker.deps.Tasks = repo
	worker.cfg.RecoveryInterval = time.Millisecond

	require.NoError(t, worker.Start())
	t.Cleanup(worker.Stop)
	require.Eventually(t, func() bool {
		return repo.recoverCalls.Load() >= 2
	}, time.Second, 10*time.Millisecond)
}

func TestMediaWorkerProviderHonorsEnabledFlag(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
			queue := &lifecycleMediaQueue{}
			cfg := validMediaTaskLifecycleConfig(enabled)

			worker, err := ProvideMediaWorker(
				queue,
				fixture.repo,
				fixture.artifactWriter,
				fixture.worker.deps.Scheduler,
				fixture.worker.deps.Models,
				fixture.worker.deps.Adapters,
				fixture.worker.deps.Precharger,
				fixture.billing,
				fixture.metrics,
				cfg,
			)
			require.NoError(t, err)
			require.NotNil(t, worker)
			if enabled {
				require.Equal(t, int64(1), queue.ensureCalls.Load())
				worker.Stop()
			} else {
				require.Zero(t, queue.ensureCalls.Load())
			}
		})
	}
}

func TestMediaWorkerConfigUsesEveryDeploymentRuntimeValue(t *testing.T) {
	cfg := validMediaTaskLifecycleConfig(true)
	workerCfg := mediaWorkerConfigFrom(cfg)

	require.Equal(t, 3, workerCfg.WorkerCount)
	require.Equal(t, 11*time.Second, workerCfg.TaskTimeout)
	require.Equal(t, 7*time.Second, workerCfg.LeaseTTL)
	require.Equal(t, 2*time.Second, workerCfg.LeaseRenewInterval)
	require.Equal(t, 5*time.Second, workerCfg.PollInterval)
	require.Equal(t, 13*time.Second, workerCfg.RecoveryInterval)
	require.Equal(t, 17, workerCfg.RecoveryBatchSize)
	require.Equal(t, 19*time.Millisecond, workerCfg.StreamBlock)
}

func validMediaTaskLifecycleConfig(enabled bool) *config.Config {
	return &config.Config{MediaTasks: config.MediaTaskConfig{
		Enabled:                    enabled,
		WorkerCount:                3,
		TaskTimeoutSeconds:         11,
		LeaseTTLSeconds:            7,
		LeaseRenewIntervalSeconds:  2,
		PollIntervalSeconds:        5,
		RecoveryIntervalSeconds:    13,
		RecoveryBatchSize:          17,
		StreamBlockMilliseconds:    19,
		ContentProxyTimeoutSeconds: 23,
		MaxContentBytes:            29,
	}}
}

func newIdleMediaWorkerForLifecycleTest(t *testing.T) *MediaWorker {
	t.Helper()
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.worker.cfg.WorkerCount = 1
	fixture.worker.cfg.PollInterval = 5 * time.Millisecond
	fixture.worker.cfg.RecoveryInterval = time.Hour
	return fixture.worker
}

type lifecycleMediaQueue struct {
	workerQueue
	ensureErr    error
	ensureCalls  atomic.Int64
	receiveCalls atomic.Int64
	panicReceive atomic.Bool
}

func (q *lifecycleMediaQueue) EnsureGroups(context.Context) error {
	q.ensureCalls.Add(1)
	return q.ensureErr
}

func (q *lifecycleMediaQueue) Receive(ctx context.Context, block time.Duration) (*MediaQueueMessage, error) {
	q.receiveCalls.Add(1)
	if q.panicReceive.CompareAndSwap(true, false) {
		panic("consumer loop panic")
	}
	timer := time.NewTimer(block)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, ErrMediaQueueReceiveTimeout
	}
}

type lifecycleMediaTaskRepository struct {
	*workerTaskRepository
	panicRecover atomic.Bool
	recoverCalls atomic.Int64
}

func (r *lifecycleMediaTaskRepository) ListRecoverable(ctx context.Context, now time.Time, limit int) ([]MediaTask, error) {
	r.recoverCalls.Add(1)
	if r.panicRecover.CompareAndSwap(true, false) {
		panic("recovery loop panic")
	}
	return r.workerTaskRepository.ListRecoverable(ctx, now, limit)
}
