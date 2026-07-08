package handler

import (
	"errors"
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ModelPricingHandler struct {
	service *service.ModelPricingPageService
}

func NewModelPricingHandler(service *service.ModelPricingPageService) *ModelPricingHandler {
	return &ModelPricingHandler{service: service}
}

func (h *ModelPricingHandler) ListModels(c *gin.Context) {
	models, err := h.service.ListModels(c.Request.Context(), service.ModelPricingQuery{
		Q:          strings.TrimSpace(c.Query("q")),
		Platform:   strings.TrimSpace(c.Query("platform")),
		Capability: strings.TrimSpace(c.Query("capability")),
	})
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("MODEL_PRICING_QUERY_FAILED", "model pricing query failed"))
		return
	}
	response.Success(c, models)
}

func (h *ModelPricingHandler) GetModel(c *gin.Context) {
	model, err := h.service.GetModel(c.Request.Context(), c.Param("model"))
	if err != nil {
		if errors.Is(err, service.ErrModelPricingNotFound) {
			response.ErrorWithDetails(c, http.StatusNotFound, "model pricing not found", "MODEL_PRICING_NOT_FOUND", nil)
			return
		}
		response.ErrorFrom(c, infraerrors.InternalServer("MODEL_PRICING_QUERY_FAILED", "model pricing query failed"))
		return
	}
	response.Success(c, model)
}
