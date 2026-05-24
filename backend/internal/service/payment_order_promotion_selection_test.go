package service

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestComputeOrderAmounts_RechargeSkipsPromotionWhenUserDidNotSelectOne(t *testing.T) {
	rate := 0.5
	promo := activePromotion(1, "Half Off", domain.PromotionTypeRecharge, 10)
	promo.DiscountMode = domain.PromotionDiscountModeReducePay
	promo.RechargeRate = &rate

	svc := &PaymentService{
		promotionResolver: NewPromotionResolver(&promotionRepoStub{activeByType: []Promotion{promo}}),
	}

	comp, err := svc.computeOrderAmounts(context.Background(), CreateOrderRequest{
		UserID:      100,
		Amount:      100,
		OrderType:   payment.OrderTypeBalance,
		PromotionID: 0,
	}, nil, &PaymentConfig{
		BalanceRechargeMultiplier: 1,
	})
	require.NoError(t, err)
	require.Nil(t, comp.PromotionID)
	require.Nil(t, comp.RechargeHit)
	require.Zero(t, comp.DiscountAmount)
	require.InDelta(t, 100.0, comp.PaymentBase, 0.001)
	require.InDelta(t, 100.0, comp.OrderAmount, 0.001)
}

func TestComputeOrderAmounts_RechargeAppliesPromotionWhenUserSelectedOne(t *testing.T) {
	rate := 0.5
	promo := activePromotion(9, "Half Off", domain.PromotionTypeRecharge, 10)
	promo.DiscountMode = domain.PromotionDiscountModeReducePay
	promo.RechargeRate = &rate

	svc := &PaymentService{
		promotionResolver: NewPromotionResolver(&promotionRepoStub{
			getByID: &promo,
		}),
	}

	comp, err := svc.computeOrderAmounts(context.Background(), CreateOrderRequest{
		UserID:      100,
		Amount:      100,
		OrderType:   payment.OrderTypeBalance,
		PromotionID: 9,
	}, nil, &PaymentConfig{
		BalanceRechargeMultiplier: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, comp.PromotionID)
	require.Equal(t, int64(9), *comp.PromotionID)
	require.NotNil(t, comp.RechargeHit)
	require.InDelta(t, 50.0, comp.PaymentBase, 0.001)
	require.InDelta(t, 50.0, comp.DiscountAmount, 0.001)
}

func TestComputeOrderAmounts_SubscriptionSkipsPromotionWhenUserDidNotSelectOne(t *testing.T) {
	discountRate := 0.8
	promo := activePromotion(7, "Plan Sale", domain.PromotionTypeSubscription, 10)
	promo.PlanRules = []PromotionPlanRule{{
		PlanID:       88,
		DiscountMode: domain.PromotionDiscountModeRate,
		DiscountRate: &discountRate,
	}}

	svc := &PaymentService{
		promotionResolver: NewPromotionResolver(&promotionRepoStub{
			activeByPlan: []Promotion{promo},
		}),
	}
	plan := &dbent.SubscriptionPlan{ID: 88, Price: 100}

	comp, err := svc.computeOrderAmounts(context.Background(), CreateOrderRequest{
		UserID:      100,
		OrderType:   payment.OrderTypeSubscription,
		PlanID:      88,
		PromotionID: 0,
	}, plan, &PaymentConfig{})
	require.NoError(t, err)
	require.Nil(t, comp.PromotionID)
	require.Nil(t, comp.SubscriptionHit)
	require.Zero(t, comp.DiscountAmount)
	require.InDelta(t, 100.0, comp.PaymentBase, 0.001)
}
