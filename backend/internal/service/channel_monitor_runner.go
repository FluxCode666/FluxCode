package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/alitto/pond/v2"
)

type MonitorScheduler interface {
	Schedule(m *ChannelMonitor)
	Unschedule(id int64)
}

type monitorRunnerSvc interface {
	ListEnabledMonitors(ctx context.Context) ([]*ChannelMonitor, error)
	RunCheck(ctx context.Context, id int64) ([]*CheckResult, error)
}

type ChannelMonitorRunner struct {
	svc             monitorRunnerSvc
	runtimeProvider ChannelMonitorRuntimeProvider

	pool         pond.Pool
	parentCtx    context.Context
	parentCancel context.CancelFunc

	mu      sync.Mutex
	tasks   map[int64]*scheduledMonitor
	wg      sync.WaitGroup
	started bool
	stopped bool

	inFlight   map[int64]struct{}
	inFlightMu sync.Mutex
}

type scheduledMonitor struct {
	id       int64
	name     string
	interval time.Duration
	cancel   context.CancelFunc
}

func NewChannelMonitorRunner(svc *ChannelMonitorService, runtimeProvider ChannelMonitorRuntimeProvider) *ChannelMonitorRunner {
	return newChannelMonitorRunner(svc, runtimeProvider)
}

func newChannelMonitorRunner(svc monitorRunnerSvc, runtimeProvider ChannelMonitorRuntimeProvider) *ChannelMonitorRunner {
	ctx, cancel := context.WithCancel(context.Background())
	return &ChannelMonitorRunner{
		svc:             svc,
		runtimeProvider: runtimeProvider,
		pool:            pond.NewPool(monitorWorkerConcurrency),
		parentCtx:       ctx,
		parentCancel:    cancel,
		tasks:           make(map[int64]*scheduledMonitor),
		inFlight:        make(map[int64]struct{}),
	}
}

func (r *ChannelMonitorRunner) canRun(ctx context.Context) bool {
	if r == nil || r.runtimeProvider == nil {
		return false
	}
	return r.runtimeProvider.GetChannelMonitorRuntime(ctx).Enabled
}

func (r *ChannelMonitorRunner) Start() {
	if r == nil || r.svc == nil {
		return
	}
	r.mu.Lock()
	if r.started || r.stopped {
		r.mu.Unlock()
		return
	}
	r.started = true
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), monitorStartupLoadTimeout)
	defer cancel()
	enabled, err := r.svc.ListEnabledMonitors(ctx)
	if err != nil {
		slog.Error("channel_monitor: load enabled monitors failed at startup", "error", err)
		return
	}
	for _, m := range enabled {
		r.Schedule(m)
	}
	slog.Info("channel_monitor: runner started", "scheduled_tasks", len(enabled))
}

func (r *ChannelMonitorRunner) Schedule(m *ChannelMonitor) {
	if r == nil || m == nil {
		return
	}
	if !m.Enabled {
		r.Unschedule(m.ID)
		return
	}
	interval := time.Duration(m.IntervalSeconds) * time.Second
	if interval <= 0 {
		slog.Error("channel_monitor: skip schedule for invalid interval", "monitor_id", m.ID, "interval_seconds", m.IntervalSeconds)
		return
	}

	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	if !r.started {
		r.mu.Unlock()
		slog.Warn("channel_monitor: schedule before runner started, skip", "monitor_id", m.ID, "name", m.Name)
		return
	}
	if existing, ok := r.tasks[m.ID]; ok {
		existing.cancel()
	}
	ctx, cancel := context.WithCancel(r.parentCtx)
	task := &scheduledMonitor{
		id:       m.ID,
		name:     m.Name,
		interval: interval,
		cancel:   cancel,
	}
	r.tasks[m.ID] = task
	r.wg.Add(1)
	r.mu.Unlock()

	go r.runScheduled(ctx, task)
}

func (r *ChannelMonitorRunner) Unschedule(id int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	task, ok := r.tasks[id]
	if ok {
		delete(r.tasks, id)
	}
	r.mu.Unlock()
	if ok {
		task.cancel()
	}
}

func (r *ChannelMonitorRunner) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	r.stopped = true
	r.parentCancel()
	r.tasks = nil
	r.mu.Unlock()

	r.wg.Wait()
	r.pool.StopAndWait()
}

func (r *ChannelMonitorRunner) runScheduled(ctx context.Context, task *scheduledMonitor) {
	defer r.wg.Done()

	r.fire(ctx, task)
	ticker := time.NewTicker(task.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.fire(ctx, task)
		}
	}
}

func (r *ChannelMonitorRunner) fire(ctx context.Context, task *scheduledMonitor) {
	if !r.canRun(ctx) {
		return
	}
	if !r.tryAcquireInFlight(task.id) {
		slog.Debug("channel_monitor: skip already in-flight", "monitor_id", task.id, "name", task.name)
		return
	}
	if _, ok := r.pool.TrySubmit(func() {
		r.runOne(task.id, task.name)
	}); !ok {
		r.releaseInFlight(task.id)
		slog.Warn("channel_monitor: worker pool full, skip submission", "monitor_id", task.id, "name", task.name)
	}
}

func (r *ChannelMonitorRunner) tryAcquireInFlight(id int64) bool {
	r.inFlightMu.Lock()
	defer r.inFlightMu.Unlock()
	if _, exists := r.inFlight[id]; exists {
		return false
	}
	r.inFlight[id] = struct{}{}
	return true
}

func (r *ChannelMonitorRunner) releaseInFlight(id int64) {
	r.inFlightMu.Lock()
	delete(r.inFlight, id)
	r.inFlightMu.Unlock()
}

func (r *ChannelMonitorRunner) runOne(id int64, name string) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorRequestTimeout+monitorPingTimeout+monitorRunOneBuffer)
	defer cancel()
	defer r.releaseInFlight(id)
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("channel_monitor: runner panic", "monitor_id", id, "name", name, "panic", rec)
		}
	}()

	if _, err := r.svc.RunCheck(ctx, id); err != nil {
		slog.Warn("channel_monitor: run check failed", "monitor_id", id, "name", name, "error", err)
	}
}
