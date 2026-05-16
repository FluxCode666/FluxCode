package service

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServiceWireUsesPaymentProviderForPostConstructionDependencies(t *testing.T) {
	content, err := os.ReadFile("wire.go")
	require.NoError(t, err)
	wireSource := string(content)

	require.Contains(t, wireSource, "func ProvidePaymentService(")
	require.Contains(t, wireSource, "paymentService.SetReferralService(referralService)")
	require.Contains(t, wireSource, "paymentService.SetSalesCommissionService(salesCommissionService)")
	require.Contains(t, wireSource, "ProvidePaymentService,")
	require.NotContains(t, strings.ReplaceAll(wireSource, "func ProvidePaymentService", ""), "\n\tNewPaymentService,")
}
