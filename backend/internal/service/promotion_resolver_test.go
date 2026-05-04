package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// --- stub repo for resolver tests ---

type promotionRepoStub struct {
	activeByType  []Promotion
	activeByPlan  []Promotion
	usageCounts   map[string]int // key: "promoID:planID:userID"
	listErr       error
	countUsageErr error
	// unused but required by interface
	created    *Promotion
	getByID    *Promotion
	getByIDErr error
}

func (s *promotionRepoStub) Create(_ context.Context, p *Promotion) error {
	s.created = p
	return nil
}
func (s *promotionRepoStub) GetByID(_ context.Context, id int64) (*Promotion, error) {
	if s.getByIDErr != nil {
		return nil, s.getByIDErr
	}
	if s.getByID != nil && s.getByID.ID == id {
		return s.getByID, nil
	}
	return nil, ErrPromotionNotFound
}
func (s *promotionRepoStub) GetByIDForUpdate(_ context.Context, _ int64) (*Promotion, error) {
	return s.GetByID(context.Background(), 0)
}
func (s *promotionRepoStub) Update(_ context.Context, _ *Promotion) error { return nil }
func (s *promotionRepoStub) Delete(_ context.Context, _ int64) error      { return nil }
func (s *promotionRepoStub) List(_ context.Context, _ pagination.PaginationParams, _ PromotionListFilter) ([]Promotion, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *promotionRepoStub) ListActiveByType(_ context.Context, _ string) ([]Promotion, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.activeByType, nil
}
func (s *promotionRepoStub) ListActiveByPlanID(_ context.Context, _ int64) ([]Promotion, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.activeByPlan, nil
}
func (s *promotionRepoStub) ReplacePlanRules(_ context.Context, _ int64, _ []PromotionPlanRule) error {
	return nil
}
func (s *promotionRepoStub) ListPlanRulesByPromotionID(_ context.Context, _ int64) ([]PromotionPlanRule, error) {
	return nil, nil
}
func (s *promotionRepoStub) ListPlanRulesByPromotionIDs(_ context.Context, _ []int64) (map[int64][]PromotionPlanRule, error) {
	return nil, nil
}
func (s *promotionRepoStub) CreateUsage(_ context.Context, _ *PromotionUsage) error { return nil }
func (s *promotionRepoStub) CountUsageByUser(_ context.Context, promoID int64, planID *int64, userID int64) (int, error) {
	if s.countUsageErr != nil {
		return 0, s.countUsageErr
	}
	if s.usageCounts == nil {
		return 0, nil
	}
	pid := int64(0)
	if planID != nil {
		pid = *planID
	}
	key := usageKey(promoID, pid, userID)
	return s.usageCounts[key], nil
}
func (s *promotionRepoStub) ListUsagesByPromotion(_ context.Context, _ int64, _ pagination.PaginationParams) ([]PromotionUsage, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *promotionRepoStub) DeleteUsagesByOrderID(_ context.Context, _ int64) error { return nil }
func (s *promotionRepoStub) CountActiveOrdersByPromotion(_ context.Context, _ int64) (int, error) {
	return 0, nil
}

func usageKey(promoID, planID, userID int64) string {
	return fmt.Sprintf("%d:%d:%d", promoID, planID, userID)
}

// helpers
func promoFloat64Ptr(v float64) *float64 { return &v }

func activePromotion(id int64, name string, ptype string, priority int) Promotion {
	return Promotion{
		ID:            id,
		Name:          name,
		PromotionType: ptype,
		Status:        domain.PromotionStatusActive,
		Priority:      priority,
	}
}

// ==================== Recharge Resolver Tests ====================

func TestResolveRechargeDiscount_ReducePay(t *testing.T) {
	rate := 0.8
	p := activePromotion(1, "Summer Sale", domain.PromotionTypeRecharge, 10)
	p.DiscountMode = domain.PromotionDiscountModeReducePay
	p.RechargeRate = &rate

	repo := &promotionRepoStub{activeByType: []Promotion{p}}
	resolver := NewPromotionResolver(repo)

	result, err := resolver.ResolveRechargeDiscount(context.Background(), 100, 100.0)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, domain.PromotionDiscountModeReducePay, result.Mode)
	require.InDelta(t, 100.0, result.OriginalAmount, 0.001)
	require.InDelta(t, 80.0, result.PaymentAmount, 0.001)
	require.InDelta(t, 20.0, result.DiscountAmount, 0.001)
	require.InDelta(t, 100.0, result.CreditedAmount, 0.001)
	require.InDelta(t, 0.0, result.BonusAmount, 0.001)
}

func TestResolveRechargeDiscount_BonusCredit(t *testing.T) {
	rate := 1.2
	p := activePromotion(2, "Bonus Event", domain.PromotionTypeRecharge, 10)
	p.DiscountMode = domain.PromotionDiscountModeBonusCredit
	p.RechargeBonusRate = &rate

	repo := &promotionRepoStub{activeByType: []Promotion{p}}
	resolver := NewPromotionResolver(repo)

	result, err := resolver.ResolveRechargeDiscount(context.Background(), 100, 50.0)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, domain.PromotionDiscountModeBonusCredit, result.Mode)
	require.InDelta(t, 50.0, result.OriginalAmount, 0.001)
	require.InDelta(t, 50.0, result.PaymentAmount, 0.001) // bonus mode: pay = original
	require.InDelta(t, 60.0, result.CreditedAmount, 0.001)
	require.InDelta(t, 10.0, result.BonusAmount, 0.001)
	require.InDelta(t, 0.0, result.DiscountAmount, 0.001)
}

func TestResolveRechargeDiscount_PicksBestSaving(t *testing.T) {
	rate80 := 0.8
	rate50 := 0.5
	p1 := activePromotion(1, "20% off", domain.PromotionTypeRecharge, 10)
	p1.DiscountMode = domain.PromotionDiscountModeReducePay
	p1.RechargeRate = &rate80

	p2 := activePromotion(2, "50% off", domain.PromotionTypeRecharge, 5)
	p2.DiscountMode = domain.PromotionDiscountModeReducePay
	p2.RechargeRate = &rate50

	repo := &promotionRepoStub{activeByType: []Promotion{p1, p2}}
	resolver := NewPromotionResolver(repo)

	result, err := resolver.ResolveRechargeDiscount(context.Background(), 100, 100.0)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(2), result.Promotion.ID, "should pick 50% off as best saving")
	require.InDelta(t, 50.0, result.DiscountAmount, 0.001)
}

func TestResolveRechargeDiscount_SameSavingPicksHigherPriority(t *testing.T) {
	rate := 0.8
	p1 := activePromotion(1, "LowPri", domain.PromotionTypeRecharge, 5)
	p1.DiscountMode = domain.PromotionDiscountModeReducePay
	p1.RechargeRate = &rate

	p2 := activePromotion(2, "HighPri", domain.PromotionTypeRecharge, 100)
	p2.DiscountMode = domain.PromotionDiscountModeReducePay
	p2.RechargeRate = &rate

	repo := &promotionRepoStub{activeByType: []Promotion{p1, p2}}
	resolver := NewPromotionResolver(repo)

	result, err := resolver.ResolveRechargeDiscount(context.Background(), 100, 100.0)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(2), result.Promotion.ID, "same saving -> higher priority wins")
}

func TestResolveRechargeDiscount_SkipsExpired(t *testing.T) {
	rate := 0.8
	past := time.Now().Add(-time.Hour)
	p := activePromotion(1, "Expired", domain.PromotionTypeRecharge, 10)
	p.DiscountMode = domain.PromotionDiscountModeReducePay
	p.RechargeRate = &rate
	p.EndsAt = &past

	repo := &promotionRepoStub{activeByType: []Promotion{p}}
	resolver := NewPromotionResolver(repo)

	result, err := resolver.ResolveRechargeDiscount(context.Background(), 100, 100.0)
	require.NoError(t, err)
	require.Nil(t, result, "expired promotion should not match")
}

func TestResolveRechargeDiscount_SkipsNotYetStarted(t *testing.T) {
	rate := 0.8
	future := time.Now().Add(24 * time.Hour)
	p := activePromotion(1, "Future", domain.PromotionTypeRecharge, 10)
	p.DiscountMode = domain.PromotionDiscountModeReducePay
	p.RechargeRate = &rate
	p.StartsAt = &future

	repo := &promotionRepoStub{activeByType: []Promotion{p}}
	resolver := NewPromotionResolver(repo)

	result, err := resolver.ResolveRechargeDiscount(context.Background(), 100, 100.0)
	require.NoError(t, err)
	require.Nil(t, result, "not-yet-started promotion should not match")
}

func TestResolveRechargeDiscount_SkipsUsedUp(t *testing.T) {
	rate := 0.8
	p := activePromotion(1, "Limited", domain.PromotionTypeRecharge, 10)
	p.DiscountMode = domain.PromotionDiscountModeReducePay
	p.RechargeRate = &rate
	p.MaxUsesPerUser = 1

	repo := &promotionRepoStub{
		activeByType: []Promotion{p},
		usageCounts:  map[string]int{usageKey(1, 0, 100): 1},
	}
	resolver := NewPromotionResolver(repo)

	result, err := resolver.ResolveRechargeDiscount(context.Background(), 100, 100.0)
	require.NoError(t, err)
	require.Nil(t, result, "usage limit reached → skip")
}

func TestResolveRechargeDiscount_NilResolver(t *testing.T) {
	var resolver *PromotionResolver
	result, err := resolver.ResolveRechargeDiscount(context.Background(), 1, 100)
	require.NoError(t, err)
	require.Nil(t, result)
}

func TestResolveRechargeDiscount_ZeroAmount(t *testing.T) {
	resolver := NewPromotionResolver(&promotionRepoStub{})
	result, err := resolver.ResolveRechargeDiscount(context.Background(), 1, 0)
	require.NoError(t, err)
	require.Nil(t, result)
}

func TestResolveRechargeDiscount_InvalidRate(t *testing.T) {
	// rate > 1 should produce nil candidate
	rate := 1.5
	p := activePromotion(1, "Bad", domain.PromotionTypeRecharge, 10)
	p.DiscountMode = domain.PromotionDiscountModeReducePay
	p.RechargeRate = &rate

	repo := &promotionRepoStub{activeByType: []Promotion{p}}
	resolver := NewPromotionResolver(repo)

	result, err := resolver.ResolveRechargeDiscount(context.Background(), 100, 100.0)
	require.NoError(t, err)
	require.Nil(t, result, "rate > 1 is invalid for reduce_pay")
}

// ==================== Subscription Resolver Tests ====================

func TestResolveSubscriptionDiscount_Rate(t *testing.T) {
	rate := 0.8
	p := activePromotion(10, "Sub 20% off", domain.PromotionTypeSubscription, 10)
	p.PlanRules = []PromotionPlanRule{{
		PlanID:       100,
		DiscountMode: domain.PromotionDiscountModeRate,
		DiscountRate: &rate,
	}}

	repo := &promotionRepoStub{activeByPlan: []Promotion{p}}
	resolver := NewPromotionResolver(repo)

	result, err := resolver.ResolveSubscriptionDiscount(context.Background(), 1, 100, 50.0)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.InDelta(t, 50.0, result.OriginalPrice, 0.001)
	require.InDelta(t, 40.0, result.FinalPrice, 0.001)
	require.InDelta(t, 10.0, result.DiscountAmount, 0.001)
}

func TestResolveSubscriptionDiscount_Amount(t *testing.T) {
	amount := 15.0
	p := activePromotion(10, "Sub ¥15 off", domain.PromotionTypeSubscription, 10)
	p.PlanRules = []PromotionPlanRule{{
		PlanID:         100,
		DiscountMode:   domain.PromotionDiscountModeAmount,
		DiscountAmount: &amount,
	}}

	repo := &promotionRepoStub{activeByPlan: []Promotion{p}}
	resolver := NewPromotionResolver(repo)

	result, err := resolver.ResolveSubscriptionDiscount(context.Background(), 1, 100, 50.0)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.InDelta(t, 50.0, result.OriginalPrice, 0.001)
	require.InDelta(t, 35.0, result.FinalPrice, 0.001)
	require.InDelta(t, 15.0, result.DiscountAmount, 0.001)
}

func TestResolveSubscriptionDiscount_FloorPrice(t *testing.T) {
	rate := 0.1 // 90% off
	p := activePromotion(10, "Huge discount", domain.PromotionTypeSubscription, 10)
	p.PlanRules = []PromotionPlanRule{{
		PlanID:        100,
		DiscountMode:  domain.PromotionDiscountModeRate,
		DiscountRate:  &rate,
		MinPriceFloor: 30.0,
	}}

	repo := &promotionRepoStub{activeByPlan: []Promotion{p}}
	resolver := NewPromotionResolver(repo)

	result, err := resolver.ResolveSubscriptionDiscount(context.Background(), 1, 100, 50.0)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.InDelta(t, 30.0, result.FinalPrice, 0.001, "floor should clamp final price")
	require.InDelta(t, 20.0, result.DiscountAmount, 0.001)
}

func TestResolveSubscriptionDiscount_PicksBestPrice(t *testing.T) {
	rate80 := 0.8
	rate60 := 0.6
	p1 := activePromotion(10, "20% off", domain.PromotionTypeSubscription, 10)
	p1.PlanRules = []PromotionPlanRule{{PlanID: 100, DiscountMode: domain.PromotionDiscountModeRate, DiscountRate: &rate80}}

	p2 := activePromotion(20, "40% off", domain.PromotionTypeSubscription, 5)
	p2.PlanRules = []PromotionPlanRule{{PlanID: 100, DiscountMode: domain.PromotionDiscountModeRate, DiscountRate: &rate60}}

	repo := &promotionRepoStub{activeByPlan: []Promotion{p1, p2}}
	resolver := NewPromotionResolver(repo)

	result, err := resolver.ResolveSubscriptionDiscount(context.Background(), 1, 100, 100.0)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(20), result.Promotion.ID, "lower final price wins")
	require.InDelta(t, 60.0, result.FinalPrice, 0.001)
}

func TestResolveSubscriptionDiscount_NoPlanRule(t *testing.T) {
	p := activePromotion(10, "No Rule", domain.PromotionTypeSubscription, 10)
	// plan rules for plan 200, but we query for plan 100
	rate := 0.8
	p.PlanRules = []PromotionPlanRule{{PlanID: 200, DiscountMode: domain.PromotionDiscountModeRate, DiscountRate: &rate}}

	repo := &promotionRepoStub{activeByPlan: []Promotion{p}}
	resolver := NewPromotionResolver(repo)

	result, err := resolver.ResolveSubscriptionDiscount(context.Background(), 1, 100, 50.0)
	require.NoError(t, err)
	require.Nil(t, result, "no matching plan rule → nil")
}

func TestResolveSubscriptionDiscount_SkipsUsedUp(t *testing.T) {
	rate := 0.8
	p := activePromotion(10, "Limited", domain.PromotionTypeSubscription, 10)
	p.MaxUsesPerUser = 1
	p.PlanRules = []PromotionPlanRule{{PlanID: 100, DiscountMode: domain.PromotionDiscountModeRate, DiscountRate: &rate}}

	repo := &promotionRepoStub{
		activeByPlan: []Promotion{p},
		usageCounts:  map[string]int{usageKey(10, 100, 1): 1},
	}
	resolver := NewPromotionResolver(repo)

	result, err := resolver.ResolveSubscriptionDiscount(context.Background(), 1, 100, 50.0)
	require.NoError(t, err)
	require.Nil(t, result, "usage limit reached → skip")
}

func TestResolveSubscriptionDiscount_RuleMaxUsesOverridesPromotion(t *testing.T) {
	rate := 0.8
	p := activePromotion(10, "RuleLimit", domain.PromotionTypeSubscription, 10)
	p.MaxUsesPerUser = 5 // promotion level
	p.PlanRules = []PromotionPlanRule{{
		PlanID:         100,
		DiscountMode:   domain.PromotionDiscountModeRate,
		DiscountRate:   &rate,
		MaxUsesPerUser: 2, // rule level overrides
	}}

	repo := &promotionRepoStub{
		activeByPlan: []Promotion{p},
		usageCounts:  map[string]int{usageKey(10, 100, 1): 2},
	}
	resolver := NewPromotionResolver(repo)

	result, err := resolver.ResolveSubscriptionDiscount(context.Background(), 1, 100, 50.0)
	require.NoError(t, err)
	require.Nil(t, result, "rule max_uses=2, used=2 → skip")
}

func TestResolveSubscriptionDiscount_DiscountNotBelowOriginal(t *testing.T) {
	// Amount discount of 0 → final == original → should be nil (no benefit)
	amount := 0.0
	p := activePromotion(10, "Zero off", domain.PromotionTypeSubscription, 10)
	p.PlanRules = []PromotionPlanRule{{PlanID: 100, DiscountMode: domain.PromotionDiscountModeAmount, DiscountAmount: &amount}}

	repo := &promotionRepoStub{activeByPlan: []Promotion{p}}
	resolver := NewPromotionResolver(repo)

	result, err := resolver.ResolveSubscriptionDiscount(context.Background(), 1, 100, 50.0)
	require.NoError(t, err)
	require.Nil(t, result, "zero discount → no benefit → nil")
}

// ==================== VerifyAndCount Tests ====================

func TestVerifyAndCount_WithinLimit(t *testing.T) {
	p := activePromotion(1, "Test", domain.PromotionTypeRecharge, 10)
	p.MaxUsesPerUser = 3

	repo := &promotionRepoStub{usageCounts: map[string]int{usageKey(1, 0, 100): 2}}
	resolver := NewPromotionResolver(repo)

	ok, used, err := resolver.VerifyAndCount(context.Background(), &p, nil, 100, nil)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 2, used)
}

func TestVerifyAndCount_AtLimit(t *testing.T) {
	p := activePromotion(1, "Test", domain.PromotionTypeRecharge, 10)
	p.MaxUsesPerUser = 2

	repo := &promotionRepoStub{usageCounts: map[string]int{usageKey(1, 0, 100): 2}}
	resolver := NewPromotionResolver(repo)

	ok, used, err := resolver.VerifyAndCount(context.Background(), &p, nil, 100, nil)
	require.NoError(t, err)
	require.False(t, ok, "at limit → false")
	require.Equal(t, 2, used)
}

func TestVerifyAndCount_Unlimited(t *testing.T) {
	p := activePromotion(1, "Test", domain.PromotionTypeRecharge, 10)
	p.MaxUsesPerUser = 0 // unlimited

	repo := &promotionRepoStub{usageCounts: map[string]int{usageKey(1, 0, 100): 999}}
	resolver := NewPromotionResolver(repo)

	ok, _, err := resolver.VerifyAndCount(context.Background(), &p, nil, 100, nil)
	require.NoError(t, err)
	require.True(t, ok, "max=0 → unlimited → always ok")
}

func TestVerifyAndCount_NilPromotion(t *testing.T) {
	resolver := NewPromotionResolver(&promotionRepoStub{})
	ok, used, err := resolver.VerifyAndCount(context.Background(), nil, nil, 100, nil)
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, 0, used)
}

// ==================== Domain Model Tests ====================

func TestPromotion_IsActive(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	tests := []struct {
		name   string
		p      Promotion
		expect bool
	}{
		{"active no time window", Promotion{Status: domain.PromotionStatusActive}, true},
		{"disabled", Promotion{Status: domain.PromotionStatusDisabled}, false},
		{"started", Promotion{Status: domain.PromotionStatusActive, StartsAt: &past}, true},
		{"not started", Promotion{Status: domain.PromotionStatusActive, StartsAt: &future}, false},
		{"not ended", Promotion{Status: domain.PromotionStatusActive, EndsAt: &future}, true},
		{"ended", Promotion{Status: domain.PromotionStatusActive, EndsAt: &past}, false},
		{"in window", Promotion{Status: domain.PromotionStatusActive, StartsAt: &past, EndsAt: &future}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expect, tc.p.IsActive(now))
		})
	}
}

func TestPromotion_EffectiveMaxUses(t *testing.T) {
	p := Promotion{MaxUsesPerUser: 5}

	require.Equal(t, 5, p.EffectiveMaxUses(nil))
	require.Equal(t, 5, p.EffectiveMaxUses(&PromotionPlanRule{MaxUsesPerUser: 0}))
	require.Equal(t, 2, p.EffectiveMaxUses(&PromotionPlanRule{MaxUsesPerUser: 2}))
}
