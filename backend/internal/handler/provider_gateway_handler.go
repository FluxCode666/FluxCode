package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

type providerGatewayExecutor interface {
	Execute(context.Context, service.ProviderGatewayRequest) (*service.ProviderGatewayResult, error)
	ExecuteStream(context.Context, service.ProviderGatewayRequest) (*service.ProviderGatewayStreamResult, error)
}

type providerUsageRecorder interface {
	RecordProviderUsage(context.Context, *service.ProviderRecordUsageInput) error
}

type ProviderGatewayHandler struct {
	gateway           providerGatewayExecutor
	usage             providerUsageRecorder
	billing           embeddingBillingEligibilityChecker
	apiKeyService     *service.APIKeyService
	concurrencyHelper *ConcurrencyHelper
	maxSwitches       int
	cfg               *config.Config
}

func NewProviderGatewayHandler(
	gateway *service.ProviderGatewayService,
	usage *service.GatewayService,
	billing *service.BillingCacheService,
	apiKeyService *service.APIKeyService,
	concurrency *service.ConcurrencyService,
	cfg *config.Config,
) *ProviderGatewayHandler {
	maxSwitches := 10
	if cfg != nil && cfg.Gateway.MaxAccountSwitches > 0 {
		maxSwitches = cfg.Gateway.MaxAccountSwitches
	}
	return &ProviderGatewayHandler{
		gateway: gateway, usage: usage, billing: billing, apiKeyService: apiKeyService,
		concurrencyHelper: NewConcurrencyHelper(concurrency, SSEPingFormatComment, 0),
		maxSwitches:       maxSwitches, cfg: cfg,
	}
}

// HandleOrFallback routes cut-over groups through the capability-driven
// provider gateway. Groups without an active route snapshot stay on the legacy
// dispatcher during migration.
func (h *ProviderGatewayHandler) HandleOrFallback(
	c *gin.Context,
	protocol service.ProtocolFamily,
	fallback gin.HandlerFunc,
) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		h.writeError(c, protocol, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if !providerPoolEnabled(apiKey.Group) {
		fallback(c)
		return
	}
	if h == nil || h.gateway == nil || h.usage == nil || h.billing == nil ||
		h.concurrencyHelper == nil || h.concurrencyHelper.concurrencyService == nil {
		h.writeError(c, protocol, http.StatusServiceUnavailable, "provider_gateway_unavailable", "Provider gateway is unavailable")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || apiKey.User == nil || apiKey.GroupID == nil {
		h.writeError(c, protocol, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.writeError(c, protocol, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.writeError(c, protocol, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		h.writeError(c, protocol, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}
	if err := validateProviderJSONKeys(body); err != nil {
		h.writeError(c, protocol, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if model == "" {
		h.writeError(c, protocol, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	stream := gjson.GetBytes(body, "stream").Bool()
	if protocol == service.ProtocolEmbeddings && stream {
		h.writeError(c, protocol, http.StatusBadRequest, "invalid_request_error", "Embeddings do not support streaming")
		return
	}
	setOpsRequestContext(c, model, stream, body)
	requestType := service.RequestTypeSync
	if stream {
		requestType = service.RequestTypeStream
	} else if protocol == service.ProtocolEmbeddings {
		requestType = service.RequestTypeEmbedding
	}
	setOpsEndpointContext(c, "", int16(requestType))

	ctx := c.Request.Context()
	release, acquired, err := h.concurrencyHelper.TryAcquireUserSlotForAPIKey(ctx, subject.UserID, subject.Concurrency, apiKey.ID)
	if err != nil {
		h.writeError(c, protocol, http.StatusServiceUnavailable, "concurrency_error", "Unable to acquire concurrency slot")
		return
	}
	if !acquired {
		h.writeError(c, protocol, http.StatusTooManyRequests, "rate_limit_error", "Too many concurrent requests")
		return
	}
	release = wrapReleaseOnDone(ctx, release)
	if release != nil {
		defer release()
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if err := h.billing.CheckBillingEligibility(ctx, apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		status, code, message := billingErrorDetails(err)
		h.writeError(c, protocol, status, code, message)
		return
	}
	request := service.ProviderGatewayRequest{
		TraceID: strings.TrimSpace(c.GetHeader("X-Request-ID")),
		GroupID: *apiKey.GroupID, LogicalModel: model, Protocol: protocol,
		UserID: subject.UserID, APIKeyID: apiKey.ID,
		SnapshotVersion: *apiKey.Group.ActiveRouteSnapshotVersion,
		Body:            body, Headers: c.Request.Header.Clone(),
		SessionHash: providerSessionHash(apiKey.ID, body, c.Request.Header),
		MaxSwitches: h.maxSwitches,
	}
	if stream {
		h.handleStream(c, protocol, request, apiKey, subscription, body)
		return
	}
	result, err := h.gateway.Execute(ctx, request)
	if err != nil {
		h.writeGatewayError(c, protocol, err)
		return
	}
	h.setRouteContext(c, result.Candidate, requestType)
	if err := h.recordUsage(ctx, result, apiKey, subscription, body, c); err != nil {
		h.writeError(c, protocol, http.StatusInternalServerError, "billing_failed", "Provider usage billing failed")
		return
	}
	copyProviderResponseHeaders(c.Writer.Header(), result.Headers)
	contentType := strings.TrimSpace(result.Headers.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(result.StatusCode, contentType, result.Body)
}

func (h *ProviderGatewayHandler) handleStream(
	c *gin.Context,
	protocol service.ProtocolFamily,
	request service.ProviderGatewayRequest,
	apiKey *service.APIKey,
	subscription *service.UserSubscription,
	body []byte,
) {
	result, err := h.gateway.ExecuteStream(c.Request.Context(), request)
	if err != nil {
		h.writeGatewayError(c, protocol, err)
		return
	}
	defer func() { _ = result.Body.Close() }()
	h.setRouteContext(c, result.Candidate, service.RequestTypeStream)
	copyProviderResponseHeaders(c.Writer.Header(), result.Headers)
	if strings.TrimSpace(c.Writer.Header().Get("Content-Type")) == "" {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
	}
	c.Status(result.StatusCode)
	buffer := make([]byte, 32*1024)
	clientDisconnected := false
	streamCompleted := false
	for {
		n, readErr := result.Body.Read(buffer)
		if n > 0 && !clientDisconnected {
			if _, writeErr := c.Writer.Write(buffer[:n]); writeErr != nil {
				clientDisconnected = true
			} else {
				c.Writer.Flush()
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				streamCompleted = true
			} else {
				logger.FromContext(c.Request.Context()).Warn("provider stream ended before EOF", zap.Error(readErr))
			}
			break
		}
	}
	if !streamCompleted {
		return
	}
	billingCtx := context.WithoutCancel(c.Request.Context())
	if err := h.recordUsage(billingCtx, result.BillingResult(), apiKey, subscription, body, c); err != nil {
		logger.FromContext(billingCtx).Error("provider stream usage billing failed", zap.Error(err))
	}
}

// validateProviderJSONKeys rejects duplicate keys at every object depth. JSON
// parsers are allowed to disagree about whether the first or last duplicate
// wins; accepting duplicates could therefore make routing inspect a different
// model or feature set from the request ultimately sent upstream.
func validateProviderJSONKeys(body []byte) error {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := validateProviderJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain exactly one JSON value")
		}
		return errors.New("failed to parse request body")
	}
	return nil
}

func validateProviderJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return errors.New("failed to parse request body")
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, keyErr := decoder.Token()
			if keyErr != nil {
				return errors.New("failed to parse request body")
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("failed to parse request body")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate JSON field %q is not allowed", key)
			}
			seen[key] = struct{}{}
			if err := validateProviderJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := validateProviderJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return errors.New("failed to parse request body")
	}
	if _, err := decoder.Token(); err != nil {
		return errors.New("failed to parse request body")
	}
	return nil
}

func (h *ProviderGatewayHandler) recordUsage(
	ctx context.Context,
	result *service.ProviderGatewayResult,
	apiKey *service.APIKey,
	subscription *service.UserSubscription,
	body []byte,
	c *gin.Context,
) error {
	return h.usage.RecordProviderUsage(ctx, &service.ProviderRecordUsageInput{
		Result: result, APIKey: apiKey, User: apiKey.User, Subscription: subscription,
		UserAgent: c.GetHeader("User-Agent"), IPAddress: ip.GetClientIP(c),
		RequestPayloadHash: service.HashUsageRequestPayload(body), APIKeyService: h.apiKeyService,
	})
}

func (h *ProviderGatewayHandler) setRouteContext(c *gin.Context, candidate service.RouteCandidate, requestType service.RequestType) {
	if candidate.Account != nil {
		setOpsSelectedAccount(c, candidate.Account.ID, "provider")
	}
	setOpsEndpointContext(c, candidate.Capability.UpstreamModel, int16(requestType))
}

func (h *ProviderGatewayHandler) writeGatewayError(c *gin.Context, protocol service.ProtocolFamily, err error) {
	var upstreamErr *service.ProviderUpstreamError
	if errors.As(err, &upstreamErr) && upstreamErr.StatusCode > 0 && len(upstreamErr.Body) > 0 {
		copyProviderResponseHeaders(c.Writer.Header(), upstreamErr.Headers)
		contentType := strings.TrimSpace(upstreamErr.Headers.Get("Content-Type"))
		if contentType == "" {
			contentType = "application/json"
		}
		c.Data(upstreamErr.StatusCode, contentType, upstreamErr.Body)
		return
	}
	switch {
	case errors.Is(err, service.ErrProviderContinuationUnavailable):
		h.writeError(c, protocol, http.StatusConflict, "continuation_unavailable", "The previous response route is unavailable")
	case errors.Is(err, service.ErrNoProviderRoute), errors.Is(err, service.ErrProviderProtocolMismatch),
		errors.Is(err, service.ErrProviderFeatureUnsupported):
		h.writeError(c, protocol, http.StatusBadRequest, "unsupported_provider_route", "No compatible provider route is available")
	case errors.Is(err, service.ErrProviderAttemptsExhausted):
		h.writeError(c, protocol, http.StatusBadGateway, "provider_routes_exhausted", "All provider routes were exhausted")
	default:
		h.writeError(c, protocol, http.StatusBadGateway, "provider_upstream_error", "Provider upstream request failed")
	}
}

func (h *ProviderGatewayHandler) writeError(
	c *gin.Context,
	protocol service.ProtocolFamily,
	status int,
	code string,
	message string,
) {
	if protocol == service.ProtocolAnthropicMessages {
		c.JSON(status, gin.H{"type": "error", "error": gin.H{"type": code, "message": message}})
		return
	}
	c.JSON(status, gin.H{"error": gin.H{"type": code, "code": code, "message": message}})
}

func providerPoolEnabled(group *service.Group) bool {
	return group != nil && group.ActiveRouteSnapshotVersion != nil && *group.ActiveRouteSnapshotVersion > 0
}

func providerSessionHash(apiKeyID int64, body []byte, headers http.Header) string {
	seed := strings.TrimSpace(headers.Get("X-Session-ID"))
	if seed == "" {
		seed = strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
	}
	if seed == "" {
		return ""
	}
	return service.HashUsageRequestPayload([]byte(strings.Join([]string{strconv.FormatInt(apiKeyID, 10), seed}, "\x00")))
}

func copyProviderResponseHeaders(target, source http.Header) {
	for name, values := range source {
		if strings.EqualFold(name, "Content-Length") {
			continue
		}
		for _, value := range values {
			target.Add(name, value)
		}
	}
}
