package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

func RegisterModelPricingRoutes(v1 *gin.RouterGroup, h *handler.Handlers) {
	if h == nil || h.ModelPricing == nil {
		return
	}
	modelPricing := v1.Group("/model-pricing")
	{
		modelPricing.GET("/models", h.ModelPricing.ListModels)
		modelPricing.GET("/models/:model", h.ModelPricing.GetModel)
	}
}
