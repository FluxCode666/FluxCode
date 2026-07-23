package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

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
	channels    modelPricingChannelLister
	groups      modelPricingGroupLister
	billing     modelPricingBilling
	performance ModelPerformanceMetricsReader
	now         func() time.Time
}

func NewModelPricingPageService(channelService *ChannelService, groupService *GroupService, billingService *BillingService, performanceReader ModelPerformanceMetricsReader) *ModelPricingPageService {
	return newModelPricingPageService(channelService, groupService, billingService, performanceReader)
}

func NewModelPricingPageServiceForTest(channels modelPricingChannelLister, groups modelPricingGroupLister, billing modelPricingBilling) *ModelPricingPageService {
	return newModelPricingPageService(channels, groups, billing, nil)
}

func newModelPricingPageServiceWithPerformanceForTest(channels modelPricingChannelLister, groups modelPricingGroupLister, billing modelPricingBilling, performanceReader ModelPerformanceMetricsReader) *ModelPricingPageService {
	return newModelPricingPageService(channels, groups, billing, performanceReader)
}

func newModelPricingPageService(channels modelPricingChannelLister, groups modelPricingGroupLister, billing modelPricingBilling, performanceReader ModelPerformanceMetricsReader) *ModelPricingPageService {
	return &ModelPricingPageService{
		channels:    channels,
		groups:      groups,
		billing:     billing,
		performance: performanceReader,
		now:         time.Now,
	}
}

type ModelPricingQuery struct {
	Q                string
	Platform         string
	Capability       string
	GroupID          int64
	PerformanceRange ModelPerformanceRange
}

type ModelPricingGroupOption struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
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
	ID                  string                  `json:"id"`
	DisplayName         string                  `json:"display_name"`
	Platform            string                  `json:"platform"`
	Platforms           []string                `json:"platforms"`
	Capabilities        []string                `json:"capabilities"`
	SupportedGroupCount int                     `json:"supported_group_count"`
	OfficialPrice       ModelPricingAmount      `json:"official_price"`
	LowestGroupPrice    ModelPricingAmount      `json:"lowest_group_price"`
	Performance         ModelPerformanceMetrics `json:"performance"`
}

type ModelPricingGroupPrice struct {
	GroupID        int64                   `json:"group_id"`
	GroupName      string                  `json:"group_name"`
	RateMultiplier float64                 `json:"rate_multiplier"`
	BillingMode    string                  `json:"billing_mode"`
	Price          ModelPricingAmount      `json:"price"`
	Multipliers    ModelPricingMultipliers `json:"multipliers"`
	Performance    ModelPerformanceMetrics `json:"performance"`
}

type ModelPricingModelDetail struct {
	ID               string                             `json:"id"`
	DisplayName      string                             `json:"display_name"`
	Platform         string                             `json:"platform"`
	Platforms        []string                           `json:"platforms"`
	Capabilities     []string                           `json:"capabilities"`
	OfficialPrice    ModelPricingAmount                 `json:"official_price"`
	Groups           []ModelPricingGroupPrice           `json:"groups"`
	Performance      ModelPerformanceMetrics            `json:"performance"`
	PerformanceTrend []ModelPerformanceHourlyTrendPoint `json:"performance_trend"`
}

var ErrModelPricingNotFound = errors.New("model pricing not found")

type modelCatalogItem struct {
	ID           string
	Platforms    map[string]struct{}
	Capabilities map[string]struct{}
	Official     *ModelPricing
	Groups       []modelCatalogGroup
}

type modelCatalogGroup struct {
	GroupID        int64
	GroupName      string
	Platform       string
	Capabilities   map[string]struct{}
	RateMultiplier float64
	BillingMode    BillingMode
	Resolved       ModelPricingAmount
}

func (s *ModelPricingPageService) ListModels(ctx context.Context, query ModelPricingQuery) ([]ModelPricingModelSummary, error) {
	window, err := s.resolvePerformanceWindow(query.PerformanceRange)
	if err != nil {
		return nil, err
	}
	catalog, err := s.buildCatalog(ctx)
	if err != nil {
		return nil, err
	}
	models := make([]ModelPricingModelSummary, 0, len(catalog))
	modelIDs := make([]string, 0, len(catalog))
	for _, item := range catalog {
		if !matchesModelPricingQuery(item, query) {
			continue
		}
		filteredGroups := item.groupsMatchingFilters(query)
		models = append(models, ModelPricingModelSummary{
			ID:                  item.ID,
			DisplayName:         displayModelName(item.ID),
			Platform:            item.PlatformDisplay(),
			Platforms:           item.SortedPlatforms(),
			Capabilities:        sortedStrings(item.Capabilities),
			SupportedGroupCount: len(item.Groups),
			OfficialPrice:       modelPricingToAmount(item.Official),
			LowestGroupPrice:    lowestGroupPrice(filteredGroups),
		})
		modelIDs = append(modelIDs, item.ID)
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Platform == models[j].Platform {
			return models[i].ID < models[j].ID
		}
		return models[i].Platform < models[j].Platform
	})
	if len(modelIDs) == 0 || s.performance == nil {
		return models, nil
	}
	var groupID *int64
	if query.GroupID > 0 {
		groupID = &query.GroupID
	}
	metrics, err := s.performance.ListModelPerformanceSummaries(ctx, window, modelIDs, groupID)
	if err != nil {
		return nil, err
	}
	for i := range models {
		if metric, ok := metrics[models[i].ID]; ok {
			models[i].Performance = metric
		}
	}
	return models, nil
}

func (s *ModelPricingPageService) ListGroups(ctx context.Context) ([]ModelPricingGroupOption, error) {
	groups, err := s.groups.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	options := make([]ModelPricingGroupOption, 0, len(groups))
	for _, group := range groups {
		if !isModelPricingVisibleGroup(group) || !isModelPricingPayAsYouGoGroup(group) {
			continue
		}
		options = append(options, ModelPricingGroupOption{
			ID:       group.ID,
			Name:     group.Name,
			Platform: group.Platform,
		})
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].Platform == options[j].Platform {
			if options[i].Name == options[j].Name {
				return options[i].ID < options[j].ID
			}
			return options[i].Name < options[j].Name
		}
		return options[i].Platform < options[j].Platform
	})
	return options, nil
}

func (s *ModelPricingPageService) GetModel(ctx context.Context, model string) (*ModelPricingModelDetail, error) {
	return s.GetModelWithRange(ctx, model, ModelPerformanceRange24Hours)
}

func (s *ModelPricingPageService) GetModelWithRange(ctx context.Context, model string, performanceRange ModelPerformanceRange) (*ModelPricingModelDetail, error) {
	window, err := s.resolvePerformanceWindow(performanceRange)
	if err != nil {
		return nil, err
	}
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
	groupIDs := make([]int64, 0, len(item.Groups))
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
		groupIDs = append(groupIDs, group.GroupID)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].RateMultiplier == groups[j].RateMultiplier {
			return groups[i].GroupName < groups[j].GroupName
		}
		return groups[i].RateMultiplier < groups[j].RateMultiplier
	})
	detail := &ModelPricingModelDetail{
		ID:            item.ID,
		DisplayName:   displayModelName(item.ID),
		Platform:      item.PlatformDisplay(),
		Platforms:     item.SortedPlatforms(),
		Capabilities:  sortedStrings(item.Capabilities),
		OfficialPrice: modelPricingToAmount(item.Official),
		Groups:        groups,
	}
	if s.performance == nil {
		return detail, nil
	}
	performance, err := s.performance.GetModelPerformanceDetail(ctx, window, item.ID, groupIDs)
	if err != nil {
		return nil, err
	}
	if performance == nil {
		return detail, nil
	}
	detail.Performance = performance.Overall
	detail.PerformanceTrend = performance.Trend
	for i := range detail.Groups {
		if metric, ok := performance.Groups[detail.Groups[i].GroupID]; ok {
			detail.Groups[i].Performance = metric
		}
	}
	return detail, nil
}

func (s *ModelPricingPageService) resolvePerformanceWindow(performanceRange ModelPerformanceRange) (ModelPerformanceWindow, error) {
	now := time.Now
	if s != nil && s.now != nil {
		now = s.now
	}
	return ResolveModelPerformanceWindow(now(), performanceRange)
}

func (s *ModelPricingPageService) buildCatalog(ctx context.Context) (map[string]*modelCatalogItem, error) {
	channels, err := s.listAllActiveChannels(ctx)
	if err != nil {
		return nil, err
	}
	groups, err := s.groups.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	groupsByID := make(map[int64]Group, len(groups))
	for _, group := range groups {
		if isModelPricingVisibleGroup(group) {
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
						Platforms:    map[string]struct{}{},
						Capabilities: map[string]struct{}{},
						Official:     official,
					}
					catalog[model] = item
				}
				item.addPlatform(pricing.Platform)
				capabilities := capabilitiesSet(pricing.Capabilities)
				for capability := range capabilities {
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
						Platform:       pricing.Platform,
						Capabilities:   capabilities,
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

func (s *ModelPricingPageService) listAllActiveChannels(ctx context.Context) ([]Channel, error) {
	params := pagination.PaginationParams{Page: 1, PageSize: 1000, SortBy: "id", SortOrder: "asc"}
	all := make([]Channel, 0, params.PageSize)
	for {
		channels, pageResult, err := s.channels.List(ctx, params, StatusActive, "")
		if err != nil {
			return nil, err
		}
		all = append(all, channels...)
		if len(channels) == 0 {
			break
		}
		if pageResult != nil && pageResult.Pages > 0 && params.Page >= pageResult.Pages {
			break
		}
		if pageResult != nil && pageResult.Total > 0 && int64(len(all)) >= pageResult.Total {
			break
		}
		if pageResult == nil && len(channels) < params.Limit() {
			break
		}
		params.Page++
	}
	return all, nil
}

func isWildcardModelPattern(model string) bool { return strings.Contains(model, "*") }

func isModelPricingVisibleGroup(group Group) bool {
	return group.Status == StatusActive && !group.IsFallbackGroup
}

func isModelPricingPayAsYouGoGroup(group Group) bool {
	return group.SubscriptionType == "" || group.SubscriptionType == SubscriptionTypeStandard
}

func (i *modelCatalogItem) addPlatform(platform string) {
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return
	}
	if i.Platforms == nil {
		i.Platforms = map[string]struct{}{}
	}
	i.Platforms[platform] = struct{}{}
}

func (i *modelCatalogItem) PlatformDisplay() string {
	return strings.Join(i.SortedPlatforms(), ", ")
}

func (i *modelCatalogItem) SortedPlatforms() []string {
	return sortedStrings(i.Platforms)
}

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

func lowestGroupPrice(groups []modelCatalogGroup) ModelPricingAmount {
	var lowest ModelPricingAmount
	found := false
	for _, group := range groups {
		final := applyGroupMultiplier(group.Resolved, group.RateMultiplier)
		if !found || modelPricingAmountLess(final, lowest) {
			lowest = final
			found = true
		}
	}
	return lowest
}

func modelPricingAmountLess(a, b ModelPricingAmount) bool {
	aRank := modelPricingAmountRank(a)
	bRank := modelPricingAmountRank(b)
	for i := range aRank {
		if aRank[i] == bRank[i] {
			continue
		}
		return aRank[i] < bRank[i]
	}
	return false
}

func modelPricingAmountRank(amount ModelPricingAmount) []float64 {
	return []float64{
		positivePriceRank(amount.InputPrice),
		positivePriceRank(amount.OutputPrice),
		positivePriceRank(amount.CacheWritePrice),
		positivePriceRank(amount.CacheReadPrice),
		positivePriceRank(amount.PerRequestPrice),
		positivePriceRank(amount.ImageOutputPrice),
	}
}

func positivePriceRank(value float64) float64 {
	if value > 0 {
		return value
	}
	return 1 << 62
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
			if !wildcardPricingMatchesModel(pricing, item.ID) {
				continue
			}
			contributed := false
			for _, groupID := range channel.GroupIDs {
				group, ok := groupsByID[groupID]
				if !ok || group.Platform != pricing.Platform {
					continue
				}
				resolved := resolveDisplayPricing(*item.Official, pricing)
				if appendGroupIfMissing(&item.Groups, modelCatalogGroup{
					GroupID:        group.ID,
					GroupName:      group.Name,
					Platform:       pricing.Platform,
					Capabilities:   capabilitiesSet(pricing.Capabilities),
					RateMultiplier: normalizeRateMultiplier(group.RateMultiplier),
					BillingMode:    normalizeBillingMode(pricing.BillingMode),
					Resolved:       resolved,
				}) {
					contributed = true
				}
			}
			if contributed {
				item.addPlatform(pricing.Platform)
				for _, capability := range NormalizeModelCapabilities(pricing.Capabilities) {
					item.Capabilities[capability] = struct{}{}
				}
			}
		}
	}
}

func wildcardPricingMatchesModel(pricing ChannelModelPricing, model string) bool {
	for _, pattern := range pricing.Models {
		if isWildcardModelPattern(pattern) && modelPatternMatches(pattern, model) {
			return true
		}
	}
	return false
}

func appendGroupIfMissing(groups *[]modelCatalogGroup, next modelCatalogGroup) bool {
	for i := range *groups {
		if (*groups)[i].GroupID == next.GroupID {
			return false
		}
	}
	*groups = append(*groups, next)
	return true
}

func matchesModelPricingQuery(item *modelCatalogItem, query ModelPricingQuery) bool {
	if len(item.groupsMatchingFilters(query)) == 0 {
		return false
	}
	q := strings.ToLower(strings.TrimSpace(query.Q))
	if q == "" {
		return true
	}
	if strings.Contains(strings.ToLower(item.ID), q) || strings.Contains(strings.ToLower(displayModelName(item.ID)), q) || strings.Contains(strings.ToLower(item.PlatformDisplay()), q) {
		return true
	}
	for platform := range item.Platforms {
		if strings.Contains(strings.ToLower(platform), q) {
			return true
		}
	}
	for capability := range item.Capabilities {
		if strings.Contains(capability, q) {
			return true
		}
	}
	return false
}

func (i *modelCatalogItem) groupsMatchingFilters(query ModelPricingQuery) []modelCatalogGroup {
	groups := make([]modelCatalogGroup, 0, len(i.Groups))
	for _, group := range i.Groups {
		if group.matchesFilters(query) {
			groups = append(groups, group)
		}
	}
	return groups
}

func (g modelCatalogGroup) matchesFilters(query ModelPricingQuery) bool {
	if query.GroupID > 0 && g.GroupID != query.GroupID {
		return false
	}
	platform := strings.TrimSpace(query.Platform)
	if platform != "" && !strings.EqualFold(g.Platform, platform) {
		return false
	}
	capability := strings.ToLower(strings.TrimSpace(query.Capability))
	if capability != "" {
		_, ok := g.Capabilities[capability]
		return ok
	}
	return true
}

func capabilitiesSet(capabilities []string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, capability := range NormalizeModelCapabilities(capabilities) {
		set[capability] = struct{}{}
	}
	return set
}

func sortedStrings(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for item := range set {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
