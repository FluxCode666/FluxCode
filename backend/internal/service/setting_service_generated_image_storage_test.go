package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type generatedImageStorageSettingRepoStub struct {
	values  map[string]string
	updates map[string]string
}

func (s *generatedImageStorageSettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *generatedImageStorageSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *generatedImageStorageSettingRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *generatedImageStorageSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *generatedImageStorageSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	s.updates = make(map[string]string, len(settings))
	for k, v := range settings {
		s.updates[k] = v
	}
	return nil
}

func (s *generatedImageStorageSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for k, v := range s.values {
		out[k] = v
	}
	return out, nil
}

func (s *generatedImageStorageSettingRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestSettingService_ParseSettings_DefaultsGeneratedImageStorageToDB(t *testing.T) {
	svc := NewSettingService(&generatedImageStorageSettingRepoStub{}, &config.Config{})

	settings := svc.parseSettings(map[string]string{})

	require.Equal(t, GeneratedImageStorageSourceDB, settings.GeneratedImageStorageSource)
	require.Equal(t, GeneratedImageStorageSourceDB, settings.GeneratedImageStorageConfigSource)
	require.Equal(t, "openai/generated-images", settings.QiniuPrefix)
	require.True(t, settings.QiniuUseHTTPS)
	require.Equal(t, 30, settings.QiniuUploadTimeoutSeconds)
	require.Equal(t, 3600, settings.QiniuTokenTTLSeconds)
	require.False(t, settings.QiniuSecretKeyConfigured)
}

func TestSettingService_ParseSettings_DefaultsGeneratedImageStorageConfigSourceToUseSource(t *testing.T) {
	svc := NewSettingService(&generatedImageStorageSettingRepoStub{}, &config.Config{})

	settings := svc.parseSettings(map[string]string{
		SettingKeyGeneratedImageStorageSource: GeneratedImageStorageSourceQiniu,
	})

	require.Equal(t, GeneratedImageStorageSourceQiniu, settings.GeneratedImageStorageSource)
	require.Equal(t, GeneratedImageStorageSourceQiniu, settings.GeneratedImageStorageConfigSource)
}

func TestSettingService_UpdateSettings_GeneratedImageStorageQiniuFields(t *testing.T) {
	repo := &generatedImageStorageSettingRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		GeneratedImageStorageSource:       GeneratedImageStorageSourceQiniu,
		GeneratedImageStorageConfigSource: GeneratedImageStorageSourceQiniu,
		QiniuAccessKey:                    " ak ",
		QiniuSecretKey:                    " sk ",
		QiniuBucket:                       " generated-images ",
		QiniuCDNDomain:                    " cdn.example.com ",
		QiniuPrefix:                       " openai/generated ",
		QiniuUseHTTPS:                     true,
		QiniuUploadTimeoutSeconds:         15,
		QiniuTokenTTLSeconds:              600,
		MediaSyncTimeoutBillingPolicy:     MediaTimeoutBillingPolicyPenalty,
		MediaVideoStorageMode:             MediaVideoStorageModeHybrid,
	})

	require.NoError(t, err)
	require.Equal(t, GeneratedImageStorageSourceQiniu, repo.updates[SettingKeyGeneratedImageStorageSource])
	require.Equal(t, GeneratedImageStorageSourceQiniu, repo.updates[SettingKeyGeneratedImageStorageConfigSource])
	require.Equal(t, "ak", repo.updates[SettingKeyQiniuAccessKey])
	require.Equal(t, "sk", repo.updates[SettingKeyQiniuSecretKey])
	require.Equal(t, "generated-images", repo.updates[SettingKeyQiniuBucket])
	require.Equal(t, "cdn.example.com", repo.updates[SettingKeyQiniuCDNDomain])
	require.Equal(t, "openai/generated", repo.updates[SettingKeyQiniuPrefix])
	require.Equal(t, "true", repo.updates[SettingKeyQiniuUseHTTPS])
	require.Equal(t, "15", repo.updates[SettingKeyQiniuUploadTimeoutSeconds])
	require.Equal(t, "600", repo.updates[SettingKeyQiniuTokenTTLSeconds])
}

func TestSettingService_UpdateSettings_AllowsConfiguringQiniuWhileUsingDB(t *testing.T) {
	repo := &generatedImageStorageSettingRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		GeneratedImageStorageSource:       GeneratedImageStorageSourceDB,
		GeneratedImageStorageConfigSource: GeneratedImageStorageSourceQiniu,
		QiniuPrefix:                       "openai/generated",
		QiniuUseHTTPS:                     true,
		MediaSyncTimeoutBillingPolicy:     MediaTimeoutBillingPolicyPenalty,
		MediaVideoStorageMode:             MediaVideoStorageModeHybrid,
	})

	require.NoError(t, err)
	require.Equal(t, GeneratedImageStorageSourceDB, repo.updates[SettingKeyGeneratedImageStorageSource])
	require.Equal(t, GeneratedImageStorageSourceQiniu, repo.updates[SettingKeyGeneratedImageStorageConfigSource])
}

func TestSettingService_UpdateSettings_RejectsInvalidGeneratedImageStorageSource(t *testing.T) {
	repo := &generatedImageStorageSettingRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		GeneratedImageStorageSource:   "filesystem",
		MediaSyncTimeoutBillingPolicy: MediaTimeoutBillingPolicyPenalty,
		MediaVideoStorageMode:         MediaVideoStorageModeHybrid,
	})

	require.Error(t, err)
	require.Equal(t, "INVALID_GENERATED_IMAGE_STORAGE_SOURCE", infraerrors.Reason(err))
	require.Nil(t, repo.updates)
}

func TestSettingService_UpdateSettings_RejectsInvalidGeneratedImageStorageConfigSource(t *testing.T) {
	repo := &generatedImageStorageSettingRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		GeneratedImageStorageSource:       GeneratedImageStorageSourceDB,
		GeneratedImageStorageConfigSource: "filesystem",
		MediaSyncTimeoutBillingPolicy:     MediaTimeoutBillingPolicyPenalty,
		MediaVideoStorageMode:             MediaVideoStorageModeHybrid,
	})

	require.Error(t, err)
	require.Equal(t, "INVALID_GENERATED_IMAGE_STORAGE_CONFIG_SOURCE", infraerrors.Reason(err))
	require.Nil(t, repo.updates)
}

func TestSettingService_GetGeneratedImageStorageSettings_ReadsQiniuFields(t *testing.T) {
	repo := &generatedImageStorageSettingRepoStub{
		values: map[string]string{
			SettingKeyGeneratedImageStorageSource: GeneratedImageStorageSourceQiniu,
			SettingKeyQiniuAccessKey:              "ak",
			SettingKeyQiniuSecretKey:              "sk",
			SettingKeyQiniuBucket:                 "generated-images",
			SettingKeyQiniuCDNDomain:              "cdn.example.com",
			SettingKeyQiniuPrefix:                 "openai/generated",
			SettingKeyQiniuUseHTTPS:               "true",
			SettingKeyQiniuUploadTimeoutSeconds:   "15",
			SettingKeyQiniuTokenTTLSeconds:        "600",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetGeneratedImageStorageSettings(context.Background())

	require.NoError(t, err)
	require.Equal(t, GeneratedImageStorageSourceQiniu, settings.Source)
	require.Equal(t, "ak", settings.QiniuAccessKey)
	require.Equal(t, "sk", settings.QiniuSecretKey)
	require.Equal(t, "generated-images", settings.QiniuBucket)
	require.Equal(t, "cdn.example.com", settings.QiniuCDNDomain)
	require.Equal(t, "openai/generated", settings.QiniuPrefix)
	require.True(t, settings.QiniuUseHTTPS)
	require.Equal(t, 15, settings.QiniuUploadTimeoutSeconds)
	require.Equal(t, 600, settings.QiniuTokenTTLSeconds)
}
