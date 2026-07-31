package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type providerGatewayHandlerExecutorStub struct {
	stream *service.ProviderGatewayStreamResult
}

func (s *providerGatewayHandlerExecutorStub) Execute(context.Context, service.ProviderGatewayRequest) (*service.ProviderGatewayResult, error) {
	return nil, errors.New("unexpected non-stream execution")
}

func (s *providerGatewayHandlerExecutorStub) ExecuteStream(context.Context, service.ProviderGatewayRequest) (*service.ProviderGatewayStreamResult, error) {
	return s.stream, nil
}

type providerUsageRecorderStub struct {
	calls int
}

func (s *providerUsageRecorderStub) RecordProviderUsage(context.Context, *service.ProviderRecordUsageInput) error {
	s.calls++
	return nil
}

type providerFailingResponseWriter struct {
	gin.ResponseWriter
	writes int
}

func (w *providerFailingResponseWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, errors.New("client disconnected")
}

type providerTrackingReadCloser struct {
	reader io.Reader
	err    error
	reads  int
	closed bool
}

func (r *providerTrackingReadCloser) Read(p []byte) (int, error) {
	r.reads++
	n, err := r.reader.Read(p)
	if err == io.EOF && r.err != nil {
		return n, r.err
	}
	return n, err
}

func (r *providerTrackingReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestProviderGatewayHandlerUsesLegacyFallbackUntilRouteSnapshotCutover(t *testing.T) {
	group := &service.Group{ID: 7, Platform: "arbitrary-vendor", Status: service.StatusActive, Hydrated: true}
	apiKey := &service.APIKey{ID: 9, GroupID: &group.ID, Group: group}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"model-a"}`))
	c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	fallbackCalled := false

	var handler *ProviderGatewayHandler
	handler.HandleOrFallback(c, service.ProtocolChatCompletions, func(c *gin.Context) {
		fallbackCalled = true
		c.Status(http.StatusNoContent)
		c.Writer.WriteHeaderNow()
	})

	require.True(t, fallbackCalled)
	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestProviderGatewayHandlerCutoverDoesNotDispatchByGroupPlatform(t *testing.T) {
	version := int64(1)
	group := &service.Group{
		ID: 7, Platform: "unknown-new-provider", Status: service.StatusActive, Hydrated: true,
		ActiveRouteSnapshotVersion: &version,
	}
	apiKey := &service.APIKey{ID: 9, GroupID: &group.ID, Group: group, User: &service.User{ID: 3}}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"model-a"}`))
	c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	fallbackCalled := false

	(&ProviderGatewayHandler{}).HandleOrFallback(c, service.ProtocolChatCompletions, func(c *gin.Context) {
		fallbackCalled = true
	})

	require.False(t, fallbackCalled)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), "provider_gateway_unavailable")
}

func TestValidateProviderJSONKeysRejectsDuplicateModelAtAnyDepth(t *testing.T) {
	tests := []string{
		`{"model":"route-model","model":"upstream-model"}`,
		`{"model":"route-model","metadata":{"tenant":"one","tenant":"two"}}`,
		`{"model":"route-model","input":[{"role":"user","role":"assistant"}]}`,
	}

	for _, body := range tests {
		err := validateProviderJSONKeys([]byte(body))
		require.Error(t, err)
		require.Contains(t, err.Error(), "duplicate JSON field")
	}
}

func TestProviderGatewayHandlerBillsOnceAfterClientStreamWriteFails(t *testing.T) {
	body := &providerTrackingReadCloser{reader: strings.NewReader("data: first\n\ndata: second\n\n")}
	usage := &providerUsageRecorderStub{}
	handler := &ProviderGatewayHandler{
		gateway: &providerGatewayHandlerExecutorStub{stream: &service.ProviderGatewayStreamResult{
			Body: body, Headers: http.Header{"Content-Type": []string{"text/event-stream"}}, StatusCode: http.StatusOK,
		}},
		usage: usage,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	failingWriter := &providerFailingResponseWriter{ResponseWriter: c.Writer}
	c.Writer = failingWriter
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	apiKey := &service.APIKey{ID: 9, User: &service.User{ID: 3}}

	handler.handleStream(c, service.ProtocolChatCompletions, service.ProviderGatewayRequest{}, apiKey, nil, []byte(`{"model":"model-a","stream":true}`))

	require.Equal(t, 1, failingWriter.writes)
	require.GreaterOrEqual(t, body.reads, 2, "客户端断开后仍应读取上游直到 EOF")
	require.True(t, body.closed)
	require.Equal(t, 1, usage.calls)
}

func TestProviderGatewayHandlerDoesNotBillInterruptedUpstreamStream(t *testing.T) {
	body := &providerTrackingReadCloser{
		reader: strings.NewReader("data: partial\n\n"),
		err:    errors.New("upstream reset"),
	}
	usage := &providerUsageRecorderStub{}
	handler := &ProviderGatewayHandler{
		gateway: &providerGatewayHandlerExecutorStub{stream: &service.ProviderGatewayStreamResult{
			Body: body, Headers: http.Header{"Content-Type": []string{"text/event-stream"}}, StatusCode: http.StatusOK,
		}},
		usage: usage,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	apiKey := &service.APIKey{ID: 9, User: &service.User{ID: 3}}

	handler.handleStream(c, service.ProtocolChatCompletions, service.ProviderGatewayRequest{}, apiKey, nil, []byte(`{"model":"model-a","stream":true}`))

	require.True(t, body.closed)
	require.Zero(t, usage.calls)
}
