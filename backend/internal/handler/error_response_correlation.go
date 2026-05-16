package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
)

func addErrorCorrelationFields(c *gin.Context, payload gin.H) {
	if payload == nil {
		return
	}
	if traceID := requestCorrelationValue(c, ctxkey.TraceID, "X-Trace-ID"); traceID != "" {
		payload["trace_id"] = traceID
	}
	if requestID := requestCorrelationValue(c, ctxkey.RequestID, "X-Request-ID"); requestID != "" {
		payload["request_id"] = requestID
	}
}

func requestCorrelationValue(c *gin.Context, key ctxkey.Key, headerName string) string {
	if c == nil {
		return ""
	}
	if c.Request != nil {
		if value, _ := c.Request.Context().Value(key).(string); strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if c.Writer != nil {
		return strings.TrimSpace(c.Writer.Header().Get(headerName))
	}
	return ""
}
