//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRateLimitServiceOpenAIHTML403SkipsAccountPenalty(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 501, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	for _, body := range []string{
		"<!DOCTYPE html><html><body>Forbidden</body></html>",
		"\n  <HTML><body>Forbidden</body></HTML>",
	} {
		require.False(t, service.HandleUpstreamError(context.Background(), account, http.StatusForbidden, http.Header{}, []byte(body)))
	}
	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.tempCalls)
}

func TestRateLimitServiceStructuredOpenAI403StillPenalizes(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 502, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	require.True(t, service.HandleUpstreamError(
		context.Background(), account, http.StatusForbidden, http.Header{},
		[]byte(`{"error":{"message":"account forbidden"}}`),
	))
	require.Equal(t, 1, repo.setErrorCalls)
}

func TestIsHTMLResponse(t *testing.T) {
	require.True(t, isHTMLResponse([]byte(" <!doctype html><html></html>")))
	require.True(t, isHTMLResponse([]byte("<HTML lang=en>")))
	require.False(t, isHTMLResponse([]byte(`{"error":{"message":"forbidden"}}`)))
	require.False(t, isHTMLResponse([]byte("Forbidden")))
}
