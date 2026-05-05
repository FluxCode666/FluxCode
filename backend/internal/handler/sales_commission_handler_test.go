package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestSalesCommissionHandlerRequiresAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSalesCommissionHandler(&service.SalesCommissionService{})
	r := gin.New()
	r.GET("/sales-commissions/summary", h.GetSummary)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/sales-commissions/summary", nil))

	require.Equal(t, http.StatusUnauthorized, w.Code)
}
