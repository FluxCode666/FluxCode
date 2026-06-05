package service

import (
	"encoding/json"
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	OpenAIOAuthSessionTerminatedCode           = "app_session_terminated"
	OpenAIOAuthSessionTerminatedReason         = "OPENAI_OAUTH_SESSION_TERMINATED"
	OpenAIOAuthSessionTerminatedMessage        = "OpenAI OAuth 会话已结束，请重新授权该账号。"
	OpenAIOAuthSessionTerminatedGatewayMessage = "OpenAI OAuth 会话已结束，当前没有可用的账号或号池，请管理员重新授权 OpenAI OAuth 账号。"
)

// NewOpenAIOAuthSessionTerminatedError returns the admin-facing refresh error.
func NewOpenAIOAuthSessionTerminatedError() *infraerrors.ApplicationError {
	return infraerrors.New(http.StatusBadRequest, OpenAIOAuthSessionTerminatedReason, OpenAIOAuthSessionTerminatedMessage)
}

func IsOpenAIOAuthSessionTerminatedError(err error) bool {
	if err == nil {
		return false
	}
	if infraerrors.Reason(err) == OpenAIOAuthSessionTerminatedReason {
		return true
	}
	return IsOpenAIOAuthSessionTerminatedText(err.Error())
}

func IsOpenAIOAuthSessionTerminatedText(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	return strings.Contains(text, OpenAIOAuthSessionTerminatedCode) ||
		(strings.Contains(text, "session has ended") && strings.Contains(text, "log in again"))
}

func IsOpenAIOAuthSessionTerminatedResponse(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	return IsOpenAIOAuthSessionTerminatedText(string(body))
}

func NewOpenAIOAuthSessionTerminatedFailoverError() *UpstreamFailoverError {
	return &UpstreamFailoverError{
		StatusCode:   http.StatusUnauthorized,
		ResponseBody: openAIOAuthSessionTerminatedResponseBody(),
	}
}

func openAIOAuthSessionTerminatedResponseBody() []byte {
	body, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": OpenAIOAuthSessionTerminatedMessage,
			"type":    "invalid_request_error",
			"code":    OpenAIOAuthSessionTerminatedCode,
		},
	})
	if err != nil {
		return []byte(`{"error":{"code":"app_session_terminated","message":"OpenAI OAuth session terminated"}}`)
	}
	return body
}
