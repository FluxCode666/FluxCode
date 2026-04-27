//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateProviderForInstanceUsesInstanceProviderKey(t *testing.T) {
	t.Parallel()

	providerInstance := &dbent.PaymentProviderInstance{
		ID:          42,
		ProviderKey: payment.TypeEasyPay,
		PaymentMode: "popup",
	}

	p, err := createProviderForInstance(providerInstance, map[string]string{
		"pid":       "1001",
		"pkey":      "secret",
		"apiBase":   "https://pay.example.com",
		"notifyUrl": "https://app.example.com/notify",
		"returnUrl": "https://app.example.com/return",
	})
	require.NoError(t, err)
	assert.Equal(t, payment.TypeEasyPay, p.ProviderKey())
}

func TestPaymentDetachedContextIgnoresParentCancellation(t *testing.T) {
	t.Parallel()

	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	detached, cancel := paymentDetachedContext(parent, time.Second)
	defer cancel()

	assert.NoError(t, detached.Err())
}
