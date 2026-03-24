//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPricingPlanRepository_ListGroupsWithPlans_OrdersAndLoadsJSONFields(t *testing.T) {
	ctx := context.Background()
	repo := NewPricingPlanRepository(integrationDB).(*pricingPlanRepository)
	prefix := fmt.Sprintf("it-pricing-%d-", time.Now().UnixNano())

	cleanupPricingPlanFixtures(t, ctx, prefix)
	t.Cleanup(func() {
		cleanupPricingPlanFixtures(t, ctx, prefix)
	})

	groupLaterID := insertPricingGroupFixture(t, ctx, prefix+"later", 20, service.StatusActive)
	groupFirstID := insertPricingGroupFixture(t, ctx, prefix+"first", 10, service.StatusActive)

	insertPricingPlanFixture(t, ctx, pricingPlanFixture{
		GroupID:        groupFirstID,
		Name:           prefix + "pro",
		SortOrder:      20,
		Status:         service.StatusActive,
		PriceCurrency:  "CNY",
		PricePeriod:    "month",
		FeaturesJSON:   `["Feature B"]`,
		ContactMethods: `[{"type":"telegram","value":"@later"}]`,
	})
	insertPricingPlanFixture(t, ctx, pricingPlanFixture{
		GroupID:        groupFirstID,
		Name:           prefix + "starter",
		SortOrder:      10,
		Status:         service.StatusActive,
		PriceCurrency:  "CNY",
		PricePeriod:    "week",
		FeaturesJSON:   `["Feature A","Feature C"]`,
		ContactMethods: `[{"type":"telegram","value":"@flux"}]`,
	})
	insertPricingPlanFixture(t, ctx, pricingPlanFixture{
		GroupID:        groupLaterID,
		Name:           prefix + "enterprise",
		SortOrder:      5,
		Status:         service.StatusActive,
		PriceCurrency:  "USD",
		PricePeriod:    "year",
		FeaturesJSON:   `["Dedicated"]`,
		ContactMethods: `[]`,
	})

	groups, err := repo.ListGroupsWithPlans(ctx, false)
	require.NoError(t, err)

	filtered := filterPricingGroupsByPrefix(groups, prefix)
	require.Len(t, filtered, 2)
	require.Equal(t, prefix+"first", filtered[0].Name)
	require.Equal(t, prefix+"later", filtered[1].Name)

	require.Len(t, filtered[0].Plans, 2)
	require.Equal(t, prefix+"starter", filtered[0].Plans[0].Name)
	require.Equal(t, "week", filtered[0].Plans[0].PricePeriod)
	require.Equal(t, []string{"Feature A", "Feature C"}, filtered[0].Plans[0].Features)
	require.Equal(t, []service.PricingPlanContactMethod{{Type: "telegram", Value: "@flux"}}, filtered[0].Plans[0].ContactMethods)
	require.Equal(t, prefix+"pro", filtered[0].Plans[1].Name)
}

func TestPricingPlanRepository_ListGroupsWithPlans_OnlyActiveFiltersDisabledRows(t *testing.T) {
	ctx := context.Background()
	repo := NewPricingPlanRepository(integrationDB).(*pricingPlanRepository)
	prefix := fmt.Sprintf("it-pricing-active-%d-", time.Now().UnixNano())

	cleanupPricingPlanFixtures(t, ctx, prefix)
	t.Cleanup(func() {
		cleanupPricingPlanFixtures(t, ctx, prefix)
	})

	activeGroupID := insertPricingGroupFixture(t, ctx, prefix+"active", 10, service.StatusActive)
	disabledGroupID := insertPricingGroupFixture(t, ctx, prefix+"disabled", 20, service.StatusDisabled)

	insertPricingPlanFixture(t, ctx, pricingPlanFixture{
		GroupID:        activeGroupID,
		Name:           prefix + "visible",
		SortOrder:      10,
		Status:         service.StatusActive,
		PriceCurrency:  "CNY",
		PricePeriod:    "month",
		FeaturesJSON:   `["Visible"]`,
		ContactMethods: `[]`,
	})
	insertPricingPlanFixture(t, ctx, pricingPlanFixture{
		GroupID:        activeGroupID,
		Name:           prefix + "hidden-plan",
		SortOrder:      20,
		Status:         service.StatusDisabled,
		PriceCurrency:  "CNY",
		PricePeriod:    "month",
		FeaturesJSON:   `["Hidden"]`,
		ContactMethods: `[]`,
	})
	insertPricingPlanFixture(t, ctx, pricingPlanFixture{
		GroupID:        disabledGroupID,
		Name:           prefix + "hidden-group",
		SortOrder:      5,
		Status:         service.StatusActive,
		PriceCurrency:  "CNY",
		PricePeriod:    "month",
		FeaturesJSON:   `["Ignored"]`,
		ContactMethods: `[]`,
	})

	groups, err := repo.ListGroupsWithPlans(ctx, true)
	require.NoError(t, err)

	filtered := filterPricingGroupsByPrefix(groups, prefix)
	require.Len(t, filtered, 1)
	require.Equal(t, prefix+"active", filtered[0].Name)
	require.Len(t, filtered[0].Plans, 1)
	require.Equal(t, prefix+"visible", filtered[0].Plans[0].Name)
}

type pricingPlanFixture struct {
	GroupID        int64
	Name           string
	SortOrder      int
	Status         string
	PriceCurrency  string
	PricePeriod    string
	FeaturesJSON   string
	ContactMethods string
}

func cleanupPricingPlanFixtures(t *testing.T, ctx context.Context, prefix string) {
	t.Helper()

	_, err := integrationDB.ExecContext(ctx, `DELETE FROM pricing_plans WHERE name LIKE $1`, prefix+"%")
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `DELETE FROM pricing_plan_groups WHERE name LIKE $1`, prefix+"%")
	require.NoError(t, err)
}

func insertPricingGroupFixture(t *testing.T, ctx context.Context, name string, sortOrder int, status string) int64 {
	t.Helper()

	var id int64
	err := integrationDB.QueryRowContext(ctx, `
INSERT INTO pricing_plan_groups (name, sort_order, status)
VALUES ($1, $2, $3)
RETURNING id
`, name, sortOrder, status).Scan(&id)
	require.NoError(t, err)
	return id
}

func insertPricingPlanFixture(t *testing.T, ctx context.Context, fixture pricingPlanFixture) int64 {
	t.Helper()

	var id int64
	err := integrationDB.QueryRowContext(ctx, `
INSERT INTO pricing_plans (
  group_id,
  name,
  sort_order,
  status,
  price_currency,
  price_period,
  features,
  contact_methods
)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb)
RETURNING id
`, fixture.GroupID, fixture.Name, fixture.SortOrder, fixture.Status, fixture.PriceCurrency, fixture.PricePeriod, fixture.FeaturesJSON, fixture.ContactMethods).Scan(&id)
	require.NoError(t, err)
	return id
}

func filterPricingGroupsByPrefix(groups []service.PricingPlanGroup, prefix string) []service.PricingPlanGroup {
	filtered := make([]service.PricingPlanGroup, 0)
	for _, group := range groups {
		if strings.HasPrefix(group.Name, prefix) {
			filtered = append(filtered, group)
		}
	}
	return filtered
}
