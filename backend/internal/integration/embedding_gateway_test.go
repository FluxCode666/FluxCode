package integration

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/server/routes"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newEmbeddingGatewayContractRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	groupID := int64(41)
	user := &service.User{ID: 17, Concurrency: 1}
	apiKey := &service.APIKey{
		ID:      23,
		UserID:  user.ID,
		User:    user,
		GroupID: &groupID,
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformEmbedding,
		},
		Quota:  10,
		Status: service.StatusAPIKeyActive,
	}
	auth := servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyAPIKey), apiKey)
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{
			UserID:      user.ID,
			Concurrency: user.Concurrency,
		})
		c.Next()
	})
	cfg := &config.Config{}
	cfg.Gateway.MaxBodySize = 1 << 20
	cfg.Gateway.Embedding.RequestMaxBytes = 1 << 20

	routes.RegisterGatewayRoutes(
		router,
		&handler.Handlers{
			Gateway:       &handler.GatewayHandler{},
			OpenAIGateway: handler.NewOpenAIGatewayHandler(nil, nil, nil, nil, nil, nil, cfg),
		},
		auth,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
	)
	return router
}

func TestEmbeddingGatewayRoutesExposeOnlyTheDedicatedContract(t *testing.T) {
	router := newEmbeddingGatewayContractRouter()

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{name: "create embeddings", method: http.MethodPost, path: "/v1/embeddings", body: `{"model":"embed-public","input":"route-input-canary"}`, wantStatus: http.StatusServiceUnavailable},
		{name: "list embedding models", method: http.MethodGet, path: "/v1/models", wantStatus: http.StatusServiceUnavailable},
		{name: "existing usage endpoint", method: http.MethodGet, path: "/v1/usage", wantStatus: http.StatusOK},
		{name: "chat denied", method: http.MethodPost, path: "/v1/chat/completions", body: `{}`, wantStatus: http.StatusNotFound},
		{name: "unversioned embedding alias denied", method: http.MethodPost, path: "/embeddings", body: `{}`, wantStatus: http.StatusNotFound},
		{name: "wrong embedding method denied", method: http.MethodGet, path: "/v1/embeddings", wantStatus: http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)

			require.Equal(t, tc.wantStatus, recorder.Code)
			require.NotContains(t, recorder.Body.String(), "route-input-canary")
		})
	}
}
