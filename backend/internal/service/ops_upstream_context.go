package service

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Gin context keys used by Ops error logger for capturing upstream error details.
// These keys are set by gateway services and consumed by handler/ops_error_logger.go.
const (
	OpsUpstreamStatusCodeKey   = "ops_upstream_status_code"
	OpsUpstreamErrorMessageKey = "ops_upstream_error_message"
	OpsUpstreamErrorDetailKey  = "ops_upstream_error_detail"
	OpsUpstreamErrorsKey       = "ops_upstream_errors"

	// Best-effort capture of the current upstream request body so ops can
	// retry the specific upstream attempt (not just the client request).
	// This value is sanitized+trimmed before being persisted.
	OpsUpstreamRequestBodyKey = "ops_upstream_request_body"

	// Optional stage latencies (milliseconds) for troubleshooting and alerting.
	OpsAuthLatencyMsKey      = "ops_auth_latency_ms"
	OpsRoutingLatencyMsKey   = "ops_routing_latency_ms"
	OpsUpstreamLatencyMsKey  = "ops_upstream_latency_ms"
	OpsResponseLatencyMsKey  = "ops_response_latency_ms"
	OpsTimeToFirstTokenMsKey = "ops_time_to_first_token_ms"
	// OpenAI WS 关键观测字段
	OpsOpenAIWSQueueWaitMsKey = "ops_openai_ws_queue_wait_ms"
	OpsOpenAIWSConnPickMsKey  = "ops_openai_ws_conn_pick_ms"
	OpsOpenAIWSConnReusedKey  = "ops_openai_ws_conn_reused"
	OpsOpenAIWSConnIDKey      = "ops_openai_ws_conn_id"

	// OpsSkipPassthroughKey 由 applyErrorPassthroughRule 在命中 skip_monitoring=true 的规则时设置。
	// ops_error_logger 中间件检查此 key，为 true 时跳过错误记录。
	OpsSkipPassthroughKey = "ops_skip_passthrough"
)

// opsUpstreamRequestBodyEarlyCap is the byte-level cap applied when copying the
// upstream request body into each OpsUpstreamErrorEvent.  This matches the
// final storage cap used by sanitizeOpsUpstreamEvents (10KB) so we avoid
// allocating large intermediate strings during retries/failovers.
const opsUpstreamRequestBodyEarlyCap = 10 * 1024

const (
	opsUpstreamDebugLogBodyMaxBytes   = 10 * 1024
	opsUpstreamDebugLogPromptMaxBytes = opsUpstreamDebugLogBodyMaxBytes
	debugLogPromptTruncationMarker    = "...[truncated]"
)

// truncateStringBytes truncates s to at most maxBytes bytes without splitting
// a multi-byte UTF-8 character.
func truncateStringBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Walk backwards from the cap to avoid splitting a multi-byte rune.
	for maxBytes > 0 && maxBytes < len(s) {
		// If the byte at maxBytes is a UTF-8 continuation byte, back up.
		if s[maxBytes]&0xC0 == 0x80 {
			maxBytes--
		} else {
			break
		}
	}
	return s[:maxBytes]
}

func setOpsUpstreamRequestBody(c *gin.Context, body []byte) {
	if c == nil || len(body) == 0 {
		return
	}
	// 热路径避免 string(body) 额外分配，按需在落库前再转换。
	c.Set(OpsUpstreamRequestBodyKey, body)
}

func SetOpsLatencyMs(c *gin.Context, key string, value int64) {
	if c == nil || strings.TrimSpace(key) == "" || value < 0 {
		return
	}
	c.Set(key, value)
}

// SetOpsUpstreamError is the exported wrapper for setOpsUpstreamError, used by
// handler-layer code (e.g. failover-exhausted paths) that needs to record the
// original upstream status code before mapping it to a client-facing code.
func SetOpsUpstreamError(c *gin.Context, upstreamStatusCode int, upstreamMessage, upstreamDetail string) {
	setOpsUpstreamError(c, upstreamStatusCode, upstreamMessage, upstreamDetail)
}

func setOpsUpstreamError(c *gin.Context, upstreamStatusCode int, upstreamMessage, upstreamDetail string) {
	if c == nil {
		return
	}
	if upstreamStatusCode > 0 {
		c.Set(OpsUpstreamStatusCodeKey, upstreamStatusCode)
	}
	if msg := strings.TrimSpace(upstreamMessage); msg != "" {
		c.Set(OpsUpstreamErrorMessageKey, msg)
	}
	if detail := strings.TrimSpace(upstreamDetail); detail != "" {
		c.Set(OpsUpstreamErrorDetailKey, detail)
	}
}

// OpsUpstreamErrorEvent describes one upstream error attempt during a single gateway request.
// It is stored in ops_error_logs.upstream_errors as a JSON array.
type OpsUpstreamErrorEvent struct {
	AtUnixMs int64 `json:"at_unix_ms,omitempty"`

	// Passthrough 表示本次请求是否命中“原样透传（仅替换认证）”分支。
	// 该字段用于排障与灰度评估；存入 JSON，不涉及 DB schema 变更。
	Passthrough bool `json:"passthrough,omitempty"`

	// Context
	Platform    string `json:"platform,omitempty"`
	AccountID   int64  `json:"account_id,omitempty"`
	AccountName string `json:"account_name,omitempty"`

	// Outcome
	UpstreamStatusCode int    `json:"upstream_status_code,omitempty"`
	UpstreamRequestID  string `json:"upstream_request_id,omitempty"`

	// UpstreamURL is the actual upstream URL that was called (host + path, query/fragment stripped).
	// Helps debug 404/routing errors by showing which endpoint was targeted.
	UpstreamURL string `json:"upstream_url,omitempty"`

	// Best-effort upstream request capture (sanitized+trimmed).
	// Required for retrying a specific upstream attempt.
	UpstreamRequestBody string `json:"upstream_request_body,omitempty"`

	// Best-effort upstream response capture (sanitized+trimmed).
	UpstreamResponseBody string `json:"upstream_response_body,omitempty"`

	// Kind: http_error | request_error | retry_exhausted | failover
	Kind string `json:"kind,omitempty"`

	Message string `json:"message,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

func appendOpsUpstreamError(c *gin.Context, ev OpsUpstreamErrorEvent) {
	if c == nil {
		return
	}
	if ev.AtUnixMs <= 0 {
		ev.AtUnixMs = time.Now().UnixMilli()
	}
	ev.Platform = strings.TrimSpace(ev.Platform)
	ev.UpstreamRequestID = strings.TrimSpace(ev.UpstreamRequestID)
	ev.UpstreamRequestBody = strings.TrimSpace(ev.UpstreamRequestBody)
	ev.UpstreamResponseBody = strings.TrimSpace(ev.UpstreamResponseBody)
	ev.Kind = strings.TrimSpace(ev.Kind)
	ev.UpstreamURL = strings.TrimSpace(ev.UpstreamURL)
	ev.Message = strings.TrimSpace(ev.Message)
	ev.Detail = strings.TrimSpace(ev.Detail)
	if ev.Message != "" {
		ev.Message = sanitizeUpstreamErrorMessage(ev.Message)
	}
	debugRequestBody := ev.UpstreamRequestBody

	// If the caller didn't explicitly pass upstream request body but the gateway
	// stored it on the context, attach it so ops can retry this specific attempt.
	// Truncate early to opsUpstreamRequestBodyEarlyCap (matching the final storage
	// cap in sanitizeOpsUpstreamEvents) to avoid allocating full multi-MB string
	// copies on every retry/failover event.
	if ev.UpstreamRequestBody == "" {
		if v, ok := c.Get(OpsUpstreamRequestBodyKey); ok {
			switch raw := v.(type) {
			case string:
				debugRequestBody = strings.TrimSpace(raw)
				ev.UpstreamRequestBody = truncateStringBytes(debugRequestBody, opsUpstreamRequestBodyEarlyCap)
			case []byte:
				debugRequestBody = strings.TrimSpace(string(raw))
				if len(raw) > opsUpstreamRequestBodyEarlyCap {
					// Truncate at a UTF-8 safe boundary to avoid splitting multi-byte characters.
					n := opsUpstreamRequestBodyEarlyCap
					for n > 0 && n < len(raw) && raw[n]&0xC0 == 0x80 {
						n--
					}
					raw = raw[:n]
				}
				ev.UpstreamRequestBody = strings.TrimSpace(string(raw))
			}
		}
	}

	var existing []*OpsUpstreamErrorEvent
	if v, ok := c.Get(OpsUpstreamErrorsKey); ok {
		if arr, ok := v.([]*OpsUpstreamErrorEvent); ok {
			existing = arr
		}
	}

	evCopy := ev
	existing = append(existing, &evCopy)
	c.Set(OpsUpstreamErrorsKey, existing)

	logOpsUpstreamErrorEvent(c, &evCopy, debugRequestBody)
	checkSkipMonitoringForUpstreamEvent(c, &evCopy)
}

func logOpsUpstreamErrorEvent(c *gin.Context, ev *OpsUpstreamErrorEvent, debugRequestBody string) {
	if c == nil || ev == nil {
		return
	}

	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	fields := []zap.Field{
		zap.String("component", "service.upstream_error"),
		zap.String("kind", strings.TrimSpace(ev.Kind)),
		zap.Int("upstream_status_code", ev.UpstreamStatusCode),
		zap.String("platform", strings.TrimSpace(ev.Platform)),
		zap.Int64("account_id", ev.AccountID),
		zap.String("account_name", strings.TrimSpace(ev.AccountName)),
		zap.String("upstream_request_id", strings.TrimSpace(ev.UpstreamRequestID)),
		zap.String("upstream_url", strings.TrimSpace(ev.UpstreamURL)),
		zap.String("upstream_error_message", strings.TrimSpace(ev.Message)),
	}

	if body := strings.TrimSpace(debugRequestBody); body != "" {
		sanitizedBody, truncated := sanitizeRequestBodyForDebugLog(body, opsUpstreamDebugLogPromptMaxBytes)
		if sanitizedBody == "" {
			sanitizedBody = "<non-json request body redacted>"
		}
		fields = append(fields,
			zap.String("upstream_request_body", sanitizedBody),
			zap.Bool("upstream_request_body_truncated", truncated),
		)
	}

	responseBody := strings.TrimSpace(ev.UpstreamResponseBody)
	if responseBody == "" {
		responseBody = strings.TrimSpace(ev.Detail)
	}
	if responseBody != "" {
		fields = append(fields,
			zap.String("upstream_response_body", truncateString(responseBody, opsUpstreamDebugLogBodyMaxBytes)),
		)
	}

	logger.FromContext(ctx).With(fields...).Warn("upstream model request failed")
}

// sanitizeRequestBodyForDebugLog 保留完整 JSON 结构和非提示词字段，只截断用户/系统提示词。
// 凭据等敏感字段仍沿用 Ops 日志的递归脱敏规则。
func sanitizeRequestBodyForDebugLog(raw string, maxPromptBytes int) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}

	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return "", false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return "", false
	}

	decoded = redactSensitiveJSON(decoded)
	decoded, truncated := truncateDebugRequestPrompts(decoded, maxPromptBytes)
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return "", truncated
	}
	return string(encoded), truncated
}

func truncateDebugRequestPrompts(value any, maxPromptBytes int) (any, bool) {
	root, ok := value.(map[string]any)
	if !ok {
		return value, false
	}

	out := shallowCopyMap(root)
	truncated := false
	for key, fieldValue := range root {
		var next any
		var changed bool
		switch normalizeDebugPromptKey(key) {
		case "system", "systemprompt", "systeminstruction", "instructions", "prompt", "userprompt":
			next, changed = truncateDebugPromptValue(fieldValue, maxPromptBytes)
		case "messages":
			next, changed = truncateDebugPromptMessages(fieldValue, maxPromptBytes, false)
		case "contents", "input":
			next, changed = truncateDebugPromptMessages(fieldValue, maxPromptBytes, true)
		case "request":
			next, changed = truncateDebugRequestPrompts(fieldValue, maxPromptBytes)
		default:
			continue
		}
		out[key] = next
		truncated = truncated || changed
	}
	return out, truncated
}

func truncateDebugPromptMessages(value any, maxPromptBytes int, missingRoleIsUser bool) (any, bool) {
	switch typed := value.(type) {
	case string:
		return truncateDebugPromptString(typed, maxPromptBytes)
	case []any:
		out := make([]any, len(typed))
		truncated := false
		for i, item := range typed {
			next, changed := truncateDebugPromptMessage(item, maxPromptBytes, missingRoleIsUser)
			out[i] = next
			truncated = truncated || changed
		}
		return out, truncated
	case map[string]any:
		return truncateDebugPromptMessage(typed, maxPromptBytes, missingRoleIsUser)
	default:
		return value, false
	}
}

func truncateDebugPromptMessage(value any, maxPromptBytes int, missingRoleIsUser bool) (any, bool) {
	message, ok := value.(map[string]any)
	if !ok {
		return value, false
	}

	role, _ := message["role"].(string)
	role = strings.ToLower(strings.TrimSpace(role))
	messageType, _ := message["type"].(string)
	messageType = normalizeDebugPromptKey(messageType)

	isPrompt := role == "user" || role == "system" || role == "developer"
	if role == "" && missingRoleIsUser {
		if isDebugNonPromptContentType(messageType) {
			isPrompt = false
		} else {
			_, hasContent := message["content"]
			_, hasParts := message["parts"]
			_, hasText := message["text"]
			isPrompt = hasContent || hasParts || hasText
		}
	}
	if !isPrompt {
		return value, false
	}

	out := shallowCopyMap(message)
	truncated := false
	for key, fieldValue := range message {
		switch normalizeDebugPromptKey(key) {
		case "content", "parts", "text", "prompt":
			next, changed := truncateDebugPromptValue(fieldValue, maxPromptBytes)
			out[key] = next
			truncated = truncated || changed
		}
	}
	return out, truncated
}

func truncateDebugPromptValue(value any, maxPromptBytes int) (any, bool) {
	switch typed := value.(type) {
	case string:
		return truncateDebugPromptString(typed, maxPromptBytes)
	case []any:
		out := make([]any, len(typed))
		truncated := false
		for i, item := range typed {
			next, changed := truncateDebugPromptValue(item, maxPromptBytes)
			out[i] = next
			truncated = truncated || changed
		}
		return out, truncated
	case map[string]any:
		contentType, _ := typed["type"].(string)
		contentType = normalizeDebugPromptKey(contentType)
		if isDebugNonPromptContentType(contentType) {
			return value, false
		}

		out := shallowCopyMap(typed)
		truncated := false
		for key, fieldValue := range typed {
			switch normalizeDebugPromptKey(key) {
			case "content", "parts", "text", "prompt":
				next, changed := truncateDebugPromptValue(fieldValue, maxPromptBytes)
				out[key] = next
				truncated = truncated || changed
			}
		}
		return out, truncated
	default:
		return value, false
	}
}

func isDebugNonPromptContentType(contentType string) bool {
	switch contentType {
	case "functioncall", "functioncalloutput", "functionresponse", "toolcall", "tooluseresult",
		"inputimage", "image", "document":
		return true
	}
	return strings.HasSuffix(contentType, "toolresult")
}

func truncateDebugPromptString(value string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value, false
	}
	prefixBytes := maxBytes - len(debugLogPromptTruncationMarker)
	if prefixBytes < 0 {
		prefixBytes = 0
	}
	return truncateStringBytes(value, prefixBytes) + debugLogPromptTruncationMarker, true
}

func normalizeDebugPromptKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "")
	return strings.ReplaceAll(value, "-", "")
}

// checkSkipMonitoringForUpstreamEvent checks whether the upstream error event
// matches a passthrough rule with skip_monitoring=true and, if so, sets the
// OpsSkipPassthroughKey on the context.  This ensures intermediate retry /
// failover errors (which never go through the final applyErrorPassthroughRule
// path) can still suppress ops_error_logs recording.
func checkSkipMonitoringForUpstreamEvent(c *gin.Context, ev *OpsUpstreamErrorEvent) {
	if ev.UpstreamStatusCode == 0 {
		return
	}

	svc := getBoundErrorPassthroughService(c)
	if svc == nil {
		return
	}

	// Use the best available body representation for keyword matching.
	// Even when body is empty, MatchRule can still match rules that only
	// specify ErrorCodes (no Keywords), so we always call it.
	body := ev.Detail
	if body == "" {
		body = ev.Message
	}

	rule := svc.MatchRule(ev.Platform, ev.UpstreamStatusCode, []byte(body))
	if rule != nil && rule.SkipMonitoring {
		c.Set(OpsSkipPassthroughKey, true)
	}
}

func marshalOpsUpstreamErrors(events []*OpsUpstreamErrorEvent) *string {
	if len(events) == 0 {
		return nil
	}
	// Ensure we always store a valid JSON value.
	raw, err := json.Marshal(events)
	if err != nil || len(raw) == 0 {
		return nil
	}
	s := string(raw)
	return &s
}

func ParseOpsUpstreamErrors(raw string) ([]*OpsUpstreamErrorEvent, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []*OpsUpstreamErrorEvent{}, nil
	}
	var out []*OpsUpstreamErrorEvent
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// safeUpstreamURL returns scheme + host + path from a URL, stripping query/fragment
// to avoid leaking sensitive query parameters (e.g. OAuth tokens).
func safeUpstreamURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if idx := strings.IndexByte(rawURL, '?'); idx >= 0 {
		rawURL = rawURL[:idx]
	}
	if idx := strings.IndexByte(rawURL, '#'); idx >= 0 {
		rawURL = rawURL[:idx]
	}
	return rawURL
}
