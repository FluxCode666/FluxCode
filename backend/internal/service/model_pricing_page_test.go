package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type modelPricingChannelListerStub struct {
	channels []Channel
	pages    map[int][]Channel
	err      error
	calls    []pagination.PaginationParams
}

func (s *modelPricingChannelListerStub) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]Channel, *pagination.PaginationResult, error) {
	if s.err != nil {
		return nil, nil, s.err
	}
	s.calls = append(s.calls, params)
	if s.pages != nil {
		pageChannels := s.pages[params.Page]
		total := 0
		for _, channels := range s.pages {
			total += len(channels)
		}
		return pageChannels, &pagination.PaginationResult{
			Total:    int64(total),
			Page:     params.Page,
			PageSize: params.Limit(),
			Pages:    len(s.pages),
		}, nil
	}
	return s.channels, &pagination.PaginationResult{Total: int64(len(s.channels)), Page: params.Page, PageSize: params.Limit(), Pages: 1}, nil
}

type modelPricingGroupListerStub struct {
	groups []Group
	err    error
}

func (s *modelPricingGroupListerStub) ListActive(ctx context.Context) ([]Group, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.groups, nil
}

type modelPricingBillingStub struct {
	prices map[string]*ModelPricing
	errs   map[string]error
}

type modelPerformanceReaderStub struct {
	listCalls   []modelPerformanceSummaryQuery
	detailCalls []modelPerformanceDetailQuery
	listResult  map[string]ModelPerformanceMetrics
	detail      *ModelPerformanceDetail
	err         error
}

type modelPerformanceSummaryQuery struct {
	Window  ModelPerformanceWindow
	Models  []string
	GroupID *int64
}

type modelPerformanceDetailQuery struct {
	Window   ModelPerformanceWindow
	Model    string
	GroupIDs []int64
}

func (s *modelPerformanceReaderStub) ListModelPerformanceSummaries(ctx context.Context, window ModelPerformanceWindow, models []string, groupID *int64) (map[string]ModelPerformanceMetrics, error) {
	if s.err != nil {
		return nil, s.err
	}
	modelCopy := append([]string(nil), models...)
	var groupCopy *int64
	if groupID != nil {
		value := *groupID
		groupCopy = &value
	}
	s.listCalls = append(s.listCalls, modelPerformanceSummaryQuery{Window: window, Models: modelCopy, GroupID: groupCopy})
	return s.listResult, nil
}

func (s *modelPerformanceReaderStub) GetModelPerformanceDetail(ctx context.Context, window ModelPerformanceWindow, model string, groupIDs []int64) (*ModelPerformanceDetail, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.detailCalls = append(s.detailCalls, modelPerformanceDetailQuery{
		Window:   window,
		Model:    model,
		GroupIDs: append([]int64(nil), groupIDs...),
	})
	return s.detail, nil
}

func (s *modelPricingBillingStub) GetModelPricing(model string) (*ModelPricing, error) {
	if err := s.errs[model]; err != nil {
		return nil, err
	}
	price, ok := s.prices[model]
	if !ok {
		return nil, errors.New("missing model price")
	}
	cp := *price
	return &cp, nil
}

func TestModelPricingPageServiceListModelsAggregatesConcreteEnabledChannelModels(t *testing.T) {
	input := floatPtr(0.000003)
	output := floatPtr(0.000015)
	svc := NewModelPricingPageServiceForTest(
		&modelPricingChannelListerStub{channels: []Channel{{
			ID:       10,
			Status:   StatusActive,
			GroupIDs: []int64{1, 2, 3},
			ModelPricing: []ChannelModelPricing{{
				Platform:     "anthropic",
				Models:       []string{"claude-sonnet-4", "claude-*"},
				Capabilities: []string{"streaming"},
				BillingMode:  BillingModeToken,
				InputPrice:   input,
				OutputPrice:  output,
			}},
		}}},
		&modelPricingGroupListerStub{groups: []Group{
			{ID: 1, Name: "基础组", Platform: "anthropic", Status: StatusActive, RateMultiplier: 1.2},
			{ID: 2, Name: "禁用组", Platform: "anthropic", Status: StatusDisabled, RateMultiplier: 1.0},
			{ID: 3, Name: "兜底组", Platform: "anthropic", Status: StatusActive, IsFallbackGroup: true, RateMultiplier: 0.1},
		}},
		&modelPricingBillingStub{prices: map[string]*ModelPricing{
			"claude-sonnet-4": {
				InputPricePerToken:         0.000003,
				OutputPricePerToken:        0.000015,
				CacheCreationPricePerToken: 0.00000375,
				CacheReadPricePerToken:     0.0000003,
			},
		}},
	)

	models, err := svc.ListModels(context.Background(), ModelPricingQuery{})
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "claude-sonnet-4", models[0].ID)
	require.Equal(t, "anthropic", models[0].Platform)
	require.Equal(t, []string{"streaming"}, models[0].Capabilities)
	require.Equal(t, 1, models[0].SupportedGroupCount)
	require.Equal(t, 0.000003, models[0].OfficialPrice.InputPrice)
	require.InEpsilon(t, 0.0000036, models[0].LowestGroupPrice.InputPrice, 0.000001)
	require.InEpsilon(t, 0.000018, models[0].LowestGroupPrice.OutputPrice, 0.000001)
}

func TestModelPricingPageServiceListModelsUsesLowestGroupPrice(t *testing.T) {
	input := floatPtr(0.000006)
	output := floatPtr(0.000012)
	svc := NewModelPricingPageServiceForTest(
		&modelPricingChannelListerStub{channels: []Channel{{
			ID:       10,
			Status:   StatusActive,
			GroupIDs: []int64{1, 2},
			ModelPricing: []ChannelModelPricing{{
				Platform:     "anthropic",
				Models:       []string{"claude-sonnet-4"},
				Capabilities: []string{"streaming"},
				BillingMode:  BillingModeToken,
				InputPrice:   input,
				OutputPrice:  output,
			}},
		}}},
		&modelPricingGroupListerStub{groups: []Group{
			{ID: 1, Name: "专业组", Platform: "anthropic", Status: StatusActive, RateMultiplier: 2},
			{ID: 2, Name: "基础组", Platform: "anthropic", Status: StatusActive, RateMultiplier: 1},
		}},
		&modelPricingBillingStub{prices: map[string]*ModelPricing{
			"claude-sonnet-4": {
				InputPricePerToken:  0.000003,
				OutputPricePerToken: 0.000015,
			},
		}},
	)

	models, err := svc.ListModels(context.Background(), ModelPricingQuery{})
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, 2, models[0].SupportedGroupCount)
	require.Equal(t, 0.000003, models[0].OfficialPrice.InputPrice)
	require.Equal(t, 0.000006, models[0].LowestGroupPrice.InputPrice)
	require.Equal(t, 0.000012, models[0].LowestGroupPrice.OutputPrice)
}

func TestModelPricingPageServiceGetModelAppliesChannelOverrideAndGroupMultiplier(t *testing.T) {
	input := floatPtr(0.000006)
	svc := NewModelPricingPageServiceForTest(
		&modelPricingChannelListerStub{channels: []Channel{
			{
				ID:       10,
				Status:   StatusActive,
				GroupIDs: []int64{},
				ModelPricing: []ChannelModelPricing{{
					Platform:     "anthropic",
					Models:       []string{"claude-opus-4"},
					Capabilities: []string{"streaming"},
					BillingMode:  BillingModeToken,
				}},
			},
			{
				ID:       11,
				Status:   StatusActive,
				GroupIDs: []int64{1},
				ModelPricing: []ChannelModelPricing{{
					Platform:     "anthropic",
					Models:       []string{"claude-*"},
					Capabilities: []string{"tools"},
					BillingMode:  BillingModeToken,
					InputPrice:   input,
				}},
			},
		}},
		&modelPricingGroupListerStub{groups: []Group{
			{ID: 1, Name: "专业组", Platform: "anthropic", Status: StatusActive, RateMultiplier: 2},
			{ID: 2, Name: "未绑定组", Platform: "anthropic", Status: StatusActive, RateMultiplier: 1},
		}},
		&modelPricingBillingStub{prices: map[string]*ModelPricing{
			"claude-opus-4": {
				InputPricePerToken:  0.000003,
				OutputPricePerToken: 0.000015,
			},
		}},
	)

	detail, err := svc.GetModel(context.Background(), "claude-opus-4")
	require.NoError(t, err)
	require.Equal(t, "claude-opus-4", detail.ID)
	require.Equal(t, []string{"streaming", "tools"}, detail.Capabilities)
	require.Len(t, detail.Groups, 1)
	require.Equal(t, "专业组", detail.Groups[0].GroupName)
	require.Equal(t, 0.000012, detail.Groups[0].Price.InputPrice)
	require.Equal(t, 4.0, detail.Groups[0].Multipliers.InputPrice)
	require.Equal(t, 0.00003, detail.Groups[0].Price.OutputPrice)
	require.Equal(t, 2.0, detail.Groups[0].Multipliers.OutputPrice)
}

func TestModelPricingPageServiceGetModelKeepsExactPricingBeforeWildcard(t *testing.T) {
	exactInput := floatPtr(0.000006)
	wildcardInput := floatPtr(0.000009)
	svc := NewModelPricingPageServiceForTest(
		&modelPricingChannelListerStub{channels: []Channel{{
			ID:       10,
			Status:   StatusActive,
			GroupIDs: []int64{1},
			ModelPricing: []ChannelModelPricing{
				{
					Platform:     "anthropic",
					Models:       []string{"claude-opus-4"},
					Capabilities: []string{"streaming"},
					BillingMode:  BillingModeToken,
					InputPrice:   exactInput,
				},
				{
					Platform:     "anthropic",
					Models:       []string{"claude-*"},
					Capabilities: []string{"tools"},
					BillingMode:  BillingModeToken,
					InputPrice:   wildcardInput,
				},
			},
		}}},
		&modelPricingGroupListerStub{groups: []Group{
			{ID: 1, Name: "基础组", Platform: "anthropic", Status: StatusActive, RateMultiplier: 2},
		}},
		&modelPricingBillingStub{prices: map[string]*ModelPricing{
			"claude-opus-4": {
				InputPricePerToken:  0.000003,
				OutputPricePerToken: 0.000015,
			},
		}},
	)

	detail, err := svc.GetModel(context.Background(), "claude-opus-4")
	require.NoError(t, err)
	require.Len(t, detail.Groups, 1)
	require.Equal(t, "基础组", detail.Groups[0].GroupName)
	require.Equal(t, 0.000012, detail.Groups[0].Price.InputPrice)
	require.Equal(t, 4.0, detail.Groups[0].Multipliers.InputPrice)
	require.Equal(t, []string{"streaming"}, detail.Capabilities)
}

func TestModelPricingPageServiceListModelsFiltersSearchPlatformAndCapability(t *testing.T) {
	svc := NewModelPricingPageServiceForTest(
		&modelPricingChannelListerStub{channels: []Channel{{
			ID:       10,
			Status:   StatusActive,
			GroupIDs: []int64{1, 2},
			ModelPricing: []ChannelModelPricing{
				{Platform: "anthropic", Models: []string{"claude-sonnet-4"}, Capabilities: []string{"streaming"}, BillingMode: BillingModeToken},
				{Platform: "openai", Models: []string{"gpt-image-1"}, Capabilities: []string{"tools"}, BillingMode: BillingModeToken},
				{Platform: "embedding", Models: []string{"text-embedding-3-small"}, Capabilities: []string{"embedding"}, BillingMode: BillingModeToken},
			},
		}}},
		&modelPricingGroupListerStub{groups: []Group{
			{ID: 1, Name: "基础组", Platform: "anthropic", Status: StatusActive, RateMultiplier: 1},
			{ID: 2, Name: "嵌入组", Platform: "embedding", Status: StatusActive, RateMultiplier: 1},
		}},
		&modelPricingBillingStub{prices: map[string]*ModelPricing{
			"claude-sonnet-4":        {InputPricePerToken: 0.000003},
			"gpt-image-1":            {InputPricePerToken: 0.000001},
			"text-embedding-3-small": {InputPricePerToken: 0.00000002},
		}},
	)

	models, err := svc.ListModels(context.Background(), ModelPricingQuery{Q: "streaming", Platform: "anthropic", Capability: "streaming"})
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "claude-sonnet-4", models[0].ID)

	models, err = svc.ListModels(context.Background(), ModelPricingQuery{Platform: "embedding", Capability: "embedding"})
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "text-embedding-3-small", models[0].ID)
	require.Equal(t, []string{"embedding"}, models[0].Capabilities)
}

func TestModelPricingPageServiceListGroupsReturnsVisibleStandardGroupsOnly(t *testing.T) {
	svc := NewModelPricingPageServiceForTest(
		&modelPricingChannelListerStub{},
		&modelPricingGroupListerStub{groups: []Group{
			{ID: 1, Name: "基础组", Platform: "anthropic", Status: StatusActive, SubscriptionType: SubscriptionTypeStandard},
			{ID: 2, Name: "兼容旧数据组", Platform: "openai", Status: StatusActive},
			{ID: 3, Name: "订阅组", Platform: "anthropic", Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription},
			{ID: 4, Name: "禁用组", Platform: "anthropic", Status: StatusDisabled, SubscriptionType: SubscriptionTypeStandard},
			{ID: 5, Name: "兜底组", Platform: "anthropic", Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, IsFallbackGroup: true},
		}},
		&modelPricingBillingStub{},
	)

	groups, err := svc.ListGroups(context.Background())
	require.NoError(t, err)
	require.Equal(t, []ModelPricingGroupOption{
		{ID: 1, Name: "基础组", Platform: "anthropic"},
		{ID: 2, Name: "兼容旧数据组", Platform: "openai"},
	}, groups)
}

func TestModelPricingPageServiceListModelsFiltersByGroupID(t *testing.T) {
	svc := NewModelPricingPageServiceForTest(
		&modelPricingChannelListerStub{channels: []Channel{
			{
				ID:       10,
				Status:   StatusActive,
				GroupIDs: []int64{1},
				ModelPricing: []ChannelModelPricing{{
					Platform:    "anthropic",
					Models:      []string{"claude-sonnet-4"},
					BillingMode: BillingModeToken,
				}},
			},
			{
				ID:       11,
				Status:   StatusActive,
				GroupIDs: []int64{2},
				ModelPricing: []ChannelModelPricing{{
					Platform:    "openai",
					Models:      []string{"gpt-4.1"},
					BillingMode: BillingModeToken,
				}},
			},
		}},
		&modelPricingGroupListerStub{groups: []Group{
			{ID: 1, Name: "Anthropic 组", Platform: "anthropic", Status: StatusActive, SubscriptionType: SubscriptionTypeStandard},
			{ID: 2, Name: "OpenAI 组", Platform: "openai", Status: StatusActive, SubscriptionType: SubscriptionTypeStandard},
		}},
		&modelPricingBillingStub{prices: map[string]*ModelPricing{
			"claude-sonnet-4": {InputPricePerToken: 0.000003},
			"gpt-4.1":         {InputPricePerToken: 0.000002},
		}},
	)

	models, err := svc.ListModels(context.Background(), ModelPricingQuery{GroupID: 2})
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "gpt-4.1", models[0].ID)
}

func TestModelPricingPageServiceListModelsUsesFilteredPriceScope(t *testing.T) {
	streamingInput := floatPtr(0.000001)
	toolsInput := floatPtr(0.000010)
	visionInput := floatPtr(0.000005)
	svc := NewModelPricingPageServiceForTest(
		&modelPricingChannelListerStub{channels: []Channel{
			{
				ID:       10,
				Status:   StatusActive,
				GroupIDs: []int64{1},
				ModelPricing: []ChannelModelPricing{{
					Platform:     "anthropic",
					Models:       []string{"claude-sonnet-4"},
					Capabilities: []string{"streaming"},
					BillingMode:  BillingModeToken,
					InputPrice:   streamingInput,
				}},
			},
			{
				ID:       11,
				Status:   StatusActive,
				GroupIDs: []int64{2},
				ModelPricing: []ChannelModelPricing{{
					Platform:     "openrouter",
					Models:       []string{"claude-sonnet-4"},
					Capabilities: []string{"tools"},
					BillingMode:  BillingModeToken,
					InputPrice:   toolsInput,
				}},
			},
			{
				ID:       12,
				Status:   StatusActive,
				GroupIDs: []int64{3},
				ModelPricing: []ChannelModelPricing{{
					Platform:     "anthropic",
					Models:       []string{"claude-sonnet-4"},
					Capabilities: []string{"vision"},
					BillingMode:  BillingModeToken,
					InputPrice:   visionInput,
				}},
			},
		}},
		&modelPricingGroupListerStub{groups: []Group{
			{ID: 1, Name: "Anthropic 流式组", Platform: "anthropic", Status: StatusActive, SubscriptionType: SubscriptionTypeStandard},
			{ID: 2, Name: "OpenRouter 工具组", Platform: "openrouter", Status: StatusActive, SubscriptionType: SubscriptionTypeStandard},
			{ID: 3, Name: "Anthropic 视觉组", Platform: "anthropic", Status: StatusActive, SubscriptionType: SubscriptionTypeStandard},
		}},
		&modelPricingBillingStub{prices: map[string]*ModelPricing{
			"claude-sonnet-4": {InputPricePerToken: 0.000003},
		}},
	)

	models, err := svc.ListModels(context.Background(), ModelPricingQuery{})
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, 0.000001, models[0].LowestGroupPrice.InputPrice)

	models, err = svc.ListModels(context.Background(), ModelPricingQuery{GroupID: 2})
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, 0.000010, models[0].LowestGroupPrice.InputPrice)

	models, err = svc.ListModels(context.Background(), ModelPricingQuery{Platform: "openrouter"})
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, 0.000010, models[0].LowestGroupPrice.InputPrice)

	models, err = svc.ListModels(context.Background(), ModelPricingQuery{Capability: "tools"})
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, 0.000010, models[0].LowestGroupPrice.InputPrice)

	models, err = svc.ListModels(context.Background(), ModelPricingQuery{Platform: "anthropic", Capability: "vision"})
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, 0.000005, models[0].LowestGroupPrice.InputPrice)

	models, err = svc.ListModels(context.Background(), ModelPricingQuery{Platform: "openrouter", Capability: "streaming"})
	require.NoError(t, err)
	require.Empty(t, models)
}

func TestModelPricingPageServiceListModelsAggregatesAcrossChannelPages(t *testing.T) {
	channels := &modelPricingChannelListerStub{pages: map[int][]Channel{
		1: {{
			ID:       1,
			Status:   StatusActive,
			GroupIDs: []int64{1},
			ModelPricing: []ChannelModelPricing{{
				Platform:     "anthropic",
				Models:       []string{"claude-sonnet-4"},
				Capabilities: []string{"streaming"},
				BillingMode:  BillingModeToken,
			}},
		}},
		2: {{
			ID:       2,
			Status:   StatusActive,
			GroupIDs: []int64{2},
			ModelPricing: []ChannelModelPricing{{
				Platform:     "anthropic",
				Models:       []string{"claude-3-7-sonnet"},
				Capabilities: []string{"streaming"},
				BillingMode:  BillingModeToken,
			}},
		}},
	}}
	svc := NewModelPricingPageServiceForTest(
		channels,
		&modelPricingGroupListerStub{groups: []Group{
			{ID: 1, Name: "基础组", Platform: "anthropic", Status: StatusActive, RateMultiplier: 1},
			{ID: 2, Name: "专业组", Platform: "anthropic", Status: StatusActive, RateMultiplier: 1},
		}},
		&modelPricingBillingStub{prices: map[string]*ModelPricing{
			"claude-sonnet-4":   {InputPricePerToken: 0.000003},
			"claude-3-7-sonnet": {InputPricePerToken: 0.000004},
		}},
	)

	models, err := svc.ListModels(context.Background(), ModelPricingQuery{})
	require.NoError(t, err)
	require.Len(t, models, 2)
	require.Equal(t, []int{1, 2}, []int{channels.calls[0].Page, channels.calls[1].Page})
	require.Equal(t, 1000, channels.calls[0].Limit())
	require.ElementsMatch(t, []string{"claude-sonnet-4", "claude-3-7-sonnet"}, []string{models[0].ID, models[1].ID})
}

func TestModelPricingPageServiceAggregatesSharedModelAcrossPlatforms(t *testing.T) {
	svc := NewModelPricingPageServiceForTest(
		&modelPricingChannelListerStub{channels: []Channel{
			{
				ID:       10,
				Status:   StatusActive,
				GroupIDs: []int64{1},
				ModelPricing: []ChannelModelPricing{{
					Platform:     "anthropic",
					Models:       []string{"claude-sonnet-4"},
					Capabilities: []string{"streaming"},
					BillingMode:  BillingModeToken,
				}},
			},
			{
				ID:       11,
				Status:   StatusActive,
				GroupIDs: []int64{2},
				ModelPricing: []ChannelModelPricing{{
					Platform:        "openrouter",
					Models:          []string{"claude-sonnet-4"},
					Capabilities:    []string{"tools"},
					BillingMode:     BillingModePerRequest,
					PerRequestPrice: floatPtr(0.02),
				}},
			},
		}},
		&modelPricingGroupListerStub{groups: []Group{
			{ID: 1, Name: "Anthropic 组", Platform: "anthropic", Status: StatusActive, RateMultiplier: 1},
			{ID: 2, Name: "OpenRouter 组", Platform: "openrouter", Status: StatusActive, RateMultiplier: 1.5},
		}},
		&modelPricingBillingStub{prices: map[string]*ModelPricing{
			"claude-sonnet-4": {
				InputPricePerToken:  0.000003,
				OutputPricePerToken: 0.000015,
			},
		}},
	)

	models, err := svc.ListModels(context.Background(), ModelPricingQuery{Platform: "openrouter"})
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "anthropic, openrouter", models[0].Platform)
	require.ElementsMatch(t, []string{"streaming", "tools"}, models[0].Capabilities)

	detail, err := svc.GetModel(context.Background(), "claude-sonnet-4")
	require.NoError(t, err)
	require.Equal(t, "anthropic, openrouter", detail.Platform)
	require.ElementsMatch(t, []string{"streaming", "tools"}, detail.Capabilities)
	require.Len(t, detail.Groups, 2)
	require.Equal(t, "Anthropic 组", detail.Groups[0].GroupName)
	require.Equal(t, "OpenRouter 组", detail.Groups[1].GroupName)
	require.Equal(t, BillingModePerRequest, BillingMode(detail.Groups[1].BillingMode))
	require.Equal(t, 0.03, detail.Groups[1].Price.PerRequestPrice)
}

func TestResolveModelPerformanceWindowUsesCompleteUTCHours(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 3, 45, 0, time.FixedZone("CST", 8*60*60))

	window, err := ResolveModelPerformanceWindow(now, ModelPerformanceRange24Hours)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, time.July, 20, 3, 0, 0, 0, time.UTC), window.End)
	require.Equal(t, 24*time.Hour, window.End.Sub(window.Start))

	weekly, err := ResolveModelPerformanceWindow(now, ModelPerformanceRange7Days)
	require.NoError(t, err)
	require.Equal(t, 7*24*time.Hour, weekly.End.Sub(weekly.Start))

	_, err = ResolveModelPerformanceWindow(now, ModelPerformanceRange("30d"))
	require.Error(t, err)
}

func TestModelPricingPageServiceAttachesSelectedGroupPerformanceToCards(t *testing.T) {
	groupID := int64(2)
	reader := &modelPerformanceReaderStub{listResult: map[string]ModelPerformanceMetrics{
		"claude-sonnet-4": {
			TPS:                floatPtr(11.5),
			Availability:       floatPtr(98.5),
			AverageFirstToken:  floatPtr(245),
			AverageRequestTime: floatPtr(800),
		},
	}}
	svc := newModelPricingPageServiceWithPerformanceForTest(
		&modelPricingChannelListerStub{channels: []Channel{{
			ID:       10,
			Status:   StatusActive,
			GroupIDs: []int64{1, 2},
			ModelPricing: []ChannelModelPricing{{
				Platform: "anthropic", Models: []string{"claude-sonnet-4"}, BillingMode: BillingModeToken,
			}},
		}}},
		&modelPricingGroupListerStub{groups: []Group{
			{ID: 1, Name: "基础组", Platform: "anthropic", Status: StatusActive},
			{ID: 2, Name: "专业组", Platform: "anthropic", Status: StatusActive},
		}},
		&modelPricingBillingStub{prices: map[string]*ModelPricing{"claude-sonnet-4": {}}},
		reader,
	)
	svc.now = func() time.Time { return time.Date(2026, time.July, 20, 12, 30, 0, 0, time.UTC) }

	models, err := svc.ListModels(context.Background(), ModelPricingQuery{GroupID: groupID, PerformanceRange: ModelPerformanceRange7Days})
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, 11.5, *models[0].Performance.TPS)
	require.Equal(t, 245.0, *models[0].Performance.AverageFirstToken)
	require.Len(t, reader.listCalls, 1)
	require.Equal(t, &groupID, reader.listCalls[0].GroupID)
	require.Equal(t, 7*24*time.Hour, reader.listCalls[0].Window.End.Sub(reader.listCalls[0].Window.Start))
	require.Equal(t, []string{"claude-sonnet-4"}, reader.listCalls[0].Models)
}

func TestModelPricingPageServiceDetailKeepsOverallPerformanceAndReturnsNullWithoutSamples(t *testing.T) {
	reader := &modelPerformanceReaderStub{detail: &ModelPerformanceDetail{
		Overall: ModelPerformanceMetrics{TPS: floatPtr(9), Availability: floatPtr(99), AverageFirstToken: floatPtr(320), AverageRequestTime: floatPtr(1200)},
		Groups: map[int64]ModelPerformanceMetrics{
			1: {TPS: floatPtr(7), Availability: floatPtr(97), AverageFirstToken: floatPtr(400), AverageRequestTime: floatPtr(1500)},
		},
		Trend: []ModelPerformanceHourlyTrendPoint{{
			BucketStart:       time.Date(2026, time.July, 20, 3, 0, 0, 0, time.UTC),
			Availability:      floatPtr(99),
			AverageFirstToken: floatPtr(320),
		}},
	}}
	svc := newModelPricingPageServiceWithPerformanceForTest(
		&modelPricingChannelListerStub{channels: []Channel{{
			ID:       10,
			Status:   StatusActive,
			GroupIDs: []int64{1, 2},
			ModelPricing: []ChannelModelPricing{{
				Platform: "anthropic", Models: []string{"claude-sonnet-4"}, BillingMode: BillingModeToken,
			}},
		}}},
		&modelPricingGroupListerStub{groups: []Group{
			{ID: 1, Name: "基础组", Platform: "anthropic", Status: StatusActive},
			{ID: 2, Name: "专业组", Platform: "anthropic", Status: StatusActive},
		}},
		&modelPricingBillingStub{prices: map[string]*ModelPricing{"claude-sonnet-4": {}}},
		reader,
	)
	svc.now = func() time.Time { return time.Date(2026, time.July, 20, 12, 30, 0, 0, time.UTC) }

	detail, err := svc.GetModelWithRange(context.Background(), "claude-sonnet-4", ModelPerformanceRange24Hours)
	require.NoError(t, err)
	require.Equal(t, 9.0, *detail.Performance.TPS)
	groupMetrics := map[int64]ModelPerformanceMetrics{}
	for _, group := range detail.Groups {
		groupMetrics[group.GroupID] = group.Performance
	}
	require.Equal(t, 7.0, *groupMetrics[1].TPS)
	require.Nil(t, groupMetrics[2].TPS)
	require.Len(t, detail.PerformanceTrend, 1)
	require.Equal(t, time.UTC, detail.PerformanceTrend[0].BucketStart.Location())
	require.Len(t, reader.detailCalls, 1)
	require.Equal(t, "claude-sonnet-4", reader.detailCalls[0].Model)
	require.Equal(t, []int64{1, 2}, reader.detailCalls[0].GroupIDs)
}

func floatPtr(v float64) *float64 { return &v }
