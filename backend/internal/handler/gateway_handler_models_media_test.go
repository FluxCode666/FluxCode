package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayModelsRejectsMediaGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID: 1,
		Group: &service.Group{
			ID:       9,
			Platform: service.PlatformMedia,
		},
	})

	(&GatewayHandler{}).Models(ctx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var response struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "error", response.Type)
	require.Equal(t, infraerrors.Reason(service.ErrMediaGroupTextGatewayUnsupported), response.Error.Type)
	require.Equal(t, infraerrors.Message(service.ErrMediaGroupTextGatewayUnsupported), response.Error.Message)
}
