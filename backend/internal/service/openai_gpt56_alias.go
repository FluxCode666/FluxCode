package service

import "strings"

func normalizeGPT56ModelAlias(model string) (string, bool) {
	modelID := normalizeGPT56ModelID(model)
	if modelID == "gpt-5.6" {
		return "gpt-5.6-sol", true
	}
	if !strings.HasPrefix(modelID, "gpt-5.6-") {
		return "", false
	}
	suffix := strings.TrimPrefix(modelID, "gpt-5.6-")
	for _, variant := range []string{"sol", "terra", "luna"} {
		if suffix == variant {
			return "gpt-5.6-" + variant, true
		}
		if strings.HasPrefix(suffix, variant+"-") {
			variantSuffix := strings.TrimPrefix(suffix, variant+"-")
			if variantSuffix == "preview" || isGPT56ReasoningSuffix(variantSuffix) || isGPT56DateSuffix(variantSuffix) {
				return "gpt-5.6-" + variant, true
			}
			return "", false
		}
	}
	if isGPT56ReasoningSuffix(suffix) || isGPT56DateSuffix(suffix) {
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
	modelID = strings.Trim(modelID, "-")
	if strings.HasPrefix(modelID, "gpt5") {
		modelID = "gpt-5" + strings.TrimPrefix(modelID, "gpt5")
	}
	return modelID
}

func isGPT56ReasoningSuffix(raw string) bool {
	value := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(raw)))
	switch value {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

func isGPT56DateSuffix(raw string) bool {
	parts := strings.Split(raw, "-")
	if len(parts) != 3 || len(parts[0]) != 4 || len(parts[1]) != 2 || len(parts[2]) != 2 {
		return false
	}
	for _, part := range parts {
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func appendUsageBillingModelCandidate(candidates []string, seen map[string]struct{}, model string) []string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return candidates
	}
	key := strings.ToLower(trimmed)
	if _, ok := seen[key]; ok {
		return candidates
	}
	seen[key] = struct{}{}
	candidates = append(candidates, trimmed)
	return candidates
}

func usageBillingModelCandidates(primary string, alternates ...string) []string {
	seen := make(map[string]struct{}, len(alternates)+1)
	sources := append([]string{primary}, alternates...)
	var candidates []string
	for _, source := range sources {
		candidates = appendUsageBillingModelCandidate(candidates, seen, source)
	}
	for _, source := range sources {
		if canonical, ok := normalizeGPT56ModelAlias(source); ok {
			candidates = appendUsageBillingModelCandidate(candidates, seen, canonical)
		}
	}
	return candidates
}
