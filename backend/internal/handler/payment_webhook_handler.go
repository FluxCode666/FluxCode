package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// PaymentWebhookHandler handles payment provider webhook callbacks.
type PaymentWebhookHandler struct {
	paymentService *service.PaymentService
	registry       *payment.Registry
}

// maxWebhookBodySize is the maximum allowed webhook request body size (1 MB).
const maxWebhookBodySize = 1 << 20

// webhookLogTruncateLen is the maximum length of raw body logged on verify failure.
const webhookLogTruncateLen = 200

// NewPaymentWebhookHandler creates a new PaymentWebhookHandler.
func NewPaymentWebhookHandler(paymentService *service.PaymentService, registry *payment.Registry) *PaymentWebhookHandler {
	return &PaymentWebhookHandler{
		paymentService: paymentService,
		registry:       registry,
	}
}

// EasyPayNotify handles EasyPay payment notifications.
// POST /api/v1/payment/webhook/easypay
func (h *PaymentWebhookHandler) EasyPayNotify(c *gin.Context) {
	h.handleNotify(c, payment.TypeEasyPay)
}

// AlipayNotify handles Alipay payment notifications.
// POST /api/v1/payment/webhook/alipay
func (h *PaymentWebhookHandler) AlipayNotify(c *gin.Context) {
	h.handleNotify(c, payment.TypeAlipay)
}

// WxpayNotify handles WeChat Pay payment notifications.
// POST /api/v1/payment/webhook/wxpay
func (h *PaymentWebhookHandler) WxpayNotify(c *gin.Context) {
	h.handleNotify(c, payment.TypeWxpay)
}

// StripeWebhook handles Stripe webhook events.
// POST /api/v1/payment/webhook/stripe
func (h *PaymentWebhookHandler) StripeWebhook(c *gin.Context) {
	h.handleNotify(c, payment.TypeStripe)
}

// handleNotify is the shared logic for all provider webhook handlers.
func (h *PaymentWebhookHandler) handleNotify(c *gin.Context, providerKey string) {
	var rawBody string
	if c.Request.Method == http.MethodGet {
		// GET callbacks (e.g. EasyPay) pass params as URL query string
		rawBody = c.Request.URL.RawQuery
	} else {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxWebhookBodySize))
		if err != nil {
			slog.Error("[Payment Webhook] failed to read body", "provider", providerKey, "error", err)
			c.String(http.StatusBadRequest, "failed to read body")
			return
		}
		rawBody = string(body)
	}

	// Extract out_trade_no to look up the order's specific provider instance.
	// This is needed when multiple instances of the same provider exist (e.g. multiple EasyPay accounts).
	outTradeNo := extractOutTradeNo(rawBody, providerKey)

	providers, err := h.paymentService.GetWebhookProviderCandidates(c.Request.Context(), providerKey, outTradeNo)
	if err != nil {
		slog.Warn("[Payment Webhook] provider not found", "provider", providerKey, "outTradeNo", outTradeNo, "error", err)
		writeSuccessResponse(c, providerKey)
		return
	}

	headers := make(map[string]string)
	for k := range c.Request.Header {
		headers[strings.ToLower(k)] = c.GetHeader(k)
	}

	notification, ignore, err := verifyNotificationWithProviders(c.Request.Context(), providers, rawBody, headers)
	if err != nil {
		truncatedBody := rawBody
		if len(truncatedBody) > webhookLogTruncateLen {
			truncatedBody = truncatedBody[:webhookLogTruncateLen] + "...(truncated)"
		}
		slog.Error("[Payment Webhook] verify failed", "provider", providerKey, "error", err, "method", c.Request.Method, "bodyLen", len(rawBody), "candidates", len(providers))
		slog.Debug("[Payment Webhook] verify failed body", "provider", providerKey, "rawBody", truncatedBody)
		c.String(http.StatusBadRequest, "verify failed")
		return
	}

	// nil notification means irrelevant event (e.g. Stripe non-payment event); return success.
	if ignore || notification == nil {
		writeSuccessResponse(c, providerKey)
		return
	}

	if err := h.paymentService.HandlePaymentNotification(c.Request.Context(), notification, providerKey); err != nil {
		slog.Error("[Payment Webhook] handle notification failed", "provider", providerKey, "error", err)
		c.String(http.StatusInternalServerError, "handle failed")
		return
	}

	writeSuccessResponse(c, providerKey)
}

// extractOutTradeNo parses the webhook body to find the out_trade_no.
// This allows looking up the correct provider instance before verification.
func extractOutTradeNo(rawBody, providerKey string) string {
	switch providerKey {
	case payment.TypeEasyPay, payment.TypeAlipay:
		values, err := url.ParseQuery(rawBody)
		if err == nil {
			return values.Get("out_trade_no")
		}
	case payment.TypeStripe:
		var payload struct {
			Data struct {
				Object struct {
					Metadata map[string]string `json:"metadata"`
				} `json:"object"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(rawBody), &payload); err == nil {
			return strings.TrimSpace(payload.Data.Object.Metadata["orderId"])
		}
	}
	// WxPay notifications need instance-specific keys for decryption, so the
	// handler falls back to trying all configured candidates when extraction is
	// not possible here.
	return ""
}

func verifyNotificationWithProviders(ctx context.Context, providers []payment.Provider, rawBody string, headers map[string]string) (*payment.PaymentNotification, bool, error) {
	var errs []string
	for _, p := range providers {
		notification, err := p.VerifyNotification(ctx, rawBody, headers)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", p.ProviderKey(), err))
			continue
		}
		if notification == nil {
			return nil, true, nil
		}
		return notification, false, nil
	}
	if len(errs) == 0 {
		return nil, false, fmt.Errorf("no webhook provider candidates")
	}
	return nil, false, errors.New(strings.Join(errs, "; "))
}

// wxpaySuccessResponse is the JSON response expected by WeChat Pay webhook.
type wxpaySuccessResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WeChat Pay webhook success response constants.
const (
	wxpaySuccessCode    = "SUCCESS"
	wxpaySuccessMessage = "成功"
)

// writeSuccessResponse sends the provider-specific success response.
// WeChat Pay requires JSON {"code":"SUCCESS","message":"成功"};
// Stripe expects an empty 200; others accept plain text "success".
func writeSuccessResponse(c *gin.Context, providerKey string) {
	switch providerKey {
	case payment.TypeWxpay:
		c.JSON(http.StatusOK, wxpaySuccessResponse{Code: wxpaySuccessCode, Message: wxpaySuccessMessage})
	case payment.TypeStripe:
		c.String(http.StatusOK, "")
	default:
		c.String(http.StatusOK, "success")
	}
}
