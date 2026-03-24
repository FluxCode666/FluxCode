package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type pricingPlanRepositoryStub struct {
	listGroupsWithPlansFn func(ctx context.Context, onlyActive bool) ([]PricingPlanGroup, error)
	getGroupByIDFn        func(ctx context.Context, id int64) (*PricingPlanGroup, error)
	createGroupFn         func(ctx context.Context, group *PricingPlanGroup) error
	updateGroupFn         func(ctx context.Context, group *PricingPlanGroup) error
	deleteGroupFn         func(ctx context.Context, id int64) error
	getPlanByIDFn         func(ctx context.Context, id int64) (*PricingPlan, error)
	createPlanFn          func(ctx context.Context, plan *PricingPlan) error
	updatePlanFn          func(ctx context.Context, plan *PricingPlan) error
	deletePlanFn          func(ctx context.Context, id int64) error
}

func (s *pricingPlanRepositoryStub) ListGroupsWithPlans(ctx context.Context, onlyActive bool) ([]PricingPlanGroup, error) {
	if s.listGroupsWithPlansFn == nil {
		return nil, nil
	}
	return s.listGroupsWithPlansFn(ctx, onlyActive)
}

func (s *pricingPlanRepositoryStub) GetGroupByID(ctx context.Context, id int64) (*PricingPlanGroup, error) {
	if s.getGroupByIDFn == nil {
		return nil, nil
	}
	return s.getGroupByIDFn(ctx, id)
}

func (s *pricingPlanRepositoryStub) CreateGroup(ctx context.Context, group *PricingPlanGroup) error {
	if s.createGroupFn == nil {
		return nil
	}
	return s.createGroupFn(ctx, group)
}

func (s *pricingPlanRepositoryStub) UpdateGroup(ctx context.Context, group *PricingPlanGroup) error {
	if s.updateGroupFn == nil {
		return nil
	}
	return s.updateGroupFn(ctx, group)
}

func (s *pricingPlanRepositoryStub) DeleteGroup(ctx context.Context, id int64) error {
	if s.deleteGroupFn == nil {
		return nil
	}
	return s.deleteGroupFn(ctx, id)
}

func (s *pricingPlanRepositoryStub) GetPlanByID(ctx context.Context, id int64) (*PricingPlan, error) {
	if s.getPlanByIDFn == nil {
		return nil, nil
	}
	return s.getPlanByIDFn(ctx, id)
}

func (s *pricingPlanRepositoryStub) CreatePlan(ctx context.Context, plan *PricingPlan) error {
	if s.createPlanFn == nil {
		return nil
	}
	return s.createPlanFn(ctx, plan)
}

func (s *pricingPlanRepositoryStub) UpdatePlan(ctx context.Context, plan *PricingPlan) error {
	if s.updatePlanFn == nil {
		return nil
	}
	return s.updatePlanFn(ctx, plan)
}

func (s *pricingPlanRepositoryStub) DeletePlan(ctx context.Context, id int64) error {
	if s.deletePlanFn == nil {
		return nil
	}
	return s.deletePlanFn(ctx, id)
}

func TestPricingPlanService_ListPublicGroups_FiltersInactivePlansAndEmptyGroups(t *testing.T) {
	t.Parallel()

	svc := NewPricingPlanService(&pricingPlanRepositoryStub{
		listGroupsWithPlansFn: func(ctx context.Context, onlyActive bool) ([]PricingPlanGroup, error) {
			require.True(t, onlyActive)

			return []PricingPlanGroup{
				{
					ID:     1,
					Name:   "Starter",
					Status: StatusActive,
					Plans: []PricingPlan{
						{ID: 10, Name: "Monthly", Status: StatusActive},
						{ID: 11, Name: "Hidden", Status: StatusDisabled},
					},
				},
				{
					ID:     2,
					Name:   "Empty",
					Status: StatusActive,
				},
			}, nil
		},
	})

	groups, err := svc.ListPublicGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, int64(1), groups[0].ID)
	require.Len(t, groups[0].Plans, 1)
	require.Equal(t, int64(10), groups[0].Plans[0].ID)
}

func TestPricingPlanService_CreatePlan_AppliesDefaultsAndNormalizesCollections(t *testing.T) {
	t.Parallel()

	var created *PricingPlan
	svc := NewPricingPlanService(&pricingPlanRepositoryStub{
		getGroupByIDFn: func(ctx context.Context, id int64) (*PricingPlanGroup, error) {
			require.Equal(t, int64(8), id)
			return &PricingPlanGroup{ID: id, Name: "Starter", Status: StatusActive}, nil
		},
		createPlanFn: func(ctx context.Context, plan *PricingPlan) error {
			copyPlan := *plan
			created = &copyPlan
			return nil
		},
	})

	name := " Pro "
	blank := "   "
	contactType := " telegram "
	contactValue := " @flux "

	plan, err := svc.CreatePlan(context.Background(), CreatePricingPlanInput{
		GroupID:   8,
		Name:      name,
		PriceText: &blank,
		Features:  []string{" Fast ", " ", " Stable "},
		ContactMethods: []PricingPlanContactMethod{
			{Type: contactType, Value: contactValue},
			{Type: "", Value: "invalid"},
			{Type: "email", Value: ""},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, created)
	require.Equal(t, "Pro", plan.Name)
	require.Equal(t, "Pro", created.Name)
	require.Equal(t, "CNY", created.PriceCurrency)
	require.Equal(t, "month", created.PricePeriod)
	require.Equal(t, StatusActive, created.Status)
	require.Nil(t, created.PriceText)
	require.Equal(t, []string{"Fast", "Stable"}, created.Features)
	require.Equal(t, []PricingPlanContactMethod{{Type: "telegram", Value: "@flux"}}, created.ContactMethods)
}

func TestPricingPlanService_UpdatePlan_RejectsNegativePrice(t *testing.T) {
	t.Parallel()

	updateCalled := false
	svc := NewPricingPlanService(&pricingPlanRepositoryStub{
		getPlanByIDFn: func(ctx context.Context, id int64) (*PricingPlan, error) {
			return &PricingPlan{ID: id, GroupID: 1, Name: "Starter", Status: StatusActive}, nil
		},
		updatePlanFn: func(ctx context.Context, plan *PricingPlan) error {
			updateCalled = true
			return nil
		},
	})

	negative := -1.0
	_, err := svc.UpdatePlan(context.Background(), 10, UpdatePricingPlanInput{
		PriceAmount: &negative,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "price_amount must be >= 0")
	require.False(t, updateCalled)
}
