package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mediaModelRouteAdminRepository struct{}

func (mediaModelRouteAdminRepository) ListAdmin(context.Context) ([]service.MediaModelAdminRecord, error) {
	return []service.MediaModelAdminRecord{}, nil
}

func (mediaModelRouteAdminRepository) GetAdminByID(context.Context, int64) (*service.MediaModelAdminRecord, error) {
	return nil, service.ErrMediaModelDefinitionNotFound
}

func (mediaModelRouteAdminRepository) CreateAdmin(context.Context, service.MediaModelAdminRecord) (*service.MediaModelAdminRecord, error) {
	return nil, nil
}

func (mediaModelRouteAdminRepository) UpdateAdmin(context.Context, int64, service.MediaModelAdminRecord) (*service.MediaModelAdminRecord, error) {
	return nil, nil
}

func (mediaModelRouteAdminRepository) DeleteAdmin(context.Context, int64) error { return nil }

func TestRegisterAdminRoutesIncludesMediaModelManagement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	resolver := service.NewMediaAdapterResolver(service.NewMediaAdapterRegistry())
	mediaModelService := service.NewMediaModelAdminService(mediaModelRouteAdminRepository{}, nil, nil, nil, resolver)
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		MediaModel: adminhandler.NewMediaModelAdminHandler(mediaModelService),
	}}
	RegisterAdminRoutes(v1, handlers, middleware.AdminAuthMiddleware(func(c *gin.Context) { c.Next() }))

	registered := make(map[string]struct{})
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	for _, route := range []string{
		"GET /api/v1/admin/media-models",
		"POST /api/v1/admin/media-models",
		"GET /api/v1/admin/media-models/preflight",
		"POST /api/v1/admin/media-models/request-mapping-preview",
		"GET /api/v1/admin/media-models/:id",
		"PUT /api/v1/admin/media-models/:id",
		"DELETE /api/v1/admin/media-models/:id",
		"GET /api/v1/admin/groups/:id/media-model-scopes",
		"PUT /api/v1/admin/groups/:id/media-model-scopes",
		"GET /api/v1/admin/settings/media-storage",
		"PUT /api/v1/admin/settings/media-storage",
		"POST /api/v1/admin/settings/media-storage/test",
	} {
		_, ok := registered[route]
		require.True(t, ok, "missing route %s", route)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/media-models/preflight", nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"safe":true`)
}

func TestMediaRequestMappingPreviewRouteRequiresAdminMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	resolver := service.NewMediaAdapterResolver(service.NewMediaAdapterRegistry())
	mediaModelService := service.NewMediaModelAdminService(mediaModelRouteAdminRepository{}, nil, nil, nil, resolver)
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		MediaModel: adminhandler.NewMediaModelAdminHandler(mediaModelService),
	}}
	authCalls := 0
	RegisterAdminRoutes(v1, handlers, middleware.AdminAuthMiddleware(func(c *gin.Context) {
		authCalls++
		c.AbortWithStatus(http.StatusUnauthorized)
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media-models/request-mapping-preview", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Equal(t, 1, authCalls)
}
