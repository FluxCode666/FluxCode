package service

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func openAIStreamFailedEventErrorCode(payload []byte) string {
	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.code").String()))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.code").String()))
	}
	return code
}

func isOpenAIUpstreamCapacityShedEvent(payload []byte) bool {
	switch openAIStreamFailedEventErrorCode(payload) {
	case "server_is_overloaded", "slow_down":
		return true
	default:
		return false
	}
}

func sanitizeOpenAICapacityShedErrorCodeForClient(payload []byte) ([]byte, bool) {
	if len(payload) == 0 || !gjson.ValidBytes(payload) || !isOpenAIUpstreamCapacityShedEvent(payload) {
		return payload, false
	}
	updated := payload
	changed := false
	for _, path := range []string{"response.error.code", "error.code"} {
		switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(updated, path).String())) {
		case "server_is_overloaded", "slow_down":
		default:
			continue
		}
		next, err := sjson.SetBytes(updated, path, "server_error")
		if err != nil {
			return payload, false
		}
		updated = next
		changed = true
	}
	return updated, changed
}

func openAIStreamFailedEventSemanticStatus(payload []byte, message string) int {
	if isOpenAIContextWindowError(message, payload) {
		return http.StatusBadRequest
	}
	code := openAIStreamFailedEventErrorCode(payload)
	errType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.type").String()))
	if errType == "" {
		errType = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.type").String()))
	}
	combined := strings.TrimSpace(errType + " " + code + " " + strings.ToLower(strings.TrimSpace(message)))
	switch {
	case strings.Contains(combined, "rate_limit"):
		return http.StatusTooManyRequests
	case strings.Contains(errType, "invalid_request"):
		return http.StatusBadRequest
	case strings.Contains(combined, "authentication") || strings.Contains(combined, "unauthorized") || strings.Contains(combined, "invalid_api_key"):
		return http.StatusUnauthorized
	case strings.Contains(combined, "permission") || strings.Contains(combined, "forbidden") || strings.Contains(combined, "access denied"):
		return http.StatusForbidden
	case code == "server_is_overloaded" || code == "slow_down":
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadGateway
	}
}

func openAIStreamFailureStatus(payload []byte, message string) int {
	if openAIStreamFailedEventSemanticStatus(payload, message) == http.StatusTooManyRequests {
		return http.StatusTooManyRequests
	}
	return http.StatusBadGateway
}

func openAIStreamFailedEventShouldFailover(payload []byte, message string) bool {
	if isOpenAIContextWindowError(message, payload) {
		return false
	}
	if openAIStreamFailureStatus(payload, message) == http.StatusTooManyRequests {
		return true
	}
	if isOpenAITransientProcessingError(http.StatusBadRequest, message, payload) {
		return true
	}
	code := openAIStreamFailedEventErrorCode(payload)
	errType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.type").String()))
	if errType == "" {
		errType = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.type").String()))
	}
	combined := strings.ToLower(strings.TrimSpace(message + " " + code + " " + errType))
	if combined == "" {
		return true
	}
	for _, marker := range []string{"invalid_request", "content_policy", "policy", "safety", "high-risk cyber", "not allowed", "violat"} {
		if strings.Contains(combined, marker) {
			return false
		}
	}
	return true
}

func openAIStreamFailedEventRetryableOnSameAccount(account *Account, payload []byte, message string) bool {
	if account == nil {
		return false
	}
	if isOpenAIUpstreamCapacityShedEvent(payload) {
		return true
	}
	if !account.IsPoolMode() {
		return false
	}
	semanticStatus := openAIStreamFailedEventSemanticStatus(payload, message)
	return account.IsPoolModeRetryableStatus(semanticStatus) ||
		isOpenAITransientProcessingError(http.StatusBadRequest, message, payload)
}

func (s *OpenAIGatewayService) newOpenAIStreamFailoverError(
	c *gin.Context,
	account *Account,
	passthrough bool,
	upstreamRequestID string,
	payload []byte,
	message string,
	responseHeaders ...http.Header,
) *UpstreamFailoverError {
	message = sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	if message == "" {
		message = "OpenAI stream disconnected before completion"
	}
	statusCode := openAIStreamFailureStatus(payload, message)
	setOpsUpstreamError(c, statusCode, message, "")
	if account != nil {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
			UpstreamStatusCode: statusCode, UpstreamRequestID: strings.TrimSpace(upstreamRequestID),
			Passthrough: passthrough, Kind: "failover", Message: message,
		})
	}
	var headers http.Header
	if len(responseHeaders) > 0 && responseHeaders[0] != nil {
		headers = responseHeaders[0].Clone()
	}
	errType := "upstream_error"
	if statusCode == http.StatusTooManyRequests {
		errType = "rate_limit_error"
	}
	body, _ := json.Marshal(gin.H{"error": gin.H{"type": errType, "message": message}})
	return &UpstreamFailoverError{
		StatusCode:             statusCode,
		ResponseBody:           body,
		ResponseHeaders:        headers,
		RetryableOnSameAccount: openAIStreamFailedEventRetryableOnSameAccount(account, payload, message),
		RequestScopedTransient: isOpenAIUpstreamCapacityShedEvent(payload),
	}
}
