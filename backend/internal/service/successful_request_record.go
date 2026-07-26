package service

import (
	"context"
	"time"
)

// SuccessfulRequestRecord 是一个已鉴权且 HTTP 状态码为 2xx 的网关请求快照。
// 请求/响应正文按原文保存；调用方必须通过访问控制和保留期保护这些数据。
type SuccessfulRequestRecord struct {
	ID                  int64     `json:"-"`
	EventID             string    `json:"event_id"`
	UsageLogID          *int64    `json:"usage_log_id,omitempty"`
	UserID              int64     `json:"user_id"`
	APIKeyID            int64     `json:"api_key_id"`
	GroupID             *int64    `json:"group_id,omitempty"`
	TraceID             string    `json:"trace_id,omitempty"`
	RequestID           string    `json:"request_id"`
	ClientRequestID     string    `json:"client_request_id,omitempty"`
	Method              string    `json:"method"`
	Endpoint            string    `json:"endpoint"`
	RoutePattern        string    `json:"route_pattern,omitempty"`
	Model               string    `json:"model,omitempty"`
	StatusCode          int       `json:"status_code"`
	Stream              bool      `json:"stream"`
	ClientDisconnected  bool      `json:"client_disconnected"`
	DurationMS          int       `json:"duration_ms"`
	RequestContentType  string    `json:"request_content_type,omitempty"`
	ResponseContentType string    `json:"response_content_type,omitempty"`
	RequestBody         *string   `json:"request_body,omitempty"`
	ResponseBody        *string   `json:"response_body,omitempty"`
	RequestBodyBytes    int64     `json:"request_body_bytes"`
	ResponseBodyBytes   int64     `json:"response_body_bytes"`
	RequestTruncated    bool      `json:"request_body_truncated"`
	ResponseTruncated   bool      `json:"response_body_truncated"`
	CreatedAt           time.Time `json:"created_at"`
	RecordedAt          time.Time `json:"-"`
}

// SuccessfulRequestRecordRepository 定义成功请求快照的幂等写入端口。
type SuccessfulRequestRecordRepository interface {
	// Create 以 EventID 幂等写入。inserted=false 表示重复消息已被忽略。
	Create(ctx context.Context, record *SuccessfulRequestRecord) (inserted bool, err error)
	// ReconcileUnlinked 通过服务端生成的 client_request_id 将异步写入顺序导致的空 usage_log_id 补齐。
	ReconcileUnlinked(ctx context.Context, limit int) (updated int64, err error)
}
