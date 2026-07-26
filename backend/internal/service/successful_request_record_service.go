package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	successfulRequestStreamName                    = "sub2api:successful_request_records"
	successfulRequestConsumerGroup                 = "successful-request-recorders"
	successfulRequestPublishTimeout                = 500 * time.Millisecond
	successfulRequestConsumeBatchSize        int64 = 16
	successfulRequestClaimIdle                     = 30 * time.Second
	successfulRequestMaxRetries              int64 = 5
	successfulRequestConsumerBlock                 = time.Second
	successfulRequestDBTimeout                     = 5 * time.Second
	successfulRequestRetryTTL                      = 7 * 24 * time.Hour
	successfulRequestReconcileInterval             = 10 * time.Second
	successfulRequestSettingsRefreshInterval       = 30 * time.Second
	successfulRequestReconcileBatch                = 500
	successfulRequestDLQMaxLen               int64 = 10_000
)

// SuccessfulRequestRecordStats 暴露成功请求记录链路的关键运行计数。
type SuccessfulRequestRecordStats struct {
	Published       uint64
	Persisted       uint64
	Duplicates      uint64
	PublishFallback uint64
	Failed          uint64
	DeadLettered    uint64
}

// SuccessfulRequestRecordQueueOptions 仅用于代码级构造（主要供隔离测试使用）。
// 生产依赖注入不传入该参数，因此队列拓扑和重试策略始终使用本文件内的固定默认值。
type SuccessfulRequestRecordQueueOptions struct {
	StreamName       string
	ConsumerGroup    string
	PublishTimeout   time.Duration
	ConsumeBatchSize int64
	ClaimIdle        time.Duration
	MaxRetries       int64
}

func defaultSuccessfulRequestRecordQueueOptions() SuccessfulRequestRecordQueueOptions {
	return SuccessfulRequestRecordQueueOptions{
		StreamName:       successfulRequestStreamName,
		ConsumerGroup:    successfulRequestConsumerGroup,
		PublishTimeout:   successfulRequestPublishTimeout,
		ConsumeBatchSize: successfulRequestConsumeBatchSize,
		ClaimIdle:        successfulRequestClaimIdle,
		MaxRetries:       successfulRequestMaxRetries,
	}
}

// SuccessfulRequestRecordService 通过 Redis Stream 异步持久化成功请求快照。
//
// 投递语义为 at-least-once：数据库以 event_id 幂等，消费者只有在写库成功后
// 才 ACK。Redis 不可用时 Publish 会降级为直接写数据库。
type SuccessfulRequestRecordService struct {
	repo      SuccessfulRequestRecordRepository
	redis     *redis.Client
	encryptor SecretEncryptor
	settings  *SettingService
	runtime   atomic.Value // *SuccessfulRequestRecordRuntimeSettings
	queue     SuccessfulRequestRecordQueueOptions

	consumerName string
	workerCtx    context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	startOnce    sync.Once
	stopOnce     sync.Once

	published       atomic.Uint64
	persisted       atomic.Uint64
	duplicates      atomic.Uint64
	publishFallback atomic.Uint64
	failed          atomic.Uint64
	deadLettered    atomic.Uint64
}

func NewSuccessfulRequestRecordService(
	repo SuccessfulRequestRecordRepository,
	redisClient *redis.Client,
	encryptor SecretEncryptor,
	settingService *SettingService,
	queueOptions ...SuccessfulRequestRecordQueueOptions,
) *SuccessfulRequestRecordService {
	hostname, _ := os.Hostname()
	if strings.TrimSpace(hostname) == "" {
		hostname = "unknown"
	}
	workerCtx, cancel := context.WithCancel(context.Background())
	queue := defaultSuccessfulRequestRecordQueueOptions()
	if len(queueOptions) > 0 {
		candidate := queueOptions[0]
		if strings.TrimSpace(candidate.StreamName) != "" {
			queue.StreamName = strings.TrimSpace(candidate.StreamName)
		}
		if strings.TrimSpace(candidate.ConsumerGroup) != "" {
			queue.ConsumerGroup = strings.TrimSpace(candidate.ConsumerGroup)
		}
		if candidate.PublishTimeout > 0 {
			queue.PublishTimeout = candidate.PublishTimeout
		}
		if candidate.ConsumeBatchSize > 0 {
			queue.ConsumeBatchSize = candidate.ConsumeBatchSize
		}
		if candidate.ClaimIdle > 0 {
			queue.ClaimIdle = candidate.ClaimIdle
		}
		if candidate.MaxRetries > 0 {
			queue.MaxRetries = candidate.MaxRetries
		}
	}
	service := &SuccessfulRequestRecordService{
		repo:         repo,
		redis:        redisClient,
		encryptor:    encryptor,
		settings:     settingService,
		queue:        queue,
		consumerName: hostname + "-" + uuid.NewString(),
		workerCtx:    workerCtx,
		cancel:       cancel,
	}
	service.runtime.Store(&SuccessfulRequestRecordRuntimeSettings{
		Enabled:      false,
		MaxBodyBytes: DefaultSuccessfulRequestRecordsMaxBodyBytes,
	})
	service.refreshRuntimeSettings(context.Background())
	if settingService != nil {
		settingService.SetOnUpdateCallback(func() {
			service.refreshRuntimeSettings(context.Background())
		})
	}
	return service
}

func (s *SuccessfulRequestRecordService) Enabled() bool {
	if s == nil {
		return false
	}
	settings, _ := s.runtime.Load().(*SuccessfulRequestRecordRuntimeSettings)
	return settings != nil && settings.Enabled
}

func (s *SuccessfulRequestRecordService) MaxBodyBytes() int64 {
	if s == nil {
		return 0
	}
	settings, _ := s.runtime.Load().(*SuccessfulRequestRecordRuntimeSettings)
	if settings == nil || settings.MaxBodyBytes <= 0 {
		return DefaultSuccessfulRequestRecordsMaxBodyBytes
	}
	return settings.MaxBodyBytes
}

func (s *SuccessfulRequestRecordService) refreshRuntimeSettings(ctx context.Context) {
	if s == nil || s.settings == nil {
		return
	}
	refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(nonNilContext(ctx)), successfulRequestDBTimeout)
	defer cancel()
	settings, err := s.settings.GetSuccessfulRequestRecordRuntimeSettings(refreshCtx)
	if err != nil {
		logger.LegacyPrintf("service.successful_request_record", "[SuccessfulRequestRecord] refresh runtime settings failed: %v", err)
		return
	}
	if settings.MaxBodyBytes < MinSuccessfulRequestRecordsMaxBodyBytes || settings.MaxBodyBytes > MaxSuccessfulRequestRecordsMaxBodyBytes {
		settings.MaxBodyBytes = DefaultSuccessfulRequestRecordsMaxBodyBytes
	}
	if settings.Enabled && !s.settings.IsTotpEncryptionKeyConfigured() {
		settings.Enabled = false
		logger.LegacyPrintf("service.successful_request_record", "[SuccessfulRequestRecord] recording remains disabled: TOTP_ENCRYPTION_KEY is not explicitly configured")
	}
	s.runtime.Store(&settings)
}

func (s *SuccessfulRequestRecordService) Stats() SuccessfulRequestRecordStats {
	if s == nil {
		return SuccessfulRequestRecordStats{}
	}
	return SuccessfulRequestRecordStats{
		Published:       s.published.Load(),
		Persisted:       s.persisted.Load(),
		Duplicates:      s.duplicates.Load(),
		PublishFallback: s.publishFallback.Load(),
		Failed:          s.failed.Load(),
		DeadLettered:    s.deadLettered.Load(),
	}
}

// Start 启动 Redis Stream 消费者和动态设置刷新任务。
// 关闭采集只会阻止新消息发布，已经入队的消息仍会被消费，避免数据丢失。
func (s *SuccessfulRequestRecordService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		s.wg.Add(3)
		go s.consumeLoop()
		go s.reconcileLoop()
		go s.settingsRefreshLoop()
		logger.LegacyPrintf("service.successful_request_record",
			"[SuccessfulRequestRecord] consumer started stream=%s group=%s consumer=%s enabled=%t",
			s.queue.StreamName, s.queue.ConsumerGroup, s.consumerName, s.Enabled())
	})
}

func (s *SuccessfulRequestRecordService) settingsRefreshLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(successfulRequestSettingsRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.workerCtx.Done():
			return
		case <-ticker.C:
			s.refreshRuntimeSettings(s.workerCtx)
		}
	}
}

// reconcileLoop 独立于 Redis 消费循环运行，用于处理正文先于 usage_logs 入库的情况。
func (s *SuccessfulRequestRecordService) reconcileLoop() {
	defer s.wg.Done()
	if s.repo == nil {
		logger.LegacyPrintf("service.successful_request_record", "[SuccessfulRequestRecord] reconciliation not ready: missing repository")
		return
	}

	ticker := time.NewTicker(successfulRequestReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.workerCtx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(s.workerCtx, successfulRequestDBTimeout)
			updated, err := s.repo.ReconcileUnlinked(ctx, successfulRequestReconcileBatch)
			cancel()
			if err != nil && !errors.Is(err, context.Canceled) {
				logger.LegacyPrintf("service.successful_request_record", "[SuccessfulRequestRecord] reconcile usage_log_id failed: %v", err)
				continue
			}
			if updated > 0 {
				logger.LegacyPrintf("service.successful_request_record", "[SuccessfulRequestRecord] reconciled usage_log_id rows=%d", updated)
			}
		}
	}
}

func (s *SuccessfulRequestRecordService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.wg.Wait()
	})
}

// Publish 将原始快照加密后投递到 Redis Stream。
// MQ 不可用或加密失败时，使用短超时直接写数据库作为可靠性降级。
func (s *SuccessfulRequestRecordService) Publish(ctx context.Context, record *SuccessfulRequestRecord) error {
	if !s.Enabled() || record == nil {
		return nil
	}
	s.normalizeRecord(record)
	if err := validateSuccessfulRequestRecord(record); err != nil {
		return err
	}

	payload, err := json.Marshal(record)
	if err == nil {
		if s.encryptor == nil {
			err = errors.New("successful request record encryptor is not ready")
		} else {
			var encrypted string
			encrypted, err = s.encryptor.Encrypt(string(payload))
			if err == nil {
				err = s.publishEncrypted(ctx, record.EventID, encrypted)
			}
		}
	}
	if err == nil {
		s.published.Add(1)
		return nil
	}

	s.publishFallback.Add(1)
	fallbackCtx, cancel := context.WithTimeout(context.WithoutCancel(nonNilContext(ctx)), successfulRequestDBTimeout)
	defer cancel()
	inserted, fallbackErr := s.repo.Create(fallbackCtx, record)
	if fallbackErr != nil {
		s.failed.Add(1)
		return errors.Join(fmt.Errorf("publish successful request record: %w", err), fmt.Errorf("database fallback: %w", fallbackErr))
	}
	if inserted {
		s.persisted.Add(1)
	} else {
		s.duplicates.Add(1)
	}
	logger.FromContext(fallbackCtx).With(
		zap.String("component", "service.successful_request_record"),
		zap.String("event_id", record.EventID),
	).Warn("successful_request_record.mq_publish_failed_db_fallback_succeeded", zap.Error(err))
	return nil
}

func (s *SuccessfulRequestRecordService) publishEncrypted(ctx context.Context, eventID, encrypted string) error {
	if s.redis == nil {
		return errors.New("redis client is not ready")
	}
	pubCtx, cancel := context.WithTimeout(context.WithoutCancel(nonNilContext(ctx)), s.queue.PublishTimeout)
	defer cancel()
	return s.redis.XAdd(pubCtx, &redis.XAddArgs{
		Stream: s.queue.StreamName,
		Values: map[string]any{
			"event_id": eventID,
			"payload":  encrypted,
		},
	}).Err()
}

func (s *SuccessfulRequestRecordService) consumeLoop() {
	defer s.wg.Done()
	if s.redis == nil || s.repo == nil || s.encryptor == nil {
		logger.LegacyPrintf("service.successful_request_record", "[SuccessfulRequestRecord] consumer not ready: missing dependency")
		return
	}
	// 配置加载器会在未显式提供 TOTP_ENCRYPTION_KEY 时为当前进程生成临时密钥。
	// 临时密钥只能保证进程内可用，不能解密其他实例或重启前已经入队的消息；
	// 因此此类实例不得加入消费组，否则会把有效消息误判为毒消息并送入 DLQ。
	if s.settings == nil || !s.settings.IsTotpEncryptionKeyConfigured() {
		logger.LegacyPrintf("service.successful_request_record", "[SuccessfulRequestRecord] consumer disabled: fixed TOTP_ENCRYPTION_KEY is not configured")
		return
	}

	claimInterval := s.queue.ClaimIdle / 2
	if claimInterval < 5*time.Second {
		claimInterval = 5 * time.Second
	}
	claimTicker := time.NewTicker(claimInterval)
	defer claimTicker.Stop()

	for {
		if err := s.workerCtx.Err(); err != nil {
			return
		}
		if err := s.ensureConsumerGroup(s.workerCtx); err != nil {
			logger.LegacyPrintf("service.successful_request_record", "[SuccessfulRequestRecord] ensure consumer group failed: %v", err)
			if !waitForContext(s.workerCtx, time.Second) {
				return
			}
			continue
		}

		select {
		case <-claimTicker.C:
			s.claimPending(s.workerCtx)
		default:
		}

		streams, err := s.redis.XReadGroup(s.workerCtx, &redis.XReadGroupArgs{
			Group:    s.queue.ConsumerGroup,
			Consumer: s.consumerName,
			Streams:  []string{s.queue.StreamName, ">"},
			Count:    s.queue.ConsumeBatchSize,
			Block:    successfulRequestConsumerBlock,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			logger.LegacyPrintf("service.successful_request_record", "[SuccessfulRequestRecord] read stream failed: %v", err)
			if !waitForContext(s.workerCtx, time.Second) {
				return
			}
			continue
		}
		for _, stream := range streams {
			for _, message := range stream.Messages {
				s.processMessage(s.workerCtx, message)
			}
		}
	}
}

func (s *SuccessfulRequestRecordService) ensureConsumerGroup(ctx context.Context) error {
	err := s.redis.XGroupCreateMkStream(ctx, s.queue.StreamName, s.queue.ConsumerGroup, "0").Err()
	if err == nil || strings.Contains(strings.ToUpper(err.Error()), "BUSYGROUP") {
		return nil
	}
	return err
}

func (s *SuccessfulRequestRecordService) claimPending(ctx context.Context) {
	start := "0-0"
	for page := 0; page < 8; page++ {
		messages, next, err := s.redis.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   s.queue.StreamName,
			Group:    s.queue.ConsumerGroup,
			Consumer: s.consumerName,
			MinIdle:  s.queue.ClaimIdle,
			Start:    start,
			Count:    s.queue.ConsumeBatchSize,
		}).Result()
		if err != nil {
			if !errors.Is(err, redis.Nil) && !errors.Is(err, context.Canceled) {
				logger.LegacyPrintf("service.successful_request_record", "[SuccessfulRequestRecord] claim pending failed: %v", err)
			}
			return
		}
		for _, message := range messages {
			s.processMessage(ctx, message)
		}
		if len(messages) == 0 || next == "0-0" || next == start {
			return
		}
		start = next
	}
}

func (s *SuccessfulRequestRecordService) processMessage(parent context.Context, message redis.XMessage) {
	encrypted := streamStringValue(message.Values["payload"])
	if encrypted == "" {
		s.handleMessageFailure(parent, message, errors.New("missing encrypted payload"))
		return
	}
	decrypted, err := s.encryptor.Decrypt(encrypted)
	if err != nil {
		s.handleMessageFailure(parent, message, fmt.Errorf("decrypt payload: %w", err))
		return
	}
	var record SuccessfulRequestRecord
	if err := json.Unmarshal([]byte(decrypted), &record); err != nil {
		s.handleMessageFailure(parent, message, fmt.Errorf("decode payload: %w", err))
		return
	}
	if err := validateSuccessfulRequestRecord(&record); err != nil {
		s.handleMessageFailure(parent, message, err)
		return
	}

	ctx, cancel := context.WithTimeout(parent, successfulRequestDBTimeout)
	inserted, err := s.repo.Create(ctx, &record)
	cancel()
	if err != nil {
		s.handleMessageFailure(parent, message, fmt.Errorf("persist payload: %w", err))
		return
	}
	if err := s.ackAndDelete(parent, message.ID); err != nil {
		s.handleMessageFailure(parent, message, fmt.Errorf("ack payload: %w", err))
		return
	}
	if inserted {
		s.persisted.Add(1)
	} else {
		s.duplicates.Add(1)
	}
	_ = s.redis.Del(parent, s.retryKey(message.ID)).Err()
}

func (s *SuccessfulRequestRecordService) ackAndDelete(ctx context.Context, messageID string) error {
	pipe := s.redis.TxPipeline()
	pipe.XAck(ctx, s.queue.StreamName, s.queue.ConsumerGroup, messageID)
	pipe.XDel(ctx, s.queue.StreamName, messageID)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *SuccessfulRequestRecordService) handleMessageFailure(ctx context.Context, message redis.XMessage, cause error) {
	s.failed.Add(1)
	attemptKey := s.retryKey(message.ID)
	pipe := s.redis.TxPipeline()
	attemptCmd := pipe.Incr(ctx, attemptKey)
	pipe.Expire(ctx, attemptKey, successfulRequestRetryTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		logger.LegacyPrintf("service.successful_request_record", "[SuccessfulRequestRecord] update retry count failed: id=%s err=%v", message.ID, err)
		return
	}
	attempt := attemptCmd.Val()
	if attempt < s.queue.MaxRetries {
		logger.LegacyPrintf("service.successful_request_record", "[SuccessfulRequestRecord] consume failed: id=%s attempt=%d/%d err=%v", message.ID, attempt, s.queue.MaxRetries, cause)
		return
	}

	payload := streamStringValue(message.Values["payload"])
	errorSummary := logredact.RedactText(cause.Error())
	if len(errorSummary) > 512 {
		errorSummary = errorSummary[:512]
	}
	if err := s.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: s.queue.StreamName + ":dlq",
		MaxLen: successfulRequestDLQMaxLen,
		Approx: true,
		Values: map[string]any{
			"source_id": message.ID,
			"event_id":  streamStringValue(message.Values["event_id"]),
			"payload":   payload,
			"error":     errorSummary,
			"failed_at": time.Now().UTC().Format(time.RFC3339Nano),
		},
	}).Err(); err != nil {
		logger.LegacyPrintf("service.successful_request_record", "[SuccessfulRequestRecord] publish DLQ failed: id=%s err=%v", message.ID, err)
		return
	}
	if err := s.ackAndDelete(ctx, message.ID); err != nil {
		logger.LegacyPrintf("service.successful_request_record", "[SuccessfulRequestRecord] ACK after DLQ failed: id=%s err=%v", message.ID, err)
		return
	}
	_ = s.redis.Del(ctx, attemptKey).Err()
	s.deadLettered.Add(1)
}

func (s *SuccessfulRequestRecordService) retryKey(messageID string) string {
	return s.queue.StreamName + ":retry:" + messageID
}

func (s *SuccessfulRequestRecordService) normalizeRecord(record *SuccessfulRequestRecord) {
	record.EventID = trimTo(record.EventID, 64)
	if record.EventID == "" {
		record.EventID = uuid.NewString()
	}
	record.TraceID = trimTo(record.TraceID, 64)
	record.RequestID = trimTo(record.RequestID, 64)
	if record.RequestID == "" {
		record.RequestID = record.EventID
	}
	record.ClientRequestID = trimTo(record.ClientRequestID, 128)
	record.Method = strings.ToUpper(trimTo(record.Method, 8))
	record.Endpoint = trimTo(record.Endpoint, 256)
	record.RoutePattern = trimTo(record.RoutePattern, 256)
	record.Model = trimTo(record.Model, 100)
	record.RequestContentType = trimTo(record.RequestContentType, 128)
	record.ResponseContentType = trimTo(record.ResponseContentType, 128)
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.DurationMS < 0 {
		record.DurationMS = 0
	}
	if record.RequestBodyBytes < 0 {
		record.RequestBodyBytes = 0
	}
	if record.ResponseBodyBytes < 0 {
		record.ResponseBodyBytes = 0
	}
	maxBodyBytes := s.MaxBodyBytes()
	if record.RequestBody != nil && int64(len(*record.RequestBody)) > maxBodyBytes {
		record.RequestBody = nil
		record.RequestTruncated = true
	}
	if record.ResponseBody != nil && int64(len(*record.ResponseBody)) > maxBodyBytes {
		record.ResponseBody = nil
		record.ResponseTruncated = true
	}
}

func validateSuccessfulRequestRecord(record *SuccessfulRequestRecord) error {
	if record == nil {
		return errors.New("successful request record is nil")
	}
	if strings.TrimSpace(record.EventID) == "" {
		return errors.New("successful request record event_id is required")
	}
	if _, err := uuid.Parse(record.EventID); err != nil {
		return fmt.Errorf("successful request record event_id must be a UUID: %w", err)
	}
	if record.UserID <= 0 || record.APIKeyID <= 0 {
		return errors.New("successful request record user_id and api_key_id must be positive")
	}
	if strings.TrimSpace(record.RequestID) == "" || strings.TrimSpace(record.Method) == "" || strings.TrimSpace(record.Endpoint) == "" {
		return errors.New("successful request record request_id, method and endpoint are required")
	}
	if record.StatusCode < 200 || record.StatusCode >= 300 {
		return fmt.Errorf("successful request record status_code must be 2xx: %d", record.StatusCode)
	}
	return nil
}

func streamStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return ""
	}
}

func trimTo(value string, max int) string {
	value = strings.TrimSpace(value)
	if max > 0 && len(value) > max {
		return value[:max]
	}
	return value
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func waitForContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
