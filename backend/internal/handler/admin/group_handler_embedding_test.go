//go:build unit

package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGroupHandlerCreateAndUpdateEmbeddingBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewGroupHandler(newStubAdminService(), nil, nil)
	router := gin.New()
	router.POST("/groups", handler.Create)
	router.PUT("/groups/:id", handler.Update)

	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/groups", bytes.NewBufferString(`{"name":"embedding","platform":"embedding"}`)))
	require.NotEqual(t, http.StatusBadRequest, create.Code)

	update := httptest.NewRecorder()
	router.ServeHTTP(update, httptest.NewRequest(http.MethodPut, "/groups/1", bytes.NewBufferString(`{"platform":"embedding"}`)))
	require.NotEqual(t, http.StatusBadRequest, update.Code)
}
