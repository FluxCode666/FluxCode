package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestSalesCommissionHandlerCreateSettlementValidatesAmount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSalesCommissionHandler(&service.SalesCommissionService{})
	r := gin.New()
	r.POST("/admin/sales-commissions/settlements", h.CreateSettlement)

	req := httptest.NewRequest(http.MethodPost, "/admin/sales-commissions/settlements", strings.NewReader(`{"sales_user_id":1,"amount_cny":0}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "amount")
}
