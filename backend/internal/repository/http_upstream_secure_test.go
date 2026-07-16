package repository

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

var errSecureDialStopped = errors.New("secure dial stopped")

type secureResolverSequence struct {
	results [][]net.IPAddr
	calls   int
}

func (r *secureResolverSequence) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	index := r.calls
	r.calls++
	if index >= len(r.results) {
		index = len(r.results) - 1
	}
	return append([]net.IPAddr(nil), r.results[index]...), nil
}

func TestSecureHTTPUpstreamRejectsCrossHostRedirect(t *testing.T) {
	policy := service.SecureHTTPUpstreamPolicy{AllowedHosts: []string{"media.example"}, RequireAllowlist: true}
	original, err := http.NewRequest(http.MethodGet, "https://media.example/video.mp4", nil)
	require.NoError(t, err)
	redirect, err := http.NewRequest(http.MethodGet, "https://evil.example/private", nil)
	require.NoError(t, err)

	err = validateSecureRedirect(redirect, []*http.Request{original}, policy)
	require.Error(t, err)
}

func TestSecureHTTPUpstreamRejectsHTTPSDowngrade(t *testing.T) {
	policy := service.SecureHTTPUpstreamPolicy{
		AllowedHosts: []string{"media.example", "cdn.example"}, RequireAllowlist: true, AllowInsecureHTTP: true,
	}
	original, err := http.NewRequest(http.MethodGet, "https://media.example/video.mp4", nil)
	require.NoError(t, err)
	redirect, err := http.NewRequest(http.MethodGet, "http://cdn.example/video.mp4", nil)
	require.NoError(t, err)

	err = validateSecureRedirect(redirect, []*http.Request{original}, policy)
	require.Error(t, err)
}

func TestSecureHTTPUpstreamRejectsPrivateRedirectIndependentOfGlobalConfig(t *testing.T) {
	policy := service.SecureHTTPUpstreamPolicy{AllowedHosts: []string{"media.example"}, AllowInsecureHTTP: true}
	original, err := http.NewRequest(http.MethodGet, "http://media.example/video.mp4", nil)
	require.NoError(t, err)
	redirect, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/private", nil)
	require.NoError(t, err)

	err = validateSecureRedirect(redirect, []*http.Request{original}, policy)
	require.Error(t, err)
}

func TestSecureHTTPUpstreamStripsCredentialsOnCrossOriginRedirect(t *testing.T) {
	policy := service.SecureHTTPUpstreamPolicy{
		AllowedHosts: []string{"media.example", "cdn.example"}, RequireAllowlist: true,
	}
	original, err := http.NewRequest(http.MethodGet, "https://media.example/video.mp4", nil)
	require.NoError(t, err)
	redirect, err := http.NewRequest(http.MethodGet, "https://cdn.example/video.mp4", nil)
	require.NoError(t, err)
	redirect.Header = http.Header{
		"Authorization":  []string{"Bearer secret"},
		"X-Api-Key":      []string{"secret-api-key"},
		"X-Goog-Api-Key": []string{"secret-google-key"},
		"Accept":         []string{"video/mp4"},
	}

	err = validateSecureRedirect(redirect, []*http.Request{original}, policy)
	require.NoError(t, err)
	require.Empty(t, redirect.Header.Get("Authorization"))
	require.Empty(t, redirect.Header.Get("X-Api-Key"))
	require.Empty(t, redirect.Header.Get("X-Goog-Api-Key"))
	require.Equal(t, "video/mp4", redirect.Header.Get("Accept"))
}

func TestSecureHTTPUpstreamKeepsCredentialsOnSameOriginRedirect(t *testing.T) {
	policy := service.SecureHTTPUpstreamPolicy{AllowedHosts: []string{"media.example"}, RequireAllowlist: true}
	original, err := http.NewRequest(http.MethodGet, "https://media.example/video.mp4", nil)
	require.NoError(t, err)
	redirect, err := http.NewRequest(http.MethodGet, "https://media.example:443/final.mp4", nil)
	require.NoError(t, err)
	redirect.Header.Set("X-Api-Key", "same-origin-key")

	err = validateSecureRedirect(redirect, []*http.Request{original}, policy)
	require.NoError(t, err)
	require.Equal(t, "same-origin-key", redirect.Header.Get("X-Api-Key"))
}

func TestSecureHTTPUpstreamFailsClosedForProxy(t *testing.T) {
	upstream := &httpUpstreamService{}
	req, err := http.NewRequest(http.MethodGet, "https://media.example/video.mp4", nil)
	require.NoError(t, err)
	_, err = upstream.DoSecure(req, "http://proxy.example:8080", 1, 1, service.SecureHTTPUpstreamPolicy{
		AllowedHosts: []string{"media.example"}, RequireAllowlist: true,
	})
	require.ErrorIs(t, err, service.ErrSecureHTTPUpstreamProxyUnsupported)
}

func TestSecureHTTPUpstreamBindsValidatedResolutionToDial(t *testing.T) {
	resolver := &secureResolverSequence{results: [][]net.IPAddr{
		{{IP: net.ParseIP("203.0.113.7")}},
		{{IP: net.ParseIP("127.0.0.1")}},
	}}
	var dialed string
	upstream := &httpUpstreamService{
		secureResolver: resolver,
		secureDialContext: func(_ context.Context, _, address string) (net.Conn, error) {
			dialed = address
			return nil, errSecureDialStopped
		},
	}
	dial := upstream.secureDirectDialer()
	_, err := dial(context.Background(), "tcp", "media.example:443")
	require.ErrorIs(t, err, errSecureDialStopped)
	require.Equal(t, 1, resolver.calls)
	require.Equal(t, "203.0.113.7:443", dialed)
}

func TestSecureHTTPUpstreamSkipsPrivateResolvedIPsAndDialsPublicIPv6(t *testing.T) {
	resolver := &secureResolverSequence{results: [][]net.IPAddr{{
		{IP: net.ParseIP("127.0.0.1")},
		{IP: net.ParseIP("2606:4700:4700::1111")},
	}}}
	var dialed string
	upstream := &httpUpstreamService{
		secureResolver: resolver,
		secureDialContext: func(_ context.Context, _, address string) (net.Conn, error) {
			dialed = address
			return nil, errSecureDialStopped
		},
	}
	dial := upstream.secureDirectDialer()
	_, err := dial(context.Background(), "tcp", "media.example:8443")
	require.ErrorIs(t, err, errSecureDialStopped)
	require.Equal(t, "[2606:4700:4700::1111]:8443", dialed)
}
