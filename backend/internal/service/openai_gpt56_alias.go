package service

import "strings"

func normalizeGPT56ModelAlias(model string) (string, bool) {
	modelID := normalizeGPT56ModelID(model)
	if modelID == "" {
		return "", false
	}
	if modelID == "gpt-5.6" {
		return "gpt-5.6-sol", true
	}
	if !strings.HasPrefix(modelID, "gpt-5.6-") {
		return "", false
	}

	suffix := strings.TrimPrefix(modelID, "gpt-5.6-")
	for _, variant := range []string{"sol", "terra", "luna"} {
		if suffix == variant || strings.HasPrefix(suffix, variant+"-") {
			return "gpt-5.6-" + variant, true
		}
	}
	if isGPT56ReasoningSuffix(suffix) {
		return "gpt-5.6-sol", true
	}
	return "", false
}

func isGPT56KnownModel(model string) bool {
	_, ok := normalizeGPT56ModelAlias(model)
	return ok
}

func isGPT56BareOrEffortAlias(model string) bool {
	modelID := normalizeGPT56ModelID(model)
	if modelID == "gpt-5.6" {
		return true
	}
	return strings.HasPrefix(modelID, "gpt-5.6-") &&
		isGPT56ReasoningSuffix(strings.TrimPrefix(modelID, "gpt-5.6-"))
}

func normalizeGPT56ModelID(model string) string {
	modelID := strings.TrimSpace(model)
	if strings.Contains(modelID, "/") {
		parts := strings.Split(modelID, "/")
		modelID = parts[len(parts)-1]
	}
	modelID = strings.ToLower(modelID)
	modelID = strings.NewReplacer("_", "-", " ", "-").Replace(modelID)
	for strings.Contains(modelID, "--") {
		modelID = strings.ReplaceAll(modelID, "--", "-")
	}
	return strings.Trim(modelID, "-")
}

func isGPT56ReasoningSuffix(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.NewReplacer("-", "", "_", "", " ", "").Replace(value)
	switch value {
	case "none", "minimal", "low", "medium", "high", "xhigh", "extrahigh", "max":
		return true
	default:
		return false
	}
}
