package service

import (
	"context"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/shopspring/decimal"
)

// RechargeDiscount 描述充值订单命中的活动结果
type RechargeDiscount struct {
	Promotion       Promotion
	Mode            string  // domain.PromotionDiscountModeReducePay / BonusCredit
	OriginalAmount  float64 // 用户输入的充值金额
	CreditedAmount  float64 // 实际到账金额（bonus_credit 模式 = original * bonus_rate；reduce_pay 模式 = original，由全局 multiplier 决定，这里仅描述活动产生的覆盖）
	PaymentAmount   float64 // 用户实际需要支付（不含手续费的金额，下游再乘 1+feeRate）
	DiscountAmount  float64 // 节省金额（reduce_pay 模式：original - paymentAmount；bonus_credit 模式：0）
	BonusAmount     float64 // 加送金额（bonus_credit 模式：creditedAmount - originalAmount；reduce_pay 模式：0）
}

// SubscriptionDiscount 描述订阅订单命中的活动结果
type SubscriptionDiscount struct {
	Promotion      Promotion
	Rule           PromotionPlanRule
	OriginalPrice  float64 // plan.Price
	FinalPrice     float64 // 折后价（>= rule.MinPriceFloor）
	DiscountAmount float64 // OriginalPrice - FinalPrice
}

// PromotionResolver 负责挑选"对用户最优"的活动并计算折扣金额
type PromotionResolver struct {
	repo PromotionRepository
}

// NewPromotionResolver 构造函数
func NewPromotionResolver(repo PromotionRepository) *PromotionResolver {
	return &PromotionResolver{repo: repo}
}

// ResolveRechargeDiscount 为充值订单挑选最优活动；未命中返回 (nil, nil)
//
// userAmount 为用户输入的充值金额（即原始 limit_amount），>0
func (r *PromotionResolver) ResolveRechargeDiscount(ctx context.Context, userID int64, userAmount float64) (*RechargeDiscount, error) {
	if r == nil || r.repo == nil || userAmount <= 0 {
		return nil, nil
	}
	promotions, err := r.repo.ListActiveByType(ctx, domain.PromotionTypeRecharge)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var best *RechargeDiscount
	for i := range promotions {
		p := promotions[i]
		if !p.IsActive(now) {
			continue
		}
		// 限次预检（履约阶段会在事务内复查）
		if p.MaxUsesPerUser > 0 {
			used, err := r.repo.CountUsageByUser(ctx, p.ID, nil, userID)
			if err != nil {
				return nil, err
			}
			if used >= p.MaxUsesPerUser {
				continue
			}
		}
		candidate := buildRechargeCandidate(&p, userAmount)
		if candidate == nil {
			continue
		}
		if best == nil || rechargeBetter(candidate, best) {
			best = candidate
		}
	}
	return best, nil
}

// ResolveSubscriptionDiscount 为订阅订单挑选最优活动；未命中返回 (nil, nil)
//
// planPrice 为 subscription_plans.price
func (r *PromotionResolver) ResolveSubscriptionDiscount(ctx context.Context, userID, planID int64, planPrice float64) (*SubscriptionDiscount, error) {
	if r == nil || r.repo == nil || planPrice <= 0 {
		return nil, nil
	}
	promotions, err := r.repo.ListActiveByPlanID(ctx, planID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var best *SubscriptionDiscount
	for i := range promotions {
		p := promotions[i]
		if !p.IsActive(now) {
			continue
		}
		rule := findPlanRule(&p, planID)
		if rule == nil {
			continue
		}
		// 限次预检
		maxUses := p.EffectiveMaxUses(rule)
		if maxUses > 0 {
			pid := planID
			used, err := r.repo.CountUsageByUser(ctx, p.ID, &pid, userID)
			if err != nil {
				return nil, err
			}
			if used >= maxUses {
				continue
			}
		}
		candidate := buildSubscriptionCandidate(&p, rule, planPrice)
		if candidate == nil {
			continue
		}
		if best == nil || subscriptionBetter(candidate, best) {
			best = candidate
		}
	}
	return best, nil
}

// VerifyAndCount 在事务内复查限次（防并发超卖）。已在事务上下文里调用，
// promotion 行已经 SELECT FOR UPDATE 锁定。
//
// 返回 (true, usedCount) 表示限次仍未达到；(false, usedCount) 表示已达上限，调用方应放弃应用此活动。
func (r *PromotionResolver) VerifyAndCount(ctx context.Context, promotion *Promotion, rule *PromotionPlanRule, userID int64, planID *int64) (bool, int, error) {
	if promotion == nil {
		return false, 0, nil
	}
	maxUses := promotion.EffectiveMaxUses(rule)
	used, err := r.repo.CountUsageByUser(ctx, promotion.ID, planID, userID)
	if err != nil {
		return false, 0, err
	}
	if maxUses > 0 && used >= maxUses {
		return false, used, nil
	}
	return true, used, nil
}

// --- helpers ---

func findPlanRule(p *Promotion, planID int64) *PromotionPlanRule {
	if p == nil {
		return nil
	}
	for i := range p.PlanRules {
		if p.PlanRules[i].PlanID == planID {
			return &p.PlanRules[i]
		}
	}
	return nil
}

func buildRechargeCandidate(p *Promotion, userAmount float64) *RechargeDiscount {
	if p == nil {
		return nil
	}
	switch p.DiscountMode {
	case domain.PromotionDiscountModeReducePay:
		if p.RechargeRate == nil {
			return nil
		}
		rate := *p.RechargeRate
		if rate <= 0 || rate > 1 {
			return nil
		}
		pay := decimal.NewFromFloat(userAmount).
			Mul(decimal.NewFromFloat(rate)).
			Round(2).InexactFloat64()
		discount := decimal.NewFromFloat(userAmount).
			Sub(decimal.NewFromFloat(pay)).
			Round(2).InexactFloat64()
		if discount < 0 {
			discount = 0
		}
		return &RechargeDiscount{
			Promotion:      *p,
			Mode:           p.DiscountMode,
			OriginalAmount: userAmount,
			CreditedAmount: userAmount,
			PaymentAmount:  pay,
			DiscountAmount: discount,
		}
	case domain.PromotionDiscountModeBonusCredit:
		if p.RechargeBonusRate == nil {
			return nil
		}
		rate := *p.RechargeBonusRate
		if rate < 1 {
			return nil
		}
		credited := decimal.NewFromFloat(userAmount).
			Mul(decimal.NewFromFloat(rate)).
			Round(2).InexactFloat64()
		bonus := decimal.NewFromFloat(credited).
			Sub(decimal.NewFromFloat(userAmount)).
			Round(2).InexactFloat64()
		if bonus < 0 {
			bonus = 0
		}
		return &RechargeDiscount{
			Promotion:      *p,
			Mode:           p.DiscountMode,
			OriginalAmount: userAmount,
			CreditedAmount: credited,
			PaymentAmount:  userAmount,
			BonusAmount:    bonus,
		}
	}
	return nil
}

func buildSubscriptionCandidate(p *Promotion, rule *PromotionPlanRule, planPrice float64) *SubscriptionDiscount {
	if p == nil || rule == nil {
		return nil
	}
	original := decimal.NewFromFloat(planPrice)
	floor := decimal.NewFromFloat(math.Max(0, rule.MinPriceFloor))

	var final decimal.Decimal
	switch rule.DiscountMode {
	case domain.PromotionDiscountModeRate:
		if rule.DiscountRate == nil {
			return nil
		}
		rate := *rule.DiscountRate
		if rate <= 0 || rate > 1 {
			return nil
		}
		final = original.Mul(decimal.NewFromFloat(rate))
	case domain.PromotionDiscountModeAmount:
		if rule.DiscountAmount == nil {
			return nil
		}
		amount := *rule.DiscountAmount
		if amount < 0 {
			return nil
		}
		final = original.Sub(decimal.NewFromFloat(amount))
	default:
		return nil
	}
	if final.LessThan(floor) {
		final = floor
	}
	final = final.Round(2)
	if final.GreaterThanOrEqual(original) {
		// 折后价不低于原价说明活动无效
		return nil
	}
	finalF := final.InexactFloat64()
	discount := original.Sub(final).Round(2).InexactFloat64()
	if discount < 0 {
		discount = 0
	}
	return &SubscriptionDiscount{
		Promotion:      *p,
		Rule:           *rule,
		OriginalPrice:  planPrice,
		FinalPrice:     finalF,
		DiscountAmount: discount,
	}
}

// rechargeBetter 比较两个充值候选：
//
// 用户视角"省钱量"=  reduce_pay 模式的 DiscountAmount；
//                   bonus_credit 模式的 BonusAmount；
// 两者数值都是"用户多得到的价值"，可直接比较。
//
// 若节省一致，则比较 promotion.Priority 高的优先。
func rechargeBetter(candidate, current *RechargeDiscount) bool {
	cs := candidate.DiscountAmount + candidate.BonusAmount
	bs := current.DiscountAmount + current.BonusAmount
	if cs != bs {
		return cs > bs
	}
	return candidate.Promotion.Priority > current.Promotion.Priority
}

func subscriptionBetter(candidate, current *SubscriptionDiscount) bool {
	if candidate.FinalPrice != current.FinalPrice {
		return candidate.FinalPrice < current.FinalPrice
	}
	return candidate.Promotion.Priority > current.Promotion.Priority
}
