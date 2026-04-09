package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterPricingPlanRoutes registers public pricing page endpoints (no auth).
func RegisterPricingPlanRoutes(v1 *gin.RouterGroup, h *handler.Handlers) {
	if h == nil || h.PricingPlan == nil {
		return
	}
	pricing := v1.Group("/pricing")
	{
		pricing.GET("/plan-groups", h.PricingPlan.ListPublicGroups)
	}
}
