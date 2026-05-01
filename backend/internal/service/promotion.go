package service

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// Promotion 促销活动领域类型（充值/订阅）
type Promotion struct {
	ID                int64
	Name              string
	Description       string
	PromotionType     string  // domain.PromotionType*
	DiscountMode      string  // 充值时为 reduce_pay/bonus_credit；订阅留空
	RechargeRate      *float64 // reduce_pay 时使用
	RechargeBonusRate *float64 // bonus_credit 时使用
	MaxUsesPerUser    int      // 0 = 不限
	StartsAt          *time.Time
	EndsAt            *time.Time
	Status            string // domain.PromotionStatus*
	Priority          int
	CreatedAt         time.Time
	UpdatedAt         time.Time

	// 关联
	PlanRules []PromotionPlanRule
}

// PromotionPlanRule 订阅活动按 plan 配置的折扣规则
type PromotionPlanRule struct {
	ID              int64
	PromotionID     int64
	PlanID          int64
	DiscountMode    string   // domain.PromotionDiscountModeRate / Amount
	DiscountRate    *float64 // rate 模式
	DiscountAmount  *float64 // amount 模式
	MinPriceFloor   float64
	MaxUsesPerUser  int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// PromotionUsage 用户使用活动的记录
type PromotionUsage struct {
	ID             int64
	PromotionID    int64
	PlanID         *int64
	UserID         int64
	OrderID        int64
	DiscountAmount float64
	BonusAmount    float64
	UsedAt         time.Time
}

// IsActive 活动当前是否处于"激活 + 在时间窗口内"
func (p *Promotion) IsActive(now time.Time) bool {
	if p == nil {
		return false
	}
	if p.Status != domain.PromotionStatusActive {
		return false
	}
	if p.StartsAt != nil && now.Before(*p.StartsAt) {
		return false
	}
	if p.EndsAt != nil && !now.Before(*p.EndsAt) {
		return false
	}
	return true
}

// IsRecharge 判断是否为充值活动
func (p *Promotion) IsRecharge() bool {
	return p != nil && p.PromotionType == domain.PromotionTypeRecharge
}

// IsSubscription 判断是否为订阅活动
func (p *Promotion) IsSubscription() bool {
	return p != nil && p.PromotionType == domain.PromotionTypeSubscription
}

// EffectiveMaxUses 返回针对某 plan 实际生效的限次
// 订阅活动：plan 规则 max>0 优先，否则跟随活动级
// 充值活动：直接返回活动级
func (p *Promotion) EffectiveMaxUses(rule *PromotionPlanRule) int {
	if p == nil {
		return 0
	}
	if rule != nil && rule.MaxUsesPerUser > 0 {
		return rule.MaxUsesPerUser
	}
	return p.MaxUsesPerUser
}
