package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type failingModelPricingChannels struct{}

func (f *failingModelPricingChannels) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]service.Channel, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("boom")
}

type emptyModelPricingGroups struct{}

func (e *emptyModelPricingGroups) ListActive(ctx context.Context) ([]service.Group, error) {
	return nil, nil
}

type emptyModelPricingBilling struct{}

func (e *emptyModelPricingBilling) GetModelPricing(model string) (*service.ModelPricing, error) {
	return nil, errors.New("missing")
}

type singleModelPricingChannels struct{}

func (s *singleModelPricingChannels) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]service.Channel, *pagination.PaginationResult, error) {
	return []service.Channel{{
		ID:       1,
		Status:   service.StatusActive,
		GroupIDs: []int64{2},
		ModelPricing: []service.ChannelModelPricing{{
			Platform:     "anthropic",
			Models:       []string{"claude-sonnet-4"},
			Capabilities: []string{"streaming"},
			BillingMode:  service.BillingModeToken,
		}},
	}}, &pagination.PaginationResult{Total: 1}, nil
}

type slashModelPricingChannels struct{}

func (s *slashModelPricingChannels) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]service.Channel, *pagination.PaginationResult, error) {
	return []service.Channel{{
		ID:       1,
		Status:   service.StatusActive,
		GroupIDs: []int64{2},
		ModelPricing: []service.ChannelModelPricing{{
			Platform:     "openrouter",
			Models:       []string{"openai/gpt-4.1"},
			Capabilities: []string{"streaming"},
			BillingMode:  service.BillingModeToken,
		}},
	}}, &pagination.PaginationResult{Total: 1}, nil
}

type singleModelPricingGroups struct{}

func (s *singleModelPricingGroups) ListActive(ctx context.Context) ([]service.Group, error) {
	return []service.Group{{
		ID:             2,
		Name:           "基础组",
		Platform:       "anthropic",
		Status:         service.StatusActive,
		RateMultiplier: 1,
	}}, nil
}

type slashModelPricingGroups struct{}

func (s *slashModelPricingGroups) ListActive(ctx context.Context) ([]service.Group, error) {
	return []service.Group{{
		ID:             2,
		Name:           "OpenRouter 组",
		Platform:       "openrouter",
		Status:         service.StatusActive,
		RateMultiplier: 1,
	}}, nil
}

type singleModelPricingBilling struct{}

func (s *singleModelPricingBilling) GetModelPricing(model string) (*service.ModelPricing, error) {
	return &service.ModelPricing{
		InputPricePerToken:  0.000003,
		OutputPricePerToken: 0.000015,
	}, nil
}

func TestModelPricingHandlerListModelsReturnsQueryError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModelPricingHandler(service.NewModelPricingPageServiceForTest(
		&failingModelPricingChannels{},
		&emptyModelPricingGroups{},
		&emptyModelPricingBilling{},
	))
	router := gin.New()
	router.GET("/api/v1/model-pricing/models", h.ListModels)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/model-pricing/models?q=claude", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Contains(t, rec.Body.String(), "MODEL_PRICING_QUERY_FAILED")
	require.Contains(t, rec.Body.String(), "模型定价查询失败")
}

func TestModelPricingHandlerRejectsInvalidPerformanceRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModelPricingHandler(service.NewModelPricingPageServiceForTest(
		&singleModelPricingChannels{},
		&singleModelPricingGroups{},
		&singleModelPricingBilling{},
	))
	router := gin.New()
	router.GET("/api/v1/model-pricing/models", h.ListModels)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/model-pricing/models?range=30d", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "MODEL_PERFORMANCE_RANGE_INVALID")
}

func TestModelPricingHandlerGetModelReturnsDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModelPricingHandler(service.NewModelPricingPageServiceForTest(
		&singleModelPricingChannels{},
		&singleModelPricingGroups{},
		&singleModelPricingBilling{},
	))
	router := gin.New()
	router.GET("/api/v1/model-pricing/models/:model", h.GetModel)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/model-pricing/models/claude-sonnet-4", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data service.ModelPricingModelDetail `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "claude-sonnet-4", body.Data.ID)
	require.Len(t, body.Data.Groups, 1)
}

func TestModelPricingHandlerGetModelReturnsLocalizedNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModelPricingHandler(service.NewModelPricingPageServiceForTest(
		&singleModelPricingChannels{},
		&singleModelPricingGroups{},
		&singleModelPricingBilling{},
	))
	router := gin.New()
	router.GET("/api/v1/model-pricing/models/:model", h.GetModel)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/model-pricing/models/claude-opus-4", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "MODEL_PRICING_NOT_FOUND")
	require.Contains(t, rec.Body.String(), "模型定价不存在")
}

func TestModelPricingHandlerGetModelSupportsQueryParamSlashIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModelPricingHandler(service.NewModelPricingPageServiceForTest(
		&slashModelPricingChannels{},
		&slashModelPricingGroups{},
		&singleModelPricingBilling{},
	))
	router := gin.New()
	router.GET("/api/v1/model-pricing/model", h.GetModel)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/model-pricing/model?model=openai%2Fgpt-4.1", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data service.ModelPricingModelDetail `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "openai/gpt-4.1", body.Data.ID)
	require.Equal(t, "openrouter", body.Data.Platform)
}

func TestModelPricingHandlerGetModelAcceptsSevenDayPerformanceRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModelPricingHandler(service.NewModelPricingPageServiceForTest(
		&slashModelPricingChannels{},
		&slashModelPricingGroups{},
		&singleModelPricingBilling{},
	))
	router := gin.New()
	router.GET("/api/v1/model-pricing/model", h.GetModel)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/model-pricing/model?model=openai%2Fgpt-4.1&range=7d", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}
