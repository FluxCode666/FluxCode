package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestGeneratedImageCleanupServiceCleanupOnceSkipsWhenDisabled(t *testing.T) {
	store := &generatedImageCleanupStoreStub{}
	settings := NewSettingService(&openAIImagesSettingRepoStub{values: map[string]string{
		SettingKeyGeneratedImageCleanupEnabled: "false",
		SettingKeyOpenAIImageURLCacheTTLHours:  "48",
	}}, &config.Config{})
	svc := NewGeneratedImageCleanupService(store, settings)
	svc.now = func() time.Time {
		return time.Date(2026, 6, 28, 4, 0, 0, 0, time.UTC)
	}

	deleted, err := svc.cleanupOnce(context.Background())

	require.NoError(t, err)
	require.Zero(t, deleted)
	require.Zero(t, store.deleteBeforeCalls)
}

func TestGeneratedImageCleanupServiceCleanupOnceDeletesBeforeConfiguredTTL(t *testing.T) {
	now := time.Date(2026, 6, 28, 4, 0, 0, 0, time.UTC)
	store := &generatedImageCleanupStoreStub{deleted: 3}
	settings := NewSettingService(&openAIImagesSettingRepoStub{values: map[string]string{
		SettingKeyGeneratedImageCleanupEnabled: "true",
		SettingKeyOpenAIImageURLCacheTTLHours:  "48",
	}}, &config.Config{})
	svc := NewGeneratedImageCleanupService(store, settings)
	svc.now = func() time.Time {
		return now
	}

	deleted, err := svc.cleanupOnce(context.Background())

	require.NoError(t, err)
	require.EqualValues(t, 3, deleted)
	require.Equal(t, 1, store.deleteBeforeCalls)
	require.Equal(t, now.Add(-48*time.Hour), store.lastCutoff)
}

func TestGeneratedImageCleanupServiceCleanupOnceReturnsDeleteError(t *testing.T) {
	store := &generatedImageCleanupStoreStub{deleteErr: errors.New("delete failed")}
	settings := NewSettingService(&openAIImagesSettingRepoStub{values: map[string]string{
		SettingKeyGeneratedImageCleanupEnabled: "true",
		SettingKeyOpenAIImageURLCacheTTLHours:  "48",
	}}, &config.Config{})
	svc := NewGeneratedImageCleanupService(store, settings)

	deleted, err := svc.cleanupOnce(context.Background())

	require.ErrorContains(t, err, "delete failed")
	require.Zero(t, deleted)
	require.Equal(t, 1, store.deleteBeforeCalls)
}

func TestNextGeneratedImageCleanupRunAfterUsesFourAMInLocation(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)

	beforeFour := time.Date(2026, 6, 28, 3, 30, 0, 0, loc)
	require.Equal(t,
		time.Date(2026, 6, 28, 4, 0, 0, 0, loc),
		nextGeneratedImageCleanupRunAfter(beforeFour, loc),
	)

	afterFour := time.Date(2026, 6, 28, 4, 1, 0, 0, loc)
	require.Equal(t,
		time.Date(2026, 6, 29, 4, 0, 0, 0, loc),
		nextGeneratedImageCleanupRunAfter(afterFour, loc),
	)
}

func TestSettingServiceGeneratedImageCleanupDefaultsAndUpdates(t *testing.T) {
	repo := &openAIImagesSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	settings := svc.parseSettings(repo.values)
	require.False(t, settings.GeneratedImageCleanupEnabled)
	require.False(t, svc.IsGeneratedImageCleanupEnabled(context.Background()))

	settings.GeneratedImageCleanupEnabled = true
	require.NoError(t, svc.UpdateSettings(context.Background(), settings))

	require.Equal(t, "true", repo.values[SettingKeyGeneratedImageCleanupEnabled])
	require.True(t, svc.IsGeneratedImageCleanupEnabled(context.Background()))
}

type generatedImageCleanupStoreStub struct {
	deleteBeforeCalls int
	lastCutoff        time.Time
	deleted           int64
	deleteErr         error
}

func (s *generatedImageCleanupStoreStub) Create(ctx context.Context, image *GeneratedImage) (*GeneratedImage, error) {
	return image, nil
}

func (s *generatedImageCleanupStoreStub) List(ctx context.Context, params GeneratedImageListParams) ([]GeneratedImage, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (s *generatedImageCleanupStoreStub) GetContent(ctx context.Context, id int64) ([]byte, string, error) {
	return nil, "", nil
}

func (s *generatedImageCleanupStoreStub) DeleteByDateRange(ctx context.Context, startAt, endAt time.Time) (int64, error) {
	return 0, nil
}

func (s *generatedImageCleanupStoreStub) DeleteBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	s.deleteBeforeCalls++
	s.lastCutoff = cutoff
	if s.deleteErr != nil {
		return 0, s.deleteErr
	}
	return s.deleted, nil
}
