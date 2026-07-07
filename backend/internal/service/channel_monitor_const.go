package service

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	monitorRequestTimeout           = 45 * time.Second
	monitorPingTimeout              = 8 * time.Second
	monitorDegradedThreshold        = 6 * time.Second
	monitorHistoryRetentionDays     = 30
	monitorRollupRetentionDays      = 30
	monitorMaintenanceMaxDaysPerRun = 35
	monitorWorkerConcurrency        = 5
	monitorStartupLoadTimeout       = 10 * time.Second
	monitorMinIntervalSeconds       = 15
	monitorMaxIntervalSeconds       = 3600
	monitorMessageMaxBytes          = 500
	monitorResponseMaxBytes         = 64 * 1024
	monitorErrorBodySnippetMaxBytes = 300
	monitorChallengeMin             = 1
	monitorChallengeMax             = 50

	providerOpenAIPath           = "/v1/chat/completions"
	providerOpenAIResponsesPath  = "/v1/responses"
	providerAnthropicPath        = "/v1/messages"
	providerGeminiPathTemplate   = "/v1beta/models/%s:generateContent"
	monitorAnthropicAPIVersion   = "2023-06-01"
	monitorChallengeMaxTokens    = 50
	monitorRunOneBuffer          = 10 * time.Second
	monitorIdleConnTimeout       = 30 * time.Second
	monitorTLSHandshakeTimeout   = 10 * time.Second
	monitorResponseHeaderTimeout = 30 * time.Second
	monitorPingDiscardMaxBytes   = 1024
	monitorDialTimeout           = 10 * time.Second
	monitorDialKeepAlive         = 30 * time.Second
)

const (
	MonitorProviderOpenAI    = "openai"
	MonitorProviderAnthropic = "anthropic"
	MonitorProviderGemini    = "gemini"

	MonitorStatusOperational = "operational"
	MonitorStatusDegraded    = "degraded"
	MonitorStatusFailed      = "failed"
	MonitorStatusError       = "error"

	MonitorBodyOverrideModeOff     = "off"
	MonitorBodyOverrideModeMerge   = "merge"
	MonitorBodyOverrideModeReplace = "replace"

	MonitorAPIModeChatCompletions = "chat_completions"
	MonitorAPIModeResponses       = "responses"

	MonitorHistoryDefaultLimit = 100
	MonitorHistoryMaxLimit     = 1000
	monitorTimelineMaxPoints   = 60

	monitorAvailability7Days  = 7
	monitorAvailability15Days = 15
	monitorAvailability30Days = 30
)

const (
	ChannelMonitorProviderOpenAI    = MonitorProviderOpenAI
	ChannelMonitorProviderAnthropic = MonitorProviderAnthropic
	ChannelMonitorProviderGemini    = MonitorProviderGemini

	ChannelMonitorStatusOperational = MonitorStatusOperational
	ChannelMonitorStatusDegraded    = MonitorStatusDegraded
	ChannelMonitorStatusFailed      = MonitorStatusFailed
	ChannelMonitorStatusError       = MonitorStatusError

	ChannelMonitorAPIModeChatCompletions = MonitorAPIModeChatCompletions
	ChannelMonitorAPIModeResponses       = MonitorAPIModeResponses

	ChannelMonitorBodyOverrideOff     = MonitorBodyOverrideModeOff
	ChannelMonitorBodyOverrideMerge   = MonitorBodyOverrideModeMerge
	ChannelMonitorBodyOverrideReplace = MonitorBodyOverrideModeReplace

	ChannelMonitorMinIntervalSeconds     = monitorMinIntervalSeconds
	ChannelMonitorMaxIntervalSeconds     = monitorMaxIntervalSeconds
	ChannelMonitorFallbackIntervalSecond = 60
)

var (
	ErrChannelMonitorNotFound = infraerrors.NotFound(
		"CHANNEL_MONITOR_NOT_FOUND", "channel monitor not found",
	)
	ErrChannelMonitorInvalidProvider = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_PROVIDER", "provider must be one of openai/anthropic/gemini",
	)
	ErrChannelMonitorInvalidAPIMode = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_API_MODE", "api_mode must be chat_completions or responses; responses is only supported for openai",
	)
	ErrChannelMonitorInvalidRequestBody = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_REQUEST_BODY", "openai replace-mode body_override must include non-empty messages for chat_completions or non-empty instructions and input for responses",
	)
	ErrChannelMonitorInvalidInterval = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_INTERVAL", "interval_seconds must be in [15, 3600]",
	)
	ErrChannelMonitorInvalidJitter = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_JITTER", "jitter_seconds must be >= 0 and interval_seconds - jitter_seconds must be >= 15",
	)
	ErrChannelMonitorInvalidEndpoint = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_ENDPOINT", "endpoint must be a valid https origin",
	)
	ErrChannelMonitorEndpointScheme = infraerrors.BadRequest(
		"CHANNEL_MONITOR_ENDPOINT_SCHEME", "endpoint must use https scheme",
	)
	ErrChannelMonitorEndpointPath = infraerrors.BadRequest(
		"CHANNEL_MONITOR_ENDPOINT_PATH", "endpoint must be base origin only",
	)
	ErrChannelMonitorEndpointPrivate = infraerrors.BadRequest(
		"CHANNEL_MONITOR_ENDPOINT_PRIVATE", "endpoint must be a public host",
	)
	ErrChannelMonitorMissingAPIKey = infraerrors.BadRequest(
		"CHANNEL_MONITOR_MISSING_API_KEY", "api_key is required when creating a monitor",
	)
	ErrChannelMonitorMissingPrimaryModel = infraerrors.BadRequest(
		"CHANNEL_MONITOR_MISSING_PRIMARY_MODEL", "primary_model is required",
	)
	ErrChannelMonitorAPIKeyDecryptFailed = infraerrors.InternalServer(
		"CHANNEL_MONITOR_KEY_DECRYPT_FAILED", "api key decryption failed; please re-edit the monitor with a fresh key",
	)
)
