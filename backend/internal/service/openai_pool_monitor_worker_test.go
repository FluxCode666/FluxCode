package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type workerProxyRepoStub struct {
	ProxyRepository
	proxies []Proxy
	err     error
}

func (s *workerProxyRepoStub) ListActive(ctx context.Context) ([]Proxy, error) {
	_ = ctx
	if s.err != nil {
		return nil, s.err
	}
	out := make([]Proxy, len(s.proxies))
	copy(out, s.proxies)
	return out, nil
}

type workerProbeStub struct {
	mu        sync.Mutex
	latency   map[string]int64
	callCount map[string]int
}

func (s *workerProbeStub) ProbeProxy(ctx context.Context, proxyURL string) (*ProxyExitInfo, int64, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.callCount == nil {
		s.callCount = map[string]int{}
	}
	s.callCount[proxyURL]++
	if s.latency != nil {
		if latency, ok := s.latency[proxyURL]; ok {
			return &ProxyExitInfo{}, latency, nil
		}
	}
	return &ProxyExitInfo{}, 0, nil
}

type workerSnapshotStoreStub struct {
	mu    sync.Mutex
	items map[int64]PlatformConnectivityStatus
}

func (s *workerSnapshotStoreStub) GetByProxyIDs(ctx context.Context, platform string, proxyIDs []int64) (map[int64]PlatformConnectivityStatus, error) {
	_ = ctx
	_ = platform
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[int64]PlatformConnectivityStatus{}
	for _, proxyID := range proxyIDs {
		if snapshot, ok := s.items[proxyID]; ok {
			out[proxyID] = snapshot
		}
	}
	return out, nil
}

func (s *workerSnapshotStoreStub) UpsertIfNewer(ctx context.Context, platform string, proxyID int64, snapshot PlatformConnectivityStatus) error {
	_ = ctx
	_ = platform
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = map[int64]PlatformConnectivityStatus{}
	}
	s.items[proxyID] = snapshot
	return nil
}

func TestOpenAIPoolMonitorWorker_RunActiveProxyConnectivityProbe_WritesSnapshots(t *testing.T) {
	proxies := []Proxy{
		{ID: 1, Protocol: "http", Host: "127.0.0.1", Port: 18081},
		{ID: 2, Protocol: "http", Host: "127.0.0.1", Port: 18082},
	}
	proxyRepo := &workerProxyRepoStub{proxies: proxies}
	prober := &workerProbeStub{latency: map[string]int64{
		proxies[0].URL(): 11,
		proxies[1].URL(): 22,
	}}
	store := &workerSnapshotStoreStub{}

	worker := &OpenAIPoolMonitorWorker{
		proxyRepo:                 proxyRepo,
		proxyProber:               prober,
		connectivitySnapshotStore: store,
	}

	worker.runActiveProxyConnectivityProbe(context.Background())

	require.Len(t, store.items, 2)
	require.Equal(t, 1, prober.callCount[proxies[0].URL()])
	require.Equal(t, 1, prober.callCount[proxies[1].URL()])
	require.True(t, store.items[1].Reachable)
	require.Equal(t, int64(11), store.items[1].LatencyMs)
	require.True(t, store.items[2].Reachable)
	require.Equal(t, int64(22), store.items[2].LatencyMs)
}

func TestOpenAIPoolMonitorWorker_ShouldRunMonitorTasks_IndependentIntervals(t *testing.T) {
	worker := &OpenAIPoolMonitorWorker{}
	cfg := &AccountPoolAlertConfig{
		PoolThresholdEnabled:    true,
		ProxyActiveProbeEnabled: true,
	}
	poolInterval := 10 * time.Minute
	proxyInterval := 2 * time.Minute

	start := time.Unix(1_700_000_000, 0)
	runPool, runProxy := worker.shouldRunMonitorTasks(start, cfg, poolInterval, proxyInterval)
	require.True(t, runPool)
	require.True(t, runProxy)

	worker.markMonitorTasksExecuted(start, true, true)

	runPool, runProxy = worker.shouldRunMonitorTasks(start.Add(2*time.Minute), cfg, poolInterval, proxyInterval)
	require.False(t, runPool)
	require.True(t, runProxy)

	worker.markMonitorTasksExecuted(start.Add(2*time.Minute), false, true)

	runPool, runProxy = worker.shouldRunMonitorTasks(start.Add(10*time.Minute), cfg, poolInterval, proxyInterval)
	require.True(t, runPool)
	require.True(t, runProxy)
}

func TestOpenAIPoolMonitorWorker_ShouldRunMonitorTasks_ProxyOnly(t *testing.T) {
	worker := &OpenAIPoolMonitorWorker{}
	cfg := &AccountPoolAlertConfig{
		PoolThresholdEnabled:    false,
		ProxyActiveProbeEnabled: true,
	}
	proxyInterval := 2 * time.Minute
	now := time.Unix(1_700_000_000, 0)

	runPool, runProxy := worker.shouldRunMonitorTasks(now, cfg, 10*time.Minute, proxyInterval)
	require.False(t, runPool)
	require.True(t, runProxy)

	worker.markMonitorTasksExecuted(now, false, true)

	runPool, runProxy = worker.shouldRunMonitorTasks(now.Add(1*time.Minute), cfg, 10*time.Minute, proxyInterval)
	require.False(t, runPool)
	require.False(t, runProxy)

	runPool, runProxy = worker.shouldRunMonitorTasks(now.Add(2*time.Minute), cfg, 10*time.Minute, proxyInterval)
	require.False(t, runPool)
	require.True(t, runProxy)
}

func TestOpenAIPoolMonitorWorker_MinEnabledMonitorInterval(t *testing.T) {
	cfg := &AccountPoolAlertConfig{
		PoolThresholdEnabled:      true,
		ProxyActiveProbeEnabled:   true,
		CheckIntervalMinutes:      10,
		ProxyProbeIntervalMinutes: 2,
	}
	poolInterval := normalizePoolMonitorInterval(cfg.CheckIntervalMinutes, defaultPoolMonitorCheckIntervalMinutes)
	proxyInterval := normalizePoolMonitorInterval(cfg.ProxyProbeIntervalMinutes, defaultProxyProbeIntervalMinutes)
	require.Equal(t, 2*time.Minute, minEnabledMonitorInterval(cfg, poolInterval, proxyInterval))

	cfg.ProxyActiveProbeEnabled = false
	require.Equal(t, 10*time.Minute, minEnabledMonitorInterval(cfg, poolInterval, proxyInterval))

	cfg.PoolThresholdEnabled = false
	cfg.ProxyActiveProbeEnabled = true
	require.Equal(t, 2*time.Minute, minEnabledMonitorInterval(cfg, poolInterval, proxyInterval))
}
