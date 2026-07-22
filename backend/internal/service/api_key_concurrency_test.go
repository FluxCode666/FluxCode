//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type apiKeyConcurrencyCacheStub struct {
	stubConcurrencyCacheForTest
	counts      map[int64]int
	trackedIDs  []int64
	releasedIDs []int64
	requestIDs  []string
}

var _ APIKeyConcurrencyCache = (*apiKeyConcurrencyCacheStub)(nil)

func (c *apiKeyConcurrencyCacheStub) TrackAPIKeySlot(_ context.Context, apiKeyID int64, requestID string) error {
	c.trackedIDs = append(c.trackedIDs, apiKeyID)
	c.requestIDs = append(c.requestIDs, requestID)
	return nil
}

func (c *apiKeyConcurrencyCacheStub) ReleaseAPIKeySlot(_ context.Context, apiKeyID int64, requestID string) error {
	c.releasedIDs = append(c.releasedIDs, apiKeyID)
	c.requestIDs = append(c.requestIDs, requestID)
	return nil
}

func (c *apiKeyConcurrencyCacheStub) GetAPIKeyConcurrencyBatch(_ context.Context, apiKeyIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(apiKeyIDs))
	for _, apiKeyID := range apiKeyIDs {
		result[apiKeyID] = c.counts[apiKeyID]
	}
	return result, nil
}

func TestConcurrencyService_TrackAPIKeySlot(t *testing.T) {
	cache := &apiKeyConcurrencyCacheStub{}
	svc := NewConcurrencyService(cache)

	release := svc.TrackAPIKeySlot(context.Background(), 42)
	require.Equal(t, []int64{42}, cache.trackedIDs)
	require.Len(t, cache.requestIDs, 1)
	require.NotEmpty(t, cache.requestIDs[0])

	release()
	require.Equal(t, []int64{42}, cache.releasedIDs)
	require.Len(t, cache.requestIDs, 2)
	require.Equal(t, cache.requestIDs[0], cache.requestIDs[1])
}

type apiKeyListRepoStub struct {
	apiKeyRepoStub
	keys []APIKey
}

func (s *apiKeyListRepoStub) ListByUserID(_ context.Context, _ int64, params pagination.PaginationParams, _ APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	return s.keys, &pagination.PaginationResult{
		Total:    int64(len(s.keys)),
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    1,
	}, nil
}

func TestAPIKeyService_ListFillsCurrentConcurrency(t *testing.T) {
	repo := &apiKeyListRepoStub{keys: []APIKey{{ID: 11}, {ID: 22}}}
	cache := &apiKeyConcurrencyCacheStub{counts: map[int64]int{11: 3}}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, nil, nil)
	svc.SetConcurrencyService(NewConcurrencyService(cache))

	keys, _, err := svc.List(context.Background(), 1, pagination.DefaultPagination(), APIKeyListFilters{})
	require.NoError(t, err)
	require.Equal(t, 3, keys[0].CurrentConcurrency)
	require.Zero(t, keys[1].CurrentConcurrency)
}
