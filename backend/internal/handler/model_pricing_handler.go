package handler

import (
	"errors"
	"net/http"
	"strconv"
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
	performanceRange, err := parseModelPricingPerformanceRange(c.Query("range"))
	if err != nil {
		response.ErrorWithDetails(c, http.StatusBadRequest, "性能时间范围只能为 24h 或 7d", "MODEL_PERFORMANCE_RANGE_INVALID", nil)
		return
	}
	models, err := h.service.ListModels(c.Request.Context(), service.ModelPricingQuery{
		Q:                strings.TrimSpace(c.Query("q")),
		Platform:         strings.TrimSpace(c.Query("platform")),
		Capability:       strings.TrimSpace(c.Query("capability")),
		GroupID:          parseModelPricingGroupID(c.Query("group_id")),
		PerformanceRange: performanceRange,
	})
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("MODEL_PRICING_QUERY_FAILED", "模型定价查询失败"))
		return
	}
	response.Success(c, models)
}

func (h *ModelPricingHandler) ListGroups(c *gin.Context) {
	groups, err := h.service.ListGroups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("MODEL_PRICING_QUERY_FAILED", "模型定价查询失败"))
		return
	}
	response.Success(c, groups)
}

func (h *ModelPricingHandler) GetModel(c *gin.Context) {
	performanceRange, err := parseModelPricingPerformanceRange(c.Query("range"))
	if err != nil {
		response.ErrorWithDetails(c, http.StatusBadRequest, "性能时间范围只能为 24h 或 7d", "MODEL_PERFORMANCE_RANGE_INVALID", nil)
		return
	}
	modelID := strings.TrimSpace(c.Query("model"))
	if modelID == "" {
		modelID = strings.TrimSpace(c.Param("model"))
	}
	model, err := h.service.GetModelWithRange(c.Request.Context(), modelID, performanceRange)
	if err != nil {
		if errors.Is(err, service.ErrModelPricingNotFound) {
			response.ErrorWithDetails(c, http.StatusNotFound, "模型定价不存在", "MODEL_PRICING_NOT_FOUND", nil)
			return
		}
		response.ErrorFrom(c, infraerrors.InternalServer("MODEL_PRICING_QUERY_FAILED", "模型定价查询失败"))
		return
	}
	response.Success(c, model)
}

func parseModelPricingPerformanceRange(value string) (service.ModelPerformanceRange, error) {
	return service.ParseModelPerformanceRange(value)
}

func parseModelPricingGroupID(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	groupID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || groupID < 0 {
		return 0
	}
	return groupID
}
