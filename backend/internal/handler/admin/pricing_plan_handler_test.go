package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type pricingPlanAdminRepoStub struct {
	listGroupsWithPlansFn func(ctx context.Context, onlyActive bool) ([]service.PricingPlanGroup, error)
}

func (s *pricingPlanAdminRepoStub) ListGroupsWithPlans(ctx context.Context, onlyActive bool) ([]service.PricingPlanGroup, error) {
	if s.listGroupsWithPlansFn == nil {
		return nil, nil
	}
	return s.listGroupsWithPlansFn(ctx, onlyActive)
}

func (s *pricingPlanAdminRepoStub) GetGroupByID(ctx context.Context, id int64) (*service.PricingPlanGroup, error) {
	return nil, nil
}

func (s *pricingPlanAdminRepoStub) CreateGroup(ctx context.Context, group *service.PricingPlanGroup) error {
	return nil
}

func (s *pricingPlanAdminRepoStub) UpdateGroup(ctx context.Context, group *service.PricingPlanGroup) error {
	return nil
}

func (s *pricingPlanAdminRepoStub) DeleteGroup(ctx context.Context, id int64) error {
	return nil
}

func (s *pricingPlanAdminRepoStub) GetPlanByID(ctx context.Context, id int64) (*service.PricingPlan, error) {
	return nil, nil
}

func (s *pricingPlanAdminRepoStub) CreatePlan(ctx context.Context, plan *service.PricingPlan) error {
	return nil
}

func (s *pricingPlanAdminRepoStub) UpdatePlan(ctx context.Context, plan *service.PricingPlan) error {
	return nil
}

func (s *pricingPlanAdminRepoStub) DeletePlan(ctx context.Context, id int64) error {
	return nil
}

func TestPricingPlanHandler_ListGroups_ReturnsDTO(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := service.NewPricingPlanService(&pricingPlanAdminRepoStub{
		listGroupsWithPlansFn: func(ctx context.Context, onlyActive bool) ([]service.PricingPlanGroup, error) {
			require.False(t, onlyActive)
			return []service.PricingPlanGroup{
				{
					ID:        1,
					Name:      "Starter",
					SortOrder: 1,
					Status:    service.StatusActive,
					Plans: []service.PricingPlan{
						{
							ID:            10,
							GroupID:       1,
							Name:          "Monthly",
							PriceCurrency: "CNY",
							PricePeriod:   "month",
							Status:        service.StatusActive,
						},
					},
				},
			}, nil
		},
	})

	router := gin.New()
	router.GET("/admin/pricing/plan-groups", NewPricingPlanHandler(svc).ListGroups)

	req := httptest.NewRequest(http.MethodGet, "/admin/pricing/plan-groups", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"name":"Starter"`)
	require.Contains(t, rec.Body.String(), `"price_currency":"CNY"`)
}
