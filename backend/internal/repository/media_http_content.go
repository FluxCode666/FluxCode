package repository

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

const (
	defaultMediaContentTimeout  = 2 * time.Minute
	defaultMediaContentMaxBytes = 64 << 20
)

var allowedMediaRequestHeaders = map[string]struct{}{
	"Authorization":  {},
	"Accept":         {},
	"User-Agent":     {},
	"X-Api-Key":      {},
	"X-Goog-Api-Key": {},
}

type mediaHTTPContentReader struct {
	upstream          service.HTTPUpstream
	allowInsecureHTTP bool
	allowedHosts      []string
	requireAllowlist  bool
	timeout           time.Duration
	maxBytes          int64
}

func NewMediaHTTPContentReader(upstream service.HTTPUpstream, cfg *config.Config) service.MediaHTTPContentReader {
	reader := &mediaHTTPContentReader{
		upstream: upstream, timeout: defaultMediaContentTimeout, maxBytes: defaultMediaContentMaxBytes,
	}
	if cfg != nil {
		reader.allowInsecureHTTP = cfg.Security.URLAllowlist.AllowInsecureHTTP
		reader.allowedHosts = append([]string(nil), cfg.Security.URLAllowlist.UpstreamHosts...)
		reader.requireAllowlist = cfg.Security.URLAllowlist.Enabled
		if cfg.Server.MaxRequestBodySize > 0 {
			reader.maxBytes = cfg.Server.MaxRequestBodySize
		}
		if cfg.MediaTasks.ContentProxyTimeoutSeconds > 0 {
			reader.timeout = time.Duration(cfg.MediaTasks.ContentProxyTimeoutSeconds) * time.Second
		}
		if cfg.MediaTasks.MaxContentBytes > 0 {
			reader.maxBytes = cfg.MediaTasks.MaxContentBytes
		}
	}
	return reader
}

func ProvideMediaHTTPContentReader(upstream service.HTTPUpstream, cfg *config.Config) service.MediaHTTPContentReader {
	return NewMediaHTTPContentReader(upstream, cfg)
}

func (r *mediaHTTPContentReader) ValidateURL(raw string) (string, error) {
	if r == nil {
		return "", errors.New("media content reader is nil")
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", errors.New("invalid media content url")
	}
	if parsed.User != nil {
		return "", errors.New("media content url userinfo is not allowed")
	}
	return urlvalidator.ValidateHTTPURL(raw, r.allowInsecureHTTP, urlvalidator.ValidationOptions{
		AllowedHosts: r.allowedHosts, RequireAllowlist: r.requireAllowlist, AllowPrivate: false,
	})
}

func (r *mediaHTTPContentReader) Open(ctx context.Context, input service.MediaHTTPContentRequest) (*service.MediaContent, error) {
	if r == nil || r.upstream == nil {
		return nil, service.ErrMediaContentUnavailable
	}
	if input.Account == nil {
		return nil, service.ErrMediaContentAccountRequired
	}
	normalized, err := r.ValidateURL(input.URL)
	if err != nil {
		return nil, fmt.Errorf("unsafe media content url: %w", err)
	}
	if input.ByteRange != "" {
		if err := service.ValidateMediaRange(input.ByteRange); err != nil {
			return nil, err
		}
	}
	requestCtx, cancel := context.WithTimeout(ctx, r.timeout)
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, normalized, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create media content request: %w", err)
	}
	copyAllowedMediaHeaders(req.Header, input.Headers)
	if input.ByteRange != "" {
		req.Header.Set("Range", input.ByteRange)
	}
	secureUpstream, ok := r.upstream.(service.SecureHTTPUpstream)
	if !ok {
		cancel()
		return nil, service.ErrMediaSecureUpstreamRequired
	}
	resp, err := secureUpstream.DoSecure(
		req,
		input.Account.EffectiveProxyURL(),
		input.Account.ID,
		input.Account.Concurrency,
		service.SecureHTTPUpstreamPolicy{
			AllowedHosts: append([]string(nil), r.allowedHosts...), RequireAllowlist: r.requireAllowlist,
			AllowInsecureHTTP: r.allowInsecureHTTP,
		},
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("request media content: %w", err)
	}
	if resp == nil || resp.Body == nil {
		cancel()
		return nil, service.ErrMediaContentUnavailable
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		_ = resp.Body.Close()
		cancel()
		if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			return nil, service.ErrMediaRangeNotSatisfiable
		}
		return nil, service.ErrMediaContentUnavailable
	}
	if resp.ContentLength > r.maxBytes {
		_ = resp.Body.Close()
		cancel()
		return nil, service.ErrMediaContentTooLarge
	}
	body := &boundedCancelReadCloser{
		body: resp.Body, cancel: cancel, remaining: r.maxBytes,
	}
	return &service.MediaContent{
		Body: body, StatusCode: resp.StatusCode, ContentType: safeMediaContentType(resp.Header.Get("Content-Type")),
		ContentLength: resp.ContentLength, ContentRange: resp.Header.Get("Content-Range"),
		AcceptRanges: resp.Header.Get("Accept-Ranges"),
	}, nil
}

func copyAllowedMediaHeaders(dst, src http.Header) {
	for name, values := range src {
		canonical := http.CanonicalHeaderKey(name)
		if _, ok := allowedMediaRequestHeaders[canonical]; !ok {
			continue
		}
		for _, value := range values {
			dst.Add(canonical, value)
		}
	}
}

func safeMediaContentType(value string) string {
	value = strings.TrimSpace(strings.SplitN(value, "\r", 2)[0])
	value = strings.TrimSpace(strings.SplitN(value, "\n", 2)[0])
	if value == "" {
		return "application/octet-stream"
	}
	return value
}

type boundedCancelReadCloser struct {
	body      io.ReadCloser
	cancel    context.CancelFunc
	remaining int64
	closeOnce sync.Once
	closeErr  error
	done      bool
}

func (r *boundedCancelReadCloser) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	if r.remaining == 0 {
		var probe [1]byte
		n, err := r.body.Read(probe[:])
		if n > 0 {
			_ = r.Close()
			return 0, service.ErrMediaContentTooLarge
		}
		if err != nil {
			return 0, err
		}
		return 0, nil
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.body.Read(p)
	r.remaining -= int64(n)
	if errors.Is(err, io.EOF) {
		r.done = true
	}
	return n, err
}

func (r *boundedCancelReadCloser) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		r.closeErr = r.body.Close()
		r.cancel()
	})
	return r.closeErr
}
