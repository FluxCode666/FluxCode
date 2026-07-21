package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	ErrInvalidMediaStorageProvider     = errors.New("invalid media storage provider")
	ErrMediaStorageProviderUnavailable = errors.New("media storage provider unavailable")
	ErrMediaStorageProviderConflict    = errors.New("media storage provider conflict")
	ErrInvalidMediaObjectKey           = errors.New("invalid media object key")
	ErrMediaStorageIntegrity           = errors.New("media storage integrity check failed")
)

// MediaStorageProviderResolver returns the backend used for new writes. Reads
// and deletes never consult it: they are routed by the provider snapshot saved
// on the artifact itself.
type MediaStorageProviderResolver interface {
	ResolveMediaStorageProvider(ctx context.Context) (string, error)
}

type MediaStorageProviderResolverFunc func(context.Context) (string, error)

func (f MediaStorageProviderResolverFunc) ResolveMediaStorageProvider(ctx context.Context) (string, error) {
	if f == nil {
		return "", ErrMediaStorageProviderUnavailable
	}
	return f(ctx)
}

// MediaArtifactObjectStoreHealthChecker is implemented by stores that can run
// a real write/read/delete readiness probe.
type MediaArtifactObjectStoreHealthChecker interface {
	Check(ctx context.Context) error
}

// MediaArtifactObjectStoreRouter keeps historical objects readable when the
// active write provider changes. It deliberately has no provider fallback.
type MediaArtifactObjectStoreRouter struct {
	resolver MediaStorageProviderResolver
	stores   map[string]MediaArtifactObjectStore
}

func NewMediaArtifactObjectStoreRouter(
	resolver MediaStorageProviderResolver,
	stores map[string]MediaArtifactObjectStore,
) (*MediaArtifactObjectStoreRouter, error) {
	if resolver == nil {
		return nil, fmt.Errorf("%w: resolver is nil", ErrMediaStorageProviderUnavailable)
	}
	if len(stores) == 0 {
		return nil, fmt.Errorf("%w: no stores are registered", ErrMediaStorageProviderUnavailable)
	}
	registered := make(map[string]MediaArtifactObjectStore, len(stores))
	for rawProvider, store := range stores {
		provider, err := normalizeMediaStorageProvider(rawProvider)
		if err != nil {
			return nil, err
		}
		if store == nil {
			return nil, fmt.Errorf("%w: provider %s has a nil store", ErrMediaStorageProviderUnavailable, provider)
		}
		if _, exists := registered[provider]; exists {
			return nil, fmt.Errorf("%w: duplicate provider %s", ErrInvalidMediaStorageProvider, provider)
		}
		registered[provider] = store
	}
	return &MediaArtifactObjectStoreRouter{resolver: resolver, stores: registered}, nil
}

func (r *MediaArtifactObjectStoreRouter) Put(ctx context.Context, input MediaArtifactInput) (*MediaArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil || r.resolver == nil {
		return nil, ErrMediaStorageProviderUnavailable
	}
	rawProvider, err := r.resolver.ResolveMediaStorageProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve media storage provider: %w", err)
	}
	provider, store, err := r.store(rawProvider)
	if err != nil {
		return nil, err
	}
	input.StorageProvider = provider
	artifact, err := store.Put(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("put media artifact with provider %s: %w", provider, err)
	}
	if artifact == nil {
		return nil, fmt.Errorf("%w: provider %s returned a nil artifact", ErrMediaStorageIntegrity, provider)
	}
	if artifact.StorageProvider == "" {
		artifact.StorageProvider = provider
	} else {
		actual, normalizeErr := normalizeMediaStorageProvider(artifact.StorageProvider)
		if normalizeErr != nil || actual != provider {
			return nil, fmt.Errorf("%w: selected=%s returned=%s", ErrMediaStorageProviderConflict, provider, artifact.StorageProvider)
		}
		artifact.StorageProvider = actual
	}
	return artifact, nil
}

func (r *MediaArtifactObjectStoreRouter) PutStream(
	ctx context.Context,
	input MediaArtifactInput,
	body io.Reader,
) (*MediaArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil || r.resolver == nil {
		return nil, ErrMediaStorageProviderUnavailable
	}
	rawProvider, err := r.resolver.ResolveMediaStorageProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve media storage provider: %w", err)
	}
	provider, store, err := r.store(rawProvider)
	if err != nil {
		return nil, err
	}
	streamStore, ok := store.(MediaArtifactStreamObjectStore)
	if !ok {
		return nil, fmt.Errorf("%w: provider %s does not support streaming writes", ErrMediaStorageProviderUnavailable, provider)
	}
	input.StorageProvider = provider
	artifact, err := streamStore.PutStream(ctx, input, body)
	if err != nil {
		return nil, fmt.Errorf("stream media artifact with provider %s: %w", provider, err)
	}
	if artifact == nil {
		return nil, fmt.Errorf("%w: provider %s returned a nil artifact", ErrMediaStorageIntegrity, provider)
	}
	if artifact.StorageProvider == "" {
		artifact.StorageProvider = provider
	} else {
		actual, normalizeErr := normalizeMediaStorageProvider(artifact.StorageProvider)
		if normalizeErr != nil || actual != provider {
			return nil, fmt.Errorf("%w: selected=%s returned=%s", ErrMediaStorageProviderConflict, provider, artifact.StorageProvider)
		}
		artifact.StorageProvider = actual
	}
	return artifact, nil
}

func (r *MediaArtifactObjectStoreRouter) Open(
	ctx context.Context,
	artifact *MediaArtifact,
	byteRange string,
) (*MediaContent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if artifact == nil {
		return nil, ErrMediaArtifactNotFound
	}
	provider, store, err := r.store(artifact.StorageProvider)
	if err != nil {
		return nil, err
	}
	copy := *artifact
	copy.StorageProvider = provider
	return store.Open(ctx, &copy, byteRange)
}

func (r *MediaArtifactObjectStoreRouter) Discard(ctx context.Context, input MediaArtifactInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	provider, store, err := r.store(input.StorageProvider)
	if err != nil {
		return err
	}
	input.StorageProvider = provider
	if err := store.Discard(ctx, input); err != nil {
		return fmt.Errorf("discard media artifact with provider %s: %w", provider, err)
	}
	return nil
}

func (r *MediaArtifactObjectStoreRouter) store(rawProvider string) (string, MediaArtifactObjectStore, error) {
	if r == nil {
		return "", nil, ErrMediaStorageProviderUnavailable
	}
	provider, err := normalizeMediaStorageProvider(rawProvider)
	if err != nil {
		return "", nil, err
	}
	store := r.stores[provider]
	if store == nil {
		return "", nil, fmt.Errorf("%w: %s", ErrMediaStorageProviderUnavailable, provider)
	}
	return provider, store, nil
}

func normalizeMediaStorageProvider(raw string) (string, error) {
	provider := strings.ToLower(strings.TrimSpace(raw))
	if provider == "" || len(provider) > 32 {
		return "", fmt.Errorf("%w: %q", ErrInvalidMediaStorageProvider, raw)
	}
	for i := 0; i < len(provider); i++ {
		char := provider[i]
		if (char >= 'a' && char <= 'z') || (i > 0 && char >= '0' && char <= '9') ||
			(i > 0 && (char == '-' || char == '_')) {
			continue
		}
		return "", fmt.Errorf("%w: %q", ErrInvalidMediaStorageProvider, raw)
	}
	return provider, nil
}
