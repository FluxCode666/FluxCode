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

// TestSalesCommissionHandler_GetOverview_RejectsBadDates handler 自身只负责 query 解析，
// 当 start/end 不是合法 YYYY-MM-DD 时应直接 400 而不调用 service。
func TestSalesCommissionHandler_GetOverview_RejectsBadDates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSalesCommissionHandler(&service.SalesCommissionService{})
	r := gin.New()
	r.GET("/admin/sales-commissions/overview", h.GetOverview)

	for _, tc := range []struct {
		name string
		url  string
	}{
		{"invalid start", "/admin/sales-commissions/overview?range=custom&start=not-a-date&end=2026-05-31"},
		{"invalid end", "/admin/sales-commissions/overview?range=custom&start=2026-05-01&end=not-a-date"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestSalesCommissionHandler_RecomputeMissing_RejectsInvalidJSONBody 当请求体不是合法 JSON 时
// handler 应当直接 400，不进入 service。
func TestSalesCommissionHandler_RecomputeMissing_RejectsInvalidJSONBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSalesCommissionHandler(&service.SalesCommissionService{})
	r := gin.New()
	r.POST("/admin/sales-commissions/recompute", h.RecomputeMissing)

	req := httptest.NewRequest(http.MethodPost, "/admin/sales-commissions/recompute",
		strings.NewReader(`{"limit": "not-a-number"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
