package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	handlerpkg "github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type pricingPlanRoutesRepoStub struct {
	listGroupsWithPlansFn func(ctx context.Context, onlyActive bool) ([]service.PricingPlanGroup, error)
}

func (s *pricingPlanRoutesRepoStub) ListGroupsWithPlans(ctx context.Context, onlyActive bool) ([]service.PricingPlanGroup, error) {
	if s.listGroupsWithPlansFn == nil {
		return nil, nil
	}
	return s.listGroupsWithPlansFn(ctx, onlyActive)
}

func (s *pricingPlanRoutesRepoStub) GetGroupByID(ctx context.Context, id int64) (*service.PricingPlanGroup, error) {
	return nil, nil
}

func (s *pricingPlanRoutesRepoStub) CreateGroup(ctx context.Context, group *service.PricingPlanGroup) error {
	return nil
}

func (s *pricingPlanRoutesRepoStub) UpdateGroup(ctx context.Context, group *service.PricingPlanGroup) error {
	return nil
}

func (s *pricingPlanRoutesRepoStub) DeleteGroup(ctx context.Context, id int64) error {
	return nil
}

func (s *pricingPlanRoutesRepoStub) GetPlanByID(ctx context.Context, id int64) (*service.PricingPlan, error) {
	return nil, nil
}

func (s *pricingPlanRoutesRepoStub) CreatePlan(ctx context.Context, plan *service.PricingPlan) error {
	return nil
}

func (s *pricingPlanRoutesRepoStub) UpdatePlan(ctx context.Context, plan *service.PricingPlan) error {
	return nil
}

func (s *pricingPlanRoutesRepoStub) DeletePlan(ctx context.Context, id int64) error {
	return nil
}

func newPricingPlanHandlersForRoutes(t *testing.T) *handlerpkg.Handlers {
	t.Helper()

	svc := service.NewPricingPlanService(&pricingPlanRoutesRepoStub{
		listGroupsWithPlansFn: func(ctx context.Context, onlyActive bool) ([]service.PricingPlanGroup, error) {
			if onlyActive {
				return []service.PricingPlanGroup{{
					ID:        1,
					Name:      "Starter",
					SortOrder: 1,
					Status:    service.StatusActive,
					Plans: []service.PricingPlan{{
						ID:            10,
						GroupID:       1,
						Name:          "Monthly",
						PriceCurrency: "CNY",
						PricePeriod:   "month",
						Status:        service.StatusActive,
					}},
				}}, nil
			}

			return []service.PricingPlanGroup{{
				ID:        1,
				Name:      "Starter",
				SortOrder: 1,
				Status:    service.StatusActive,
				Plans: []service.PricingPlan{{
					ID:            10,
					GroupID:       1,
					Name:          "Monthly",
					PriceCurrency: "CNY",
					PricePeriod:   "month",
					Status:        service.StatusActive,
				}},
			}}, nil
		},
	})

	return &handlerpkg.Handlers{
		PricingPlan: handlerpkg.NewPricingPlanHandler(svc),
		Admin: &handlerpkg.AdminHandlers{
			PricingPlan: adminhandler.NewPricingPlanHandler(svc),
		},
	}
}

func TestPricingPlanRoutes_PublicPlanGroupsPathIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	RegisterPricingPlanRoutes(v1, newPricingPlanHandlersForRoutes(t))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pricing/plan-groups", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"name":"Starter"`)
}

func TestPricingPlanRoutes_AdminPlanGroupsPathIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminGroup := router.Group("/api/v1/admin")
	registerPricingPlanRoutes(adminGroup, newPricingPlanHandlersForRoutes(t))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/pricing/plan-groups", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"price_currency":"CNY"`)
}
