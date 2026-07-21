package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestMediaStorageRevisionIsNeverSerialized(t *testing.T) {
	input, err := json.Marshal(MediaArtifactInput{StorageRevision: "internal-revision"})
	require.NoError(t, err)
	require.NotContains(t, string(input), "internal-revision")
	artifact, err := json.Marshal(MediaArtifact{StorageRevision: "internal-revision"})
	require.NoError(t, err)
	require.NotContains(t, string(artifact), "internal-revision")
}

type mediaStorageSettingRepoStub struct{ values map[string]string }

func (r *mediaStorageSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}
func (r *mediaStorageSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}
func (r *mediaStorageSettingRepoStub) Set(_ context.Context, key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}
func (r *mediaStorageSettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, nil
}
func (r *mediaStorageSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	return nil
}
func (r *mediaStorageSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return nil, nil
}
func (r *mediaStorageSettingRepoStub) Delete(context.Context, string) error { return nil }

type mediaStorageEncryptorStub struct{}

func (mediaStorageEncryptorStub) Encrypt(value string) (string, error) { return "enc:" + value, nil }
func (mediaStorageEncryptorStub) Decrypt(value string) (string, error) {
	if len(value) < 4 || value[:4] != "enc:" {
		return "", errors.New("bad ciphertext")
	}
	return value[4:], nil
}

type mediaStorageTesterStub struct {
	got   MediaStorageConfig
	err   error
	calls int
	hook  func()
}

func (t *mediaStorageTesterStub) TestMediaStorageConfig(_ context.Context, cfg MediaStorageConfig) error {
	t.got = cfg
	t.calls++
	if t.hook != nil {
		t.hook()
	}
	return t.err
}

type mediaStorageArtifactUsageStub struct {
	inUse map[string]bool
	err   error
}

type mediaStorageConsistencyStub struct {
	service *MediaStorageSettingsService
}

func (s *mediaStorageConsistencyStub) CommitConfig(
	ctx context.Context,
	expectedRevision string,
	encodedConfig string,
	changedProviders []string,
) error {
	if s == nil || s.service == nil || s.service.repo == nil {
		return errors.New("missing media storage consistency test service")
	}
	raw, err := s.service.repo.GetValue(ctx, settingKeyMediaStorageConfig)
	found := err == nil
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return err
	}
	if MediaStorageSettingRevision(raw, found) != expectedRevision {
		return ErrMediaStorageConfigChanged
	}
	for _, provider := range changedProviders {
		inUse, usageErr := s.service.usage.HasArtifactsForStorageProvider(ctx, provider)
		if usageErr != nil {
			return usageErr
		}
		if inUse {
			return ErrMediaStorageLocationInUse
		}
	}
	return s.service.repo.Set(ctx, settingKeyMediaStorageConfig, encodedConfig)
}

func (*mediaStorageConsistencyStub) CommitArtifact(
	context.Context,
	string,
	*MediaArtifact,
) (*MediaArtifact, error) {
	return nil, errors.New("unexpected artifact commit")
}

func (s *mediaStorageArtifactUsageStub) HasArtifactsForStorageProvider(_ context.Context, provider string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.inUse[provider], nil
}

func configureMediaStorageSettingsTestService(svc *MediaStorageSettingsService, tester *mediaStorageTesterStub) {
	svc.SetTester(tester)
	svc.SetArtifactUsageRepository(&mediaStorageArtifactUsageStub{inUse: map[string]bool{}})
	svc.SetConsistencyRepository(&mediaStorageConsistencyStub{service: svc})
}

func TestMediaStorageSettingsDefaultsToLocalDeploymentPath(t *testing.T) {
	repo := &mediaStorageSettingRepoStub{values: map[string]string{}}
	svc := NewMediaStorageSettingsService(repo, &config.Config{MediaTasks: config.MediaTaskConfig{LocalStoragePath: "/app/.fluxcode/generated"}}, mediaStorageEncryptorStub{})

	got, err := svc.GetConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, MediaStorageProviderLocal, got.Provider)
	require.Equal(t, "/app/.fluxcode/generated", got.LocalPath)
	require.False(t, got.MinIO.SecretAccessKeyConfigured)
}

func TestMediaStorageSettingsEncryptsAndPreservesSecret(t *testing.T) {
	repo := &mediaStorageSettingRepoStub{values: map[string]string{}}
	svc := NewMediaStorageSettingsService(repo, nil, mediaStorageEncryptorStub{})
	configureMediaStorageSettingsTestService(svc, &mediaStorageTesterStub{})
	input := MediaStorageConfig{
		Provider: MediaStorageProviderMinIO, LocalPath: "./data/generated",
		MinIO: MediaMinIOConfig{Endpoint: "https://minio.example.com", Bucket: "media", AccessKeyID: "access", SecretAccessKey: "secret", UseSSL: true},
	}

	got, err := svc.UpdateConfig(context.Background(), input)
	require.NoError(t, err)
	require.True(t, got.MinIO.SecretAccessKeyConfigured)
	require.Empty(t, got.MinIO.SecretAccessKey)
	require.Contains(t, repo.values[settingKeyMediaStorageConfig], "enc:secret")
	require.NotContains(t, repo.values[settingKeyMediaStorageConfig], `"secret_access_key":"secret"`)

	input.MinIO.SecretAccessKey = ""
	_, err = svc.UpdateConfig(context.Background(), input)
	require.NoError(t, err)
	effective, err := svc.LoadEffectiveConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "secret", effective.MinIO.SecretAccessKey)
}

func TestMediaStorageSettingsTestUsesSavedSecret(t *testing.T) {
	repo := &mediaStorageSettingRepoStub{values: map[string]string{}}
	svc := NewMediaStorageSettingsService(repo, nil, mediaStorageEncryptorStub{})
	configureMediaStorageSettingsTestService(svc, &mediaStorageTesterStub{})
	_, err := svc.UpdateConfig(context.Background(), MediaStorageConfig{
		Provider: MediaStorageProviderMinIO, LocalPath: "./data/generated",
		MinIO: MediaMinIOConfig{Endpoint: "minio.example.com", Bucket: "media", AccessKeyID: "access", SecretAccessKey: "secret", UseSSL: true},
	})
	require.NoError(t, err)
	tester := &mediaStorageTesterStub{}
	svc.SetTester(tester)

	err = svc.TestConfig(context.Background(), MediaStorageConfig{
		Provider: MediaStorageProviderMinIO, LocalPath: "./data/generated",
		MinIO: MediaMinIOConfig{Endpoint: "minio.example.com", Bucket: "media", AccessKeyID: "access", UseSSL: true},
	})
	require.NoError(t, err)
	require.Equal(t, "secret", tester.got.MinIO.SecretAccessKey)
	require.Equal(t, "https://minio.example.com", tester.got.MinIO.Endpoint)
}

func TestMediaStorageSettingsRejectsIncompleteActiveMinIO(t *testing.T) {
	svc := NewMediaStorageSettingsService(&mediaStorageSettingRepoStub{values: map[string]string{}}, nil, mediaStorageEncryptorStub{})
	configureMediaStorageSettingsTestService(svc, &mediaStorageTesterStub{})
	_, err := svc.UpdateConfig(context.Background(), MediaStorageConfig{Provider: MediaStorageProviderMinIO, LocalPath: "./data/generated"})
	require.Error(t, err)
}

func TestMediaStorageSettingsProbesBeforeSaving(t *testing.T) {
	repo := &mediaStorageSettingRepoStub{values: map[string]string{}}
	svc := NewMediaStorageSettingsService(repo, nil, mediaStorageEncryptorStub{})
	tester := &mediaStorageTesterStub{err: errors.New("storage unavailable")}
	configureMediaStorageSettingsTestService(svc, tester)

	_, err := svc.UpdateConfig(context.Background(), MediaStorageConfig{
		Provider: MediaStorageProviderLocal, LocalPath: "./data/generated",
	})
	require.Error(t, err)
	require.Equal(t, 1, tester.calls)
	_, saved := repo.values[settingKeyMediaStorageConfig]
	require.False(t, saved)
}

func TestMediaStorageSettingsDoesNotOverwriteConcurrentConfigChange(t *testing.T) {
	repo := &mediaStorageSettingRepoStub{values: map[string]string{}}
	svc := NewMediaStorageSettingsService(repo, nil, mediaStorageEncryptorStub{})
	tester := &mediaStorageTesterStub{hook: func() {
		require.NoError(t, repo.Set(context.Background(), settingKeyMediaStorageConfig,
			`{"provider":"local","local_path":"/concurrent","minio":{}}`))
	}}
	configureMediaStorageSettingsTestService(svc, tester)

	_, err := svc.UpdateConfig(context.Background(), MediaStorageConfig{
		Provider: MediaStorageProviderLocal, LocalPath: "/requested",
	})
	require.ErrorIs(t, err, ErrMediaStorageConfigChanged)
	require.Contains(t, repo.values[settingKeyMediaStorageConfig], "/concurrent")
}

func TestMediaStorageSettingsRejectsLocatorChangeUsedByHistoricalArtifact(t *testing.T) {
	repo := &mediaStorageSettingRepoStub{values: map[string]string{}}
	svc := NewMediaStorageSettingsService(repo, nil, mediaStorageEncryptorStub{})
	svc.SetTester(&mediaStorageTesterStub{})
	svc.SetArtifactUsageRepository(&mediaStorageArtifactUsageStub{inUse: map[string]bool{MediaStorageProviderLocal: true}})
	svc.SetConsistencyRepository(&mediaStorageConsistencyStub{service: svc})

	_, err := svc.UpdateConfig(context.Background(), MediaStorageConfig{
		Provider: MediaStorageProviderLocal, LocalPath: "./another/generated",
	})
	require.Error(t, err)
	require.NotContains(t, repo.values, settingKeyMediaStorageConfig)
}

func TestMediaStorageSettingsAllowsMinIOCredentialRotation(t *testing.T) {
	repo := &mediaStorageSettingRepoStub{values: map[string]string{}}
	svc := NewMediaStorageSettingsService(repo, nil, mediaStorageEncryptorStub{})
	configureMediaStorageSettingsTestService(svc, &mediaStorageTesterStub{})
	config := MediaStorageConfig{
		Provider: MediaStorageProviderMinIO, LocalPath: "data/generated",
		MinIO: MediaMinIOConfig{
			Endpoint: "https://minio.example.com", Bucket: "media", Prefix: "generated",
			AccessKeyID: "access", SecretAccessKey: "secret-1", UseSSL: true,
		},
	}
	_, err := svc.UpdateConfig(context.Background(), config)
	require.NoError(t, err)

	svc.SetArtifactUsageRepository(&mediaStorageArtifactUsageStub{inUse: map[string]bool{MediaStorageProviderMinIO: true}})
	config.MinIO.AccessKeyID = "rotated-access"
	config.MinIO.SecretAccessKey = "secret-2"
	_, err = svc.UpdateConfig(context.Background(), config)
	require.NoError(t, err)

	effective, err := svc.LoadEffectiveConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "rotated-access", effective.MinIO.AccessKeyID)
	require.Equal(t, "secret-2", effective.MinIO.SecretAccessKey)
}
