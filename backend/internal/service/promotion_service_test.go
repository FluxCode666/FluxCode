package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

// ==================== Validation Tests ====================

func TestValidatePromotion_RechargeReducePay(t *testing.T) {
	rate := 0.8
	p := &Promotion{
		Name:          "Test",
		PromotionType: domain.PromotionTypeRecharge,
		DiscountMode:  domain.PromotionDiscountModeReducePay,
		RechargeRate:  &rate,
		Status:        domain.PromotionStatusActive,
	}
	require.NoError(t, validatePromotion(p))
}

func TestValidatePromotion_RechargeReducePay_MissingRate(t *testing.T) {
	p := &Promotion{
		Name:          "Test",
		PromotionType: domain.PromotionTypeRecharge,
		DiscountMode:  domain.PromotionDiscountModeReducePay,
		Status:        domain.PromotionStatusActive,
	}
	err := validatePromotion(p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "recharge_rate")
}

func TestValidatePromotion_RechargeBonusCredit(t *testing.T) {
	rate := 1.2
	p := &Promotion{
		Name:              "Test",
		PromotionType:     domain.PromotionTypeRecharge,
		DiscountMode:      domain.PromotionDiscountModeBonusCredit,
		RechargeBonusRate: &rate,
		Status:            domain.PromotionStatusActive,
	}
	require.NoError(t, validatePromotion(p))
}

func TestValidatePromotion_RechargeBonusCredit_RateTooLow(t *testing.T) {
	rate := 0.5 // < 1
	p := &Promotion{
		Name:              "Test",
		PromotionType:     domain.PromotionTypeRecharge,
		DiscountMode:      domain.PromotionDiscountModeBonusCredit,
		RechargeBonusRate: &rate,
		Status:            domain.PromotionStatusActive,
	}
	err := validatePromotion(p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "recharge_bonus_rate")
}

func TestValidatePromotion_RechargeWithPlanRules_Invalid(t *testing.T) {
	rate := 0.8
	p := &Promotion{
		Name:          "Test",
		PromotionType: domain.PromotionTypeRecharge,
		DiscountMode:  domain.PromotionDiscountModeReducePay,
		RechargeRate:  &rate,
		Status:        domain.PromotionStatusActive,
		PlanRules:     []PromotionPlanRule{{PlanID: 1}},
	}
	err := validatePromotion(p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan rules")
}

func TestValidatePromotion_Subscription_Valid(t *testing.T) {
	rate := 0.8
	p := &Promotion{
		Name:          "Test",
		PromotionType: domain.PromotionTypeSubscription,
		Status:        domain.PromotionStatusActive,
		PlanRules: []PromotionPlanRule{{
			PlanID:       1,
			DiscountMode: domain.PromotionDiscountModeRate,
			DiscountRate: &rate,
		}},
	}
	require.NoError(t, validatePromotion(p))
}

func TestValidatePromotion_Subscription_NoPlanRules(t *testing.T) {
	p := &Promotion{
		Name:          "Test",
		PromotionType: domain.PromotionTypeSubscription,
		Status:        domain.PromotionStatusActive,
	}
	err := validatePromotion(p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan rule")
}

func TestValidatePromotion_Subscription_DuplicatePlanID(t *testing.T) {
	rate := 0.8
	p := &Promotion{
		Name:          "Test",
		PromotionType: domain.PromotionTypeSubscription,
		Status:        domain.PromotionStatusActive,
		PlanRules: []PromotionPlanRule{
			{PlanID: 1, DiscountMode: domain.PromotionDiscountModeRate, DiscountRate: &rate},
			{PlanID: 1, DiscountMode: domain.PromotionDiscountModeRate, DiscountRate: &rate},
		},
	}
	err := validatePromotion(p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate")
}

func TestValidatePromotion_Subscription_WithRechargeFields(t *testing.T) {
	rate := 0.8
	rr := 0.9
	p := &Promotion{
		Name:          "Test",
		PromotionType: domain.PromotionTypeSubscription,
		RechargeRate:  &rr,
		Status:        domain.PromotionStatusActive,
		PlanRules: []PromotionPlanRule{{
			PlanID:       1,
			DiscountMode: domain.PromotionDiscountModeRate,
			DiscountRate: &rate,
		}},
	}
	err := validatePromotion(p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "recharge_rate")
}

func TestValidatePromotion_EmptyName(t *testing.T) {
	rate := 0.8
	p := &Promotion{
		Name:          "",
		PromotionType: domain.PromotionTypeRecharge,
		DiscountMode:  domain.PromotionDiscountModeReducePay,
		RechargeRate:  &rate,
		Status:        domain.PromotionStatusActive,
	}
	err := validatePromotion(p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "name")
}

func TestValidatePromotion_InvalidType(t *testing.T) {
	p := &Promotion{
		Name:          "Test",
		PromotionType: "invalid",
		Status:        domain.PromotionStatusActive,
	}
	err := validatePromotion(p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "promotion_type")
}

func TestValidatePromotion_InvalidStatus(t *testing.T) {
	rate := 0.8
	p := &Promotion{
		Name:          "Test",
		PromotionType: domain.PromotionTypeRecharge,
		DiscountMode:  domain.PromotionDiscountModeReducePay,
		RechargeRate:  &rate,
		Status:        "invalid",
	}
	err := validatePromotion(p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "status")
}

func TestValidatePlanRule_Rate_Valid(t *testing.T) {
	rate := 0.5
	rule := &PromotionPlanRule{
		PlanID:       1,
		DiscountMode: domain.PromotionDiscountModeRate,
		DiscountRate: &rate,
	}
	require.NoError(t, validatePlanRule(rule))
}

func TestValidatePlanRule_Rate_OutOfRange(t *testing.T) {
	rate := 1.5
	rule := &PromotionPlanRule{
		PlanID:       1,
		DiscountMode: domain.PromotionDiscountModeRate,
		DiscountRate: &rate,
	}
	err := validatePlanRule(rule)
	require.Error(t, err)
	require.Contains(t, err.Error(), "discount_rate")
}

func TestValidatePlanRule_Amount_Valid(t *testing.T) {
	amount := 10.0
	rule := &PromotionPlanRule{
		PlanID:         1,
		DiscountMode:   domain.PromotionDiscountModeAmount,
		DiscountAmount: &amount,
	}
	require.NoError(t, validatePlanRule(rule))
}

func TestValidatePlanRule_Amount_Negative(t *testing.T) {
	amount := -5.0
	rule := &PromotionPlanRule{
		PlanID:         1,
		DiscountMode:   domain.PromotionDiscountModeAmount,
		DiscountAmount: &amount,
	}
	err := validatePlanRule(rule)
	require.Error(t, err)
	require.Contains(t, err.Error(), "discount_amount")
}

func TestValidatePlanRule_InvalidMode(t *testing.T) {
	rule := &PromotionPlanRule{
		PlanID:       1,
		DiscountMode: "unknown",
	}
	err := validatePlanRule(rule)
	require.Error(t, err)
	require.Contains(t, err.Error(), "discount_mode")
}

func TestValidatePlanRule_NegativeFloor(t *testing.T) {
	rate := 0.8
	rule := &PromotionPlanRule{
		PlanID:        1,
		DiscountMode:  domain.PromotionDiscountModeRate,
		DiscountRate:  &rate,
		MinPriceFloor: -10,
	}
	err := validatePlanRule(rule)
	require.Error(t, err)
	require.Contains(t, err.Error(), "min_price_floor")
}

// ==================== Service Delete Tests ====================

func TestPromotionService_Delete_OnlyDisabled(t *testing.T) {
	rate := 0.8
	p := &Promotion{
		ID:            1,
		Name:          "Active",
		PromotionType: domain.PromotionTypeRecharge,
		DiscountMode:  domain.PromotionDiscountModeReducePay,
		RechargeRate:  &rate,
		Status:        domain.PromotionStatusActive,
	}
	repo := &promotionRepoStub{getByID: p}
	svc := NewPromotionService(repo)

	err := svc.Delete(context.Background(), 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "disabled")
}

func TestPromotionService_SetStatus_Invalid(t *testing.T) {
	repo := &promotionRepoStub{}
	svc := NewPromotionService(repo)

	_, err := svc.SetStatus(context.Background(), 1, "invalid")
	require.Error(t, err)
	require.Contains(t, err.Error(), "status")
}

// ==================== Build Create Tests ====================

func TestBuildCreatePromotion_RechargeReducePay(t *testing.T) {
	rate := 0.8
	p, err := buildCreatePromotion(CreatePromotionInput{
		Name:          "Test",
		PromotionType: domain.PromotionTypeRecharge,
		DiscountMode:  domain.PromotionDiscountModeReducePay,
		RechargeRate:  &rate,
	})
	require.NoError(t, err)
	require.Equal(t, domain.PromotionStatusActive, p.Status, "default status should be active")
	require.Equal(t, "Test", p.Name)
}

func TestBuildCreatePromotion_SubscriptionWithRules(t *testing.T) {
	rate := 0.7
	p, err := buildCreatePromotion(CreatePromotionInput{
		Name:          "Sub Promo",
		PromotionType: domain.PromotionTypeSubscription,
		PlanRules: []PromotionPlanRuleInput{{
			PlanID:       10,
			DiscountMode: domain.PromotionDiscountModeRate,
			DiscountRate: &rate,
		}},
	})
	require.NoError(t, err)
	require.Len(t, p.PlanRules, 1)
	require.Equal(t, int64(10), p.PlanRules[0].PlanID)
}

// ==================== Apply Update Tests ====================

func TestApplyUpdatePromotion(t *testing.T) {
	rate := 0.8
	p := &Promotion{
		Name:         "Old",
		Description:  "Desc",
		DiscountMode: domain.PromotionDiscountModeReducePay,
		RechargeRate: &rate,
		Priority:     1,
	}

	newName := "New"
	newPriority := 99
	applyUpdatePromotion(p, UpdatePromotionInput{
		Name:     &newName,
		Priority: &newPriority,
	})
	require.Equal(t, "New", p.Name)
	require.Equal(t, 99, p.Priority)
	require.Equal(t, "Desc", p.Description, "unchanged fields should remain")
}

func TestApplyUpdatePromotion_ClearRechargeRate(t *testing.T) {
	rate := 0.8
	p := &Promotion{RechargeRate: &rate}

	applyUpdatePromotion(p, UpdatePromotionInput{ClearRechargeRate: true})
	require.Nil(t, p.RechargeRate)
}

func TestApplyUpdatePromotion_ReplacePlanRules(t *testing.T) {
	p := &Promotion{PlanRules: []PromotionPlanRule{{PlanID: 1}, {PlanID: 2}}}

	newRules := []PromotionPlanRuleInput{{PlanID: 3, DiscountMode: domain.PromotionDiscountModeRate, DiscountRate: promoFloat64Ptr(0.8)}}
	applyUpdatePromotion(p, UpdatePromotionInput{PlanRules: &newRules})
	require.Len(t, p.PlanRules, 1)
	require.Equal(t, int64(3), p.PlanRules[0].PlanID)
}
