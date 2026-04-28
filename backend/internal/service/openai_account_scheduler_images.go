package service

import (
	"context"
)

// SelectAccountWithSchedulerForImages selects an account for image generation requests.
// It wraps SelectAccountWithScheduler, ignoring the image capability filter since
// the current branch does not have capability-based filtering in the scheduler.
// If no account is found with the native capability, it falls back to basic (OAuth).
func (s *OpenAIGatewayService) SelectAccountWithSchedulerForImages(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	_ OpenAIImagesCapability,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	return s.SelectAccountWithSchedulerForImagesForPlatform(
		ctx,
		PlatformOpenAI,
		groupID,
		sessionHash,
		requestedModel,
		excludedIDs,
		OpenAIImagesCapabilityBasic,
	)
}

func (s *OpenAIGatewayService) SelectAccountWithSchedulerForImagesForPlatform(
	ctx context.Context,
	platform string,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	_ OpenAIImagesCapability,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	return s.SelectAccountWithSchedulerForPlatform(
		ctx,
		platform,
		groupID,
		"",
		sessionHash,
		requestedModel,
		excludedIDs,
		OpenAIUpstreamTransportHTTPSSE,
	)
}
