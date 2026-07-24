package service

import (
	"context"
	"math"
	"sort"
	"strings"
)

// EmbeddingModelEligibility is the immutable result of deciding whether one
// embedding account can serve a public model. The handler keeps this result
// through forwarding and billing so a request never observes a newer price
// configuration after the upstream response has supplied prompt_tokens.
type EmbeddingModelEligibility struct {
	Account        Account
	PublicModel    string
	ChannelMapping ChannelMappingResult
	BillingModel   string
	UpstreamModel  string
	Pricing        *ResolvedPricing
}

// PricingForPromptTokens selects a positive input price from the request's
// frozen pricing snapshot. It deliberately does not consult channel or model
// pricing services again.
func (c EmbeddingModelEligibility) PricingForPromptTokens(promptTokens int) (*ModelPricing, bool) {
	return embeddingPricingForPromptTokens(c.Pricing, promptTokens)
}

// ListAvailableEmbeddingModels returns exactly the public models that have at
// least one schedulable embedding API-key account and a positive token price.
// Unlike the generic model list, embedding never falls back to a default model.
func (s *OpenAIGatewayService) ListAvailableEmbeddingModels(ctx context.Context, groupID *int64) ([]string, error) {
	if s == nil || groupID == nil {
		return nil, nil
	}

	accounts, err := s.listSchedulableAccountsForPlatform(ctx, groupID, PlatformEmbedding)
	if err != nil {
		return nil, err
	}

	models := make(map[string]struct{})
	for i := range accounts {
		for publicModel := range embeddingAccountModelMappings(&accounts[i]) {
			candidates := s.resolveEmbeddingModelEligibilityFromAccounts(ctx, groupID, publicModel, accounts)
			if len(candidates) > 0 {
				models[publicModel] = struct{}{}
			}
		}
	}

	if len(models) == 0 {
		return nil, nil
	}
	result := make([]string, 0, len(models))
	for model := range models {
		result = append(result, model)
	}
	sort.Strings(result)
	return result, nil
}

// ResolveEmbeddingModelEligibility applies the same rules used by the model
// list to one public model. It returns one candidate per eligible account so
// the forwarding layer can preserve the account's mapped upstream model and
// pricing snapshot while using its normal scheduling policy.
func (s *OpenAIGatewayService) ResolveEmbeddingModelEligibility(ctx context.Context, groupID *int64, publicModel string) ([]EmbeddingModelEligibility, error) {
	if s == nil || groupID == nil || strings.TrimSpace(publicModel) == "" {
		return nil, nil
	}

	accounts, err := s.listSchedulableAccountsForPlatform(ctx, groupID, PlatformEmbedding)
	if err != nil {
		return nil, err
	}
	return s.resolveEmbeddingModelEligibilityFromAccounts(ctx, groupID, publicModel, accounts), nil
}

func (s *OpenAIGatewayService) resolveEmbeddingModelEligibilityFromAccounts(
	ctx context.Context,
	groupID *int64,
	publicModel string,
	accounts []Account,
) []EmbeddingModelEligibility {
	publicModel = strings.TrimSpace(publicModel)
	if publicModel == "" || s == nil || s.resolver == nil {
		return nil
	}

	channelMapping, _ := s.ResolveChannelMappingAndRestrict(ctx, groupID, publicModel)
	if strings.TrimSpace(channelMapping.MappedModel) == "" {
		channelMapping.MappedModel = publicModel
	}

	candidates := make([]EmbeddingModelEligibility, 0, len(accounts))
	for i := range accounts {
		account := accounts[i]
		if !account.IsSchedulable() || !account.IsEmbeddingAPIKey() ||
			account.GetEmbeddingBaseURL() == "" || account.GetEmbeddingAPIKey() == "" {
			continue
		}

		upstreamModel, matched := embeddingAccountModelMappings(&account)[publicModel]
		if !matched || strings.TrimSpace(upstreamModel) == "" {
			continue
		}
		upstreamModel = strings.TrimSpace(upstreamModel)

		billingModel := embeddingBillingModel(channelMapping, publicModel, upstreamModel)
		if s.embeddingModelRestrictedByChannel(ctx, groupID, billingModel) {
			continue
		}
		pricing := freezeResolvedPricing(s.resolver.Resolve(ctx, PricingInput{
			Model:   billingModel,
			GroupID: groupID,
		}))
		if !hasPositiveEmbeddingInputPricing(pricing) {
			continue
		}

		candidates = append(candidates, EmbeddingModelEligibility{
			Account:        account,
			PublicModel:    publicModel,
			ChannelMapping: channelMapping,
			BillingModel:   billingModel,
			UpstreamModel:  upstreamModel,
			Pricing:        pricing,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Account.ID < candidates[j].Account.ID
	})
	return candidates
}

func (s *OpenAIGatewayService) embeddingModelRestrictedByChannel(ctx context.Context, groupID *int64, billingModel string) bool {
	if groupID == nil || s == nil || s.channelService == nil || strings.TrimSpace(billingModel) == "" {
		return false
	}
	return s.channelService.IsModelRestricted(ctx, *groupID, billingModel)
}

func embeddingBillingModel(mapping ChannelMappingResult, publicModel, upstreamModel string) string {
	switch mapping.BillingModelSource {
	case BillingModelSourceRequested:
		return publicModel
	case BillingModelSourceUpstream:
		return upstreamModel
	case BillingModelSourceChannelMapped, "":
		return strings.TrimSpace(mapping.MappedModel)
	default:
		return strings.TrimSpace(mapping.MappedModel)
	}
}

// embeddingAccountModelMappings turns the explicit embedding configuration
// into a strong public-model allowlist. Generic Account.IsModelSupported is
// intentionally not used because it permits an empty mapping as a fallback.
// Legacy model_whitelist remains an identity mapping for existing resources.
func embeddingAccountModelMappings(account *Account) map[string]string {
	if account == nil || !account.HasEmbeddingModelConfiguration() {
		return nil
	}

	result := make(map[string]string)
	if account.Credentials != nil {
		switch raw := account.Credentials["model_mapping"].(type) {
		case map[string]any:
			for publicModel, value := range raw {
				upstreamModel, ok := value.(string)
				if publicModel = strings.TrimSpace(publicModel); ok && publicModel != "" && strings.TrimSpace(upstreamModel) != "" {
					result[publicModel] = strings.TrimSpace(upstreamModel)
				}
			}
		case map[string]string:
			for publicModel, upstreamModel := range raw {
				publicModel = strings.TrimSpace(publicModel)
				upstreamModel = strings.TrimSpace(upstreamModel)
				if publicModel != "" && upstreamModel != "" {
					result[publicModel] = upstreamModel
				}
			}
		}
	}
	if len(result) > 0 {
		return result
	}

	for _, model := range embeddingLegacyWhitelist(account) {
		result[model] = model
	}
	return result
}

func embeddingLegacyWhitelist(account *Account) []string {
	if account == nil || account.Credentials == nil {
		return nil
	}
	models := make([]string, 0)
	appendModel := func(raw string) {
		model := strings.TrimSpace(raw)
		if model != "" {
			models = append(models, model)
		}
	}
	switch whitelist := account.Credentials["model_whitelist"].(type) {
	case []any:
		for _, raw := range whitelist {
			if model, ok := raw.(string); ok {
				appendModel(model)
			}
		}
	case []string:
		for _, model := range whitelist {
			appendModel(model)
		}
	}
	return models
}

func embeddingPricingForPromptTokens(resolved *ResolvedPricing, promptTokens int) (*ModelPricing, bool) {
	if resolved == nil || resolved.Mode != BillingModeToken || promptTokens <= 0 {
		return nil, false
	}
	pricing := (&ModelPricingResolver{}).GetIntervalPricing(resolved, promptTokens)
	if pricing == nil || pricing.InputPricePerToken <= 0 {
		return nil, false
	}
	copy := *pricing
	return &copy, true
}

// hasPositiveEmbeddingInputPricing proves that every representable positive
// prompt-token count resolves to a strictly positive input price. Intervals
// may leave gaps only when the frozen base price covers those gaps.
func hasPositiveEmbeddingInputPricing(resolved *ResolvedPricing) bool {
	if resolved == nil || resolved.Mode != BillingModeToken {
		return false
	}
	if len(resolved.Intervals) == 0 {
		_, ok := embeddingPricingForPromptTokens(resolved, 1)
		return ok
	}

	intervals := append([]PricingInterval(nil), resolved.Intervals...)
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].MinTokens < intervals[j].MinTokens })
	basePositive := resolved.BasePricing != nil && resolved.BasePricing.InputPricePerToken > 0
	if !basePositive {
		if intervals[0].MinTokens != 0 {
			return false
		}
	}

	previousMax := 0
	for i := range intervals {
		interval := intervals[i]
		if interval.MinTokens < 0 || interval.MaxTokens != nil && *interval.MaxTokens <= interval.MinTokens {
			return false
		}
		if !basePositive && i > 0 && interval.MinTokens != previousMax {
			return false
		}
		if interval.MinTokens < math.MaxInt {
			if _, ok := embeddingPricingForPromptTokens(resolved, interval.MinTokens+1); !ok {
				return false
			}
		}
		if interval.MaxTokens != nil {
			previousMax = *interval.MaxTokens
			continue
		}
		return true
	}

	return basePositive
}

// freezeResolvedPricing deep-copies all mutable price and interval pointers.
// Channel caches intentionally reuse those pointers, so a shallow copy would
// let a configuration update change the price of an already-forwarded request.
func freezeResolvedPricing(resolved *ResolvedPricing) *ResolvedPricing {
	if resolved == nil {
		return nil
	}
	copy := *resolved
	if resolved.BasePricing != nil {
		base := *resolved.BasePricing
		copy.BasePricing = &base
	}
	copy.Intervals = freezePricingIntervals(resolved.Intervals)
	copy.RequestTiers = freezePricingIntervals(resolved.RequestTiers)
	return &copy
}

func freezePricingIntervals(intervals []PricingInterval) []PricingInterval {
	if len(intervals) == 0 {
		return nil
	}
	copy := make([]PricingInterval, len(intervals))
	for i := range intervals {
		copy[i] = intervals[i]
		copy[i].MaxTokens = cloneEmbeddingInt(intervals[i].MaxTokens)
		copy[i].InputPrice = cloneEmbeddingFloat(intervals[i].InputPrice)
		copy[i].OutputPrice = cloneEmbeddingFloat(intervals[i].OutputPrice)
		copy[i].CacheWritePrice = cloneEmbeddingFloat(intervals[i].CacheWritePrice)
		copy[i].CacheReadPrice = cloneEmbeddingFloat(intervals[i].CacheReadPrice)
		copy[i].PerRequestPrice = cloneEmbeddingFloat(intervals[i].PerRequestPrice)
	}
	return copy
}

func cloneEmbeddingInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneEmbeddingFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
