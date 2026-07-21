package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	MediaStorageProviderLegacy = "legacy"
	MediaStorageProviderLocal  = "local"
	MediaStorageProviderMinIO  = "minio"

	settingKeyMediaStorageConfig = MediaStorageSettingKey
	defaultMediaStoragePrefix    = "media"
	defaultMediaStorageRegion    = "us-east-1"
	mediaStorageProbeTimeout     = 15 * time.Second
)

// MediaMinIOConfig is the S3-compatible configuration used only by the new
// media artifact pipeline. It intentionally does not reuse the backup or the
// legacy generated-image storage settings.
type MediaMinIOConfig struct {
	Endpoint                  string `json:"endpoint"`
	Bucket                    string `json:"bucket"`
	AccessKeyID               string `json:"access_key_id"`
	SecretAccessKey           string `json:"secret_access_key,omitempty"`
	SecretAccessKeyConfigured bool   `json:"secret_access_key_configured,omitempty"`
	Region                    string `json:"region"`
	UseSSL                    bool   `json:"use_ssl"`
	ForcePathStyle            bool   `json:"force_path_style"`
	Prefix                    string `json:"prefix"`
}

type MediaStorageConfig struct {
	Provider  string           `json:"provider"`
	LocalPath string           `json:"local_path"`
	MinIO     MediaMinIOConfig `json:"minio"`
}

// MediaStorageConfigTester performs a real write/read/delete probe. The
// concrete storage layer implements it so saving settings and testing a draft
// use the same client and validation semantics.
type MediaStorageConfigTester interface {
	TestMediaStorageConfig(ctx context.Context, cfg MediaStorageConfig) error
}

// MediaStorageArtifactUsageRepository prevents changing a provider locator
// while artifacts still depend on it. Credentials are intentionally excluded
// from the locator comparison so they can be rotated safely.
type MediaStorageArtifactUsageRepository interface {
	HasArtifactsForStorageProvider(ctx context.Context, provider string) (bool, error)
}

type MediaStorageSettingsService struct {
	repo        SettingRepository
	cfg         *config.Config
	encryptor   SecretEncryptor
	tester      MediaStorageConfigTester
	usage       MediaStorageArtifactUsageRepository
	consistency MediaStorageConsistencyRepository
}

func NewMediaStorageSettingsService(
	repo SettingRepository,
	cfg *config.Config,
	encryptor SecretEncryptor,
) *MediaStorageSettingsService {
	return &MediaStorageSettingsService{repo: repo, cfg: cfg, encryptor: encryptor}
}

func (s *MediaStorageSettingsService) SetTester(tester MediaStorageConfigTester) {
	if s != nil {
		s.tester = tester
	}
}

func (s *MediaStorageSettingsService) SetArtifactUsageRepository(usage MediaStorageArtifactUsageRepository) {
	if s != nil {
		s.usage = usage
	}
}

func (s *MediaStorageSettingsService) SetConsistencyRepository(consistency MediaStorageConsistencyRepository) {
	if s != nil {
		s.consistency = consistency
	}
}

func (s *MediaStorageSettingsService) ResolveMediaStorageProvider(ctx context.Context) (string, error) {
	cfg, err := s.LoadEffectiveConfig(ctx)
	if err != nil {
		return "", err
	}
	return cfg.Provider, nil
}

// GetConfig returns the effective configuration with the MinIO secret removed.
func (s *MediaStorageSettingsService) GetConfig(ctx context.Context) (*MediaStorageConfig, error) {
	cfg, err := s.LoadEffectiveConfig(ctx)
	if err != nil {
		return nil, err
	}
	cfg.MinIO.SecretAccessKeyConfigured = strings.TrimSpace(cfg.MinIO.SecretAccessKey) != ""
	cfg.MinIO.SecretAccessKey = ""
	return cfg, nil
}

// LoadEffectiveConfig is for trusted server-side consumers. The returned
// secret is decrypted and must never be serialized to an API response.
func (s *MediaStorageSettingsService) LoadEffectiveConfig(ctx context.Context) (*MediaStorageConfig, error) {
	cfg, _, err := s.LoadEffectiveConfigSnapshot(ctx)
	return cfg, err
}

// LoadEffectiveConfigSnapshot returns the decrypted effective configuration and
// an opaque revision derived from the exact encrypted settings row read from
// the database. The pair must be kept together until Artifact commit.
func (s *MediaStorageSettingsService) LoadEffectiveConfigSnapshot(
	ctx context.Context,
) (*MediaStorageConfig, string, error) {
	if s == nil || s.repo == nil {
		return nil, "", errors.New("media storage settings repository is nil")
	}
	raw, err := s.repo.GetValue(ctx, settingKeyMediaStorageConfig)
	found := true
	if err != nil {
		if !errors.Is(err, ErrSettingNotFound) {
			return nil, "", fmt.Errorf("load media storage config: %w", err)
		}
		found = false
		raw = ""
	}
	if !found {
		cfg := s.defaultConfig()
		return &cfg, MediaStorageSettingRevision("", false), nil
	}
	var stored MediaStorageConfig
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, "", fmt.Errorf("decode media storage config: %w", err)
	}
	if stored.MinIO.SecretAccessKey != "" {
		if s.encryptor == nil {
			return nil, "", errors.New("media storage secret encryptor is nil")
		}
		plain, decryptErr := s.encryptor.Decrypt(stored.MinIO.SecretAccessKey)
		if decryptErr != nil {
			return nil, "", fmt.Errorf("decrypt media storage secret: %w", decryptErr)
		}
		stored.MinIO.SecretAccessKey = plain
	}
	if err := normalizeAndValidateMediaStorageConfig(&stored, stored.Provider == MediaStorageProviderMinIO); err != nil {
		return nil, "", fmt.Errorf("stored media storage config is invalid: %w", err)
	}
	return &stored, MediaStorageSettingRevision(raw, true), nil
}

func (s *MediaStorageSettingsService) UpdateConfig(ctx context.Context, input MediaStorageConfig) (*MediaStorageConfig, error) {
	if s == nil || s.repo == nil || s.encryptor == nil || s.tester == nil || s.usage == nil || s.consistency == nil {
		return nil, errors.New("media storage settings service is unavailable")
	}
	oldEffective, expectedRevision, err := s.LoadEffectiveConfigSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.MinIO.SecretAccessKey) == "" {
		input.MinIO.SecretAccessKey = oldEffective.MinIO.SecretAccessKey
	}
	if err := normalizeAndValidateMediaStorageConfig(&input, input.Provider == MediaStorageProviderMinIO); err != nil {
		return nil, invalidMediaStorageConfig(err)
	}
	if err := s.rejectLocatorChangesInUse(ctx, *oldEffective, input); err != nil {
		return nil, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, mediaStorageProbeTimeout)
	defer cancel()
	if err := s.tester.TestMediaStorageConfig(probeCtx, input); err != nil {
		return nil, infraerrors.BadRequest(
			"MEDIA_STORAGE_PROBE_FAILED",
			fmt.Sprintf("media storage write/read/delete probe failed: %v", err),
		)
	}

	stored := input
	stored.MinIO.SecretAccessKeyConfigured = false
	if stored.MinIO.SecretAccessKey != "" {
		encrypted, encryptErr := s.encryptor.Encrypt(stored.MinIO.SecretAccessKey)
		if encryptErr != nil {
			return nil, fmt.Errorf("encrypt media storage secret: %w", encryptErr)
		}
		stored.MinIO.SecretAccessKey = encrypted
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("encode media storage config: %w", err)
	}
	changedProviders := changedMediaStorageLocatorProviders(*oldEffective, input)
	if err := s.consistency.CommitConfig(ctx, expectedRevision, string(encoded), changedProviders); err != nil {
		switch {
		case errors.Is(err, ErrMediaStorageConfigChanged):
			return nil, infraerrors.Conflict(
				"MEDIA_STORAGE_CONFIG_CHANGED",
				"media storage settings changed concurrently; reload and retry",
			).WithCause(err)
		case errors.Is(err, ErrMediaStorageLocationInUse):
			return nil, infraerrors.BadRequest(
				"MEDIA_STORAGE_LOCATION_IN_USE",
				"storage location cannot be changed while historical artifacts still use it",
			).WithCause(err)
		default:
			return nil, fmt.Errorf("save media storage config: %w", err)
		}
	}
	input.MinIO.SecretAccessKeyConfigured = input.MinIO.SecretAccessKey != ""
	input.MinIO.SecretAccessKey = ""
	return &input, nil
}

func changedMediaStorageLocatorProviders(oldConfig, newConfig MediaStorageConfig) []string {
	providers := make([]string, 0, 2)
	for _, provider := range []string{MediaStorageProviderLocal, MediaStorageProviderMinIO} {
		if !mediaStorageLocatorEqual(oldConfig, newConfig, provider) {
			providers = append(providers, provider)
		}
	}
	return providers
}

func (s *MediaStorageSettingsService) rejectLocatorChangesInUse(
	ctx context.Context,
	oldConfig MediaStorageConfig,
	newConfig MediaStorageConfig,
) error {
	for _, provider := range []string{MediaStorageProviderLocal, MediaStorageProviderMinIO} {
		if mediaStorageLocatorEqual(oldConfig, newConfig, provider) {
			continue
		}
		inUse, err := s.usage.HasArtifactsForStorageProvider(ctx, provider)
		if err != nil {
			return fmt.Errorf("check media storage artifacts for provider %s: %w", provider, err)
		}
		if inUse {
			return infraerrors.BadRequest(
				"MEDIA_STORAGE_LOCATION_IN_USE",
				fmt.Sprintf("%s storage location cannot be changed while historical artifacts still use it", provider),
			)
		}
	}
	return nil
}

func mediaStorageLocatorEqual(left, right MediaStorageConfig, provider string) bool {
	switch provider {
	case MediaStorageProviderLocal:
		return filepath.Clean(left.LocalPath) == filepath.Clean(right.LocalPath)
	case MediaStorageProviderMinIO:
		return left.MinIO.Endpoint == right.MinIO.Endpoint &&
			left.MinIO.Bucket == right.MinIO.Bucket &&
			left.MinIO.Prefix == right.MinIO.Prefix
	default:
		return false
	}
}

func (s *MediaStorageSettingsService) TestConfig(ctx context.Context, input MediaStorageConfig) error {
	if s == nil || s.repo == nil || s.encryptor == nil || s.tester == nil {
		return errors.New("media storage connection tester is unavailable")
	}
	if strings.TrimSpace(input.MinIO.SecretAccessKey) == "" {
		old, found, err := s.loadStoredConfig(ctx)
		if err != nil {
			return err
		}
		if found && old.MinIO.SecretAccessKey != "" {
			plain, decryptErr := s.encryptor.Decrypt(old.MinIO.SecretAccessKey)
			if decryptErr != nil {
				return fmt.Errorf("decrypt existing media storage secret: %w", decryptErr)
			}
			input.MinIO.SecretAccessKey = plain
		}
	}
	if err := normalizeAndValidateMediaStorageConfig(&input, input.Provider == MediaStorageProviderMinIO); err != nil {
		return invalidMediaStorageConfig(err)
	}
	return s.tester.TestMediaStorageConfig(ctx, input)
}

func (s *MediaStorageSettingsService) loadStoredConfig(ctx context.Context) (MediaStorageConfig, bool, error) {
	raw, err := s.repo.GetValue(ctx, settingKeyMediaStorageConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return MediaStorageConfig{}, false, nil
		}
		return MediaStorageConfig{}, false, fmt.Errorf("load media storage config: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return MediaStorageConfig{}, false, nil
	}
	var cfg MediaStorageConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return MediaStorageConfig{}, false, fmt.Errorf("decode media storage config: %w", err)
	}
	return cfg, true, nil
}

func (s *MediaStorageSettingsService) defaultConfig() MediaStorageConfig {
	localPath := "./data/generated"
	if s != nil && s.cfg != nil && strings.TrimSpace(s.cfg.MediaTasks.LocalStoragePath) != "" {
		localPath = strings.TrimSpace(s.cfg.MediaTasks.LocalStoragePath)
	}
	return MediaStorageConfig{
		Provider:  MediaStorageProviderLocal,
		LocalPath: filepath.Clean(localPath),
		MinIO: MediaMinIOConfig{
			Region: defaultMediaStorageRegion, ForcePathStyle: true, Prefix: defaultMediaStoragePrefix,
		},
	}
}

func normalizeAndValidateMediaStorageConfig(cfg *MediaStorageConfig, requireMinIO bool) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	if cfg.Provider == "" {
		cfg.Provider = MediaStorageProviderLocal
	}
	if cfg.Provider != MediaStorageProviderLocal && cfg.Provider != MediaStorageProviderMinIO {
		return fmt.Errorf("provider must be local or minio")
	}
	cfg.LocalPath = strings.TrimSpace(cfg.LocalPath)
	if cfg.LocalPath == "" {
		return errors.New("local_path is required")
	}
	if strings.ContainsRune(cfg.LocalPath, '\x00') {
		return errors.New("local_path contains an invalid character")
	}
	cfg.LocalPath = filepath.Clean(cfg.LocalPath)

	cfg.MinIO.Endpoint = strings.TrimRight(strings.TrimSpace(cfg.MinIO.Endpoint), "/")
	cfg.MinIO.Bucket = strings.TrimSpace(cfg.MinIO.Bucket)
	cfg.MinIO.AccessKeyID = strings.TrimSpace(cfg.MinIO.AccessKeyID)
	cfg.MinIO.SecretAccessKey = strings.TrimSpace(cfg.MinIO.SecretAccessKey)
	cfg.MinIO.Region = strings.TrimSpace(cfg.MinIO.Region)
	if cfg.MinIO.Region == "" {
		cfg.MinIO.Region = defaultMediaStorageRegion
	}
	cfg.MinIO.Prefix = strings.Trim(strings.TrimSpace(cfg.MinIO.Prefix), "/")
	if cfg.MinIO.Prefix == "" {
		cfg.MinIO.Prefix = defaultMediaStoragePrefix
	}
	if strings.Contains(cfg.MinIO.Prefix, "..") || strings.ContainsRune(cfg.MinIO.Prefix, '\x00') {
		return errors.New("minio prefix is invalid")
	}
	if !requireMinIO {
		return nil
	}
	if cfg.MinIO.Endpoint == "" || cfg.MinIO.Bucket == "" || cfg.MinIO.AccessKeyID == "" || cfg.MinIO.SecretAccessKey == "" {
		return errors.New("minio endpoint, bucket, access_key_id and secret_access_key are required")
	}
	if !strings.Contains(cfg.MinIO.Endpoint, "://") {
		if cfg.MinIO.UseSSL {
			cfg.MinIO.Endpoint = "https://" + cfg.MinIO.Endpoint
		} else {
			cfg.MinIO.Endpoint = "http://" + cfg.MinIO.Endpoint
		}
	}
	parsed, err := url.Parse(cfg.MinIO.Endpoint)
	if err != nil || parsed == nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("minio endpoint must be an absolute HTTP(S) URL without credentials, query or fragment")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("minio endpoint must use HTTP or HTTPS")
	}
	if cfg.MinIO.UseSSL != (parsed.Scheme == "https") {
		return errors.New("minio use_ssl must match endpoint scheme")
	}
	return nil
}

func invalidMediaStorageConfig(err error) error {
	return infraerrors.BadRequest("INVALID_MEDIA_STORAGE_CONFIG", err.Error())
}
