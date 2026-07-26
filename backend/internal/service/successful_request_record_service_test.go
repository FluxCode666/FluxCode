package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type successfulRequestRecordRepositoryStub struct {
	created    []*SuccessfulRequestRecord
	inserted   bool
	createErr  error
	reconciled int64
	reconcile  int
}

func (s *successfulRequestRecordRepositoryStub) Create(_ context.Context, record *SuccessfulRequestRecord) (bool, error) {
	clone := *record
	s.created = append(s.created, &clone)
	return s.inserted, s.createErr
}

func (s *successfulRequestRecordRepositoryStub) ReconcileUnlinked(_ context.Context, limit int) (int64, error) {
	s.reconcile = limit
	return s.reconciled, nil
}

type successfulRequestRecordEncryptorStub struct {
	encryptedPlaintext string
	encryptErr         error
}

func (s *successfulRequestRecordEncryptorStub) Encrypt(plaintext string) (string, error) {
	s.encryptedPlaintext = plaintext
	if s.encryptErr != nil {
		return "", s.encryptErr
	}
	return "encrypted", nil
}

func (s *successfulRequestRecordEncryptorStub) Decrypt(ciphertext string) (string, error) {
	return ciphertext, nil
}

type successfulRequestRecordSettingsRepoStub struct {
	mu     sync.RWMutex
	values map[string]string
}

func (s *successfulRequestRecordSettingsRepoStub) Get(_ context.Context, key string) (*Setting, error) {
	value, err := s.GetValue(context.Background(), key)
	if err != nil {
		return nil, err
	}
	return &Setting{Key: key, Value: value}, nil
}

func (s *successfulRequestRecordSettingsRepoStub) GetValue(_ context.Context, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (s *successfulRequestRecordSettingsRepoStub) Set(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}

func (s *successfulRequestRecordSettingsRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (s *successfulRequestRecordSettingsRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *successfulRequestRecordSettingsRepoStub) GetAll(_ context.Context) (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]string, len(s.values))
	for key, value := range s.values {
		result[key] = value
	}
	return result, nil
}

func (s *successfulRequestRecordSettingsRepoStub) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
	return nil
}

func successfulRequestRecordSettingService(enabled bool, maxBodyBytes int64) *SettingService {
	repo := &successfulRequestRecordSettingsRepoStub{values: map[string]string{
		SettingKeySuccessfulRequestRecordsEnabled:      strconv.FormatBool(enabled),
		SettingKeySuccessfulRequestRecordsMaxBodyBytes: strconv.FormatInt(maxBodyBytes, 10),
	}}
	return NewSettingService(repo, &config.Config{Totp: config.TotpConfig{EncryptionKeyConfigured: true}})
}

func successfulRequestRecordServiceFixture() *SuccessfulRequestRecord {
	requestBody := `{"model":"test-model"}`
	responseBody := `{"id":"response-1"}`
	return &SuccessfulRequestRecord{
		EventID:           "5ac529da-09e9-46ad-9490-6af9e21517bb",
		UserID:            10,
		APIKeyID:          20,
		TraceID:           "trace-1",
		RequestID:         "request-1",
		ClientRequestID:   "client-request-1",
		Method:            "POST",
		Endpoint:          "/v1/messages",
		StatusCode:        200,
		RequestBody:       &requestBody,
		ResponseBody:      &responseBody,
		RequestBodyBytes:  int64(len(requestBody)),
		ResponseBodyBytes: int64(len(responseBody)),
		CreatedAt:         time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC),
	}
}

func TestSuccessfulRequestRecordServicePublishFallsBackToDatabaseWhenRedisUnavailable(t *testing.T) {
	repository := &successfulRequestRecordRepositoryStub{inserted: true}
	encryptor := &successfulRequestRecordEncryptorStub{}
	service := NewSuccessfulRequestRecordService(repository, nil, encryptor, successfulRequestRecordSettingService(true, 1024))

	err := service.Publish(context.Background(), successfulRequestRecordServiceFixture())

	require.NoError(t, err)
	require.Len(t, repository.created, 1)
	require.NotEmpty(t, encryptor.encryptedPlaintext)
	var encryptedRecord SuccessfulRequestRecord
	require.NoError(t, json.Unmarshal([]byte(encryptor.encryptedPlaintext), &encryptedRecord))
	require.Equal(t, "5ac529da-09e9-46ad-9490-6af9e21517bb", encryptedRecord.EventID)
	require.NotNil(t, encryptedRecord.RequestBody)
	require.Equal(t, `{"model":"test-model"}`, *encryptedRecord.RequestBody)
	require.Equal(t, SuccessfulRequestRecordStats{
		Persisted:       1,
		PublishFallback: 1,
	}, service.Stats())
}

func TestSuccessfulRequestRecordServicePublishReportsMQAndFallbackFailure(t *testing.T) {
	repository := &successfulRequestRecordRepositoryStub{createErr: errors.New("database unavailable")}
	encryptor := &successfulRequestRecordEncryptorStub{encryptErr: errors.New("encryption unavailable")}
	service := NewSuccessfulRequestRecordService(repository, nil, encryptor, successfulRequestRecordSettingService(true, 1024))

	err := service.Publish(context.Background(), successfulRequestRecordServiceFixture())

	require.ErrorContains(t, err, "encryption unavailable")
	require.ErrorContains(t, err, "database unavailable")
	require.Equal(t, SuccessfulRequestRecordStats{
		PublishFallback: 1,
		Failed:          1,
	}, service.Stats())
}

func TestSuccessfulRequestRecordServiceNormalizeDropsOversizedOriginalBodies(t *testing.T) {
	repository := &successfulRequestRecordRepositoryStub{inserted: true}
	service := NewSuccessfulRequestRecordService(repository, nil, &successfulRequestRecordEncryptorStub{}, successfulRequestRecordSettingService(true, 1024))
	record := successfulRequestRecordServiceFixture()
	oversized := strings.Repeat("x", 1025)
	record.RequestBody = &oversized
	record.ResponseBody = &oversized

	err := service.Publish(context.Background(), record)

	require.NoError(t, err)
	require.Nil(t, record.RequestBody)
	require.Nil(t, record.ResponseBody)
	require.True(t, record.RequestTruncated)
	require.True(t, record.ResponseTruncated)
}

func TestSuccessfulRequestRecordServiceDisabledDoesNothing(t *testing.T) {
	repository := &successfulRequestRecordRepositoryStub{inserted: true}
	service := NewSuccessfulRequestRecordService(repository, nil, &successfulRequestRecordEncryptorStub{}, successfulRequestRecordSettingService(false, 1024))

	require.NoError(t, service.Publish(context.Background(), successfulRequestRecordServiceFixture()))
	require.Empty(t, repository.created)
	require.Equal(t, SuccessfulRequestRecordStats{}, service.Stats())
}

func TestSuccessfulRequestRecordConsumerRefusesAutoGeneratedEncryptionKey(t *testing.T) {
	settingsRepo := &successfulRequestRecordSettingsRepoStub{values: map[string]string{
		SettingKeySuccessfulRequestRecordsEnabled:      "true",
		SettingKeySuccessfulRequestRecordsMaxBodyBytes: "1024",
	}}
	settings := NewSettingService(settingsRepo, &config.Config{})
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = redisClient.Close() })
	recorder := NewSuccessfulRequestRecordService(
		&successfulRequestRecordRepositoryStub{inserted: true},
		redisClient,
		&successfulRequestRecordEncryptorStub{},
		settings,
	)

	done := make(chan struct{})
	recorder.wg.Add(1)
	go func() {
		recorder.consumeLoop()
		close(done)
	}()

	select {
	case <-done:
		// 未显式配置固定密钥时，消费者必须在访问 Redis 前退出。
	case <-time.After(time.Second):
		recorder.cancel()
		<-done
		t.Fatal("consumer should not join the stream group without a fixed encryption key")
	}
}

func TestSuccessfulRequestRecordSettingsDefaultToDisabledAndOneMiB(t *testing.T) {
	repo := &successfulRequestRecordSettingsRepoStub{values: map[string]string{}}
	settings := NewSettingService(repo, &config.Config{})

	runtimeSettings, err := settings.GetSuccessfulRequestRecordRuntimeSettings(context.Background())
	require.NoError(t, err)
	require.False(t, runtimeSettings.Enabled)
	require.Equal(t, DefaultSuccessfulRequestRecordsMaxBodyBytes, runtimeSettings.MaxBodyBytes)

	allSettings, err := settings.GetAllSettings(context.Background())
	require.NoError(t, err)
	require.False(t, allSettings.SuccessfulRequestRecordsEnabled)
	require.Equal(t, DefaultSuccessfulRequestRecordsMaxBodyBytes, allSettings.SuccessfulRequestRecordsMaxBodyBytes)
}

func TestSuccessfulRequestRecordServiceRefreshesImmediatelyAfterSettingsUpdate(t *testing.T) {
	repo := &successfulRequestRecordSettingsRepoStub{values: map[string]string{
		SettingKeySuccessfulRequestRecordsEnabled:      "false",
		SettingKeySuccessfulRequestRecordsMaxBodyBytes: "1048576",
	}}
	settings := NewSettingService(repo, &config.Config{Totp: config.TotpConfig{EncryptionKeyConfigured: true}})
	recorder := NewSuccessfulRequestRecordService(
		&successfulRequestRecordRepositoryStub{inserted: true},
		nil,
		&successfulRequestRecordEncryptorStub{},
		settings,
	)
	require.False(t, recorder.Enabled())

	err := settings.UpdateSettings(context.Background(), &SystemSettings{
		SuccessfulRequestRecordsEnabled:      true,
		SuccessfulRequestRecordsMaxBodyBytes: 2048,
	})
	require.NoError(t, err)
	require.True(t, recorder.Enabled())
	require.Equal(t, int64(2048), recorder.MaxBodyBytes())

	err = settings.UpdateSettings(context.Background(), &SystemSettings{
		SuccessfulRequestRecordsEnabled:      false,
		SuccessfulRequestRecordsMaxBodyBytes: 4096,
	})
	require.NoError(t, err)
	require.False(t, recorder.Enabled())
	require.Equal(t, int64(4096), recorder.MaxBodyBytes())
}

func TestSuccessfulRequestRecordServiceRefreshesSettingsWrittenByAnotherInstance(t *testing.T) {
	repo := &successfulRequestRecordSettingsRepoStub{values: map[string]string{
		SettingKeySuccessfulRequestRecordsEnabled:      "false",
		SettingKeySuccessfulRequestRecordsMaxBodyBytes: "1048576",
	}}
	cfg := &config.Config{Totp: config.TotpConfig{EncryptionKeyConfigured: true}}
	writerSettings := NewSettingService(repo, cfg)
	readerSettings := NewSettingService(repo, cfg)
	recorder := NewSuccessfulRequestRecordService(
		&successfulRequestRecordRepositoryStub{inserted: true},
		nil,
		&successfulRequestRecordEncryptorStub{},
		readerSettings,
	)
	require.False(t, recorder.Enabled())

	err := writerSettings.UpdateSettings(context.Background(), &SystemSettings{
		SuccessfulRequestRecordsEnabled:      true,
		SuccessfulRequestRecordsMaxBodyBytes: 8192,
	})
	require.NoError(t, err)
	require.False(t, recorder.Enabled(), "另一实例的进程内回调不应直接修改本实例")

	recorder.refreshRuntimeSettings(context.Background())
	require.True(t, recorder.Enabled())
	require.Equal(t, int64(8192), recorder.MaxBodyBytes())
}

func TestSuccessfulRequestRecordSettingsRequireExplicitEncryptionKey(t *testing.T) {
	repo := &successfulRequestRecordSettingsRepoStub{values: map[string]string{}}
	settings := NewSettingService(repo, &config.Config{})

	err := settings.UpdateSettings(context.Background(), &SystemSettings{
		SuccessfulRequestRecordsEnabled:      true,
		SuccessfulRequestRecordsMaxBodyBytes: 1024,
	})
	require.ErrorContains(t, err, "TOTP_ENCRYPTION_KEY")
}

func TestSuccessfulRequestRecordSettingsRejectOutOfRangeBodyLimit(t *testing.T) {
	repo := &successfulRequestRecordSettingsRepoStub{values: map[string]string{}}
	settings := NewSettingService(repo, &config.Config{Totp: config.TotpConfig{EncryptionKeyConfigured: true}})

	for _, bodyLimit := range []int64{1023, 16*1024*1024 + 1} {
		err := settings.UpdateSettings(context.Background(), &SystemSettings{
			SuccessfulRequestRecordsEnabled:      true,
			SuccessfulRequestRecordsMaxBodyBytes: bodyLimit,
		})
		require.ErrorContains(t, err, "body limit")
	}
}
