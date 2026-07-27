package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// SuccessfulRequestRecordPublisher 是路由层需要的最小成功请求记录端口。
type SuccessfulRequestRecordPublisher interface {
	Enabled() bool
	MaxBodyBytes() int64
	Publish(ctx context.Context, record *service.SuccessfulRequestRecord) error
}

// SuccessfulRequestRecorder 捕获已鉴权网关 POST 请求的原始请求体与响应体。
// 只有 handler 完成后状态码为 2xx 才投递；正文采集有严格字节上限，超限时
// 不保存部分正文，避免产生不可解析且可能泄漏半个字段的数据。
func SuccessfulRequestRecorder(publisher SuccessfulRequestRecordPublisher) gin.HandlerFunc {
	return func(c *gin.Context) {
		if publisher == nil || !publisher.Enabled() || c.Request == nil || c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		maxBodyBytes := publisher.MaxBodyBytes()
		if maxBodyBytes <= 0 {
			c.Next()
			return
		}

		startedAt := time.Now().UTC()
		requestContentType := normalizeMediaType(c.GetHeader("Content-Type"))
		var requestCapture *limitedBodyCapture
		if c.Request.Body != nil && requestContentTypeCanBeCaptured(requestContentType) {
			requestCapture = newLimitedBodyCapture(maxBodyBytes)
			c.Request.Body = &capturingReadCloser{
				ReadCloser: c.Request.Body,
				capture:    requestCapture,
			}
		}

		responseCapture := newLimitedBodyCapture(maxBodyBytes)
		originalWriter := c.Writer
		capturedWriter := &successfulRequestCaptureWriter{
			ResponseWriter: originalWriter,
			capture:        responseCapture,
		}
		c.Writer = capturedWriter
		defer func() {
			c.Writer = originalWriter
		}()

		c.Next()

		statusCode := capturedWriter.Status()
		if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
			return
		}
		apiKey, hasAPIKey := GetAPIKeyFromContext(c)
		subject, hasSubject := GetAuthSubjectFromContext(c)
		if !hasAPIKey || apiKey == nil || apiKey.ID <= 0 || !hasSubject || subject.UserID <= 0 {
			return
		}

		responseContentType := normalizeMediaType(capturedWriter.Header().Get("Content-Type"))
		requestBody, requestBytes, requestTruncated := capturedRequestBody(c.Request, requestCapture, requestContentType)
		responseBody := capturedTextBody(responseCapture, responseContentType, responseContentTypeCanBeCaptured)

		requestRaw := captureBytes(requestCapture)
		model := extractSuccessfulRequestModel(c, requestRaw)
		stream := extractSuccessfulRequestStream(c, requestRaw, responseContentType)
		durationMS := durationMilliseconds(time.Since(startedAt))

		var groupID *int64
		if apiKey.GroupID != nil {
			value := *apiKey.GroupID
			groupID = &value
		}
		requestContext := c.Request.Context()
		record := &service.SuccessfulRequestRecord{
			EventID:             uuid.NewString(),
			UserID:              subject.UserID,
			APIKeyID:            apiKey.ID,
			GroupID:             groupID,
			TraceID:             contextString(requestContext, ctxkey.TraceID),
			RequestID:           contextString(requestContext, ctxkey.RequestID),
			ClientRequestID:     contextString(requestContext, ctxkey.ClientRequestID),
			Method:              c.Request.Method,
			Endpoint:            requestPath(c.Request),
			RoutePattern:        c.FullPath(),
			Model:               model,
			StatusCode:          statusCode,
			Stream:              stream,
			ClientDisconnected:  requestContext.Err() != nil,
			DurationMS:          durationMS,
			RequestContentType:  requestContentType,
			ResponseContentType: responseContentType,
			RequestBody:         requestBody,
			ResponseBody:        responseBody,
			RequestBodyBytes:    requestBytes,
			ResponseBodyBytes:   responseCapture.Total(),
			RequestTruncated:    requestTruncated,
			ResponseTruncated:   responseCapture.Truncated(),
			CreatedAt:           startedAt,
		}

		publishContext := context.WithoutCancel(requestContext)
		if err := publisher.Publish(publishContext, record); err != nil {
			logger.FromContext(publishContext).With(
				zap.String("component", "middleware.successful_request_recorder"),
				zap.String("event_id", record.EventID),
				zap.Int64("user_id", record.UserID),
				zap.Int64("api_key_id", record.APIKeyID),
			).Error("successful_request_record.publish_failed", zap.Error(err))
		}
	}
}

type limitedBodyCapture struct {
	max       int64
	total     int64
	truncated bool
	buffer    bytes.Buffer
}

func newLimitedBodyCapture(max int64) *limitedBodyCapture {
	return &limitedBodyCapture{max: max}
}

func (c *limitedBodyCapture) Write(p []byte) {
	if c == nil || len(p) == 0 {
		return
	}
	c.total += int64(len(p))
	remaining := c.max - int64(c.buffer.Len())
	if remaining > 0 {
		toWrite := int64(len(p))
		if toWrite > remaining {
			toWrite = remaining
		}
		_, _ = c.buffer.Write(p[:int(toWrite)])
	}
	if c.total > c.max {
		c.truncated = true
	}
}

func (c *limitedBodyCapture) Bytes() []byte {
	if c == nil {
		return nil
	}
	return c.buffer.Bytes()
}

func (c *limitedBodyCapture) Total() int64 {
	if c == nil {
		return 0
	}
	return c.total
}

func (c *limitedBodyCapture) Truncated() bool {
	return c != nil && c.truncated
}

type capturingReadCloser struct {
	io.ReadCloser
	capture *limitedBodyCapture
}

func (r *capturingReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		r.capture.Write(p[:n])
	}
	return n, err
}

type successfulRequestCaptureWriter struct {
	gin.ResponseWriter
	capture *limitedBodyCapture
}

func (w *successfulRequestCaptureWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if n > 0 {
		w.capture.Write(data[:n])
	}
	return n, err
}

func (w *successfulRequestCaptureWriter) WriteString(data string) (int, error) {
	n, err := w.ResponseWriter.WriteString(data)
	if n > 0 {
		w.capture.Write([]byte(data[:n]))
	}
	return n, err
}

func capturedRequestBody(request *http.Request, capture *limitedBodyCapture, contentType string) (*string, int64, bool) {
	if capture == nil {
		if request != nil && request.ContentLength > 0 {
			return nil, request.ContentLength, false
		}
		return nil, 0, false
	}
	total := capture.Total()
	truncated := capture.Truncated()
	if request != nil && request.ContentLength > total {
		total = request.ContentLength
		truncated = true
	}
	if truncated {
		return nil, total, true
	}
	return capturedTextBody(capture, contentType, requestContentTypeCanBeCaptured), total, false
}

func capturedTextBody(capture *limitedBodyCapture, contentType string, allowed func(string) bool) *string {
	if capture == nil || capture.Total() == 0 || capture.Truncated() || !allowed(contentType) {
		return nil
	}
	raw := capture.Bytes()
	if contentType == "" && !json.Valid(raw) {
		return nil
	}
	value := strings.ToValidUTF8(string(raw), "�")
	return &value
}

func captureBytes(capture *limitedBodyCapture) []byte {
	if capture == nil || capture.Truncated() {
		return nil
	}
	return capture.Bytes()
}

func normalizeMediaType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err == nil {
		return strings.ToLower(mediaType)
	}
	if idx := strings.IndexByte(value, ';'); idx >= 0 {
		value = value[:idx]
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func requestContentTypeCanBeCaptured(contentType string) bool {
	return contentType == "" || contentType == "application/json" || strings.HasSuffix(contentType, "+json")
}

func responseContentTypeCanBeCaptured(contentType string) bool {
	return contentType == "" ||
		contentType == "application/json" ||
		contentType == "application/x-ndjson" ||
		strings.HasSuffix(contentType, "+json") ||
		strings.HasPrefix(contentType, "text/")
}

func extractSuccessfulRequestModel(c *gin.Context, raw []byte) string {
	if model := strings.TrimSpace(contextString(c.Request.Context(), ctxkey.Model)); model != "" {
		return model
	}
	if len(raw) > 0 && json.Valid(raw) {
		if model := strings.TrimSpace(gjson.GetBytes(raw, "model").String()); model != "" {
			return model
		}
	}
	if c != nil {
		modelAction := strings.Trim(strings.TrimSpace(c.Param("modelAction")), "/")
		if modelAction != "" {
			if model, _, found := strings.Cut(modelAction, ":"); found {
				return strings.TrimSpace(model)
			}
		}
	}
	return ""
}

func extractSuccessfulRequestStream(c *gin.Context, raw []byte, responseContentType string) bool {
	if len(raw) > 0 && json.Valid(raw) && gjson.GetBytes(raw, "stream").Bool() {
		return true
	}
	if responseContentType == "text/event-stream" {
		return true
	}
	if c != nil {
		return strings.Contains(strings.ToLower(c.Request.URL.Path), "streamgeneratecontent")
	}
	return false
}

func contextString(ctx context.Context, key ctxkey.Key) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(key).(string)
	return strings.TrimSpace(value)
}

func durationMilliseconds(duration time.Duration) int {
	value := duration.Milliseconds()
	if value < 0 {
		return 0
	}
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(value)
}

func requestPath(request *http.Request) string {
	if request == nil || request.URL == nil {
		return ""
	}
	return request.URL.Path
}
