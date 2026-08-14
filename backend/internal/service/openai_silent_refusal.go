package service

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const openAIResponsesEmptyCompletedMessage = "OpenAI upstream returned an empty response.completed stream with no output and no usage"

func newOpenAIResponsesEmptyCompletedFailoverError(c *gin.Context, account *Account, upstreamRequestID string) *UpstreamFailoverError {
	accountID := int64(0)
	accountName := ""
	platform := PlatformOpenAI
	if account != nil {
		accountID = account.ID
		accountName = account.Name
		platform = account.Platform
	}
	setOpsUpstreamError(c, http.StatusBadGateway, openAIResponsesEmptyCompletedMessage, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform: platform, AccountID: accountID, AccountName: accountName,
		UpstreamStatusCode: http.StatusBadGateway, UpstreamRequestID: strings.TrimSpace(upstreamRequestID),
		Kind: "failover", Message: openAIResponsesEmptyCompletedMessage,
	})
	body, _ := json.Marshal(gin.H{"error": gin.H{
		"type": "upstream_error", "code": "openai_silent_refusal", "message": openAIResponsesEmptyCompletedMessage,
	}})
	headers := http.Header{}
	if strings.TrimSpace(upstreamRequestID) != "" {
		headers.Set("x-request-id", strings.TrimSpace(upstreamRequestID))
	}
	return &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: body, ResponseHeaders: headers}
}
