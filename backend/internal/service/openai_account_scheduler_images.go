package service

import (
	"context"
)

// SelectAccountWithSchedulerForImages selects an account for image generation requests.
// If no account is found with the native capability, it falls back to basic (OAuth).
func (s *OpenAIGatewayService) SelectAccountWithSchedulerForImages(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredCapability OpenAIImagesCapability,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	return s.SelectAccountWithSchedulerForImagesForPlatform(
		ctx,
		PlatformOpenAI,
		groupID,
		sessionHash,
		requestedModel,
		excludedIDs,
		requiredCapability,
	)
}

func (s *OpenAIGatewayService) SelectAccountWithSchedulerForImagesForPlatform(
	ctx context.Context,
	platform string,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredCapability OpenAIImagesCapability,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	selection, decision, err := s.SelectAccountWithSchedulerForPlatform(
		ctx,
		platform,
		groupID,
		"",
		sessionHash,
		requestedModel,
		excludedIDs,
		OpenAIUpstreamTransportHTTPSSE,
		requiredCapability,
	)
	if err == nil && selection != nil && selection.Account != nil {
		return selection, decision, nil
	}
	if requiredCapability == OpenAIImagesCapabilityNative {
		return s.SelectAccountWithSchedulerForPlatform(
			ctx,
			platform,
			groupID,
			"",
			sessionHash,
			requestedModel,
			excludedIDs,
			OpenAIUpstreamTransportHTTPSSE,
			OpenAIImagesCapabilityBasic,
		)
	}
	return selection, decision, err
}
