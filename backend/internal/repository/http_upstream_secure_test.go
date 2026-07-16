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
		"Referer":        []string{"https://media.example/video.mp4?sig=secret"},
	}

	err = validateSecureRedirect(redirect, []*http.Request{original}, policy)
	require.NoError(t, err)
	require.Empty(t, redirect.Header.Get("Authorization"))
	require.Empty(t, redirect.Header.Get("X-Api-Key"))
	require.Empty(t, redirect.Header.Get("X-Goog-Api-Key"))
	require.Empty(t, redirect.Header.Get("Referer"))
	require.Equal(t, "video/mp4", redirect.Header.Get("Accept"))
}

func TestSecureHTTPUpstreamNeverRestoresCredentialsAfterCrossOriginRedirect(t *testing.T) {
	policy := service.SecureHTTPUpstreamPolicy{
		AllowedHosts: []string{"media.example", "cdn.example"}, RequireAllowlist: true,
	}
	original, err := http.NewRequest(http.MethodGet, "https://media.example/video.mp4?sig=initial-secret", nil)
	require.NoError(t, err)
	firstCrossOrigin, err := http.NewRequest(http.MethodGet, "https://cdn.example/redirect", nil)
	require.NoError(t, err)
	setSecureRedirectSecrets(firstCrossOrigin)
	require.NoError(t, validateSecureRedirect(firstCrossOrigin, []*http.Request{original}, policy))
	requireSecureRedirectSecretsStripped(t, firstCrossOrigin)

	secondSameOriginURL, err := firstCrossOrigin.URL.Parse("final.mp4")
	require.NoError(t, err)
	secondSameOrigin, err := http.NewRequest(http.MethodGet, secondSameOriginURL.String(), nil)
	require.NoError(t, err)
	setSecureRedirectSecrets(secondSameOrigin)
	require.NoError(t, validateSecureRedirect(secondSameOrigin, []*http.Request{original, firstCrossOrigin}, policy))
	requireSecureRedirectSecretsStripped(t, secondSameOrigin)

	backToInitial, err := http.NewRequest(http.MethodGet, "https://media.example:443/final.mp4", nil)
	require.NoError(t, err)
	setSecureRedirectSecrets(backToInitial)
	require.NoError(t, validateSecureRedirect(backToInitial, []*http.Request{original, firstCrossOrigin, secondSameOrigin}, policy))
	requireSecureRedirectSecretsStripped(t, backToInitial)
}

func setSecureRedirectSecrets(req *http.Request) {
	req.Header = http.Header{
		"Authorization":       []string{"Bearer secret"},
		"Proxy-Authorization": []string{"Basic proxy-secret"},
		"X-Api-Key":           []string{"secret-api-key"},
		"X-Goog-Api-Key":      []string{"secret-google-key"},
		"Cookie":              []string{"session=secret"},
		"Referer":             []string{"https://media.example/video.mp4?sig=signed-secret"},
	}
}

func requireSecureRedirectSecretsStripped(t *testing.T, req *http.Request) {
	t.Helper()
	for _, name := range []string{"Authorization", "Proxy-Authorization", "X-Api-Key", "X-Goog-Api-Key", "Cookie", "Referer"} {
		require.Empty(t, req.Header.Get(name), name)
	}
}

func TestSecureHTTPUpstreamKeepsCredentialsOnSameOriginRedirect(t *testing.T) {
	policy := service.SecureHTTPUpstreamPolicy{AllowedHosts: []string{"media.example"}, RequireAllowlist: true}
	original, err := http.NewRequest(http.MethodGet, "https://media.example/video.mp4", nil)
	require.NoError(t, err)
	redirect, err := http.NewRequest(http.MethodGet, "https://media.example:443/final.mp4", nil)
	require.NoError(t, err)
	redirect.Header.Set("X-Api-Key", "same-origin-key")
	redirect.Header.Set("Referer", "https://media.example/video.mp4?sig=same-origin")

	err = validateSecureRedirect(redirect, []*http.Request{original}, policy)
	require.NoError(t, err)
	require.Equal(t, "same-origin-key", redirect.Header.Get("X-Api-Key"))
	require.Equal(t, "https://media.example/video.mp4?sig=same-origin", redirect.Header.Get("Referer"))
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
		{{IP: net.ParseIP("8.8.8.8")}},
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
	require.Equal(t, "8.8.8.8:443", dialed)
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
