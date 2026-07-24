package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/dgraph-io/ristretto"
)

var (
	ErrSchedulerCacheNotReady   = errors.New("scheduler cache not ready")
	ErrSchedulerFallbackLimited = errors.New("scheduler db fallback limited")
)

const outboxEventTimeout = 2 * time.Minute

// batchSeenKey tracks which (groupID, platform) bucket sets have already been
// rebuilt within a single pollOutbox call, to avoid redundant work when multiple
// account_changed events share the same groups.
type batchSeenKey struct {
	groupID  int64
	platform string
}

var ErrSchedulerRebuildInProgress = errors.New("scheduler rebuild already in progress")

type SchedulerSnapshotService struct {
	cache         SchedulerCache
	outboxQueue   SchedulerOutboxQueue
	accountRepo   AccountRepository
	groupRepo     GroupRepository
	cfg           *config.Config
	stopCh        chan struct{}
	stopOnce      sync.Once
	stopCancel    context.CancelFunc // 用于中断 BLOCK 读取
	wg            sync.WaitGroup
	fallbackLimit *fallbackLimiter
	lagMu         sync.Mutex
	lagFailures   int
	rebuilding    atomic.Bool // 防止手动重建重复触发

	// L1 进程内缓存：避免每次请求从 Redis 加载全量账号池
	snapshotL1    *ristretto.Cache // key: bucket.String() -> []Account
	snapshotL1TTL time.Duration
	accountL1     *ristretto.Cache // key: accountID (int64) -> *Account
	accountL1TTL  time.Duration
}

func NewSchedulerSnapshotService(
	cache SchedulerCache,
	outboxQueue SchedulerOutboxQueue,
	accountRepo AccountRepository,
	groupRepo GroupRepository,
	cfg *config.Config,
) *SchedulerSnapshotService {
	maxQPS := 0
	if cfg != nil {
		maxQPS = cfg.Gateway.Scheduling.DbFallbackMaxQPS
	}
	svc := &SchedulerSnapshotService{
		cache:         cache,
		outboxQueue:   outboxQueue,
		accountRepo:   accountRepo,
		groupRepo:     groupRepo,
		cfg:           cfg,
		stopCh:        make(chan struct{}),
		stopCancel:    func() {}, // 初始为空操作，Start 时覆盖
		fallbackLimit: newFallbackLimiter(maxQPS),
	}
	svc.initL1Cache(cfg)
	return svc
}

// initL1Cache 初始化进程内 L1 缓存
func (s *SchedulerSnapshotService) initL1Cache(cfg *config.Config) {
	if cfg == nil {
		return
	}
	sc := cfg.Gateway.Scheduling

	// 快照 L1 缓存
	if sc.SnapshotL1Size > 0 && sc.SnapshotL1TTLSeconds > 0 {
		cache, err := ristretto.NewCache(&ristretto.Config{
			NumCounters: int64(sc.SnapshotL1Size) * 10,
			MaxCost:     int64(sc.SnapshotL1Size),
			BufferItems: 64,
		})
		if err != nil {
			log.Printf("[SchedulerSnapshot] failed to create snapshot L1 cache: %v", err)
		} else {
			s.snapshotL1 = cache
			s.snapshotL1TTL = time.Duration(sc.SnapshotL1TTLSeconds) * time.Second
		}
	}

	// 单账号 L1 缓存
	if sc.SnapshotAccountL1Size > 0 && sc.SnapshotAccountL1TTLSeconds > 0 {
		cache, err := ristretto.NewCache(&ristretto.Config{
			NumCounters: int64(sc.SnapshotAccountL1Size) * 10,
			MaxCost:     int64(sc.SnapshotAccountL1Size),
			BufferItems: 64,
		})
		if err != nil {
			log.Printf("[SchedulerSnapshot] failed to create account L1 cache: %v", err)
		} else {
			s.accountL1 = cache
			s.accountL1TTL = time.Duration(sc.SnapshotAccountL1TTLSeconds) * time.Second
		}
	}
}

func (s *SchedulerSnapshotService) Start() {
	if s == nil || s.cache == nil {
		return
	}

	// initialDone 用于让 outbox/fullRebuild worker 等初始化重建完成后再启动，
	// 避免启动时多个 goroutine 同时密集查询 DB 导致连接池耗尽。
	// 扩容实例（skip_startup_init=true）跳过全量重建，直接复用已有 Redis 缓存。
	initialDone := make(chan struct{})

	if s.cfg != nil && s.cfg.SkipStartupInit {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] skip_startup_init=true, skipping initial cache rebuild")
		close(initialDone)
	} else {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.runInitialRebuild()
			close(initialDone)
		}()
	}

	interval := s.outboxPollInterval()
	if s.outboxQueue != nil && interval > 0 {
		workerCtx, workerCancel := context.WithCancel(context.Background())
		s.stopCancel = workerCancel
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			select {
			case <-initialDone:
			case <-s.stopCh:
				return
			}
			s.runOutboxWorker(workerCtx, interval)
		}()
	}

	fullInterval := s.fullRebuildInterval()
	if fullInterval > 0 {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			select {
			case <-initialDone:
			case <-s.stopCh:
				return
			}
			s.runFullRebuildWorker(fullInterval)
		}()
	}
}

func (s *SchedulerSnapshotService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.stopCancel() // 中断 BLOCK 读取
	})
	s.wg.Wait()
}

// TriggerManualRebuild 手动触发全量重建。如果已有重建在进行中，返回 ErrSchedulerRebuildInProgress。
func (s *SchedulerSnapshotService) TriggerManualRebuild() error {
	if s == nil || s.cache == nil {
		return ErrSchedulerCacheNotReady
	}
	if !s.rebuilding.CompareAndSwap(false, true) {
		return ErrSchedulerRebuildInProgress
	}
	defer s.rebuilding.Store(false)
	return s.triggerFullRebuild("manual")
}

// IsRebuilding 返回当前是否正在执行重建。
func (s *SchedulerSnapshotService) IsRebuilding() bool {
	if s == nil {
		return false
	}
	return s.rebuilding.Load()
}

func (s *SchedulerSnapshotService) ListSchedulableAccounts(ctx context.Context, groupID *int64, platform string, hasForcePlatform bool) ([]Account, bool, error) {
	useMixed := (platform == PlatformAnthropic || platform == PlatformGemini) && !hasForcePlatform
	mode := s.resolveMode(platform, hasForcePlatform)
	bucket := s.bucketFor(groupID, platform, mode)

	// L1 进程内缓存命中
	// 注意：必须返回切片的浅拷贝，因为调用方通过 &accounts[i] 获取指针后
	// 会调用 GetModelMapping() 等方法写入 Account 结构体字段（modelMappingCache map）。
	// 如果多个 goroutine 共享同一底层数组，会触发 concurrent map write panic。
	if s.snapshotL1 != nil {
		if val, ok := s.snapshotL1.Get(bucket.String()); ok {
			if cached, ok := val.([]Account); ok {
				result := make([]Account, len(cached))
				copy(result, cached)
				return result, useMixed, nil
			}
		}
	}

	if s.cache != nil {
		cached, hit, err := s.cache.GetSnapshot(ctx, bucket)
		if err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] cache read failed: bucket=%s err=%v", bucket.String(), err)
		} else if hit {
			result := derefAccounts(cached)
			s.setSnapshotL1(bucket.String(), result)
			return result, useMixed, nil
		}
	}

	if err := s.guardFallback(ctx); err != nil {
		return nil, useMixed, err
	}

	fallbackCtx, cancel := s.withFallbackTimeout(ctx)
	defer cancel()

	accounts, err := s.loadAccountsFromDB(fallbackCtx, bucket, useMixed)
	if err != nil {
		return nil, useMixed, err
	}

	if s.cache != nil {
		if err := s.cache.SetSnapshot(fallbackCtx, bucket, accounts); err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] cache write failed: bucket=%s err=%v", bucket.String(), err)
		}
	}

	s.setSnapshotL1(bucket.String(), accounts)
	return accounts, useMixed, nil
}

func (s *SchedulerSnapshotService) GetAccount(ctx context.Context, accountID int64) (*Account, error) {
	if accountID <= 0 {
		return nil, nil
	}

	// L1 进程内缓存命中
	// 注意：必须返回结构体的拷贝，原因同 ListSchedulableAccounts。
	if s.accountL1 != nil {
		if val, ok := s.accountL1.Get(accountID); ok {
			if cached, ok := val.(*Account); ok {
				copyVal := *cached
				return &copyVal, nil
			}
		}
	}

	if s.cache != nil {
		account, err := s.cache.GetAccount(ctx, accountID)
		if err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] account cache read failed: id=%d err=%v", accountID, err)
		} else if account != nil {
			s.setAccountL1(accountID, account)
			return account, nil
		}
	}

	if err := s.guardFallback(ctx); err != nil {
		return nil, err
	}
	fallbackCtx, cancel := s.withFallbackTimeout(ctx)
	defer cancel()
	return s.accountRepo.GetByID(fallbackCtx, accountID)
}

// GetGroupByID 获取分组信息（供调度器使用）
func (s *SchedulerSnapshotService) GetGroupByID(ctx context.Context, groupID int64) (*Group, error) {
	if s.groupRepo == nil {
		return nil, nil
	}
	return s.groupRepo.GetByID(ctx, groupID)
}

// UpdateAccountInCache 立即更新 Redis 中单个账号的数据（用于模型限流后立即生效）
func (s *SchedulerSnapshotService) UpdateAccountInCache(ctx context.Context, account *Account) error {
	if account == nil {
		return nil
	}
	// 同时失效 L1 缓存
	s.invalidateAccountL1(account.ID)
	if s.cache == nil {
		return nil
	}
	return s.cache.SetAccount(ctx, account)
}

func (s *SchedulerSnapshotService) runInitialRebuild() {
	if s.cache == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	buckets, err := s.cache.ListBuckets(ctx)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] list buckets failed: %v", err)
	}
	if len(buckets) == 0 {
		buckets, err = s.defaultBuckets(ctx)
		if err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] default buckets failed: %v", err)
			return
		}
	}
	if err := s.rebuildBucketsDedup(ctx, buckets, "startup"); err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] rebuild startup failed: %v", err)
	}
}

func (s *SchedulerSnapshotService) runOutboxWorker(ctx context.Context, interval time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		s.pollOutbox(ctx, interval)
	}
}

func (s *SchedulerSnapshotService) runFullRebuildWorker(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.triggerFullRebuild("interval"); err != nil {
				logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] full rebuild failed: %v", err)
			}
		case <-s.stopCh:
			return
		}
	}
}

func (s *SchedulerSnapshotService) pollOutbox(parentCtx context.Context, blockTimeout time.Duration) {
	if s.outboxQueue == nil || s.cache == nil {
		return
	}
	// Read 会在 Redis 侧阻塞等待 blockTimeout，有新消息立即返回；
	// parentCtx 被 cancel 时 BLOCK 读取会立即中断。
	readCtx, readCancel := context.WithTimeout(parentCtx, blockTimeout+5*time.Second)
	defer readCancel()

	events, err := s.outboxQueue.Read(readCtx, 200, blockTimeout)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox poll failed: %v", err)
		return
	}
	if len(events) == 0 {
		return
	}

	coalesced := coalesceOutboxEvents(events)
	if skipped := len(events) - len(coalesced); skipped > 0 {
		slog.Debug("[Scheduler] outbox coalesced", "original", len(events), "after", len(coalesced), "skipped", skipped)
	}

	// 优化：如果 batch 中包含 full_rebuild，跳过其它事件处理
	hasFullRebuild := false
	for _, event := range coalesced {
		if event.EventType == SchedulerOutboxEventFullRebuild {
			hasFullRebuild = true
			break
		}
	}

	seen := make(map[batchSeenKey]struct{})
	var handleErrors int
	for _, event := range coalesced {
		if hasFullRebuild && event.EventType != SchedulerOutboxEventFullRebuild {
			continue // full_rebuild 会重建所有 bucket，跳过其它事件
		}
		eventCtx, eventCancel := context.WithTimeout(context.Background(), outboxEventTimeout)
		err := s.handleOutboxEvent(eventCtx, event, seen)
		eventCancel()
		if err != nil {
			handleErrors++
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox handle failed: id=%s type=%s err=%v", event.StreamID, event.EventType, err)
		}
	}

	// ACK 所有事件（包括失败的），避免反复重试同一批失败事件
	streamIDs := make([]string, len(events))
	for i, event := range events {
		streamIDs[i] = event.StreamID
	}
	if handleErrors > 0 {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox batch completed with %d/%d errors", handleErrors, len(events))
	}
	ackCtx, ackCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := s.outboxQueue.Ack(ackCtx, streamIDs...); err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox ack failed: %v", err)
	}
	ackCancel()

	// 检查 lag 并在必要时触发全量重建
	lagCtx, lagCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer lagCancel()
	s.checkOutboxLag(lagCtx, events[0])
}

func (s *SchedulerSnapshotService) handleOutboxEvent(ctx context.Context, event SchedulerOutboxEvent, seen map[batchSeenKey]struct{}) error {
	switch event.EventType {
	case SchedulerOutboxEventAccountLastUsed:
		return s.handleLastUsedEvent(ctx, event.Payload)
	case SchedulerOutboxEventAccountBulkChanged:
		return s.handleBulkAccountEvent(ctx, event.Payload, seen)
	case SchedulerOutboxEventAccountGroupsChanged:
		return s.handleAccountEvent(ctx, event.AccountID, event.Payload, seen)
	case SchedulerOutboxEventAccountChanged:
		return s.handleAccountEvent(ctx, event.AccountID, event.Payload, seen)
	case SchedulerOutboxEventGroupChanged:
		return s.handleGroupEvent(ctx, event.GroupID, seen)
	case SchedulerOutboxEventFullRebuild:
		return s.triggerFullRebuild("outbox")
	default:
		return nil
	}
}

func (s *SchedulerSnapshotService) handleLastUsedEvent(ctx context.Context, payload map[string]any) error {
	if s.cache == nil || payload == nil {
		return nil
	}
	raw, ok := payload["last_used"].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	updates := make(map[int64]time.Time, len(raw))
	for key, value := range raw {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		sec, ok := toInt64(value)
		if !ok || sec <= 0 {
			continue
		}
		updates[id] = time.Unix(sec, 0)
	}
	if len(updates) == 0 {
		return nil
	}
	// 失效 L1 account 缓存中对应条目，避免 LRU 调度使用陈旧的 LastUsedAt
	for id := range updates {
		s.invalidateAccountL1(id)
	}
	return s.cache.UpdateLastUsed(ctx, updates)
}

func (s *SchedulerSnapshotService) handleBulkAccountEvent(ctx context.Context, payload map[string]any, seen map[batchSeenKey]struct{}) error {
	if payload == nil {
		return nil
	}
	if s.accountRepo == nil {
		return nil
	}

	rawIDs := parseInt64Slice(payload["account_ids"])
	if len(rawIDs) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(rawIDs))
	seenIDs := make(map[int64]struct{}, len(rawIDs))
	for _, id := range rawIDs {
		if id <= 0 {
			continue
		}
		if _, exists := seenIDs[id]; exists {
			continue
		}
		seenIDs[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}

	preloadGroupIDs := parseInt64Slice(payload["group_ids"])
	accounts, err := s.accountRepo.GetByIDs(ctx, ids)
	if err != nil {
		return err
	}

	found := make(map[int64]struct{}, len(accounts))
	rebuildGroupSet := make(map[int64]struct{}, len(preloadGroupIDs))
	for _, gid := range preloadGroupIDs {
		if gid > 0 {
			rebuildGroupSet[gid] = struct{}{}
		}
	}

	for _, account := range accounts {
		if account == nil || account.ID <= 0 {
			continue
		}
		found[account.ID] = struct{}{}
		s.invalidateAccountL1(account.ID)
		if s.cache != nil {
			if err := s.cache.SetAccount(ctx, account); err != nil {
				return err
			}
		}
		for _, gid := range account.GroupIDs {
			if gid > 0 {
				rebuildGroupSet[gid] = struct{}{}
			}
		}
	}

	if s.cache != nil {
		for _, id := range ids {
			if _, ok := found[id]; ok {
				continue
			}
			s.invalidateAccountL1(id)
			if err := s.cache.DeleteAccount(ctx, id); err != nil {
				return err
			}
		}
	}

	rebuildGroupIDs := make([]int64, 0, len(rebuildGroupSet))
	for gid := range rebuildGroupSet {
		rebuildGroupIDs = append(rebuildGroupIDs, gid)
	}
	return s.rebuildByGroupIDs(ctx, rebuildGroupIDs, "account_bulk_change", seen)
}

func (s *SchedulerSnapshotService) handleAccountEvent(ctx context.Context, accountID *int64, payload map[string]any, seen map[batchSeenKey]struct{}) error {
	if accountID == nil || *accountID <= 0 {
		return nil
	}
	if s.accountRepo == nil {
		return nil
	}

	var groupIDs []int64
	if payload != nil {
		groupIDs = parseInt64Slice(payload["group_ids"])
	}

	account, err := s.accountRepo.GetByID(ctx, *accountID)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			s.invalidateAccountL1(*accountID)
			if s.cache != nil {
				if err := s.cache.DeleteAccount(ctx, *accountID); err != nil {
					return err
				}
			}
			return s.rebuildByGroupIDs(ctx, groupIDs, "account_miss", seen)
		}
		return err
	}
	s.invalidateAccountL1(*accountID)
	if s.cache != nil {
		if err := s.cache.SetAccount(ctx, account); err != nil {
			return err
		}
	}
	if len(groupIDs) == 0 {
		groupIDs = account.GroupIDs
	}
	return s.rebuildByAccount(ctx, account, groupIDs, "account_change", seen)
}

func (s *SchedulerSnapshotService) handleGroupEvent(ctx context.Context, groupID *int64, seen map[batchSeenKey]struct{}) error {
	if groupID == nil || *groupID <= 0 {
		return nil
	}
	groupIDs := []int64{*groupID}
	return s.rebuildByGroupIDs(ctx, groupIDs, "group_change", seen)
}

func (s *SchedulerSnapshotService) rebuildByAccount(ctx context.Context, account *Account, groupIDs []int64, reason string, seen map[batchSeenKey]struct{}) error {
	if account == nil {
		return nil
	}
	groupIDs = s.normalizeGroupIDs(groupIDs)
	if len(groupIDs) == 0 {
		return nil
	}

	var firstErr error
	if err := s.rebuildBucketsForPlatform(ctx, account.Platform, groupIDs, reason, seen); err != nil && firstErr == nil {
		firstErr = err
	}
	if account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled() {
		if err := s.rebuildBucketsForPlatform(ctx, PlatformAnthropic, groupIDs, reason, seen); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := s.rebuildBucketsForPlatform(ctx, PlatformGemini, groupIDs, reason, seen); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *SchedulerSnapshotService) rebuildByGroupIDs(ctx context.Context, groupIDs []int64, reason string, seen map[batchSeenKey]struct{}) error {
	groupIDs = s.normalizeGroupIDs(groupIDs)
	if len(groupIDs) == 0 {
		return nil
	}
	platforms := []string{PlatformAnthropic, PlatformGemini, PlatformOpenAI, PlatformCodex2API, PlatformAntigravity, PlatformEmbedding}
	var firstErr error
	for _, platform := range platforms {
		if err := s.rebuildBucketsForPlatform(ctx, platform, groupIDs, reason, seen); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *SchedulerSnapshotService) rebuildBucketsForPlatform(ctx context.Context, platform string, groupIDs []int64, reason string, seen map[batchSeenKey]struct{}) error {
	if platform == "" {
		return nil
	}
	var firstErr error
	for _, gid := range groupIDs {
		// Within a single poll batch, skip (groupID, platform) pairs that were
		// already rebuilt. The first rebuild loads fresh DB data for all accounts
		// in the group, so subsequent rebuilds for the same group+platform within
		// the same batch are redundant.
		if seen != nil {
			key := batchSeenKey{gid, platform}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
		}
		if err := s.rebuildBucket(ctx, SchedulerBucket{GroupID: gid, Platform: platform, Mode: SchedulerModeSingle}, reason); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := s.rebuildBucket(ctx, SchedulerBucket{GroupID: gid, Platform: platform, Mode: SchedulerModeForced}, reason); err != nil && firstErr == nil {
			firstErr = err
		}
		if platform == PlatformAnthropic || platform == PlatformGemini {
			if err := s.rebuildBucket(ctx, SchedulerBucket{GroupID: gid, Platform: platform, Mode: SchedulerModeMixed}, reason); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (s *SchedulerSnapshotService) rebuildBuckets(ctx context.Context, buckets []SchedulerBucket, reason string) error {
	var firstErr error
	for _, bucket := range buckets {
		if err := s.rebuildBucket(ctx, bucket, reason); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *SchedulerSnapshotService) rebuildBucket(ctx context.Context, bucket SchedulerBucket, reason string) error {
	if s.cache == nil {
		return ErrSchedulerCacheNotReady
	}
	ok, err := s.cache.TryLockBucket(ctx, bucket, 30*time.Second)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	rebuildCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	accounts, err := s.loadAccountsFromDB(rebuildCtx, bucket, bucket.Mode == SchedulerModeMixed)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] rebuild failed: bucket=%s reason=%s err=%v", bucket.String(), reason, err)
		return err
	}
	if err := s.cache.SetSnapshot(rebuildCtx, bucket, accounts); err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] rebuild cache failed: bucket=%s reason=%s err=%v", bucket.String(), reason, err)
		return err
	}
	// 重建后立即失效对应的 L1 快照缓存，下次请求将从 Redis 加载最新数据
	s.invalidateSnapshotL1(bucket.String())
	slog.Debug("[Scheduler] rebuild ok", "bucket", bucket.String(), "reason", reason, "size", len(accounts))
	return nil
}

func (s *SchedulerSnapshotService) triggerFullRebuild(reason string) error {
	if s.cache == nil {
		return ErrSchedulerCacheNotReady
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	buckets, err := s.cache.ListBuckets(ctx)
	if err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] list buckets failed: %v", err)
		return err
	}
	if len(buckets) == 0 {
		buckets, err = s.defaultBuckets(ctx)
		if err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] default buckets failed: %v", err)
			return err
		}
	}
	return s.rebuildBucketsDedup(ctx, buckets, reason)
}

// rebuildBucketsDedup 全量重建多个 bucket，先收集所有账号去重写入一次，再逐 bucket 仅更新索引。
// 相比 rebuildBuckets 对每个 bucket 各写一次全量账号数据，Redis 命令数从 2N×B 降至 2N+索引。
func (s *SchedulerSnapshotService) rebuildBucketsDedup(ctx context.Context, buckets []SchedulerBucket, reason string) error {
	// 阶段 1：收集所有 bucket 的账号并去重
	type bucketData struct {
		bucket   SchedulerBucket
		accounts []Account
	}
	var results []bucketData
	allAccounts := make(map[int64]Account)

	for _, bucket := range buckets {
		ok, err := s.cache.TryLockBucket(ctx, bucket, 30*time.Second)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		rebuildCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		accounts, err := s.loadAccountsFromDB(rebuildCtx, bucket, bucket.Mode == SchedulerModeMixed)
		cancel()
		if err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] rebuild failed: bucket=%s reason=%s err=%v", bucket.String(), reason, err)
			continue
		}
		results = append(results, bucketData{bucket: bucket, accounts: accounts})
		for _, acc := range accounts {
			allAccounts[acc.ID] = acc
		}
	}

	if len(results) == 0 {
		return nil
	}

	// 阶段 2：一次性写入去重后的全量账号数据
	uniqueAccounts := make([]Account, 0, len(allAccounts))
	for _, acc := range allAccounts {
		uniqueAccounts = append(uniqueAccounts, acc)
	}
	if err := s.cache.WriteAccounts(ctx, uniqueAccounts); err != nil {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] full rebuild write accounts failed: reason=%s err=%v", reason, err)
		return err
	}

	// 阶段 3：逐 bucket 仅更新索引（不再写账号数据）
	var firstErr error
	for _, bd := range results {
		if err := s.cache.SetSnapshotIndex(ctx, bd.bucket, bd.accounts); err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] rebuild index failed: bucket=%s reason=%s err=%v", bd.bucket.String(), reason, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		s.invalidateSnapshotL1(bd.bucket.String())
		slog.Debug("[Scheduler] rebuild ok", "bucket", bd.bucket.String(), "reason", reason, "size", len(bd.accounts))
	}
	return firstErr
}

func (s *SchedulerSnapshotService) checkOutboxLag(ctx context.Context, oldest SchedulerOutboxEvent) {
	if oldest.CreatedAt.IsZero() || s.cfg == nil {
		return
	}

	lag := time.Since(oldest.CreatedAt)
	if lagSeconds := int(lag.Seconds()); lagSeconds >= s.cfg.Gateway.Scheduling.OutboxLagWarnSeconds && s.cfg.Gateway.Scheduling.OutboxLagWarnSeconds > 0 {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox lag warning: %ds", lagSeconds)
	}

	if s.cfg.Gateway.Scheduling.OutboxLagRebuildSeconds > 0 && int(lag.Seconds()) >= s.cfg.Gateway.Scheduling.OutboxLagRebuildSeconds {
		s.lagMu.Lock()
		s.lagFailures++
		failures := s.lagFailures
		s.lagMu.Unlock()

		if failures >= s.cfg.Gateway.Scheduling.OutboxLagRebuildFailures {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox lag rebuild triggered: lag=%s failures=%d", lag, failures)
			s.lagMu.Lock()
			s.lagFailures = 0
			s.lagMu.Unlock()
			if err := s.triggerFullRebuild("outbox_lag"); err != nil {
				logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox lag rebuild failed: %v", err)
			}
		}
	} else {
		s.lagMu.Lock()
		s.lagFailures = 0
		s.lagMu.Unlock()
	}

	threshold := s.cfg.Gateway.Scheduling.OutboxBacklogRebuildRows
	if threshold <= 0 || s.outboxQueue == nil {
		return
	}
	pending, err := s.outboxQueue.Pending(ctx)
	if err != nil {
		return
	}
	if pending >= int64(threshold) {
		logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox backlog rebuild triggered: pending=%d", pending)
		if err := s.triggerFullRebuild("outbox_backlog"); err != nil {
			logger.LegacyPrintf("service.scheduler_snapshot", "[Scheduler] outbox backlog rebuild failed: %v", err)
		}
	}
}

func (s *SchedulerSnapshotService) loadAccountsFromDB(ctx context.Context, bucket SchedulerBucket, useMixed bool) ([]Account, error) {
	if s.accountRepo == nil {
		return nil, ErrSchedulerCacheNotReady
	}
	groupID := bucket.GroupID
	if s.isRunModeSimple() {
		groupID = 0
	}

	if useMixed {
		platforms := []string{bucket.Platform, PlatformAntigravity}
		var accounts []Account
		var err error
		if groupID > 0 {
			accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatforms(ctx, groupID, platforms)
		} else if s.isRunModeSimple() {
			accounts, err = s.accountRepo.ListSchedulableByPlatforms(ctx, platforms)
		} else {
			accounts, err = s.accountRepo.ListSchedulableUngroupedByPlatforms(ctx, platforms)
		}
		if err != nil {
			return nil, err
		}
		filtered := make([]Account, 0, len(accounts))
		for _, acc := range accounts {
			if acc.Platform == PlatformAntigravity && !acc.IsMixedSchedulingEnabled() {
				continue
			}
			filtered = append(filtered, acc)
		}
		return filtered, nil
	}

	accountPlatforms := openAICompatibleSchedulerAccountPlatforms(bucket.Platform)
	if groupID > 0 {
		if len(accountPlatforms) == 1 {
			return s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, groupID, accountPlatforms[0])
		}
		return s.accountRepo.ListSchedulableByGroupIDAndPlatforms(ctx, groupID, accountPlatforms)
	}
	if s.isRunModeSimple() {
		if len(accountPlatforms) == 1 {
			return s.accountRepo.ListSchedulableByPlatform(ctx, accountPlatforms[0])
		}
		return s.accountRepo.ListSchedulableByPlatforms(ctx, accountPlatforms)
	}
	if len(accountPlatforms) == 1 {
		return s.accountRepo.ListSchedulableUngroupedByPlatform(ctx, accountPlatforms[0])
	}
	return s.accountRepo.ListSchedulableUngroupedByPlatforms(ctx, accountPlatforms)
}

func (s *SchedulerSnapshotService) bucketFor(groupID *int64, platform string, mode string) SchedulerBucket {
	return SchedulerBucket{
		GroupID:  s.normalizeGroupID(groupID),
		Platform: platform,
		Mode:     mode,
	}
}

func (s *SchedulerSnapshotService) normalizeGroupID(groupID *int64) int64 {
	if s.isRunModeSimple() {
		return 0
	}
	if groupID == nil || *groupID <= 0 {
		return 0
	}
	return *groupID
}

func (s *SchedulerSnapshotService) normalizeGroupIDs(groupIDs []int64) []int64 {
	if s.isRunModeSimple() {
		return []int64{0}
	}
	if len(groupIDs) == 0 {
		return []int64{0}
	}
	seen := make(map[int64]struct{}, len(groupIDs))
	out := make([]int64, 0, len(groupIDs))
	for _, id := range groupIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return []int64{0}
	}
	return out
}

func (s *SchedulerSnapshotService) resolveMode(platform string, hasForcePlatform bool) string {
	if hasForcePlatform {
		return SchedulerModeForced
	}
	if platform == PlatformAnthropic || platform == PlatformGemini {
		return SchedulerModeMixed
	}
	return SchedulerModeSingle
}

func (s *SchedulerSnapshotService) guardFallback(ctx context.Context) error {
	if s.cfg == nil || s.cfg.Gateway.Scheduling.DbFallbackEnabled {
		if s.fallbackLimit == nil || s.fallbackLimit.Allow() {
			return nil
		}
		return ErrSchedulerFallbackLimited
	}
	return ErrSchedulerCacheNotReady
}

func (s *SchedulerSnapshotService) withFallbackTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.cfg == nil || s.cfg.Gateway.Scheduling.DbFallbackTimeoutSeconds <= 0 {
		return context.WithCancel(ctx)
	}
	timeout := time.Duration(s.cfg.Gateway.Scheduling.DbFallbackTimeoutSeconds) * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return context.WithCancel(ctx)
		}
		if remaining < timeout {
			timeout = remaining
		}
	}
	return context.WithTimeout(ctx, timeout)
}

func (s *SchedulerSnapshotService) isRunModeSimple() bool {
	return s.cfg != nil && s.cfg.RunMode == config.RunModeSimple
}

func (s *SchedulerSnapshotService) outboxPollInterval() time.Duration {
	if s.cfg == nil {
		return time.Second
	}
	sec := s.cfg.Gateway.Scheduling.OutboxPollIntervalSeconds
	if sec <= 0 {
		return time.Second
	}
	return time.Duration(sec) * time.Second
}

func (s *SchedulerSnapshotService) fullRebuildInterval() time.Duration {
	if s.cfg == nil {
		return 0
	}
	sec := s.cfg.Gateway.Scheduling.FullRebuildIntervalSeconds
	if sec <= 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}

func (s *SchedulerSnapshotService) defaultBuckets(ctx context.Context) ([]SchedulerBucket, error) {
	buckets := make([]SchedulerBucket, 0)
	platforms := []string{PlatformAnthropic, PlatformGemini, PlatformOpenAI, PlatformCodex2API, PlatformAntigravity, PlatformEmbedding}
	for _, platform := range platforms {
		buckets = append(buckets, SchedulerBucket{GroupID: 0, Platform: platform, Mode: SchedulerModeSingle})
		buckets = append(buckets, SchedulerBucket{GroupID: 0, Platform: platform, Mode: SchedulerModeForced})
		if platform == PlatformAnthropic || platform == PlatformGemini {
			buckets = append(buckets, SchedulerBucket{GroupID: 0, Platform: platform, Mode: SchedulerModeMixed})
		}
	}

	if s.isRunModeSimple() || s.groupRepo == nil {
		return dedupeBuckets(buckets), nil
	}

	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return dedupeBuckets(buckets), nil
	}
	for _, group := range groups {
		if group.Platform == "" {
			continue
		}
		buckets = append(buckets, SchedulerBucket{GroupID: group.ID, Platform: group.Platform, Mode: SchedulerModeSingle})
		buckets = append(buckets, SchedulerBucket{GroupID: group.ID, Platform: group.Platform, Mode: SchedulerModeForced})
		if group.Platform == PlatformAnthropic || group.Platform == PlatformGemini {
			buckets = append(buckets, SchedulerBucket{GroupID: group.ID, Platform: group.Platform, Mode: SchedulerModeMixed})
		}
	}
	return dedupeBuckets(buckets), nil
}

func dedupeBuckets(in []SchedulerBucket) []SchedulerBucket {
	seen := make(map[string]struct{}, len(in))
	out := make([]SchedulerBucket, 0, len(in))
	for _, bucket := range in {
		key := bucket.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, bucket)
	}
	return out
}

func derefAccounts(accounts []*Account) []Account {
	if len(accounts) == 0 {
		return []Account{}
	}
	out := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		out = append(out, *account)
	}
	return out
}

func parseInt64Slice(value any) []int64 {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]int64, 0, len(raw))
	for _, item := range raw {
		if v, ok := toInt64(item); ok && v > 0 {
			out = append(out, v)
		}
	}
	return out
}

func toInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	case json.Number:
		parsed, err := strconv.ParseInt(v.String(), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

type fallbackLimiter struct {
	maxQPS int
	mu     sync.Mutex
	window time.Time
	count  int
}

func newFallbackLimiter(maxQPS int) *fallbackLimiter {
	if maxQPS <= 0 {
		return nil
	}
	return &fallbackLimiter{
		maxQPS: maxQPS,
		window: time.Now(),
	}
}

func (l *fallbackLimiter) Allow() bool {
	if l == nil || l.maxQPS <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if now.Sub(l.window) >= time.Second {
		l.window = now
		l.count = 0
	}
	if l.count >= l.maxQPS {
		return false
	}
	l.count++
	return true
}

// =============================================
// L1 进程内缓存辅助方法
// =============================================

func (s *SchedulerSnapshotService) setSnapshotL1(key string, accounts []Account) {
	if s.snapshotL1 == nil || s.snapshotL1TTL <= 0 {
		return
	}
	_ = s.snapshotL1.SetWithTTL(key, accounts, 1, s.snapshotL1TTL)
}

func (s *SchedulerSnapshotService) invalidateSnapshotL1(key string) {
	if s.snapshotL1 == nil {
		return
	}
	s.snapshotL1.Del(key)
}

func (s *SchedulerSnapshotService) setAccountL1(accountID int64, account *Account) {
	if s.accountL1 == nil || s.accountL1TTL <= 0 || account == nil {
		return
	}
	_ = s.accountL1.SetWithTTL(accountID, account, 1, s.accountL1TTL)
}

func (s *SchedulerSnapshotService) invalidateAccountL1(accountID int64) {
	if s.accountL1 == nil {
		return
	}
	s.accountL1.Del(accountID)
}

// coalesceOutboxEvents 合并同一批次内的冗余事件，减少 DB/Redis 操作次数。
//   - 同 accountID 的 account_changed / account_groups_changed：仅保留最后一条，
//     但合并所有事件的 group_ids，确保所有关联 bucket 都被重建。
//   - 同 groupID 的 group_changed：仅保留最后一条。
//   - 多条 full_rebuild：仅保留一条。
//   - account_last_used / account_bulk_changed：全部保留（不合并）。
func coalesceOutboxEvents(events []SchedulerOutboxEvent) []SchedulerOutboxEvent {
	if len(events) <= 1 {
		return events
	}

	type accountEntry struct {
		resultIdx int
		groupIDs  map[int64]struct{}
	}

	accountMap := make(map[int64]*accountEntry)
	groupMap := make(map[int64]int) // groupID -> resultIdx
	seenFullRebuild := false

	result := make([]SchedulerOutboxEvent, 0, len(events))

	for _, event := range events {
		switch event.EventType {
		case SchedulerOutboxEventAccountChanged, SchedulerOutboxEventAccountGroupsChanged:
			if event.AccountID == nil || *event.AccountID <= 0 {
				result = append(result, event)
				continue
			}
			aid := *event.AccountID
			gids := extractGroupIDs(event.Payload)

			if entry, exists := accountMap[aid]; exists {
				// 合并 group_ids 并替换为最新事件
				for _, gid := range gids {
					entry.groupIDs[gid] = struct{}{}
				}
				result[entry.resultIdx] = event
			} else {
				gidSet := make(map[int64]struct{}, len(gids))
				for _, gid := range gids {
					gidSet[gid] = struct{}{}
				}
				accountMap[aid] = &accountEntry{resultIdx: len(result), groupIDs: gidSet}
				result = append(result, event)
			}

		case SchedulerOutboxEventGroupChanged:
			if event.GroupID == nil || *event.GroupID <= 0 {
				result = append(result, event)
				continue
			}
			gid := *event.GroupID
			if idx, exists := groupMap[gid]; exists {
				result[idx] = event
			} else {
				groupMap[gid] = len(result)
				result = append(result, event)
			}

		case SchedulerOutboxEventFullRebuild:
			if !seenFullRebuild {
				seenFullRebuild = true
				result = append(result, event)
			}

		default:
			result = append(result, event)
		}
	}

	// 将合并后的 group_ids 回写到保留的事件中
	for _, entry := range accountMap {
		if len(entry.groupIDs) == 0 {
			continue
		}
		ev := &result[entry.resultIdx]
		if ev.Payload == nil {
			ev.Payload = make(map[string]any)
		}
		merged := make([]any, 0, len(entry.groupIDs))
		for gid := range entry.groupIDs {
			merged = append(merged, float64(gid))
		}
		ev.Payload["group_ids"] = merged
	}

	return result
}

func extractGroupIDs(payload map[string]any) []int64 {
	if payload == nil {
		return nil
	}
	return parseInt64Slice(payload["group_ids"])
}
