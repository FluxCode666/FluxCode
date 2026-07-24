package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayHandlerEmbeddingsRejectsSimpleModeBeforeDependencies(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"embed","input":"secret"}`))
	h := &OpenAIGatewayHandler{cfg: &config.Config{RunMode: config.RunModeSimple}}

	require.NotPanics(t, func() { h.Embeddings(c) })
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "secret")
}

func TestOpenAIGatewayHandlerEmbeddingModelsRejectsSimpleModeBeforeDependencies(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	h := &OpenAIGatewayHandler{cfg: &config.Config{RunMode: config.RunModeSimple}}

	require.NotPanics(t, func() { h.EmbeddingModels(c) })
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}
