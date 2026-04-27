//go:build unit

package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type webhookProviderStub struct {
	key    string
	verify func(context.Context, string, map[string]string) (*payment.PaymentNotification, error)
}

func (s webhookProviderStub) Name() string        { return s.key }
func (s webhookProviderStub) ProviderKey() string { return s.key }
func (s webhookProviderStub) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{s.key}
}
func (s webhookProviderStub) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	return nil, errors.New("not implemented")
}
func (s webhookProviderStub) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	return nil, errors.New("not implemented")
}
func (s webhookProviderStub) VerifyNotification(ctx context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	return s.verify(ctx, rawBody, headers)
}
func (s webhookProviderStub) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, errors.New("not implemented")
}

func TestExtractOutTradeNo(t *testing.T) {
	t.Run("alipay form body", func(t *testing.T) {
		got := extractOutTradeNo("out_trade_no=sub2_abc123&trade_status=TRADE_SUCCESS", payment.TypeAlipay)
		assert.Equal(t, "sub2_abc123", got)
	})

	t.Run("stripe metadata body", func(t *testing.T) {
		raw := `{"type":"payment_intent.succeeded","data":{"object":{"metadata":{"orderId":"sub2_20260427abcd"}}}}`
		got := extractOutTradeNo(raw, payment.TypeStripe)
		assert.Equal(t, "sub2_20260427abcd", got)
	})
}

func TestVerifyNotificationWithProviders(t *testing.T) {
	t.Run("falls through to later provider", func(t *testing.T) {
		providers := []payment.Provider{
			webhookProviderStub{
				key: payment.TypeStripe,
				verify: func(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
					return nil, errors.New("bad signature")
				},
			},
			webhookProviderStub{
				key: payment.TypeStripe,
				verify: func(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
					return &payment.PaymentNotification{OrderID: "sub2_ok", Status: payment.ProviderStatusSuccess}, nil
				},
			},
		}

		notification, ignore, err := verifyNotificationWithProviders(context.Background(), providers, "{}", nil)
		require.NoError(t, err)
		assert.False(t, ignore)
		require.NotNil(t, notification)
		assert.Equal(t, "sub2_ok", notification.OrderID)
	})

	t.Run("irrelevant event short-circuits as success", func(t *testing.T) {
		providers := []payment.Provider{
			webhookProviderStub{
				key: payment.TypeStripe,
				verify: func(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
					return nil, nil
				},
			},
		}

		notification, ignore, err := verifyNotificationWithProviders(context.Background(), providers, "{}", nil)
		require.NoError(t, err)
		assert.True(t, ignore)
		assert.Nil(t, notification)
	})
}
