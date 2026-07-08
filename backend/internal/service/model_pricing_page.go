package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type modelPricingChannelLister interface {
	List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]Channel, *pagination.PaginationResult, error)
}

type modelPricingGroupLister interface {
	ListActive(ctx context.Context) ([]Group, error)
}

type modelPricingBilling interface {
	GetModelPricing(model string) (*ModelPricing, error)
}

type ModelPricingPageService struct {
	channels modelPricingChannelLister
	groups   modelPricingGroupLister
	billing  modelPricingBilling
}

func NewModelPricingPageService(channelService *ChannelService, groupService *GroupService, billingService *BillingService) *ModelPricingPageService {
	return &ModelPricingPageService{channels: channelService, groups: groupService, billing: billingService}
}

func NewModelPricingPageServiceForTest(channels modelPricingChannelLister, groups modelPricingGroupLister, billing modelPricingBilling) *ModelPricingPageService {
	return &ModelPricingPageService{channels: channels, groups: groups, billing: billing}
}

type ModelPricingQuery struct {
	Q          string
	Platform   string
	Capability string
}

type ModelPricingAmount struct {
	InputPrice       float64                `json:"input_price"`
	OutputPrice      float64                `json:"output_price"`
	CacheWritePrice  float64                `json:"cache_write_price"`
	CacheReadPrice   float64                `json:"cache_read_price"`
	ImageOutputPrice float64                `json:"image_output_price"`
	PerRequestPrice  float64                `json:"per_request_price"`
	Intervals        []ModelPricingInterval `json:"intervals"`
}

type ModelPricingInterval struct {
	MinTokens       int     `json:"min_tokens"`
	MaxTokens       *int    `json:"max_tokens"`
	TierLabel       string  `json:"tier_label"`
	InputPrice      float64 `json:"input_price"`
	OutputPrice     float64 `json:"output_price"`
	CacheWritePrice float64 `json:"cache_write_price"`
	CacheReadPrice  float64 `json:"cache_read_price"`
	PerRequestPrice float64 `json:"per_request_price"`
}

type ModelPricingMultipliers struct {
	InputPrice       float64 `json:"input_price"`
	OutputPrice      float64 `json:"output_price"`
	CacheWritePrice  float64 `json:"cache_write_price"`
	CacheReadPrice   float64 `json:"cache_read_price"`
	ImageOutputPrice float64 `json:"image_output_price"`
	PerRequestPrice  float64 `json:"per_request_price"`
}

type ModelPricingModelSummary struct {
	ID                  string             `json:"id"`
	DisplayName         string             `json:"display_name"`
	Platform            string             `json:"platform"`
	Capabilities        []string           `json:"capabilities"`
	SupportedGroupCount int                `json:"supported_group_count"`
	OfficialPrice       ModelPricingAmount `json:"official_price"`
}

type ModelPricingGroupPrice struct {
	GroupID        int64                   `json:"group_id"`
	GroupName      string                  `json:"group_name"`
	RateMultiplier float64                 `json:"rate_multiplier"`
	BillingMode    string                  `json:"billing_mode"`
	Price          ModelPricingAmount      `json:"price"`
	Multipliers    ModelPricingMultipliers `json:"multipliers"`
}

type ModelPricingModelDetail struct {
	ID            string                   `json:"id"`
	DisplayName   string                   `json:"display_name"`
	Platform      string                   `json:"platform"`
	Capabilities  []string                 `json:"capabilities"`
	OfficialPrice ModelPricingAmount       `json:"official_price"`
	Groups        []ModelPricingGroupPrice `json:"groups"`
}

var ErrModelPricingNotFound = errors.New("model pricing not found")

type modelCatalogItem struct {
	ID           string
	Platform     string
	Capabilities map[string]struct{}
	Official     *ModelPricing
	Groups       []modelCatalogGroup
}

type modelCatalogGroup struct {
	GroupID        int64
	GroupName      string
	RateMultiplier float64
	BillingMode    BillingMode
	Resolved       ModelPricingAmount
}

func (s *ModelPricingPageService) ListModels(ctx context.Context, query ModelPricingQuery) ([]ModelPricingModelSummary, error) {
	catalog, err := s.buildCatalog(ctx)
	if err != nil {
		return nil, err
	}
	models := make([]ModelPricingModelSummary, 0, len(catalog))
	for _, item := range catalog {
		if !matchesModelPricingQuery(item, query) {
			continue
		}
		models = append(models, ModelPricingModelSummary{
			ID:                  item.ID,
			DisplayName:         displayModelName(item.ID),
			Platform:            item.Platform,
			Capabilities:        sortedStrings(item.Capabilities),
			SupportedGroupCount: len(item.Groups),
			OfficialPrice:       modelPricingToAmount(item.Official),
		})
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Platform == models[j].Platform {
			return models[i].ID < models[j].ID
		}
		return models[i].Platform < models[j].Platform
	})
	return models, nil
}

func (s *ModelPricingPageService) GetModel(ctx context.Context, model string) (*ModelPricingModelDetail, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, errors.New("model is required")
	}
	catalog, err := s.buildCatalog(ctx)
	if err != nil {
		return nil, err
	}
	item, ok := catalog[model]
	if !ok {
		return nil, ErrModelPricingNotFound
	}
	groups := make([]ModelPricingGroupPrice, 0, len(item.Groups))
	for _, group := range item.Groups {
		final := applyGroupMultiplier(group.Resolved, group.RateMultiplier)
		groups = append(groups, ModelPricingGroupPrice{
			GroupID:        group.GroupID,
			GroupName:      group.GroupName,
			RateMultiplier: group.RateMultiplier,
			BillingMode:    string(group.BillingMode),
			Price:          final,
			Multipliers:    amountMultipliers(final, modelPricingToAmount(item.Official)),
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].RateMultiplier == groups[j].RateMultiplier {
			return groups[i].GroupName < groups[j].GroupName
		}
		return groups[i].RateMultiplier < groups[j].RateMultiplier
	})
	return &ModelPricingModelDetail{
		ID:            item.ID,
		DisplayName:   displayModelName(item.ID),
		Platform:      item.Platform,
		Capabilities:  sortedStrings(item.Capabilities),
		OfficialPrice: modelPricingToAmount(item.Official),
		Groups:        groups,
	}, nil
}

func (s *ModelPricingPageService) buildCatalog(ctx context.Context) (map[string]*modelCatalogItem, error) {
	channels, _, err := s.channels.List(ctx, pagination.PaginationParams{Page: 1, PageSize: 10000, SortBy: "id", SortOrder: "asc"}, StatusActive, "")
	if err != nil {
		return nil, err
	}
	groups, err := s.groups.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	groupsByID := make(map[int64]Group, len(groups))
	for _, group := range groups {
		if group.Status == StatusActive {
			groupsByID[group.ID] = group
		}
	}
	catalog := map[string]*modelCatalogItem{}
	for i := range channels {
		channel := channels[i]
		if !channel.IsActive() {
			continue
		}
		for _, pricing := range channel.ModelPricing {
			for _, model := range pricing.Models {
				if isWildcardModelPattern(model) {
					continue
				}
				official, err := s.billing.GetModelPricing(model)
				if err != nil || official == nil {
					continue
				}
				item := catalog[model]
				if item == nil {
					item = &modelCatalogItem{
						ID:           model,
						Platform:     pricing.Platform,
						Capabilities: map[string]struct{}{},
						Official:     official,
					}
					catalog[model] = item
				}
				for _, capability := range NormalizeModelCapabilities(pricing.Capabilities) {
					item.Capabilities[capability] = struct{}{}
				}
				for _, groupID := range channel.GroupIDs {
					group, ok := groupsByID[groupID]
					if !ok || group.Platform != pricing.Platform {
						continue
					}
					resolved := resolveDisplayPricing(*official, pricing)
					item.Groups = appendOrReplaceGroup(item.Groups, modelCatalogGroup{
						GroupID:        group.ID,
						GroupName:      group.Name,
						RateMultiplier: normalizeRateMultiplier(group.RateMultiplier),
						BillingMode:    normalizeBillingMode(pricing.BillingMode),
						Resolved:       resolved,
					})
				}
			}
		}
	}
	for model, item := range catalog {
		_ = model
		attachWildcardSupportedGroups(item, channels, groupsByID)
	}
	for model, item := range catalog {
		if len(item.Groups) == 0 {
			delete(catalog, model)
		}
	}
	return catalog, nil
}

func isWildcardModelPattern(model string) bool { return strings.Contains(model, "*") }

func modelPatternMatches(pattern, model string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	model = strings.ToLower(strings.TrimSpace(model))
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(model, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == model
}

func displayModelName(model string) string { return model }

func normalizeRateMultiplier(v float64) float64 {
	if v <= 0 {
		return 1
	}
	return v
}

func normalizeBillingMode(v BillingMode) BillingMode {
	if v == "" {
		return BillingModeToken
	}
	return v
}

func resolveDisplayPricing(official ModelPricing, pricing ChannelModelPricing) ModelPricingAmount {
	base := modelPricingToAmount(&official)
	if pricing.InputPrice != nil {
		base.InputPrice = *pricing.InputPrice
	}
	if pricing.OutputPrice != nil {
		base.OutputPrice = *pricing.OutputPrice
	}
	if pricing.CacheWritePrice != nil {
		base.CacheWritePrice = *pricing.CacheWritePrice
	}
	if pricing.CacheReadPrice != nil {
		base.CacheReadPrice = *pricing.CacheReadPrice
	}
	if pricing.ImageOutputPrice != nil {
		base.ImageOutputPrice = *pricing.ImageOutputPrice
	}
	if pricing.PerRequestPrice != nil {
		base.PerRequestPrice = *pricing.PerRequestPrice
	}
	base.Intervals = intervalsToDisplayAmounts(pricing.Intervals)
	return base
}

func modelPricingToAmount(pricing *ModelPricing) ModelPricingAmount {
	if pricing == nil {
		return ModelPricingAmount{}
	}
	return ModelPricingAmount{
		InputPrice:       pricing.InputPricePerToken,
		OutputPrice:      pricing.OutputPricePerToken,
		CacheWritePrice:  pricing.CacheCreationPricePerToken,
		CacheReadPrice:   pricing.CacheReadPricePerToken,
		ImageOutputPrice: pricing.ImageOutputPricePerToken,
	}
}

func intervalsToDisplayAmounts(intervals []PricingInterval) []ModelPricingInterval {
	out := make([]ModelPricingInterval, 0, len(intervals))
	for _, iv := range filterValidIntervals(intervals) {
		out = append(out, ModelPricingInterval{
			MinTokens:       iv.MinTokens,
			MaxTokens:       iv.MaxTokens,
			TierLabel:       iv.TierLabel,
			InputPrice:      derefFloat(iv.InputPrice),
			OutputPrice:     derefFloat(iv.OutputPrice),
			CacheWritePrice: derefFloat(iv.CacheWritePrice),
			CacheReadPrice:  derefFloat(iv.CacheReadPrice),
			PerRequestPrice: derefFloat(iv.PerRequestPrice),
		})
	}
	return out
}

func applyGroupMultiplier(amount ModelPricingAmount, multiplier float64) ModelPricingAmount {
	multiplier = normalizeRateMultiplier(multiplier)
	amount.InputPrice *= multiplier
	amount.OutputPrice *= multiplier
	amount.CacheWritePrice *= multiplier
	amount.CacheReadPrice *= multiplier
	amount.ImageOutputPrice *= multiplier
	amount.PerRequestPrice *= multiplier
	for i := range amount.Intervals {
		amount.Intervals[i].InputPrice *= multiplier
		amount.Intervals[i].OutputPrice *= multiplier
		amount.Intervals[i].CacheWritePrice *= multiplier
		amount.Intervals[i].CacheReadPrice *= multiplier
		amount.Intervals[i].PerRequestPrice *= multiplier
	}
	return amount
}

func amountMultipliers(final, official ModelPricingAmount) ModelPricingMultipliers {
	return ModelPricingMultipliers{
		InputPrice:       ratio(final.InputPrice, official.InputPrice),
		OutputPrice:      ratio(final.OutputPrice, official.OutputPrice),
		CacheWritePrice:  ratio(final.CacheWritePrice, official.CacheWritePrice),
		CacheReadPrice:   ratio(final.CacheReadPrice, official.CacheReadPrice),
		ImageOutputPrice: ratio(final.ImageOutputPrice, official.ImageOutputPrice),
		PerRequestPrice:  ratio(final.PerRequestPrice, official.PerRequestPrice),
	}
}

func ratio(value, base float64) float64 {
	if base == 0 {
		return 0
	}
	return value / base
}

func derefFloat(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func appendOrReplaceGroup(groups []modelCatalogGroup, next modelCatalogGroup) []modelCatalogGroup {
	for i := range groups {
		if groups[i].GroupID == next.GroupID {
			groups[i] = next
			return groups
		}
	}
	return append(groups, next)
}

func attachWildcardSupportedGroups(item *modelCatalogItem, channels []Channel, groupsByID map[int64]Group) {
	for i := range channels {
		channel := channels[i]
		if !channel.IsActive() {
			continue
		}
		for _, pricing := range channel.ModelPricing {
			if !pricingMatchesModel(pricing, item.ID) {
				continue
			}
			if item.Platform == "" {
				item.Platform = pricing.Platform
			}
			for _, capability := range NormalizeModelCapabilities(pricing.Capabilities) {
				item.Capabilities[capability] = struct{}{}
			}
			for _, groupID := range channel.GroupIDs {
				group, ok := groupsByID[groupID]
				if !ok || group.Platform != pricing.Platform {
					continue
				}
				resolved := resolveDisplayPricing(*item.Official, pricing)
				item.Groups = appendOrReplaceGroup(item.Groups, modelCatalogGroup{
					GroupID:        group.ID,
					GroupName:      group.Name,
					RateMultiplier: normalizeRateMultiplier(group.RateMultiplier),
					BillingMode:    normalizeBillingMode(pricing.BillingMode),
					Resolved:       resolved,
				})
			}
		}
	}
}

func pricingMatchesModel(pricing ChannelModelPricing, model string) bool {
	for _, pattern := range pricing.Models {
		if modelPatternMatches(pattern, model) {
			return true
		}
	}
	return false
}

func matchesModelPricingQuery(item *modelCatalogItem, query ModelPricingQuery) bool {
	if query.Platform != "" && !strings.EqualFold(item.Platform, strings.TrimSpace(query.Platform)) {
		return false
	}
	capability := strings.ToLower(strings.TrimSpace(query.Capability))
	if capability != "" {
		if _, ok := item.Capabilities[capability]; !ok {
			return false
		}
	}
	q := strings.ToLower(strings.TrimSpace(query.Q))
	if q == "" {
		return true
	}
	if strings.Contains(strings.ToLower(item.ID), q) || strings.Contains(strings.ToLower(displayModelName(item.ID)), q) || strings.Contains(strings.ToLower(item.Platform), q) {
		return true
	}
	for capability := range item.Capabilities {
		if strings.Contains(capability, q) {
			return true
		}
	}
	return false
}

func sortedStrings(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for item := range set {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
