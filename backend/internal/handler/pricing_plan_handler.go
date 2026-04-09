package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// PricingPlanHandler provides public endpoints for the pricing page (no auth).
type PricingPlanHandler struct {
	pricingPlanService *service.PricingPlanService
}

func NewPricingPlanHandler(pricingPlanService *service.PricingPlanService) *PricingPlanHandler {
	return &PricingPlanHandler{pricingPlanService: pricingPlanService}
}

// ListPublicGroups returns active pricing plan groups (with active plans).
// GET /api/v1/pricing/plan-groups
func (h *PricingPlanHandler) ListPublicGroups(c *gin.Context) {
	groups, err := h.pricingPlanService.ListPublicGroups(c.Request.Context())
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
