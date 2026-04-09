package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	openAIPoolMonitorWorkerName           = "worker:openai_pool_monitor"
	openAIPoolMonitorAdvisoryLockID int64 = 74298347005
	openAIProxyProbeWorkerCount           = 20
	openAIProxyProbeTimeout               = 20 * time.Second
)

// OpenAIPoolMonitorWorker periodically checks OpenAI account pool thresholds.
type OpenAIPoolMonitorWorker struct {
	db                        *sql.DB
	timingWheel               *TimingWheelService
	accountRepo               AccountRepository
	proxyRepo                 ProxyRepository
	poolMonitorService        *PoolMonitorService
	alertService              *AlertService
	proxyProber               ProxyExitInfoProber
	connectivitySnapshotStore ProxyConnectivitySnapshotStore

	stopMu  sync.Mutex
	stopped bool

	stateMu                  sync.Mutex
	lastPoolThresholdCheckAt time.Time
	lastProxyProbeCheckAt    time.Time
}

func NewOpenAIPoolMonitorWorker(
	db *sql.DB,
	timingWheel *TimingWheelService,
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	poolMonitorService *PoolMonitorService,
	alertService *AlertService,
	proxyProber ProxyExitInfoProber,
	connectivitySnapshotStore ProxyConnectivitySnapshotStore,
) *OpenAIPoolMonitorWorker {
	return &OpenAIPoolMonitorWorker{
		db:                        db,
		timingWheel:               timingWheel,
		accountRepo:               accountRepo,
		proxyRepo:                 proxyRepo,
		poolMonitorService:        poolMonitorService,
		alertService:              alertService,
		proxyProber:               proxyProber,
		connectivitySnapshotStore: connectivitySnapshotStore,
	}
}

func (w *OpenAIPoolMonitorWorker) Start() {
	if w == nil || w.timingWheel == nil {
		return
	}
	w.scheduleNext(2 * time.Second)
	logger.LegacyPrintf("service.pool_monitor", "[OpenAIPoolMonitorWorker] Started")
}

func (w *OpenAIPoolMonitorWorker) Stop() {
	if w == nil || w.timingWheel == nil {
		return
	}
	w.stopMu.Lock()
	w.stopped = true
	w.stopMu.Unlock()
	w.timingWheel.Cancel(openAIPoolMonitorWorkerName)
	logger.LegacyPrintf("service.pool_monitor", "[OpenAIPoolMonitorWorker] Stopped")
}

func (w *OpenAIPoolMonitorWorker) scheduleNext(delay time.Duration) {
	if delay <= 0 {
		delay = time.Minute
	}
	if w == nil || w.timingWheel == nil {
		return
	}
	w.stopMu.Lock()
	stopped := w.stopped
	w.stopMu.Unlock()
	if stopped {
		return
	}
	w.timingWheel.Schedule(openAIPoolMonitorWorkerName, delay, w.tick)
}

func (w *OpenAIPoolMonitorWorker) tick() {
	interval := w.runOnce()
	w.scheduleNext(interval)
}

func (w *OpenAIPoolMonitorWorker) runOnce() time.Duration {
	if w == nil || w.db == nil || w.accountRepo == nil || w.poolMonitorService == nil {
		return time.Duration(defaultPoolMonitorCheckIntervalMinutes) * time.Minute
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cfg, err := w.poolMonitorService.GetConfig(ctx, PlatformOpenAI)
	if err != nil {
		logger.LegacyPrintf("service.pool_monitor", "[OpenAIPoolMonitorWorker] load config failed: %v", err)
		return time.Duration(defaultPoolMonitorCheckIntervalMinutes) * time.Minute
	}
	poolInterval := normalizePoolMonitorInterval(cfg.CheckIntervalMinutes, defaultPoolMonitorCheckIntervalMinutes)
	proxyProbeInterval := normalizePoolMonitorInterval(cfg.ProxyProbeIntervalMinutes, defaultProxyProbeIntervalMinutes)
	interval := minEnabledMonitorInterval(cfg, poolInterval, proxyProbeInterval)
	now := time.Now()

	shouldRunPool, shouldRunProxy := w.shouldRunMonitorTasks(now, cfg, poolInterval, proxyProbeInterval)
	if !shouldRunPool && !shouldRunProxy {
		return interval
	}

	conn, err := w.db.Conn(ctx)
	if err != nil {
		logger.LegacyPrintf("service.pool_monitor", "[OpenAIPoolMonitorWorker] db conn failed: %v", err)
		return interval
	}
	defer func() { _ = conn.Close() }()

	var locked bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", openAIPoolMonitorAdvisoryLockID).Scan(&locked); err != nil {
		logger.LegacyPrintf("service.pool_monitor", "[OpenAIPoolMonitorWorker] acquire lock failed: %v", err)
		return interval
	}
	if !locked {
		return interval
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", openAIPoolMonitorAdvisoryLockID)
	}()

	ranPool := false
	ranProxy := false
	if shouldRunPool && cfg.PoolThresholdEnabled {
		w.runPoolThresholdCheck(ctx, conn, cfg)
		ranPool = true
	}
	if shouldRunProxy && cfg.ProxyActiveProbeEnabled {
		w.runActiveProxyConnectivityProbe(ctx)
		ranProxy = true
	}
	if ranPool || ranProxy {
		w.markMonitorTasksExecuted(now, ranPool, ranProxy)
	}
	return interval
}

func normalizePoolMonitorInterval(minutes int, fallbackMinutes int) time.Duration {
	if minutes < 1 {
		minutes = fallbackMinutes
	}
	if minutes < 1 {
		minutes = 1
	}
	return time.Duration(minutes) * time.Minute
}

func minEnabledMonitorInterval(cfg *AccountPoolAlertConfig, poolInterval, proxyProbeInterval time.Duration) time.Duration {
	interval := poolInterval
	if cfg != nil && cfg.ProxyActiveProbeEnabled && (!cfg.PoolThresholdEnabled || proxyProbeInterval < interval) {
		interval = proxyProbeInterval
	}
	if interval <= 0 {
		interval = time.Duration(defaultPoolMonitorCheckIntervalMinutes) * time.Minute
	}
	return interval
}

func (w *OpenAIPoolMonitorWorker) shouldRunMonitorTasks(
	now time.Time,
	cfg *AccountPoolAlertConfig,
	poolInterval time.Duration,
	proxyProbeInterval time.Duration,
) (bool, bool) {
	if w == nil || cfg == nil {
		return false, false
	}

	w.stateMu.Lock()
	defer w.stateMu.Unlock()

	runPool := cfg.PoolThresholdEnabled && shouldExecuteByInterval(now, w.lastPoolThresholdCheckAt, poolInterval)
	runProxy := cfg.ProxyActiveProbeEnabled && shouldExecuteByInterval(now, w.lastProxyProbeCheckAt, proxyProbeInterval)
	return runPool, runProxy
}

func shouldExecuteByInterval(now, lastRunAt time.Time, interval time.Duration) bool {
	if interval <= 0 {
		return true
	}
	if lastRunAt.IsZero() {
		return true
	}
	return now.Sub(lastRunAt) >= interval
}

func truncateProbeLogMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	const maxRunes = 200
	runes := []rune(msg)
	if len(runes) <= maxRunes {
		return msg
	}
	return string(runes[:maxRunes]) + "..."
}

func proxyLogAddress(proxy Proxy) string {
	protocol := strings.TrimSpace(proxy.Protocol)
	host := strings.TrimSpace(proxy.Host)
	if protocol == "" && host == "" && proxy.Port <= 0 {
		return ""
	}
	if protocol == "" {
		protocol = "unknown"
	}
	return fmt.Sprintf("%s://%s:%d", protocol, host, proxy.Port)
}

func (w *OpenAIPoolMonitorWorker) markMonitorTasksExecuted(now time.Time, poolExecuted bool, proxyExecuted bool) {
	if w == nil || (!poolExecuted && !proxyExecuted) {
		return
	}
	w.stateMu.Lock()
	defer w.stateMu.Unlock()
	if poolExecuted {
		w.lastPoolThresholdCheckAt = now
	}
	if proxyExecuted {
		w.lastProxyProbeCheckAt = now
	}
}

func (w *OpenAIPoolMonitorWorker) runPoolThresholdCheck(ctx context.Context, conn *sql.Conn, cfg *AccountPoolAlertConfig) {
	if w == nil || conn == nil || cfg == nil {
		return
	}
	availableAccounts, err := w.accountRepo.ListSchedulableByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		logger.LegacyPrintf("service.pool_monitor", "[OpenAIPoolMonitorWorker] list schedulable failed: %v", err)
		return
	}
	availableAccounts = filterAccountsByDisabledProxyScheduleMode(availableAccounts, cfg.DisabledProxyScheduleMode)
	availableCount := len(availableAccounts)

	baseCount, err := w.countOpenAIBaseAccounts(ctx, conn)
	if err != nil {
		logger.LegacyPrintf("service.pool_monitor", "[OpenAIPoolMonitorWorker] count base accounts failed: %v", err)
		return
	}

	evaluation := w.poolMonitorService.EvaluatePoolThreshold(&PoolThresholdAlertConfig{
		Enabled:                 cfg.PoolThresholdEnabled,
		AvailableCountThreshold: cfg.AvailableCountThreshold,
		AvailableRatioThreshold: cfg.AvailableRatioThreshold,
		CheckIntervalMinutes:    cfg.CheckIntervalMinutes,
	}, availableCount, baseCount)
	if !evaluation.Triggered || w.alertService == nil {
		return
	}

	w.alertService.NotifyOpenAIPoolThreshold(ctx, OpenAIPoolThresholdAlert{
		Platform:              PlatformOpenAI,
		AvailableCount:        evaluation.AvailableCount,
		BaseAccountCount:      evaluation.BaseAccountCount,
		AvailableRatioPercent: evaluation.AvailableRatioPct,
		CountThreshold:        cfg.AvailableCountThreshold,
		RatioThreshold:        cfg.AvailableRatioThreshold,
		CountRuleTriggered:    evaluation.CountTriggered,
		RatioRuleTriggered:    evaluation.RatioTriggered,
	})
}

func (w *OpenAIPoolMonitorWorker) runActiveProxyConnectivityProbe(ctx context.Context) {
	if w == nil || w.proxyRepo == nil || w.proxyProber == nil || w.connectivitySnapshotStore == nil {
		return
	}

	proxies, err := w.proxyRepo.ListActive(ctx)
	if err != nil {
		logger.LegacyPrintf("service.pool_monitor", "[OpenAIPoolMonitorWorker] list active proxies failed: %v", err)
		return
	}
	if len(proxies) == 0 {
		logger.LegacyPrintf("service.pool_monitor", "[OpenAIPoolMonitorWorker] proxy connectivity probe skipped: no active proxies")
		return
	}
	logger.LegacyPrintf("service.pool_monitor", "[OpenAIPoolMonitorWorker] proxy connectivity probe started: total=%d", len(proxies))

	workerCount := openAIProxyProbeWorkerCount
	if workerCount <= 0 {
		workerCount = 1
	}
	if len(proxies) < workerCount {
		workerCount = len(proxies)
	}

	jobs := make(chan Proxy, len(proxies))
	var wg sync.WaitGroup
	var probedCount int64
	var reachableCount int64
	var unreachableCount int64
	var snapshotWriteFailedCount int64
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for proxy := range jobs {
				status := w.probeProxyOpenAIConnectivity(ctx, proxy)
				if status == nil {
					continue
				}
				atomic.AddInt64(&probedCount, 1)
				if status.Reachable {
					atomic.AddInt64(&reachableCount, 1)
					logger.LegacyPrintf("service.pool_monitor", "[OpenAIPoolMonitorWorker] proxy connectivity probe result: proxy_id=%d proxy_name=%q proxy_addr=%q reachable=true http_status=%d latency_ms=%d checked_at=%s",
						proxy.ID, proxy.Name, proxyLogAddress(proxy), status.HTTPStatus, status.LatencyMs, status.CheckedAt.Format(time.RFC3339))
				} else {
					atomic.AddInt64(&unreachableCount, 1)
					logger.LegacyPrintf("service.pool_monitor", "[OpenAIPoolMonitorWorker] Warning: proxy connectivity probe result: proxy_id=%d proxy_name=%q proxy_addr=%q reachable=false http_status=%d latency_ms=%d checked_at=%s message=%q",
						proxy.ID, proxy.Name, proxyLogAddress(proxy), status.HTTPStatus, status.LatencyMs, status.CheckedAt.Format(time.RFC3339), truncateProbeLogMessage(status.Message))
				}
				if err := w.connectivitySnapshotStore.UpsertIfNewer(ctx, PlatformConnectivityOpenAI, proxy.ID, *status); err != nil {
					atomic.AddInt64(&snapshotWriteFailedCount, 1)
					logger.LegacyPrintf("service.pool_monitor", "[OpenAIPoolMonitorWorker] save connectivity snapshot failed: proxy_id=%d proxy_name=%q proxy_addr=%q err=%v", proxy.ID, proxy.Name, proxyLogAddress(proxy), err)
				}
			}
		}()
	}

	for i := range proxies {
		jobs <- proxies[i]
	}
	close(jobs)
	wg.Wait()
	logger.LegacyPrintf("service.pool_monitor", "[OpenAIPoolMonitorWorker] proxy connectivity probe finished: total=%d probed=%d reachable=%d unreachable=%d snapshot_write_failed=%d",
		len(proxies),
		atomic.LoadInt64(&probedCount),
		atomic.LoadInt64(&reachableCount),
		atomic.LoadInt64(&unreachableCount),
		atomic.LoadInt64(&snapshotWriteFailedCount),
	)
}

func (w *OpenAIPoolMonitorWorker) probeProxyOpenAIConnectivity(ctx context.Context, proxy Proxy) *PlatformConnectivityStatus {
	if w == nil || w.proxyProber == nil {
		return nil
	}

	probeCtx, cancel := context.WithTimeout(ctx, openAIProxyProbeTimeout)
	defer cancel()

	info, latencyMs, err := w.proxyProber.ProbeProxy(probeCtx, proxy.URL())
	if err != nil {
		return &PlatformConnectivityStatus{
			Reachable: false,
			Message:   err.Error(),
			CheckedAt: time.Now(),
		}
	}
	if info == nil {
		return &PlatformConnectivityStatus{
			Reachable: false,
			Message:   "probe result is empty",
			CheckedAt: time.Now(),
		}
	}
	return &PlatformConnectivityStatus{
		Reachable: true,
		LatencyMs: latencyMs,
		CheckedAt: time.Now(),
	}
}

func (w *OpenAIPoolMonitorWorker) countOpenAIBaseAccounts(ctx context.Context, conn *sql.Conn) (int, error) {
	if conn == nil {
		return 0, fmt.Errorf("nil db conn")
	}
	const query = `
SELECT COUNT(1)
FROM accounts
WHERE deleted_at IS NULL
	AND platform = $1
	AND status = 'active'
	AND schedulable = TRUE
`
	var count int
	if err := conn.QueryRowContext(ctx, query, PlatformOpenAI).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
