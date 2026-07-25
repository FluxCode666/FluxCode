package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type successfulRequestPublisherStub struct {
	enabled bool
	maxBody int64
	records []*service.SuccessfulRequestRecord
	err     error
}

func (s *successfulRequestPublisherStub) Enabled() bool { return s.enabled }

func (s *successfulRequestPublisherStub) MaxBodyBytes() int64 { return s.maxBody }

func (s *successfulRequestPublisherStub) Publish(_ context.Context, record *service.SuccessfulRequestRecord) error {
	clone := *record
	s.records = append(s.records, &clone)
	return s.err
}

func newSuccessfulRequestRecorderTestRouter(
	t *testing.T,
	publisher SuccessfulRequestRecordPublisher,
	handler gin.HandlerFunc,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v1/messages",
		func(c *gin.Context) {
			groupID := int64(30)
			c.Set(string(ContextKeyAPIKey), &service.APIKey{ID: 20, UserID: 10, GroupID: &groupID})
			c.Set(string(ContextKeyUser), AuthSubject{UserID: 10})
			ctx := context.WithValue(c.Request.Context(), ctxkey.TraceID, "trace-1")
			ctx = context.WithValue(ctx, ctxkey.RequestID, "request-1")
			ctx = context.WithValue(ctx, ctxkey.ClientRequestID, "client-request-1")
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		},
		SuccessfulRequestRecorder(publisher),
		handler,
	)
	return router
}

func TestSuccessfulRequestRecorderCapturesSuccessfulJSONWithoutConsumingHandlerBody(t *testing.T) {
	publisher := &successfulRequestPublisherStub{enabled: true, maxBody: 1024}
	var handlerBody []byte
	router := newSuccessfulRequestRecorderTestRouter(t, publisher, func(c *gin.Context) {
		var err error
		handlerBody, err = io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		c.Data(http.StatusOK, "application/json", []byte(`{"id":"response-1"}`))
	})

	requestBody := []byte(`{"model":"claude-test","stream":false,"prompt":"hello"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, requestBody, handlerBody)
	require.Len(t, publisher.records, 1)
	record := publisher.records[0]
	require.NotNil(t, record.RequestBody)
	require.Equal(t, string(requestBody), *record.RequestBody)
	require.NotNil(t, record.ResponseBody)
	require.Equal(t, `{"id":"response-1"}`, *record.ResponseBody)
	require.Equal(t, int64(len(requestBody)), record.RequestBodyBytes)
	require.Equal(t, int64(len(`{"id":"response-1"}`)), record.ResponseBodyBytes)
	require.False(t, record.RequestTruncated)
	require.False(t, record.ResponseTruncated)
	require.Equal(t, "claude-test", record.Model)
	require.False(t, record.Stream)
	require.Equal(t, int64(10), record.UserID)
	require.Equal(t, int64(20), record.APIKeyID)
	require.Equal(t, "trace-1", record.TraceID)
}

func TestSuccessfulRequestRecorderSkipsNon2xxResponse(t *testing.T) {
	publisher := &successfulRequestPublisherStub{enabled: true, maxBody: 1024}
	router := newSuccessfulRequestRecorderTestRouter(t, publisher, func(c *gin.Context) {
		_, _ = io.Copy(io.Discard, c.Request.Body)
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad request"})
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(`{"model":"x"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), request)

	require.Empty(t, publisher.records)
}

func TestSuccessfulRequestRecorderReadsDynamicEnablementForEveryRequest(t *testing.T) {
	publisher := &successfulRequestPublisherStub{enabled: false, maxBody: 1024}
	router := newSuccessfulRequestRecorderTestRouter(t, publisher, func(c *gin.Context) {
		_, _ = io.Copy(io.Discard, c.Request.Body)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	request := func() {
		req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(`{"model":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	request()
	require.Empty(t, publisher.records)

	publisher.enabled = true
	request()
	require.Len(t, publisher.records, 1)

	publisher.enabled = false
	request()
	require.Len(t, publisher.records, 1)
}

func TestSuccessfulRequestRecorderOmitsOversizedBodies(t *testing.T) {
	publisher := &successfulRequestPublisherStub{enabled: true, maxBody: 8}
	router := newSuccessfulRequestRecorderTestRouter(t, publisher, func(c *gin.Context) {
		_, _ = io.Copy(io.Discard, c.Request.Body)
		c.Data(http.StatusOK, "application/json", []byte(`{"response":"too long"}`))
	})

	requestBody := []byte(`{"prompt":"too long"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), request)

	require.Len(t, publisher.records, 1)
	record := publisher.records[0]
	require.Nil(t, record.RequestBody)
	require.Nil(t, record.ResponseBody)
	require.Equal(t, int64(len(requestBody)), record.RequestBodyBytes)
	require.Equal(t, int64(len(`{"response":"too long"}`)), record.ResponseBodyBytes)
	require.True(t, record.RequestTruncated)
	require.True(t, record.ResponseTruncated)
}

func TestSuccessfulRequestRecorderMarksUnreadKnownLengthRequestAsTruncated(t *testing.T) {
	publisher := &successfulRequestPublisherStub{enabled: true, maxBody: 1024}
	router := newSuccessfulRequestRecorderTestRouter(t, publisher, func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json", []byte(`{"ok":true}`))
	})

	requestBody := []byte(`{"prompt":"handler does not read this"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), request)

	require.Len(t, publisher.records, 1)
	record := publisher.records[0]
	require.Nil(t, record.RequestBody)
	require.Equal(t, int64(len(requestBody)), record.RequestBodyBytes)
	require.True(t, record.RequestTruncated)
}

func TestSuccessfulRequestRecorderCapturesSSEAndOmitsBinaryPayloads(t *testing.T) {
	t.Run("sse", func(t *testing.T) {
		publisher := &successfulRequestPublisherStub{enabled: true, maxBody: 1024}
		router := newSuccessfulRequestRecorderTestRouter(t, publisher, func(c *gin.Context) {
			_, _ = io.Copy(io.Discard, c.Request.Body)
			c.Header("Content-Type", "text/event-stream")
			_, _ = c.Writer.WriteString("data: {\"type\":\"done\"}\n\n")
		})

		request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(`{"stream":true}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(httptest.NewRecorder(), request)

		require.Len(t, publisher.records, 1)
		record := publisher.records[0]
		require.True(t, record.Stream)
		require.NotNil(t, record.ResponseBody)
		require.Equal(t, "data: {\"type\":\"done\"}\n\n", *record.ResponseBody)
	})

	t.Run("binary", func(t *testing.T) {
		publisher := &successfulRequestPublisherStub{enabled: true, maxBody: 1024}
		router := newSuccessfulRequestRecorderTestRouter(t, publisher, func(c *gin.Context) {
			_, _ = io.Copy(io.Discard, c.Request.Body)
			c.Data(http.StatusOK, "application/octet-stream", []byte{0, 1, 2, 3})
		})

		requestBody := []byte("binary-input")
		request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(requestBody))
		request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
		router.ServeHTTP(httptest.NewRecorder(), request)

		require.Len(t, publisher.records, 1)
		record := publisher.records[0]
		require.Nil(t, record.RequestBody)
		require.Nil(t, record.ResponseBody)
		require.Equal(t, int64(len(requestBody)), record.RequestBodyBytes)
		require.Equal(t, int64(4), record.ResponseBodyBytes)
	})
}
