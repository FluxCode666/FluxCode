package service

import (
	"net/http"
	"net/url"
	"strings"
)

func validateProvider(provider string) error {
	if !isSupportedProvider(provider) {
		return ErrChannelMonitorInvalidProvider
	}
	return nil
}

func validateAPIMode(provider, apiMode string) error {
	apiMode = defaultAPIMode(apiMode)
	switch apiMode {
	case MonitorAPIModeChatCompletions:
		return nil
	case MonitorAPIModeResponses:
		if provider == "" || provider == MonitorProviderOpenAI {
			return nil
		}
	}
	return ErrChannelMonitorInvalidAPIMode
}

func validateInterval(seconds int) error {
	if seconds < monitorMinIntervalSeconds || seconds > monitorMaxIntervalSeconds {
		return ErrChannelMonitorInvalidInterval
	}
	return nil
}

func NormalizeChannelMonitorInterval(value int, defaultValue int) int {
	if value <= 0 {
		value = defaultValue
	}
	if value < monitorMinIntervalSeconds {
		return monitorMinIntervalSeconds
	}
	if value > monitorMaxIntervalSeconds {
		return monitorMaxIntervalSeconds
	}
	return value
}

func ValidateChannelMonitorEndpoint(endpoint string) error {
	return validateEndpoint(endpoint)
}

func validateEndpoint(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ErrChannelMonitorInvalidEndpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" {
		return ErrChannelMonitorInvalidEndpoint
	}
	if u.Scheme != "https" {
		return ErrChannelMonitorEndpointScheme
	}
	if u.Path != "" && u.Path != "/" {
		return ErrChannelMonitorEndpointPath
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return ErrChannelMonitorEndpointPath
	}
	if isPrivateOrBlockedHostname(u.Hostname()) {
		return ErrChannelMonitorEndpointPrivate
	}
	return nil
}

func normalizeEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	return strings.TrimRight(endpoint, "/")
}

func NormalizeChannelMonitorModels(models []string) []string {
	return normalizeModels(models)
}

func normalizeModels(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}

func defaultAPIMode(apiMode string) string {
	apiMode = strings.TrimSpace(apiMode)
	if apiMode == "" {
		return MonitorAPIModeChatCompletions
	}
	return apiMode
}

func defaultBodyMode(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return MonitorBodyOverrideModeOff
	}
	return mode
}

func emptyHeadersIfNil(headers map[string]string) map[string]string {
	if headers == nil {
		return map[string]string{}
	}
	return headers
}

func validateBodyModeForProtocol(provider, apiMode, mode string, body map[string]any) error {
	mode = defaultBodyMode(mode)
	switch mode {
	case MonitorBodyOverrideModeOff:
		return nil
	case MonitorBodyOverrideModeMerge, MonitorBodyOverrideModeReplace:
		if len(body) == 0 {
			return ErrChannelMonitorTemplateBodyRequired
		}
	default:
		return ErrChannelMonitorTemplateInvalidBodyMode
	}
	if provider == MonitorProviderOpenAI && mode == MonitorBodyOverrideModeReplace {
		if err := validateReplaceRequestBody(provider, defaultAPIMode(apiMode), body); err != nil {
			return ErrChannelMonitorInvalidRequestBody
		}
	}
	return nil
}

func validateExtraHeaders(headers map[string]string) error {
	for name := range headers {
		if !isValidHeaderFieldName(name) {
			return ErrChannelMonitorTemplateHeaderInvalidName
		}
		if IsForbiddenHeaderName(name) {
			return ErrChannelMonitorTemplateHeaderForbidden
		}
	}
	return nil
}

func IsForbiddenHeaderName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return true
	}
	switch name {
	case "host", "content-length", "connection", "keep-alive", "proxy-authenticate",
		"proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	case strings.ToLower(http.CanonicalHeaderKey("Authorization")),
		strings.ToLower(http.CanonicalHeaderKey("X-Api-Key")),
		strings.ToLower(http.CanonicalHeaderKey("X-Goog-Api-Key")):
		return true
	default:
		return false
	}
}

func isValidHeaderFieldName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !isHeaderTokenChar(name[i]) {
			return false
		}
	}
	return true
}

func isHeaderTokenChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c >= '0' && c <= '9':
		return true
	}
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	default:
		return false
	}
}
