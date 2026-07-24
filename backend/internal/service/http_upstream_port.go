package service

import (
	"net"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

// HTTPUpstream 上游 HTTP 请求接口
// 用于向上游 API（Claude、OpenAI、Gemini 等）发送请求
type HTTPUpstream interface {
	// Do 执行 HTTP 请求（不启用 TLS 指纹）
	Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error)

	// DoWithTLS 执行带 TLS 指纹伪装的 HTTP 请求
	//
	// profile 参数:
	//   - nil: 不启用 TLS 指纹，行为与 Do 方法相同
	//   - non-nil: 使用指定的 Profile 进行 TLS 指纹伪装
	//
	// Profile 由调用方通过 TLSFingerprintProfileService 解析后传入，
	// 支持按账号绑定的数据库 profile 或内置默认 profile。
	DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error)
}

// EmbeddingUpstreamPolicy contains the service-validated destination for one
// embedding request. ValidatedIP must be bound by the transport's actual dial;
// merely resolving it before client.Do would leave a DNS-rebinding window.
type EmbeddingUpstreamPolicy struct {
	ValidatedIP           net.IP
	ResponseHeaderTimeout time.Duration
}

// EmbeddingHTTPUpstream is deliberately separate from HTTPUpstream so the
// legacy gateway does not inherit embedding's strict SSRF and redirect policy.
// Production HTTPUpstream implementations should support it; tests may provide
// a narrow fake for this port.
type EmbeddingHTTPUpstream interface {
	DoEmbedding(req *http.Request, policy EmbeddingUpstreamPolicy) (*http.Response, error)
}

// EmbeddingTransportError reports only whether the HTTP transport can prove
// the request was never assigned a connection. The wrapped network error is
// intentionally discarded so upstream host or credential context cannot leak.
type EmbeddingTransportError struct {
	RequestNotWritten bool
}

func (e *EmbeddingTransportError) Error() string {
	return "embedding transport failed"
}
