package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const defaultEmbeddingHandlerBodyLimit int64 = 1 * 1024 * 1024

// EmbeddingModels returns only models that share the POST eligibility rules.
func (h *OpenAIGatewayHandler) EmbeddingModels(c *gin.Context) {
	if h.embeddingSimpleMode() {
		h.errorResponse(c, http.StatusServiceUnavailable, "unsupported_mode", "Embeddings are unavailable in simple run mode")
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformEmbedding {
		h.errorResponse(c, http.StatusForbidden, "permission_error", "This API key cannot access embeddings")
		return
	}
	if h.embeddingGateway == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Embedding service is unavailable")
		return
	}
	models, err := h.embeddingGateway.ListAvailableEmbeddingModels(c.Request.Context(), apiKey.GroupID)
	if err != nil {
		h.handleEmbeddingError(c, err)
		return
	}
	data := make([]gin.H, 0, len(models))
	for _, model := range models {
		data = append(data, gin.H{
			"id":       model,
			"object":   "model",
			"created":  0,
			"owned_by": "system",
		})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}

// Embeddings forwards one non-streaming OpenAI-compatible request and writes
// no vector bytes until the synchronous billing transaction has committed.
func (h *OpenAIGatewayHandler) Embeddings(c *gin.Context) {
	if h.embeddingSimpleMode() {
		h.errorResponse(c, http.StatusServiceUnavailable, "unsupported_mode", "Embeddings are unavailable in simple run mode")
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if apiKey.Group == nil || apiKey.Group.Platform != service.PlatformEmbedding {
		h.errorResponse(c, http.StatusForbidden, "permission_error", "This API key cannot access embeddings")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || apiKey.User == nil {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	if h.embeddingGateway == nil || h.embeddingBillingChecker == nil || h.concurrencyHelper == nil || h.concurrencyHelper.concurrencyService == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Embedding service is unavailable")
		return
	}

	setOpsEndpointContext(c, "", int16(service.RequestTypeEmbedding))

	ctx := c.Request.Context()
	release, acquired, err := h.concurrencyHelper.TryAcquireUserSlotForAPIKey(ctx, subject.UserID, subject.Concurrency, apiKey.ID)
	if err != nil {
		h.handleEmbeddingError(c, err)
		return
	}
	if !acquired {
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Too many concurrent requests")
		return
	}
	release = wrapReleaseOnDone(ctx, release)
	if release != nil {
		defer release()
	}

	limit := defaultEmbeddingHandlerBodyLimit
	if h.cfg != nil && h.cfg.Gateway.Embedding.RequestMaxBytes > 0 {
		limit = min(h.cfg.Gateway.Embedding.RequestMaxBytes, config.EmbeddingRequestMaxBytesHardLimit)
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	setOpsRequestContext(c, model, false, nil)

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if err := h.embeddingBillingChecker.CheckBillingEligibility(ctx, apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		status, code, message := billingErrorDetails(err)
		h.errorResponse(c, status, code, message)
		return
	}

	result, err := h.embeddingGateway.ForwardEmbeddings(ctx, service.EmbeddingForwardInput{
		GroupID: apiKey.GroupID,
		Body:    body,
	})
	if err != nil {
		h.handleEmbeddingError(c, err)
		return
	}
	setOpsSelectedAccount(c, result.Eligibility.Account.ID, service.PlatformEmbedding)
	setOpsChannelContext(c, result.Eligibility.ChannelMapping.ChannelID)
	setOpsEndpointContext(c, result.Eligibility.UpstreamModel, int16(service.RequestTypeEmbedding))

	if err := h.embeddingGateway.BillEmbedding(ctx, &service.EmbeddingBillingInput{
		Result:             result,
		APIKey:             apiKey,
		User:               apiKey.User,
		Subscription:       subscription,
		UserAgent:          c.GetHeader("User-Agent"),
		IPAddress:          ip.GetClientIP(c),
		RequestPayloadHash: service.HashUsageRequestPayload(body),
		APIKeyService:      h.apiKeyService,
	}); err != nil {
		h.handleEmbeddingBillingError(c, err)
		return
	}

	for name, values := range result.Headers {
		for _, value := range values {
			c.Writer.Header().Add(name, value)
		}
	}
	contentType := strings.TrimSpace(result.Headers.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(http.StatusOK, contentType, result.Body)
}

func (h *OpenAIGatewayHandler) embeddingSimpleMode() bool {
	return h != nil && h.cfg != nil && h.cfg.RunMode == config.RunModeSimple
}

func (h *OpenAIGatewayHandler) handleEmbeddingBillingError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrEmbeddingUnsupportedMode):
		setEmbeddingOpsErrorCategory(c, "unsupported_mode")
		h.errorResponse(c, http.StatusServiceUnavailable, "unsupported_mode", "Embeddings are unavailable in simple run mode")
	case errors.Is(err, service.ErrEmbeddingPricingInvalid):
		setEmbeddingOpsErrorCategory(c, "pricing_invalid")
		h.errorResponse(c, http.StatusBadGateway, "api_error", "Embedding usage could not be priced")
	default:
		setEmbeddingOpsErrorCategory(c, "billing_failed")
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Embedding billing failed")
	}
}

func (h *OpenAIGatewayHandler) handleEmbeddingError(c *gin.Context, err error) {
	var forwardErr *service.EmbeddingForwardError
	if errors.As(err, &forwardErr) {
		setEmbeddingOpsErrorCategory(c, forwardErr.Category)
		if forwardErr.AccountID > 0 {
			setOpsSelectedAccount(c, forwardErr.AccountID, service.PlatformEmbedding)
		}
		if forwardErr.ChannelID > 0 {
			setOpsChannelContext(c, forwardErr.ChannelID)
		}
		if strings.TrimSpace(forwardErr.UpstreamModel) != "" {
			setOpsEndpointContext(c, forwardErr.UpstreamModel, int16(service.RequestTypeEmbedding))
		}
	}
	switch {
	case errors.Is(err, service.ErrEmbeddingUnsupportedMode):
		setEmbeddingOpsErrorCategory(c, "unsupported_mode")
		h.errorResponse(c, http.StatusServiceUnavailable, "unsupported_mode", "Embeddings are unavailable in simple run mode")
	case errors.Is(err, service.ErrEmbeddingRequestInvalid):
		setEmbeddingOpsErrorCategory(c, "invalid_request")
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Invalid embedding request")
	case errors.Is(err, service.ErrEmbeddingUnavailable):
		setEmbeddingOpsErrorCategory(c, "model_unavailable")
		h.errorResponse(c, http.StatusNotFound, "invalid_request_error", "Embedding model is unavailable")
	case errors.As(err, &forwardErr) && forwardErr.Category == "concurrency_limit":
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Too many concurrent embedding requests")
	default:
		h.errorResponse(c, http.StatusBadGateway, "api_error", "Embedding upstream request failed")
	}
}

func setEmbeddingOpsErrorCategory(c *gin.Context, category string) {
	if c == nil {
		return
	}
	c.Set(service.OpsEmbeddingErrorCategoryKey, strings.TrimSpace(category))
}
