package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGrowthHandlerRejectsInvalidGranularity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewGrowthHandler(service.NewGrowthService(nil))
	router := gin.New()
	router.GET("/admin/growth/overview", handler.GetOverview)

	req := httptest.NewRequest(http.MethodGet, "/admin/growth/overview?granularity=hour", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGrowthHandlerOverviewReturnsEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewGrowthHandler(service.NewGrowthService(nil))
	router := gin.New()
	router.GET("/admin/growth/overview", handler.GetOverview)

	req := httptest.NewRequest(http.MethodGet, "/admin/growth/overview?start_date=2026-05-01&end_date=2026-05-30", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":0`)
	require.Contains(t, rec.Body.String(), `"total_users"`)
}

func TestGrowthHandlerAudienceDevicesReturnsItemsEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewGrowthHandler(service.NewGrowthService(nil))
	router := gin.New()
	router.GET("/admin/growth/audience/devices", handler.GetAudienceDevices)

	req := httptest.NewRequest(http.MethodGet, "/admin/growth/audience/devices?start_date=2026-05-01&end_date=2026-05-30", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":0`)
	require.Contains(t, rec.Body.String(), `"items"`)
}
