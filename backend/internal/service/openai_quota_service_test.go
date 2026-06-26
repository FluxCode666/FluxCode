package service

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateRedeemRequestIDUsesUUIDv4Shape(t *testing.T) {
	id, err := generateRedeemRequestID()
	require.NoError(t, err)
	require.Regexp(t, regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`), id)
}

func TestOpenAIQuotaMapUpstreamStatus(t *testing.T) {
	require.Equal(t, http.StatusUnauthorized, mapUpstreamStatus(http.StatusUnauthorized))
	require.Equal(t, http.StatusForbidden, mapUpstreamStatus(http.StatusForbidden))
	require.Equal(t, http.StatusTooManyRequests, mapUpstreamStatus(http.StatusTooManyRequests))
	require.Equal(t, http.StatusBadGateway, mapUpstreamStatus(http.StatusBadRequest))
	require.Equal(t, http.StatusBadGateway, mapUpstreamStatus(http.StatusInternalServerError))
}
