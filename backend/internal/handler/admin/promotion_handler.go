package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// PromotionHandler 管理端促销活动管理
type PromotionHandler struct {
	promotionService *service.PromotionService
}

// NewPromotionHandler 构造函数
func NewPromotionHandler(promotionService *service.PromotionService) *PromotionHandler {
	return &PromotionHandler{promotionService: promotionService}
}

// --- request DTOs ---

type promotionPlanRuleRequest struct {
	PlanID         int64    `json:"plan_id" binding:"required"`
	DiscountMode   string   `json:"discount_mode" binding:"required,oneof=rate amount"`
	DiscountRate   *float64 `json:"discount_rate"`
	DiscountAmount *float64 `json:"discount_amount"`
	MinPriceFloor  float64  `json:"min_price_floor"`
	MaxUsesPerUser int      `json:"max_uses_per_user"`
}

// CreatePromotionRequest 创建活动请求
type CreatePromotionRequest struct {
	Name              string                     `json:"name" binding:"required,max=128"`
	Description       string                     `json:"description"`
	PromotionType     string                     `json:"promotion_type" binding:"required,oneof=recharge subscription"`
	DiscountMode      string                     `json:"discount_mode"`
	RechargeRate      *float64                   `json:"recharge_rate"`
	RechargeBonusRate *float64                   `json:"recharge_bonus_rate"`
	MaxUsesPerUser    int                        `json:"max_uses_per_user"`
	StartsAt          *int64                     `json:"starts_at"`
	EndsAt            *int64                     `json:"ends_at"`
	Status            string                     `json:"status" binding:"omitempty,oneof=active disabled"`
	Priority          int                        `json:"priority"`
	PlanRules         []promotionPlanRuleRequest `json:"plan_rules"`
}

// UpdatePromotionRequest 更新活动请求（nil 表示不变）
type UpdatePromotionRequest struct {
	Name              *string                     `json:"name"`
	Description       *string                     `json:"description"`
	DiscountMode      *string                     `json:"discount_mode"`
	RechargeRate      *float64                    `json:"recharge_rate"`
	ClearRechargeRate bool                        `json:"clear_recharge_rate"`
	RechargeBonusRate *float64                    `json:"recharge_bonus_rate"`
	ClearBonusRate    bool                        `json:"clear_recharge_bonus_rate"`
	MaxUsesPerUser    *int                        `json:"max_uses_per_user"`
	StartsAt          *int64                      `json:"starts_at"`
	ClearStartsAt     bool                        `json:"clear_starts_at"`
	EndsAt            *int64                      `json:"ends_at"`
	ClearEndsAt       bool                        `json:"clear_ends_at"`
	Status            *string                     `json:"status" binding:"omitempty,oneof=active disabled"`
	Priority          *int                        `json:"priority"`
	PlanRules         *[]promotionPlanRuleRequest `json:"plan_rules"`
}

// SetPromotionStatusRequest 启用/禁用请求
type SetPromotionStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active disabled"`
}

// --- handlers ---

// List GET /api/v1/admin/promotions
func (h *PromotionHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "priority"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}
	filter := service.PromotionListFilter{
		PromotionType: strings.TrimSpace(c.Query("promotion_type")),
		Status:        strings.TrimSpace(c.Query("status")),
		Search:        strings.TrimSpace(c.Query("search")),
	}
	if len(filter.Search) > 100 {
		filter.Search = filter.Search[:100]
	}

	promotions, pageResult, err := h.promotionService.List(c.Request.Context(), params, filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.Promotion, 0, len(promotions))
	for i := range promotions {
		out = append(out, *dto.PromotionFromService(&promotions[i]))
	}
	response.Paginated(c, out, pageResult.Total, page, pageSize)
}

// GetByID GET /api/v1/admin/promotions/:id
func (h *PromotionHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid promotion ID")
		return
	}
	p, err := h.promotionService.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.PromotionFromService(p))
}

// Create POST /api/v1/admin/promotions
func (h *PromotionHandler) Create(c *gin.Context) {
	var req CreatePromotionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	input := service.CreatePromotionInput{
		Name:              req.Name,
		Description:       req.Description,
		PromotionType:     req.PromotionType,
		DiscountMode:      req.DiscountMode,
		RechargeRate:      req.RechargeRate,
		RechargeBonusRate: req.RechargeBonusRate,
		MaxUsesPerUser:    req.MaxUsesPerUser,
		Status:            req.Status,
		Priority:          req.Priority,
		PlanRules:         convertPlanRuleRequests(req.PlanRules),
	}
	if req.StartsAt != nil {
		t := time.Unix(*req.StartsAt, 0)
		input.StartsAt = &t
	}
	if req.EndsAt != nil {
		t := time.Unix(*req.EndsAt, 0)
		input.EndsAt = &t
	}
	p, err := h.promotionService.Create(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.PromotionFromService(p))
}

// Update PUT /api/v1/admin/promotions/:id
func (h *PromotionHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid promotion ID")
		return
	}
	var req UpdatePromotionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	input := service.UpdatePromotionInput{
		Name:              req.Name,
		Description:       req.Description,
		DiscountMode:      req.DiscountMode,
		RechargeRate:      req.RechargeRate,
		ClearRechargeRate: req.ClearRechargeRate,
		RechargeBonusRate: req.RechargeBonusRate,
		ClearBonusRate:    req.ClearBonusRate,
		MaxUsesPerUser:    req.MaxUsesPerUser,
		Status:            req.Status,
		Priority:          req.Priority,
		ClearStartsAt:     req.ClearStartsAt,
		ClearEndsAt:       req.ClearEndsAt,
	}
	if req.StartsAt != nil {
		t := time.Unix(*req.StartsAt, 0)
		input.StartsAt = &t
	}
	if req.EndsAt != nil {
		t := time.Unix(*req.EndsAt, 0)
		input.EndsAt = &t
	}
	if req.PlanRules != nil {
		rules := convertPlanRuleRequests(*req.PlanRules)
		input.PlanRules = &rules
	}
	p, err := h.promotionService.Update(c.Request.Context(), id, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.PromotionFromService(p))
}

// Delete DELETE /api/v1/admin/promotions/:id
func (h *PromotionHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid promotion ID")
		return
	}
	if err := h.promotionService.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Promotion deleted successfully"})
}

// SetStatus POST /api/v1/admin/promotions/:id/status
func (h *PromotionHandler) SetStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid promotion ID")
		return
	}
	var req SetPromotionStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	p, err := h.promotionService.SetStatus(c.Request.Context(), id, req.Status)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.PromotionFromService(p))
}

// Usages GET /api/v1/admin/promotions/:id/usages
func (h *PromotionHandler) Usages(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid promotion ID")
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	usages, pageResult, err := h.promotionService.ListUsages(c.Request.Context(), id, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.PromotionUsage, 0, len(usages))
	for i := range usages {
		out = append(out, *dto.PromotionUsageFromService(&usages[i]))
	}
	response.Paginated(c, out, pageResult.Total, page, pageSize)
}

// --- helpers ---

func convertPlanRuleRequests(rules []promotionPlanRuleRequest) []service.PromotionPlanRuleInput {
	if len(rules) == 0 {
		return nil
	}
	out := make([]service.PromotionPlanRuleInput, 0, len(rules))
	for _, r := range rules {
		out = append(out, service.PromotionPlanRuleInput{
			PlanID:         r.PlanID,
			DiscountMode:   r.DiscountMode,
			DiscountRate:   r.DiscountRate,
			DiscountAmount: r.DiscountAmount,
			MinPriceFloor:  r.MinPriceFloor,
			MaxUsesPerUser: r.MaxUsesPerUser,
		})
	}
	return out
}
