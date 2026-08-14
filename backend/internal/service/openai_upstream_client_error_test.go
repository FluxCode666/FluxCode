//go:build unit

package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestWriteOpenAIUpstreamClientErrorFiltersMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeOpenAIUpstreamClientError(c, http.StatusBadRequest, []byte(`{
		"error": {
			"type": "invalid_request_error",
			"code": "secret/token",
			"param": "input[8].tools[1].parameters"
		}
	}`), "invalid input")

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, "invalid_request_error", gjson.Get(recorder.Body.String(), "error.type").String())
	require.False(t, gjson.Get(recorder.Body.String(), "error.code").Exists())
	require.Equal(t, "input[8].tools[1].parameters", gjson.Get(recorder.Body.String(), "error.param").String())
}
