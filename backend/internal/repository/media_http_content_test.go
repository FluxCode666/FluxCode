package repository

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type mediaHTTPUpstreamStub struct {
	called       bool
	secureCalled bool
	policy       service.SecureHTTPUpstreamPolicy
	do           func(*http.Request) (*http.Response, error)
}

type mediaOrdinaryHTTPUpstreamStub struct {
	called bool
	do     func(*http.Request) (*http.Response, error)
}

func (s *mediaHTTPUpstreamStub) DoSecure(
	req *http.Request,
	_ string,
	_ int64,
	_ int,
	policy service.SecureHTTPUpstreamPolicy,
) (*http.Response, error) {
	s.secureCalled = true
	s.policy = policy
	return s.do(req)
}

func (s *mediaOrdinaryHTTPUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.called = true
	return s.do(req)
}

func (s *mediaOrdinaryHTTPUpstreamStub) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, "", 0, 0)
}

type mediaContentTrackedBody struct {
	io.Reader
	closed chan struct{}
}

func (b *mediaContentTrackedBody) Close() error {
	select {
	case <-b.closed:
	default:
		close(b.closed)
	}
	return nil
}

func (s *mediaHTTPUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.called = true
	return s.do(req)
}

func (s *mediaHTTPUpstreamStub) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, "", 0, 0)
}

func mediaContentTestConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{MaxRequestBodySize: 8},
		Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
			Enabled: true, UpstreamHosts: []string{"media.example"}, AllowInsecureHTTP: true,
		}},
	}
}

func TestMediaHTTPContentReaderRejectsPrivateAddress(t *testing.T) {
	upstream := &mediaOrdinaryHTTPUpstreamStub{do: func(*http.Request) (*http.Response, error) {
		t.Fatal("HTTP upstream must not be called for a private address")
		return nil, nil
	}}
	reader := NewMediaHTTPContentReader(upstream, mediaContentTestConfig())

	_, err := reader.Open(context.Background(), service.MediaHTTPContentRequest{
		URL: "http://127.0.0.1/private.mp4", Account: &service.Account{ID: 1, Concurrency: 1},
	})
	require.Error(t, err)
	require.False(t, upstream.called)
}

func TestMediaHTTPContentReaderRejectsURLUserinfoDuringValidation(t *testing.T) {
	reader := NewMediaHTTPContentReader(&mediaHTTPUpstreamStub{}, mediaContentTestConfig())

	normalized, err := reader.ValidateURL("https://user:pass@media.example/video.mp4")
	require.Error(t, err)
	require.Empty(t, normalized)
}

func TestMediaHTTPContentReaderFailsClosedWithoutSecureUpstream(t *testing.T) {
	upstream := &mediaOrdinaryHTTPUpstreamStub{do: func(*http.Request) (*http.Response, error) {
		t.Fatal("ordinary upstream path must not be used for media content")
		return nil, nil
	}}
	reader := NewMediaHTTPContentReader(upstream, mediaContentTestConfig())
	_, err := reader.Open(context.Background(), service.MediaHTTPContentRequest{
		URL: "http://media.example/video.mp4", Account: &service.Account{ID: 1, Concurrency: 1},
	})
	require.ErrorIs(t, err, service.ErrMediaSecureUpstreamRequired)
	require.False(t, upstream.called)
}

func TestMediaHTTPContentReaderUsesSecurePolicyWhenGlobalConfigIsLoose(t *testing.T) {
	cfg := mediaContentTestConfig()
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowPrivateHosts = true
	upstream := &mediaHTTPUpstreamStub{do: func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"video/mp4"}},
			ContentLength: 1, Body: io.NopCloser(strings.NewReader("x")),
		}, nil
	}}
	reader := NewMediaHTTPContentReader(upstream, cfg)
	content, err := reader.Open(context.Background(), service.MediaHTTPContentRequest{
		URL: "http://media.example/video.mp4", Account: &service.Account{ID: 1, Concurrency: 1},
	})
	require.NoError(t, err)
	require.NoError(t, content.Body.Close())
	require.True(t, upstream.secureCalled)
	require.True(t, upstream.policy.AllowInsecureHTTP)
	require.Equal(t, []string{"media.example"}, upstream.policy.AllowedHosts)
}

func TestMediaHTTPContentReaderForwardsOnlyAllowedHeadersAndSingleRange(t *testing.T) {
	upstream := &mediaHTTPUpstreamStub{do: func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "Bearer internal", req.Header.Get("Authorization"))
		require.Empty(t, req.Header.Get("Connection"))
		require.Empty(t, req.Header.Get("Cookie"))
		require.Equal(t, "bytes=2-5", req.Header.Get("Range"))
		return &http.Response{
			StatusCode: http.StatusPartialContent,
			Header: http.Header{
				"Content-Type":  []string{"video/mp4"},
				"Content-Range": []string{"bytes 2-5/10"},
				"Accept-Ranges": []string{"bytes"},
			},
			ContentLength: 4,
			Body:          io.NopCloser(strings.NewReader("2345")),
		}, nil
	}}
	reader := NewMediaHTTPContentReader(upstream, mediaContentTestConfig())

	content, err := reader.Open(context.Background(), service.MediaHTTPContentRequest{
		URL: "http://media.example/video.mp4", Account: &service.Account{ID: 1, Concurrency: 1},
		Headers: http.Header{
			"Authorization": []string{"Bearer internal"}, "Connection": []string{"keep-alive"}, "Cookie": []string{"secret"},
		},
		ByteRange: "bytes=2-5",
	})
	require.NoError(t, err)
	defer content.Body.Close()
	require.Equal(t, http.StatusPartialContent, content.StatusCode)
	require.Equal(t, "bytes 2-5/10", content.ContentRange)
}

func TestMediaHTTPContentReaderRejectsMultipleRanges(t *testing.T) {
	upstream := &mediaHTTPUpstreamStub{do: func(*http.Request) (*http.Response, error) {
		t.Fatal("HTTP upstream must not be called for multiple ranges")
		return nil, nil
	}}
	reader := NewMediaHTTPContentReader(upstream, mediaContentTestConfig())

	_, err := reader.Open(context.Background(), service.MediaHTTPContentRequest{
		URL: "http://media.example/video.mp4", Account: &service.Account{ID: 1, Concurrency: 1}, ByteRange: "bytes=0-1,4-5",
	})
	require.ErrorIs(t, err, service.ErrInvalidMediaRange)
}

func TestMediaHTTPContentReaderRejectsNonASCIIRangeWithoutUpstream(t *testing.T) {
	upstream := &mediaHTTPUpstreamStub{do: func(*http.Request) (*http.Response, error) {
		t.Fatal("HTTP upstream must not be called for a non-ASCII range")
		return nil, nil
	}}
	reader := NewMediaHTTPContentReader(upstream, mediaContentTestConfig())

	_, err := reader.Open(context.Background(), service.MediaHTTPContentRequest{
		URL: "http://media.example/video.mp4", Account: &service.Account{ID: 1, Concurrency: 1}, ByteRange: "bytes=+0-+1",
	})
	require.ErrorIs(t, err, service.ErrInvalidMediaRange)
}

func TestSafeMediaContentTypeAllowsOnlyVideo(t *testing.T) {
	require.Equal(t, "video/mp4", safeMediaContentType("video/mp4; charset=binary"))
	for _, unsafe := range []string{"", "text/html", "application/javascript", "image/svg+xml"} {
		require.Equal(t, "application/octet-stream", safeMediaContentType(unsafe))
	}
}

func TestMediaHTTPContentReaderLimitsBodyWithoutContentLengthAndCancelsOnClose(t *testing.T) {
	requestCanceled := make(chan struct{})
	bodyClosed := make(chan struct{})
	upstream := &mediaHTTPUpstreamStub{do: func(req *http.Request) (*http.Response, error) {
		go func() {
			<-req.Context().Done()
			close(requestCanceled)
		}()
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"video/mp4"}},
			ContentLength: -1, Body: &mediaContentTrackedBody{Reader: strings.NewReader("0123456789"), closed: bodyClosed},
		}, nil
	}}
	reader := NewMediaHTTPContentReader(upstream, mediaContentTestConfig())
	content, err := reader.Open(context.Background(), service.MediaHTTPContentRequest{
		URL: "http://media.example/video.mp4", Account: &service.Account{ID: 1, Concurrency: 1},
	})
	require.NoError(t, err)
	_, err = io.ReadAll(content.Body)
	require.ErrorIs(t, err, service.ErrMediaContentTooLarge)
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("request context was not canceled when the streaming limit was exceeded")
	}
	select {
	case <-bodyClosed:
	case <-time.After(time.Second):
		t.Fatal("upstream response body was not closed when the streaming limit was exceeded")
	}
	require.NoError(t, content.Body.Close())
}

func TestMediaHTTPContentReaderClosesDeclaredOversizeResponse(t *testing.T) {
	bodyClosed := make(chan struct{})
	requestCanceled := make(chan struct{})
	upstream := &mediaHTTPUpstreamStub{do: func(req *http.Request) (*http.Response, error) {
		go func() {
			<-req.Context().Done()
			close(requestCanceled)
		}()
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{}, ContentLength: 9,
			Body: &mediaContentTrackedBody{Reader: strings.NewReader("012345678"), closed: bodyClosed},
		}, nil
	}}
	reader := NewMediaHTTPContentReader(upstream, mediaContentTestConfig())
	_, err := reader.Open(context.Background(), service.MediaHTTPContentRequest{
		URL: "http://media.example/video.mp4", Account: &service.Account{ID: 1, Concurrency: 1},
	})
	require.ErrorIs(t, err, service.ErrMediaContentTooLarge)
	select {
	case <-bodyClosed:
	case <-time.After(time.Second):
		t.Fatal("declared oversized response body was not closed")
	}
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("declared oversized request context was not canceled")
	}
}

func TestMediaHTTPContentReaderMaps416AndClosesResponse(t *testing.T) {
	bodyClosed := make(chan struct{})
	upstream := &mediaHTTPUpstreamStub{do: func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusRequestedRangeNotSatisfiable, Header: http.Header{}, ContentLength: 0,
			Body: &mediaContentTrackedBody{Reader: strings.NewReader(""), closed: bodyClosed},
		}, nil
	}}
	reader := NewMediaHTTPContentReader(upstream, mediaContentTestConfig())
	_, err := reader.Open(context.Background(), service.MediaHTTPContentRequest{
		URL: "http://media.example/video.mp4", Account: &service.Account{ID: 1, Concurrency: 1}, ByteRange: "bytes=99-100",
	})
	require.ErrorIs(t, err, service.ErrMediaRangeNotSatisfiable)
	select {
	case <-bodyClosed:
	default:
		t.Fatal("416 response body was not closed")
	}
}

func TestMediaHTTPContentReaderRejectsRedirectToPrivateResolvedAddress(t *testing.T) {
	upstream := &mediaHTTPUpstreamStub{do: func(original *http.Request) (*http.Response, error) {
		redirect, err := http.NewRequestWithContext(original.Context(), http.MethodGet, "http://127.0.0.1/private.mp4", nil)
		require.NoError(t, err)
		safety := &httpUpstreamService{cfg: mediaContentTestConfig()}
		return nil, safety.redirectChecker(redirect, []*http.Request{original})
	}}
	reader := NewMediaHTTPContentReader(upstream, mediaContentTestConfig())
	_, err := reader.Open(context.Background(), service.MediaHTTPContentRequest{
		URL: "http://media.example/video.mp4", Account: &service.Account{ID: 1, Concurrency: 1},
	})
	require.Error(t, err)
}

func TestMediaHTTPContentReaderUsesHTTPUpstreamDNSValidationBeforeDial(t *testing.T) {
	cfg := mediaContentTestConfig()
	cfg.Security.URLAllowlist.UpstreamHosts = []string{"media.invalid"}
	reader := NewMediaHTTPContentReader(NewHTTPUpstream(cfg), cfg)
	_, err := reader.Open(context.Background(), service.MediaHTTPContentRequest{
		URL: "http://media.invalid/video.mp4", Account: &service.Account{ID: 1, Concurrency: 1},
	})
	require.Error(t, err)
}
