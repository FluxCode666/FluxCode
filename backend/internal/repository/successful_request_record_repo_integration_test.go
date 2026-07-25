//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestUsageLogPayloadSchemaUsesSetNullWithoutAutomaticDeletion(t *testing.T) {
	ctx := context.Background()
	var nullable string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT is_nullable
		FROM information_schema.columns
		WHERE table_schema = 'public'
			AND table_name = 'usage_log_payloads'
			AND column_name = 'usage_log_id'
	`).Scan(&nullable))
	require.Equal(t, "YES", nullable)

	var deleteRule string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT rc.delete_rule
		FROM information_schema.referential_constraints AS rc
		JOIN information_schema.table_constraints AS tc
			ON tc.constraint_catalog = rc.constraint_catalog
			AND tc.constraint_schema = rc.constraint_schema
			AND tc.constraint_name = rc.constraint_name
		WHERE tc.table_schema = 'public'
			AND tc.table_name = 'usage_log_payloads'
			AND tc.constraint_type = 'FOREIGN KEY'
	`).Scan(&deleteRule))
	require.Equal(t, "SET NULL", deleteRule)

	tx := testTx(t)
	requireIndex(t, tx, "usage_log_payloads", "uq_usage_log_payloads_event_id")
	requireIndex(t, tx, "usage_log_payloads", "uq_usage_log_payloads_usage_log_id")
}

func TestSuccessfulRequestRecordRepositoryReconcilesAndPreservesPayloadAfterUsageLogDeletion(t *testing.T) {
	ctx := context.Background()
	suffix := uuid.NewString()
	user := mustCreateUser(t, integrationEntClient, &service.User{Email: "payload-" + suffix + "@example.com"})
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "payload-" + suffix})
	apiKey := mustCreateApiKey(t, integrationEntClient, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-payload-" + suffix,
		Name:   "payload-" + suffix,
	})

	eventID := uuid.NewString()
	clientRequestID := uuid.NewString()
	requestBody := `{"model":"test-model"}`
	responseBody := `{"id":"response-1"}`
	record := &service.SuccessfulRequestRecord{
		EventID:             eventID,
		UserID:              user.ID,
		APIKeyID:            apiKey.ID,
		TraceID:             "client-controlled-trace",
		RequestID:           uuid.NewString(),
		ClientRequestID:     clientRequestID,
		Method:              "POST",
		Endpoint:            "/v1/messages",
		RoutePattern:        "/v1/messages",
		Model:               "test-model",
		StatusCode:          200,
		RequestContentType:  "application/json",
		ResponseContentType: "application/json",
		RequestBody:         &requestBody,
		ResponseBody:        &responseBody,
		RequestBodyBytes:    int64(len(requestBody)),
		ResponseBodyBytes:   int64(len(responseBody)),
		CreatedAt:           time.Now().UTC(),
	}

	repository := NewSuccessfulRequestRecordRepository(integrationDB)
	inserted, err := repository.Create(ctx, record)
	require.NoError(t, err)
	require.True(t, inserted)

	inserted, err = repository.Create(ctx, record)
	require.NoError(t, err)
	require.False(t, inserted, "event_id 重投必须幂等")

	var payloadID int64
	var linkedUsageLogID sql.NullInt64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT id, usage_log_id
		FROM usage_log_payloads
		WHERE event_id = $1
	`, eventID).Scan(&payloadID, &linkedUsageLogID))
	require.False(t, linkedUsageLogID.Valid)

	var usageLogID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		INSERT INTO usage_logs (
			user_id, api_key_id, account_id, trace_id, request_id, model
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, user.ID, apiKey.ID, account.ID, "different-trace", "client:"+clientRequestID, "test-model").Scan(&usageLogID))

	updated, err := repository.ReconcileUnlinked(ctx, 500)
	require.NoError(t, err)
	require.GreaterOrEqual(t, updated, int64(1))

	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT usage_log_id
		FROM usage_log_payloads
		WHERE id = $1
	`, payloadID).Scan(&linkedUsageLogID))
	require.True(t, linkedUsageLogID.Valid)
	require.Equal(t, usageLogID, linkedUsageLogID.Int64)

	_, err = integrationDB.ExecContext(ctx, "DELETE FROM usage_logs WHERE id = $1", usageLogID)
	require.NoError(t, err)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT usage_log_id
		FROM usage_log_payloads
		WHERE id = $1
	`, payloadID).Scan(&linkedUsageLogID))
	require.False(t, linkedUsageLogID.Valid, "删除 usage_logs 后正文行必须保留且仅清空关联")

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM usage_log_payloads WHERE event_id = $1", eventID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM api_keys WHERE id = $1", apiKey.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM accounts WHERE id = $1", account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})
}

func TestSuccessfulRequestRecordRedisStreamEncryptsConsumesAndAcknowledges(t *testing.T) {
	ctx := context.Background()
	streamName := "test:usage-log-payloads:" + uuid.NewString()
	eventID := uuid.NewString()
	t.Cleanup(func() {
		_, _ = integrationRedis.Del(context.Background(), streamName, streamName+":dlq").Result()
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM usage_log_payloads WHERE event_id = $1", eventID)
	})

	cfg := successfulRequestRecordIntegrationConfig(strings.Repeat("ab", 32))
	encryptor, err := NewAESEncryptor(cfg)
	require.NoError(t, err)
	repository := NewSuccessfulRequestRecordRepository(integrationDB)
	recorder := service.NewSuccessfulRequestRecordService(
		repository,
		integrationRedis,
		encryptor,
		successfulRequestRecordIntegrationSettings(cfg),
		service.SuccessfulRequestRecordQueueOptions{
			StreamName:       streamName,
			ConsumerGroup:    "test-consumer-group",
			PublishTimeout:   500 * time.Millisecond,
			ConsumeBatchSize: 10,
			ClaimIdle:        time.Second,
			MaxRetries:       3,
		},
	)

	requestBody := `{"model":"integration-model","prompt":"secret-prompt"}`
	responseBody := `{"id":"response-integration"}`
	record := &service.SuccessfulRequestRecord{
		EventID:             eventID,
		UserID:              10,
		APIKeyID:            20,
		TraceID:             uuid.NewString(),
		RequestID:           uuid.NewString(),
		ClientRequestID:     uuid.NewString(),
		Method:              "POST",
		Endpoint:            "/v1/messages",
		RoutePattern:        "/v1/messages",
		Model:               "integration-model",
		StatusCode:          200,
		RequestContentType:  "application/json",
		ResponseContentType: "application/json",
		RequestBody:         &requestBody,
		ResponseBody:        &responseBody,
		RequestBodyBytes:    int64(len(requestBody)),
		ResponseBodyBytes:   int64(len(responseBody)),
		CreatedAt:           time.Now().UTC(),
	}

	require.NoError(t, recorder.Publish(ctx, record))
	messages, err := integrationRedis.XRange(ctx, streamName, "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, messages, 1)
	encryptedPayload, ok := messages[0].Values["payload"].(string)
	require.True(t, ok)
	require.NotContains(t, encryptedPayload, "secret-prompt")
	decryptedPayload, err := encryptor.Decrypt(encryptedPayload)
	require.NoError(t, err)
	var queuedRecord service.SuccessfulRequestRecord
	require.NoError(t, json.Unmarshal([]byte(decryptedPayload), &queuedRecord))
	require.Equal(t, eventID, queuedRecord.EventID)
	require.NotNil(t, queuedRecord.RequestBody)
	require.Equal(t, requestBody, *queuedRecord.RequestBody)

	recorder.Start()
	t.Cleanup(recorder.Stop)
	require.Eventually(t, func() bool {
		var storedRequestBody string
		err := integrationDB.QueryRowContext(ctx, `
			SELECT request_body
			FROM usage_log_payloads
			WHERE event_id = $1
		`, eventID).Scan(&storedRequestBody)
		return err == nil && storedRequestBody == requestBody
	}, 5*time.Second, 50*time.Millisecond)
	require.Eventually(t, func() bool {
		length, err := integrationRedis.XLen(ctx, streamName).Result()
		return err == nil && length == 0
	}, 5*time.Second, 50*time.Millisecond)
}

func TestSuccessfulRequestRecordRedisStreamMovesPoisonMessageToDLQ(t *testing.T) {
	ctx := context.Background()
	streamName := "test:usage-log-payloads-dlq:" + uuid.NewString()
	t.Cleanup(func() {
		_, _ = integrationRedis.Del(context.Background(), streamName, streamName+":dlq").Result()
	})

	cfg := successfulRequestRecordIntegrationConfig(strings.Repeat("cd", 32))
	encryptor, err := NewAESEncryptor(cfg)
	require.NoError(t, err)
	recorder := service.NewSuccessfulRequestRecordService(
		NewSuccessfulRequestRecordRepository(integrationDB),
		integrationRedis,
		encryptor,
		successfulRequestRecordIntegrationSettings(cfg),
		service.SuccessfulRequestRecordQueueOptions{
			StreamName:       streamName,
			ConsumerGroup:    "test-consumer-group",
			PublishTimeout:   500 * time.Millisecond,
			ConsumeBatchSize: 10,
			ClaimIdle:        time.Second,
			MaxRetries:       1,
		},
	)

	require.NoError(t, integrationRedis.XAdd(ctx, &redis.XAddArgs{
		Stream: streamName,
		Values: map[string]any{
			"event_id": uuid.NewString(),
			"payload":  "not-an-aes-gcm-payload",
		},
	}).Err())
	recorder.Start()
	t.Cleanup(recorder.Stop)

	require.Eventually(t, func() bool {
		length, err := integrationRedis.XLen(ctx, streamName+":dlq").Result()
		return err == nil && length == 1
	}, 5*time.Second, 50*time.Millisecond)
	require.Eventually(t, func() bool {
		length, err := integrationRedis.XLen(ctx, streamName).Result()
		return err == nil && length == 0
	}, 5*time.Second, 50*time.Millisecond)
}

func successfulRequestRecordIntegrationConfig(encryptionKey string) *config.Config {
	return &config.Config{Totp: config.TotpConfig{
		EncryptionKey:           encryptionKey,
		EncryptionKeyConfigured: true,
	}}
}

type successfulRequestRecordIntegrationSettingRepo struct{}

func (successfulRequestRecordIntegrationSettingRepo) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
}

func (successfulRequestRecordIntegrationSettingRepo) GetValue(context.Context, string) (string, error) {
	return "", service.ErrSettingNotFound
}

func (successfulRequestRecordIntegrationSettingRepo) Set(context.Context, string, string) error {
	return nil
}

func (successfulRequestRecordIntegrationSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		switch key {
		case service.SettingKeySuccessfulRequestRecordsEnabled:
			values[key] = "true"
		case service.SettingKeySuccessfulRequestRecordsMaxBodyBytes:
			values[key] = "1048576"
		}
	}
	return values, nil
}

func (successfulRequestRecordIntegrationSettingRepo) SetMultiple(context.Context, map[string]string) error {
	return nil
}

func (successfulRequestRecordIntegrationSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

func (successfulRequestRecordIntegrationSettingRepo) Delete(context.Context, string) error {
	return nil
}

func successfulRequestRecordIntegrationSettings(cfg *config.Config) *service.SettingService {
	return service.NewSettingService(successfulRequestRecordIntegrationSettingRepo{}, cfg)
}
