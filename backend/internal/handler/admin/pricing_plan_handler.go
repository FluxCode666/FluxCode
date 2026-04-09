package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// PricingPlanHandler handles admin pricing plan management.
type PricingPlanHandler struct {
	pricingPlanService *service.PricingPlanService
}

func NewPricingPlanHandler(pricingPlanService *service.PricingPlanService) *PricingPlanHandler {
	return &PricingPlanHandler{pricingPlanService: pricingPlanService}
}

// ListGroups lists all pricing plan groups with plans (including inactive).
// GET /api/v1/admin/pricing/plan-groups
func (h *PricingPlanHandler) ListGroups(c *gin.Context) {
	groups, err := h.pricingPlanService.ListAdminGroups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.PricingPlanGroup, 0, len(groups))
	for i := range groups {
		out = append(out, *dto.PricingPlanGroupFromService(&groups[i]))
	}
	response.Success(c, out)
}

type CreatePricingPlanGroupRequest struct {
	Name        string  `json:"name" binding:"required"`
	Description *string `json:"description"`
	SortOrder   int     `json:"sort_order"`
	Status      string  `json:"status" binding:"omitempty,oneof=active inactive"`
}

// CreateGroup creates a pricing plan group.
// POST /api/v1/admin/pricing/plan-groups
func (h *PricingPlanHandler) CreateGroup(c *gin.Context) {
	var req CreatePricingPlanGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	group, err := h.pricingPlanService.CreateGroup(c.Request.Context(), service.CreatePricingPlanGroupInput{
		Name:        req.Name,
		Description: req.Description,
		SortOrder:   req.SortOrder,
		Status:      req.Status,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.PricingPlanGroupFromService(group))
}

type UpdatePricingPlanGroupRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	SortOrder   *int    `json:"sort_order"`
	Status      *string `json:"status" binding:"omitempty,oneof=active inactive"`
}

// UpdateGroup updates a pricing plan group.
// PUT /api/v1/admin/pricing/plan-groups/:id
func (h *PricingPlanHandler) UpdateGroup(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
	}

	var req UpdatePricingPlanGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	group, err := h.pricingPlanService.UpdateGroup(c.Request.Context(), groupID, service.UpdatePricingPlanGroupInput{
		Name:        req.Name,
		Description: req.Description,
		SortOrder:   req.SortOrder,
		Status:      req.Status,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.PricingPlanGroupFromService(group))
}

// DeleteGroup deletes a pricing plan group (and cascades its plans).
// DELETE /api/v1/admin/pricing/plan-groups/:id
func (h *PricingPlanHandler) DeleteGroup(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
	}

	if err := h.pricingPlanService.DeleteGroup(c.Request.Context(), groupID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Pricing plan group deleted successfully"})
}

type CreatePricingPlanRequest struct {
	GroupID        int64                              `json:"group_id" binding:"required"`
	Name           string                             `json:"name" binding:"required"`
	Description    *string                            `json:"description"`
	IconURL        *string                            `json:"icon_url"`
	BadgeText      *string                            `json:"badge_text"`
	Tagline        *string                            `json:"tagline"`
	PriceAmount    *float64                           `json:"price_amount"`
	PriceCurrency  string                             `json:"price_currency"`
	PricePeriod    string                             `json:"price_period"`
	PriceText      *string                            `json:"price_text"`
	Features       []string                           `json:"features"`
	ContactMethods []service.PricingPlanContactMethod `json:"contact_methods"`
	IsFeatured     bool                               `json:"is_featured"`
	SortOrder      int                                `json:"sort_order"`
	Status         string                             `json:"status" binding:"omitempty,oneof=active inactive"`
}

// CreatePlan creates a pricing plan.
// POST /api/v1/admin/pricing/plans
func (h *PricingPlanHandler) CreatePlan(c *gin.Context) {
	var req CreatePricingPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	plan, err := h.pricingPlanService.CreatePlan(c.Request.Context(), service.CreatePricingPlanInput{
		GroupID:        req.GroupID,
		Name:           req.Name,
		Description:    req.Description,
		IconURL:        req.IconURL,
		BadgeText:      req.BadgeText,
		Tagline:        req.Tagline,
		PriceAmount:    req.PriceAmount,
		PriceCurrency:  req.PriceCurrency,
		PricePeriod:    req.PricePeriod,
		PriceText:      req.PriceText,
		Features:       req.Features,
		ContactMethods: req.ContactMethods,
		IsFeatured:     req.IsFeatured,
		SortOrder:      req.SortOrder,
		Status:         req.Status,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.PricingPlanFromService(plan))
}

type UpdatePricingPlanRequest struct {
	GroupID        *int64                              `json:"group_id"`
	Name           *string                             `json:"name"`
	Description    *string                             `json:"description"`
	IconURL        *string                             `json:"icon_url"`
	BadgeText      *string                             `json:"badge_text"`
	Tagline        *string                             `json:"tagline"`
	PriceAmount    *float64                            `json:"price_amount"`
	PriceCurrency  *string                             `json:"price_currency"`
	PricePeriod    *string                             `json:"price_period"`
	PriceText      *string                             `json:"price_text"`
	Features       *[]string                           `json:"features"`
	ContactMethods *[]service.PricingPlanContactMethod `json:"contact_methods"`
	IsFeatured     *bool                               `json:"is_featured"`
	SortOrder      *int                                `json:"sort_order"`
	Status         *string                             `json:"status" binding:"omitempty,oneof=active inactive"`
}

// UpdatePlan updates a pricing plan.
// PUT /api/v1/admin/pricing/plans/:id
func (h *PricingPlanHandler) UpdatePlan(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID")
		return
	}

	var req UpdatePricingPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	plan, err := h.pricingPlanService.UpdatePlan(c.Request.Context(), planID, service.UpdatePricingPlanInput{
		GroupID:        req.GroupID,
		Name:           req.Name,
		Description:    req.Description,
		IconURL:        req.IconURL,
		BadgeText:      req.BadgeText,
		Tagline:        req.Tagline,
		PriceAmount:    req.PriceAmount,
		PriceCurrency:  req.PriceCurrency,
		PricePeriod:    req.PricePeriod,
		PriceText:      req.PriceText,
		Features:       req.Features,
		ContactMethods: req.ContactMethods,
		IsFeatured:     req.IsFeatured,
		SortOrder:      req.SortOrder,
		Status:         req.Status,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.PricingPlanFromService(plan))
}

// DeletePlan deletes a pricing plan.
// DELETE /api/v1/admin/pricing/plans/:id
func (h *PricingPlanHandler) DeletePlan(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID")
		return
	}

	if err := h.pricingPlanService.DeletePlan(c.Request.Context(), planID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Pricing plan deleted successfully"})
}
