package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountHandlerCreateMapsMediaValidationErrorToBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := newStubAdminService()
	adminSvc.createAccountErr = service.ErrInvalidMediaAccountConfig
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/api/v1/admin/accounts", handler.Create)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", bytes.NewBufferString(`{
		"name":"media",
		"platform":"media",
		"type":"apikey",
		"credentials":{"api_key":"key","base_url":"https://media.example.com"}
	}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"reason":"INVALID_MEDIA_ACCOUNT_CONFIG"`)
}
