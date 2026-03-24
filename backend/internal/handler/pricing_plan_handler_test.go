package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type pricingPlanHandlerRepoStub struct {
	listGroupsWithPlansFn func(ctx context.Context, onlyActive bool) ([]service.PricingPlanGroup, error)
}

func (s *pricingPlanHandlerRepoStub) ListGroupsWithPlans(ctx context.Context, onlyActive bool) ([]service.PricingPlanGroup, error) {
	if s.listGroupsWithPlansFn == nil {
		return nil, nil
	}
	return s.listGroupsWithPlansFn(ctx, onlyActive)
}

func (s *pricingPlanHandlerRepoStub) GetGroupByID(ctx context.Context, id int64) (*service.PricingPlanGroup, error) {
	return nil, nil
}

func (s *pricingPlanHandlerRepoStub) CreateGroup(ctx context.Context, group *service.PricingPlanGroup) error {
	return nil
}

func (s *pricingPlanHandlerRepoStub) UpdateGroup(ctx context.Context, group *service.PricingPlanGroup) error {
	return nil
}

func (s *pricingPlanHandlerRepoStub) DeleteGroup(ctx context.Context, id int64) error {
	return nil
}

func (s *pricingPlanHandlerRepoStub) GetPlanByID(ctx context.Context, id int64) (*service.PricingPlan, error) {
	return nil, nil
}

func (s *pricingPlanHandlerRepoStub) CreatePlan(ctx context.Context, plan *service.PricingPlan) error {
	return nil
}

func (s *pricingPlanHandlerRepoStub) UpdatePlan(ctx context.Context, plan *service.PricingPlan) error {
	return nil
}

func (s *pricingPlanHandlerRepoStub) DeletePlan(ctx context.Context, id int64) error {
	return nil
}

func TestPublicPricingPlanHandler_ListPublicGroups_ReturnsDTO(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := service.NewPricingPlanService(&pricingPlanHandlerRepoStub{
		listGroupsWithPlansFn: func(ctx context.Context, onlyActive bool) ([]service.PricingPlanGroup, error) {
			require.True(t, onlyActive)
			return []service.PricingPlanGroup{
				{
					ID:        1,
					Name:      "Starter",
					SortOrder: 1,
					Status:    service.StatusActive,
					Plans: []service.PricingPlan{
						{
							ID:             10,
							GroupID:        1,
							Name:           "Monthly",
							PriceCurrency:  "CNY",
							PricePeriod:    "month",
							Status:         service.StatusActive,
							ContactMethods: []service.PricingPlanContactMethod{{Type: "telegram", Value: "@flux"}},
						},
					},
				},
			}, nil
		},
	})

	router := gin.New()
	router.GET("/pricing/plan-groups", NewPricingPlanHandler(svc).ListPublicGroups)

	req := httptest.NewRequest(http.MethodGet, "/pricing/plan-groups", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"name":"Starter"`)
	require.Contains(t, rec.Body.String(), `"contact_methods":[{"type":"telegram","value":"@flux"}]`)
}
