package admin

import (
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type GrowthHandler struct {
	growthService *service.GrowthService
}

func NewGrowthHandler(growthService *service.GrowthService) *GrowthHandler {
	return &GrowthHandler{growthService: growthService}
}

func (h *GrowthHandler) growth() *service.GrowthService {
	if h == nil || h.growthService == nil {
		return service.NewGrowthService(nil)
	}
	return h.growthService
}

func (h *GrowthHandler) parseRange(c *gin.Context) (service.GrowthQueryRange, bool) {
	r, err := service.ParseGrowthQueryRange(
		c.Query("start_date"),
		c.Query("end_date"),
		c.Query("granularity"),
		time.Now(),
	)
	if err != nil {
		if errors.Is(err, service.ErrGrowthInvalidQueryRange) {
			response.BadRequest(c, err.Error())
			return service.GrowthQueryRange{}, false
		}
		response.BadRequest(c, err.Error())
		return service.GrowthQueryRange{}, false
	}
	return r, true
}

func (h *GrowthHandler) handleRange(c *gin.Context, load func(service.GrowthQueryRange) (any, error), errorMessage string) {
	r, ok := h.parseRange(c)
	if !ok {
		return
	}
	payload, err := load(r)
	if err != nil {
		response.InternalError(c, errorMessage)
		return
	}
	response.Success(c, payload)
}

func (h *GrowthHandler) GetOverview(c *gin.Context) {
	h.handleRange(c, func(r service.GrowthQueryRange) (any, error) {
		return h.growth().GetOverview(c.Request.Context(), r)
	}, "Failed to get growth overview")
}

func (h *GrowthHandler) GetUserTrend(c *gin.Context) {
	h.handleRange(c, func(r service.GrowthQueryRange) (any, error) {
		out, err := h.growth().GetUserTrend(c.Request.Context(), r)
		if err != nil {
			return nil, err
		}
		return gin.H{"series": out}, nil
	}, "Failed to get growth user trend")
}

func (h *GrowthHandler) GetUserSources(c *gin.Context) {
	h.handleRange(c, func(r service.GrowthQueryRange) (any, error) {
		out, err := h.growth().GetUserSources(c.Request.Context(), r)
		if err != nil {
			return nil, err
		}
		return gin.H{"items": out}, nil
	}, "Failed to get growth user sources")
}

func (h *GrowthHandler) GetSourcePaymentRates(c *gin.Context) {
	h.handleRange(c, func(r service.GrowthQueryRange) (any, error) {
		out, err := h.growth().GetSourcePaymentRates(c.Request.Context(), r)
		if err != nil {
			return nil, err
		}
		return gin.H{"items": out}, nil
	}, "Failed to get growth source payment rates")
}

func (h *GrowthHandler) GetRetentionMatrix(c *gin.Context) {
	h.handleRange(c, func(r service.GrowthQueryRange) (any, error) {
		return h.growth().GetRetentionMatrix(c.Request.Context(), r)
	}, "Failed to get growth retention matrix")
}

func (h *GrowthHandler) GetRetentionTrend(c *gin.Context) {
	h.handleRange(c, func(r service.GrowthQueryRange) (any, error) {
		out, err := h.growth().GetRetentionTrend(c.Request.Context(), r)
		if err != nil {
			return nil, err
		}
		return gin.H{"series": out}, nil
	}, "Failed to get growth retention trend")
}

func (h *GrowthHandler) GetPaymentFunnel(c *gin.Context) {
	h.handleRange(c, func(r service.GrowthQueryRange) (any, error) {
		return h.growth().GetPaymentFunnel(c.Request.Context(), r)
	}, "Failed to get growth payment funnel")
}

func (h *GrowthHandler) GetPaymentPlans(c *gin.Context) {
	h.handleRange(c, func(r service.GrowthQueryRange) (any, error) {
		out, err := h.growth().GetPaymentPlans(c.Request.Context(), r)
		if err != nil {
			return nil, err
		}
		return gin.H{"items": out}, nil
	}, "Failed to get growth payment plans")
}

func (h *GrowthHandler) GetFirstPayment(c *gin.Context) {
	h.handleRange(c, func(r service.GrowthQueryRange) (any, error) {
		out, err := h.growth().GetFirstPaymentBuckets(c.Request.Context(), r)
		if err != nil {
			return nil, err
		}
		return gin.H{"items": out}, nil
	}, "Failed to get growth first payment")
}

func (h *GrowthHandler) GetFeatureRanking(c *gin.Context) {
	h.handleRange(c, func(r service.GrowthQueryRange) (any, error) {
		out, err := h.growth().GetFeatureRanking(c.Request.Context(), r)
		if err != nil {
			return nil, err
		}
		return gin.H{"items": out}, nil
	}, "Failed to get growth feature ranking")
}

func (h *GrowthHandler) GetSessionMetrics(c *gin.Context) {
	h.handleRange(c, func(r service.GrowthQueryRange) (any, error) {
		return h.growth().GetSessionMetrics(c.Request.Context(), r)
	}, "Failed to get growth session metrics")
}

func (h *GrowthHandler) GetAudienceDevices(c *gin.Context) {
	h.handleRange(c, func(r service.GrowthQueryRange) (any, error) {
		out, err := h.growth().GetAudienceDevices(c.Request.Context(), r)
		if err != nil {
			return nil, err
		}
		return gin.H{"items": out}, nil
	}, "Failed to get growth audience devices")
}

func (h *GrowthHandler) GetAudienceOS(c *gin.Context) {
	h.handleRange(c, func(r service.GrowthQueryRange) (any, error) {
		out, err := h.growth().GetAudienceOS(c.Request.Context(), r)
		if err != nil {
			return nil, err
		}
		return gin.H{"items": out}, nil
	}, "Failed to get growth audience os")
}

func (h *GrowthHandler) GetAudienceBrowsers(c *gin.Context) {
	h.handleRange(c, func(r service.GrowthQueryRange) (any, error) {
		out, err := h.growth().GetAudienceBrowsers(c.Request.Context(), r)
		if err != nil {
			return nil, err
		}
		return gin.H{"items": out}, nil
	}, "Failed to get growth audience browsers")
}

func (h *GrowthHandler) GetAudienceClients(c *gin.Context) {
	h.handleRange(c, func(r service.GrowthQueryRange) (any, error) {
		out, err := h.growth().GetAudienceClients(c.Request.Context(), r)
		if err != nil {
			return nil, err
		}
		return gin.H{"items": out}, nil
	}, "Failed to get growth audience clients")
}
