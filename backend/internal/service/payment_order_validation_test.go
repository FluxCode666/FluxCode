package service

import (
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestValidateRechargeAmountRange_ReturnsSpecificMinError(t *testing.T) {
	err := validateRechargeAmountRange(2, 5, 100)
	require.Error(t, err)

	appErr := infraerrors.FromError(err)
	require.Equal(t, paymentAmountBelowMinReason, appErr.Reason)
	require.Equal(t, "5.00", appErr.Metadata["min"])
	require.Equal(t, "2.00", appErr.Metadata["amount"])
	require.Contains(t, appErr.Message, "5.00 CNY")
}

func TestValidateRechargeAmountRange_ReturnsSpecificMaxError(t *testing.T) {
	err := validateRechargeAmountRange(1200, 1, 1000)
	require.Error(t, err)

	appErr := infraerrors.FromError(err)
	require.Equal(t, paymentAmountAboveMaxReason, appErr.Reason)
	require.Equal(t, "1000.00", appErr.Metadata["max"])
	require.Equal(t, "1200.00", appErr.Metadata["amount"])
	require.Contains(t, appErr.Message, "1000.00 CNY")
}

func TestValidateRechargeAmountRange_AllowsAmountWithinConfiguredRange(t *testing.T) {
	require.NoError(t, validateRechargeAmountRange(88, 1, 1000))
	require.NoError(t, validateRechargeAmountRange(88, 0, 0))
}
