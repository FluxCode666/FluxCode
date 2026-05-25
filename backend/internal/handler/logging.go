package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func requestLogger(c *gin.Context, component string, fields ...zap.Field) *zap.Logger {
	base := logger.L()
	if c != nil && c.Request != nil {
		base = logger.FromContext(c.Request.Context())
	}

	if component != "" {
		fields = append([]zap.Field{zap.String("component", component)}, fields...)
	}
	return base.With(fields...)
}

// gatewayTraceID returns the trace_id from the response header set by RequestLogger middleware.
func gatewayTraceID(c *gin.Context) string {
	if c == nil || c.Writer == nil {
		return ""
	}
	return c.Writer.Header().Get("X-Trace-ID")
}
