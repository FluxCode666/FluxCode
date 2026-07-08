package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type modelPricingChannelListerStub struct {
	channels []Channel
	err      error
}

func (s *modelPricingChannelListerStub) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]Channel, *pagination.PaginationResult, error) {
	if s.err != nil {
		return nil, nil, s.err
	}
	return s.channels, &pagination.PaginationResult{Total: int64(len(s.channels))}, nil
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
			GroupIDs: []int64{1, 2},
			ModelPricing: []ChannelModelPricing{{
				Platform:     "anthropic",
				Models:       []string{"claude-sonnet-4", "claude-*"},
				Capabilities: []string{"chat"},
				BillingMode:  BillingModeToken,
				InputPrice:   input,
				OutputPrice:  output,
			}},
		}}},
		&modelPricingGroupListerStub{groups: []Group{
			{ID: 1, Name: "基础组", Platform: "anthropic", Status: StatusActive, RateMultiplier: 1.2},
			{ID: 2, Name: "禁用组", Platform: "anthropic", Status: StatusDisabled, RateMultiplier: 1.0},
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
	require.Equal(t, []string{"chat"}, models[0].Capabilities)
	require.Equal(t, 1, models[0].SupportedGroupCount)
	require.Equal(t, 0.000003, models[0].OfficialPrice.InputPrice)
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
					Capabilities: []string{"chat"},
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
					Capabilities: []string{"image"},
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
	require.Equal(t, []string{"chat", "image"}, detail.Capabilities)
	require.Len(t, detail.Groups, 1)
	require.Equal(t, "专业组", detail.Groups[0].GroupName)
	require.Equal(t, 0.000012, detail.Groups[0].Price.InputPrice)
	require.Equal(t, 4.0, detail.Groups[0].Multipliers.InputPrice)
	require.Equal(t, 0.00003, detail.Groups[0].Price.OutputPrice)
	require.Equal(t, 2.0, detail.Groups[0].Multipliers.OutputPrice)
}

func TestModelPricingPageServiceListModelsFiltersSearchPlatformAndCapability(t *testing.T) {
	svc := NewModelPricingPageServiceForTest(
		&modelPricingChannelListerStub{channels: []Channel{{
			ID:       10,
			Status:   StatusActive,
			GroupIDs: []int64{1},
			ModelPricing: []ChannelModelPricing{
				{Platform: "anthropic", Models: []string{"claude-sonnet-4"}, Capabilities: []string{"chat"}, BillingMode: BillingModeToken},
				{Platform: "openai", Models: []string{"gpt-image-1"}, Capabilities: []string{"image"}, BillingMode: BillingModeToken},
			},
		}}},
		&modelPricingGroupListerStub{groups: []Group{
			{ID: 1, Name: "基础组", Platform: "anthropic", Status: StatusActive, RateMultiplier: 1},
		}},
		&modelPricingBillingStub{prices: map[string]*ModelPricing{
			"claude-sonnet-4": {InputPricePerToken: 0.000003},
			"gpt-image-1":     {InputPricePerToken: 0.000001},
		}},
	)

	models, err := svc.ListModels(context.Background(), ModelPricingQuery{Q: "chat", Platform: "anthropic", Capability: "chat"})
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "claude-sonnet-4", models[0].ID)
}

func floatPtr(v float64) *float64 { return &v }
