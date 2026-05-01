package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// Promotion 相关错误
var (
	ErrPromotionNotFound = infraerrors.NotFound("PROMOTION_NOT_FOUND", "promotion not found")
	ErrPromotionInUse    = infraerrors.Conflict("PROMOTION_IN_USE", "promotion has in-progress orders and cannot be deleted")
	ErrPromotionDisabled = infraerrors.BadRequest("PROMOTION_DISABLED", "promotion is disabled")
)

// CreatePromotionInput 新建活动输入
type CreatePromotionInput struct {
	Name              string
	Description       string
	PromotionType     string
	DiscountMode      string
	RechargeRate      *float64
	RechargeBonusRate *float64
	MaxUsesPerUser    int
	StartsAt          *time.Time
	EndsAt            *time.Time
	Status            string
	Priority          int
	PlanRules         []PromotionPlanRuleInput
}

// UpdatePromotionInput 更新活动输入（PATCH 语义；nil 字段保持原值）
type UpdatePromotionInput struct {
	Name              *string
	Description       *string
	DiscountMode      *string
	RechargeRate      *float64
	RechargeBonusRate *float64
	ClearRechargeRate bool // 设为 true 表示明确清空
	ClearBonusRate    bool
	MaxUsesPerUser    *int
	StartsAt          *time.Time
	ClearStartsAt     bool
	EndsAt            *time.Time
	ClearEndsAt       bool
	Status            *string
	Priority          *int
	PlanRules         *[]PromotionPlanRuleInput // nil 表示不更新；空 slice 表示清空
}

// PromotionPlanRuleInput plan 规则输入
type PromotionPlanRuleInput struct {
	PlanID         int64
	DiscountMode   string
	DiscountRate   *float64
	DiscountAmount *float64
	MinPriceFloor  float64
	MaxUsesPerUser int
}

// PromotionService 促销活动服务
type PromotionService struct {
	repo PromotionRepository
}

// NewPromotionService 构造函数
func NewPromotionService(repo PromotionRepository) *PromotionService {
	return &PromotionService{repo: repo}
}

// Create 新建活动
func (s *PromotionService) Create(ctx context.Context, input CreatePromotionInput) (*Promotion, error) {
	p, err := buildCreatePromotion(input)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("create promotion: %w", err)
	}
	return s.repo.GetByID(ctx, p.ID)
}

// Update 更新活动
func (s *PromotionService) Update(ctx context.Context, id int64, input UpdatePromotionInput) (*Promotion, error) {
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	applyUpdatePromotion(current, input)
	if err := validatePromotion(current); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, current); err != nil {
		return nil, fmt.Errorf("update promotion: %w", err)
	}
	return s.repo.GetByID(ctx, id)
}

// Delete 删除活动（仅 disabled 且无未完成订单引用）
func (s *PromotionService) Delete(ctx context.Context, id int64) error {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p.Status != domain.PromotionStatusDisabled {
		return infraerrors.BadRequest("PROMOTION_NOT_DISABLED", "only disabled promotions can be deleted; please disable first")
	}
	count, err := s.repo.CountActiveOrdersByPromotion(ctx, id)
	if err != nil {
		return fmt.Errorf("count promotion in-progress orders: %w", err)
	}
	if count > 0 {
		return ErrPromotionInUse
	}
	return s.repo.Delete(ctx, id)
}

// SetStatus 启用/禁用
func (s *PromotionService) SetStatus(ctx context.Context, id int64, status string) (*Promotion, error) {
	if status != domain.PromotionStatusActive && status != domain.PromotionStatusDisabled {
		return nil, infraerrors.BadRequest("INVALID_STATUS", "status must be active or disabled")
	}
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	current.Status = status
	if err := s.repo.Update(ctx, current); err != nil {
		return nil, fmt.Errorf("update promotion status: %w", err)
	}
	return s.repo.GetByID(ctx, id)
}

// GetByID 获取活动详情
func (s *PromotionService) GetByID(ctx context.Context, id int64) (*Promotion, error) {
	return s.repo.GetByID(ctx, id)
}

// List 列表
func (s *PromotionService) List(ctx context.Context, params pagination.PaginationParams, filter PromotionListFilter) ([]Promotion, *pagination.PaginationResult, error) {
	return s.repo.List(ctx, params, filter)
}

// ListUsages 使用记录
func (s *PromotionService) ListUsages(ctx context.Context, id int64, params pagination.PaginationParams) ([]PromotionUsage, *pagination.PaginationResult, error) {
	return s.repo.ListUsagesByPromotion(ctx, id, params)
}

// --- helpers ---

func buildCreatePromotion(input CreatePromotionInput) (*Promotion, error) {
	p := &Promotion{
		Name:              strings.TrimSpace(input.Name),
		Description:       input.Description,
		PromotionType:     input.PromotionType,
		DiscountMode:      input.DiscountMode,
		RechargeRate:      input.RechargeRate,
		RechargeBonusRate: input.RechargeBonusRate,
		MaxUsesPerUser:    input.MaxUsesPerUser,
		StartsAt:          input.StartsAt,
		EndsAt:            input.EndsAt,
		Status:            input.Status,
		Priority:          input.Priority,
	}
	if p.Status == "" {
		p.Status = domain.PromotionStatusActive
	}
	for _, rule := range input.PlanRules {
		p.PlanRules = append(p.PlanRules, PromotionPlanRule{
			PlanID:         rule.PlanID,
			DiscountMode:   rule.DiscountMode,
			DiscountRate:   rule.DiscountRate,
			DiscountAmount: rule.DiscountAmount,
			MinPriceFloor:  rule.MinPriceFloor,
			MaxUsesPerUser: rule.MaxUsesPerUser,
		})
	}
	if err := validatePromotion(p); err != nil {
		return nil, err
	}
	return p, nil
}

func applyUpdatePromotion(p *Promotion, input UpdatePromotionInput) {
	if input.Name != nil {
		p.Name = strings.TrimSpace(*input.Name)
	}
	if input.Description != nil {
		p.Description = *input.Description
	}
	if input.DiscountMode != nil {
		p.DiscountMode = *input.DiscountMode
	}
	if input.ClearRechargeRate {
		p.RechargeRate = nil
	} else if input.RechargeRate != nil {
		v := *input.RechargeRate
		p.RechargeRate = &v
	}
	if input.ClearBonusRate {
		p.RechargeBonusRate = nil
	} else if input.RechargeBonusRate != nil {
		v := *input.RechargeBonusRate
		p.RechargeBonusRate = &v
	}
	if input.MaxUsesPerUser != nil {
		p.MaxUsesPerUser = *input.MaxUsesPerUser
	}
	if input.ClearStartsAt {
		p.StartsAt = nil
	} else if input.StartsAt != nil {
		v := *input.StartsAt
		p.StartsAt = &v
	}
	if input.ClearEndsAt {
		p.EndsAt = nil
	} else if input.EndsAt != nil {
		v := *input.EndsAt
		p.EndsAt = &v
	}
	if input.Status != nil {
		p.Status = *input.Status
	}
	if input.Priority != nil {
		p.Priority = *input.Priority
	}
	if input.PlanRules != nil {
		rules := make([]PromotionPlanRule, 0, len(*input.PlanRules))
		for _, rule := range *input.PlanRules {
			rules = append(rules, PromotionPlanRule{
				PlanID:         rule.PlanID,
				DiscountMode:   rule.DiscountMode,
				DiscountRate:   rule.DiscountRate,
				DiscountAmount: rule.DiscountAmount,
				MinPriceFloor:  rule.MinPriceFloor,
				MaxUsesPerUser: rule.MaxUsesPerUser,
			})
		}
		p.PlanRules = rules
	}
}

// validatePromotion 校验活动参数完整性
func validatePromotion(p *Promotion) error {
	if p == nil {
		return infraerrors.BadRequest("INVALID_INPUT", "promotion is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return infraerrors.BadRequest("PROMOTION_NAME_REQUIRED", "promotion name is required")
	}
	switch p.PromotionType {
	case domain.PromotionTypeRecharge, domain.PromotionTypeSubscription:
	default:
		return infraerrors.BadRequest("INVALID_PROMOTION_TYPE", "promotion_type must be recharge or subscription")
	}
	if p.Status == "" {
		p.Status = domain.PromotionStatusActive
	}
	if p.Status != domain.PromotionStatusActive && p.Status != domain.PromotionStatusDisabled {
		return infraerrors.BadRequest("INVALID_STATUS", "status must be active or disabled")
	}
	if p.MaxUsesPerUser < 0 {
		return infraerrors.BadRequest("INVALID_LIMIT", "max_uses_per_user must be >= 0")
	}
	if p.StartsAt != nil && p.EndsAt != nil && !p.EndsAt.After(*p.StartsAt) {
		return infraerrors.BadRequest("INVALID_TIME_WINDOW", "ends_at must be after starts_at")
	}

	if p.IsRecharge() {
		return validateRechargePromotion(p)
	}
	return validateSubscriptionPromotion(p)
}

func validateRechargePromotion(p *Promotion) error {
	switch p.DiscountMode {
	case domain.PromotionDiscountModeReducePay:
		if p.RechargeRate == nil {
			return infraerrors.BadRequest("RECHARGE_RATE_REQUIRED", "recharge_rate is required for reduce_pay mode")
		}
		v := *p.RechargeRate
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > 1 {
			return infraerrors.BadRequest("INVALID_RECHARGE_RATE", "recharge_rate must be in (0, 1]")
		}
	case domain.PromotionDiscountModeBonusCredit:
		if p.RechargeBonusRate == nil {
			return infraerrors.BadRequest("RECHARGE_BONUS_RATE_REQUIRED", "recharge_bonus_rate is required for bonus_credit mode")
		}
		v := *p.RechargeBonusRate
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 1 {
			return infraerrors.BadRequest("INVALID_RECHARGE_BONUS_RATE", "recharge_bonus_rate must be >= 1")
		}
	default:
		return infraerrors.BadRequest("INVALID_DISCOUNT_MODE", "recharge promotion requires discount_mode reduce_pay or bonus_credit")
	}
	if len(p.PlanRules) > 0 {
		return infraerrors.BadRequest("INVALID_PLAN_RULES", "recharge promotion must not have plan rules")
	}
	return nil
}

func validateSubscriptionPromotion(p *Promotion) error {
	if p.DiscountMode != "" {
		return infraerrors.BadRequest("INVALID_DISCOUNT_MODE", "subscription promotion must leave discount_mode empty (defined per plan)")
	}
	if p.RechargeRate != nil || p.RechargeBonusRate != nil {
		return infraerrors.BadRequest("INVALID_INPUT", "subscription promotion must not set recharge_rate / recharge_bonus_rate")
	}
	if len(p.PlanRules) == 0 {
		return infraerrors.BadRequest("PLAN_RULES_REQUIRED", "subscription promotion requires at least one plan rule")
	}
	seen := make(map[int64]bool)
	for i := range p.PlanRules {
		rule := &p.PlanRules[i]
		if rule.PlanID <= 0 {
			return infraerrors.BadRequest("INVALID_PLAN_ID", "plan rule plan_id must be > 0")
		}
		if seen[rule.PlanID] {
			return infraerrors.BadRequest("DUPLICATE_PLAN_RULE", fmt.Sprintf("duplicate plan rule for plan_id=%d", rule.PlanID))
		}
		seen[rule.PlanID] = true
		if err := validatePlanRule(rule); err != nil {
			return err
		}
	}
	return nil
}

func validatePlanRule(rule *PromotionPlanRule) error {
	switch rule.DiscountMode {
	case domain.PromotionDiscountModeRate:
		if rule.DiscountRate == nil {
			return infraerrors.BadRequest("DISCOUNT_RATE_REQUIRED", fmt.Sprintf("plan %d: discount_rate is required for rate mode", rule.PlanID))
		}
		v := *rule.DiscountRate
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > 1 {
			return infraerrors.BadRequest("INVALID_DISCOUNT_RATE", fmt.Sprintf("plan %d: discount_rate must be in (0, 1]", rule.PlanID))
		}
	case domain.PromotionDiscountModeAmount:
		if rule.DiscountAmount == nil {
			return infraerrors.BadRequest("DISCOUNT_AMOUNT_REQUIRED", fmt.Sprintf("plan %d: discount_amount is required for amount mode", rule.PlanID))
		}
		v := *rule.DiscountAmount
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return infraerrors.BadRequest("INVALID_DISCOUNT_AMOUNT", fmt.Sprintf("plan %d: discount_amount must be >= 0", rule.PlanID))
		}
	default:
		return infraerrors.BadRequest("INVALID_DISCOUNT_MODE", fmt.Sprintf("plan %d: discount_mode must be rate or amount", rule.PlanID))
	}
	if rule.MinPriceFloor < 0 {
		return infraerrors.BadRequest("INVALID_MIN_PRICE_FLOOR", fmt.Sprintf("plan %d: min_price_floor must be >= 0", rule.PlanID))
	}
	if rule.MaxUsesPerUser < 0 {
		return infraerrors.BadRequest("INVALID_LIMIT", fmt.Sprintf("plan %d: max_uses_per_user must be >= 0", rule.PlanID))
	}
	return nil
}
