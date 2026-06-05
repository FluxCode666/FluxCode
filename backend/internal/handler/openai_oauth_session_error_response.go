package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func isOpenAIOAuthSessionTerminatedFailover(failoverErr *service.UpstreamFailoverError) bool {
	return failoverErr != nil && service.IsOpenAIOAuthSessionTerminatedResponse(failoverErr.ResponseBody)
}

func openAIOAuthSessionTerminatedGatewayError() (int, string, string) {
	return http.StatusBadGateway, "upstream_error", service.OpenAIOAuthSessionTerminatedGatewayMessage
}
