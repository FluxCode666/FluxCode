package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	handlerpkg "github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type singleModelPricingChannels struct{}

func (s *singleModelPricingChannels) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]service.Channel, *pagination.PaginationResult, error) {
	return []service.Channel{{
		ID:       1,
		Status:   service.StatusActive,
		GroupIDs: []int64{1},
		ModelPricing: []service.ChannelModelPricing{{
			Platform:     "anthropic",
			Models:       []string{"claude-sonnet-4"},
			Capabilities: []string{"chat"},
			BillingMode:  service.BillingModeToken,
		}},
	}}, &pagination.PaginationResult{Total: 1}, nil
}

type singleModelPricingGroups struct{}

func (s *singleModelPricingGroups) ListActive(ctx context.Context) ([]service.Group, error) {
	return []service.Group{{
		ID:             1,
		Name:           "基础组",
		Platform:       "anthropic",
		Status:         service.StatusActive,
		RateMultiplier: 1,
	}}, nil
}

type slashModelPricingGroups struct{}

func (s *slashModelPricingGroups) ListActive(ctx context.Context) ([]service.Group, error) {
	return []service.Group{{
		ID:             1,
		Name:           "OpenRouter 组",
		Platform:       "openrouter",
		Status:         service.StatusActive,
		RateMultiplier: 1,
	}}, nil
}

type singleModelPricingBilling struct{}

func (s *singleModelPricingBilling) GetModelPricing(model string) (*service.ModelPricing, error) {
	return &service.ModelPricing{
		InputPricePerToken: 0.000003,
	}, nil
}

type slashModelPricingChannels struct{}

func (s *slashModelPricingChannels) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]service.Channel, *pagination.PaginationResult, error) {
	return []service.Channel{{
		ID:       1,
		Status:   service.StatusActive,
		GroupIDs: []int64{1},
		ModelPricing: []service.ChannelModelPricing{{
			Platform:     "openrouter",
			Models:       []string{"openai/gpt-4.1"},
			Capabilities: []string{"chat"},
			BillingMode:  service.BillingModeToken,
		}},
	}}, &pagination.PaginationResult{Total: 1}, nil
}

func TestModelPricingRoutesPublicModelsPathIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	RegisterModelPricingRoutes(v1, &handlerpkg.Handlers{
		ModelPricing: handlerpkg.NewModelPricingHandler(service.NewModelPricingPageServiceForTest(
			&singleModelPricingChannels{},
			&singleModelPricingGroups{},
			&singleModelPricingBilling{},
		)),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/model-pricing/models", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestModelPricingRoutesPublicModelQueryPathSupportsSlashIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	RegisterModelPricingRoutes(v1, &handlerpkg.Handlers{
		ModelPricing: handlerpkg.NewModelPricingHandler(service.NewModelPricingPageServiceForTest(
			&slashModelPricingChannels{},
			&slashModelPricingGroups{},
			&singleModelPricingBilling{},
		)),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/model-pricing/model?model=openai%2Fgpt-4.1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data service.ModelPricingModelDetail `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "openai/gpt-4.1", body.Data.ID)
}
