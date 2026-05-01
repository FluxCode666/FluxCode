package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// PromotionListFilter 列表筛选条件
type PromotionListFilter struct {
	PromotionType string
	Status        string
	Search        string
}

// PromotionRepository 促销活动仓储接口
type PromotionRepository interface {
	// 基础 CRUD（含子规则）
	Create(ctx context.Context, p *Promotion) error
	GetByID(ctx context.Context, id int64) (*Promotion, error)
	GetByIDForUpdate(ctx context.Context, id int64) (*Promotion, error)
	Update(ctx context.Context, p *Promotion) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params pagination.PaginationParams, filter PromotionListFilter) ([]Promotion, *pagination.PaginationResult, error)

	// resolver 用：列出某类型当前激活的活动
	ListActiveByType(ctx context.Context, promotionType string) ([]Promotion, error)
	// resolver 用：列出包含指定 plan 的激活订阅活动
	ListActiveByPlanID(ctx context.Context, planID int64) ([]Promotion, error)

	// 子规则
	ReplacePlanRules(ctx context.Context, promotionID int64, rules []PromotionPlanRule) error
	ListPlanRulesByPromotionID(ctx context.Context, promotionID int64) ([]PromotionPlanRule, error)
	ListPlanRulesByPromotionIDs(ctx context.Context, promotionIDs []int64) (map[int64][]PromotionPlanRule, error)

	// 使用记录
	CreateUsage(ctx context.Context, usage *PromotionUsage) error
	CountUsageByUser(ctx context.Context, promotionID int64, planID *int64, userID int64) (int, error)
	ListUsagesByPromotion(ctx context.Context, promotionID int64, params pagination.PaginationParams) ([]PromotionUsage, *pagination.PaginationResult, error)
	DeleteUsagesByOrderID(ctx context.Context, orderID int64) error

	// 引用统计：删除前判断是否有未完成订单引用
	CountActiveOrdersByPromotion(ctx context.Context, promotionID int64) (int, error)
}
