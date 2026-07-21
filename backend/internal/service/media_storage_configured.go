package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// ConfiguredMediaArtifactObjectStore resolves the active backend at write
// time, while reads and deletes use the provider snapshot persisted with each
// artifact. Concrete clients are cached by a hash of their effective config so
// settings changes take effect without restarting the process.
type ConfiguredMediaArtifactObjectStore struct {
	settings        *MediaStorageSettingsService
	maxContentBytes int64

	mu             sync.Mutex
	stores         map[string]MediaArtifactObjectStore
	revisionStores map[string]MediaArtifactObjectStore
}

func NewConfiguredMediaArtifactObjectStore(
	settings *MediaStorageSettingsService,
	maxContentBytes int64,
) (*ConfiguredMediaArtifactObjectStore, error) {
	if settings == nil {
		return nil, errors.New("media storage settings service is nil")
	}
	if maxContentBytes <= 0 {
		return nil, errors.New("media storage max content bytes must be positive")
	}
	return &ConfiguredMediaArtifactObjectStore{
		settings: settings, maxContentBytes: maxContentBytes,
		stores: make(map[string]MediaArtifactObjectStore), revisionStores: make(map[string]MediaArtifactObjectStore),
	}, nil
}

func (s *ConfiguredMediaArtifactObjectStore) Put(ctx context.Context, input MediaArtifactInput) (*MediaArtifact, error) {
	store, input, provider, revision, err := s.prepareWrite(ctx, input)
	if err != nil {
		return nil, err
	}
	artifact, err := store.Put(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("put media artifact with provider %s: %w", provider, err)
	}
	return finishConfiguredMediaArtifact(artifact, provider, revision)
}

func (s *ConfiguredMediaArtifactObjectStore) PutStream(
	ctx context.Context,
	input MediaArtifactInput,
	body io.Reader,
) (*MediaArtifact, error) {
	store, input, provider, revision, err := s.prepareWrite(ctx, input)
	if err != nil {
		return nil, err
	}
	streamStore, ok := store.(MediaArtifactStreamObjectStore)
	if !ok {
		return nil, fmt.Errorf("%w: provider %s does not support streaming writes", ErrMediaStorageProviderUnavailable, provider)
	}
	artifact, err := streamStore.PutStream(ctx, input, body)
	if err != nil {
		return nil, fmt.Errorf("stream media artifact with provider %s: %w", provider, err)
	}
	return finishConfiguredMediaArtifact(artifact, provider, revision)
}

func (s *ConfiguredMediaArtifactObjectStore) prepareWrite(
	ctx context.Context,
	input MediaArtifactInput,
) (MediaArtifactObjectStore, MediaArtifactInput, string, string, error) {
	cfg, revision, err := s.loadConfigSnapshot(ctx)
	if err != nil {
		return nil, MediaArtifactInput{}, "", "", err
	}
	store, err := s.storeForConfig(ctx, *cfg, cfg.Provider)
	if err != nil {
		return nil, MediaArtifactInput{}, "", "", err
	}
	s.rememberRevisionStore(revision, cfg.Provider, store)
	input.StorageProvider = cfg.Provider
	input.StorageRevision = revision
	return store, input, cfg.Provider, revision, nil
}

func finishConfiguredMediaArtifact(artifact *MediaArtifact, provider, revision string) (*MediaArtifact, error) {
	if artifact == nil || artifact.ObjectKey == "" {
		return nil, ErrMediaStorageIntegrity
	}
	if artifact.StorageProvider == "" {
		artifact.StorageProvider = provider
	}
	if artifact.StorageProvider != provider {
		return nil, ErrMediaStorageProviderConflict
	}
	artifact.StorageRevision = revision
	return artifact, nil
}

func (s *ConfiguredMediaArtifactObjectStore) Open(ctx context.Context, artifact *MediaArtifact, byteRange string) (*MediaContent, error) {
	if artifact == nil {
		return nil, ErrMediaArtifactNotFound
	}
	provider, err := normalizeMediaStorageProvider(artifact.StorageProvider)
	if err != nil || (provider != MediaStorageProviderLocal && provider != MediaStorageProviderMinIO) {
		return nil, fmt.Errorf("%w: %s", ErrMediaStorageProviderUnavailable, artifact.StorageProvider)
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	store, err := s.storeForConfig(ctx, *cfg, provider)
	if err != nil {
		return nil, err
	}
	copy := *artifact
	copy.StorageProvider = provider
	return store.Open(ctx, &copy, byteRange)
}

func (s *ConfiguredMediaArtifactObjectStore) Discard(ctx context.Context, input MediaArtifactInput) error {
	provider, err := normalizeMediaStorageProvider(input.StorageProvider)
	if err != nil || (provider != MediaStorageProviderLocal && provider != MediaStorageProviderMinIO) {
		return fmt.Errorf("%w: %s", ErrMediaStorageProviderUnavailable, input.StorageProvider)
	}
	var store MediaArtifactObjectStore
	if input.StorageRevision != "" {
		store = s.storeForRevision(input.StorageRevision, provider)
	}
	if store == nil {
		cfg, revision, loadErr := s.loadConfigSnapshot(ctx)
		if loadErr != nil {
			return loadErr
		}
		if input.StorageRevision != "" && input.StorageRevision != revision {
			return ErrMediaStorageConfigChanged
		}
		store, err = s.storeForConfig(ctx, *cfg, provider)
		if err != nil {
			return err
		}
		s.rememberRevisionStore(revision, provider, store)
	}
	input.StorageProvider = provider
	return store.Discard(ctx, input)
}

func (s *ConfiguredMediaArtifactObjectStore) Check(ctx context.Context) error {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return err
	}
	return s.TestMediaStorageConfig(ctx, *cfg)
}

func (s *ConfiguredMediaArtifactObjectStore) TestMediaStorageConfig(ctx context.Context, cfg MediaStorageConfig) error {
	if err := normalizeAndValidateMediaStorageConfig(&cfg, cfg.Provider == MediaStorageProviderMinIO); err != nil {
		return invalidMediaStorageConfig(err)
	}
	switch cfg.Provider {
	case MediaStorageProviderLocal:
		store, err := NewLocalMediaArtifactObjectStore(cfg.LocalPath, s.maxContentBytes)
		if err != nil {
			return err
		}
		defer store.Close() //nolint:errcheck
		return store.Check(ctx)
	case MediaStorageProviderMinIO:
		store, err := NewMinIOMediaArtifactObjectStore(ctx, cfg.MinIO, s.maxContentBytes)
		if err != nil {
			return err
		}
		return store.Check(ctx)
	default:
		return ErrInvalidMediaStorageProvider
	}
}

func (s *ConfiguredMediaArtifactObjectStore) loadConfig(ctx context.Context) (*MediaStorageConfig, error) {
	cfg, _, err := s.loadConfigSnapshot(ctx)
	return cfg, err
}

func (s *ConfiguredMediaArtifactObjectStore) loadConfigSnapshot(
	ctx context.Context,
) (*MediaStorageConfig, string, error) {
	if s == nil || s.settings == nil {
		return nil, "", ErrMediaStorageProviderUnavailable
	}
	return s.settings.LoadEffectiveConfigSnapshot(ctx)
}

func (s *ConfiguredMediaArtifactObjectStore) rememberRevisionStore(
	revision string,
	provider string,
	store MediaArtifactObjectStore,
) {
	if s == nil || revision == "" || provider == "" || store == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revisionStores[revision+":"+provider] = store
}

func (s *ConfiguredMediaArtifactObjectStore) storeForRevision(
	revision string,
	provider string,
) MediaArtifactObjectStore {
	if s == nil || revision == "" || provider == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revisionStores[revision+":"+provider]
}

func (s *ConfiguredMediaArtifactObjectStore) storeForConfig(
	ctx context.Context,
	cfg MediaStorageConfig,
	provider string,
) (MediaArtifactObjectStore, error) {
	key, err := mediaStorageClientCacheKey(cfg, provider)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if store := s.stores[key]; store != nil {
		return store, nil
	}
	var store MediaArtifactObjectStore
	switch provider {
	case MediaStorageProviderLocal:
		local, createErr := NewLocalMediaArtifactObjectStore(cfg.LocalPath, s.maxContentBytes)
		if createErr != nil {
			return nil, createErr
		}
		store = local
	case MediaStorageProviderMinIO:
		minio, createErr := NewMinIOMediaArtifactObjectStore(ctx, cfg.MinIO, s.maxContentBytes)
		if createErr != nil {
			return nil, createErr
		}
		store = minio
	default:
		return nil, ErrInvalidMediaStorageProvider
	}
	s.stores[key] = store
	return store, nil
}

func mediaStorageClientCacheKey(cfg MediaStorageConfig, provider string) (string, error) {
	var value any
	switch provider {
	case MediaStorageProviderLocal:
		value = struct {
			Provider string `json:"provider"`
			Path     string `json:"path"`
		}{provider, cfg.LocalPath}
	case MediaStorageProviderMinIO:
		value = struct {
			Provider string           `json:"provider"`
			MinIO    MediaMinIOConfig `json:"minio"`
		}{provider, cfg.MinIO}
	default:
		return "", ErrInvalidMediaStorageProvider
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return provider + ":" + hex.EncodeToString(sum[:]), nil
}
