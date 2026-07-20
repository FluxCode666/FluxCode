package routes

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterAdminRoutesIncludesMediaModelManagement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{}}
	RegisterAdminRoutes(v1, handlers, middleware.AdminAuthMiddleware(func(c *gin.Context) { c.Next() }))

	registered := make(map[string]struct{})
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	for _, route := range []string{
		"GET /api/v1/admin/media-models",
		"POST /api/v1/admin/media-models",
		"GET /api/v1/admin/media-models/:id",
		"PUT /api/v1/admin/media-models/:id",
		"DELETE /api/v1/admin/media-models/:id",
		"GET /api/v1/admin/groups/:id/media-model-scopes",
		"PUT /api/v1/admin/groups/:id/media-model-scopes",
	} {
		_, ok := registered[route]
		require.True(t, ok, "missing route %s", route)
	}
}
