//go:build unit

package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUpdateSettings_ReturnsCodexCLIConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newTestSettingRepo()
	settingService := service.NewSettingService(repo, &config.Config{})
	handler := NewSettingHandler(settingService, nil, nil, nil, nil, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
		c.Next()
	})
	router.PUT("/settings", handler.UpdateSettings)

	body := bytes.NewBufferString(`{
		"codex_cli_user_agent": "codex_cli_rs/9.8.7",
		"codex_cli_version": "9.8.7",
		"codex_official_client_passthrough_ua_version": true,
		"openai_usage_debug_log_enabled": true
	}`)
	req := httptest.NewRequest(http.MethodPut, "/settings", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Equal(t, "codex_cli_rs/9.8.7", envelope.Data["codex_cli_user_agent"])
	require.Equal(t, "9.8.7", envelope.Data["codex_cli_version"])
	require.Equal(t, true, envelope.Data["codex_official_client_passthrough_ua_version"])
	require.Equal(t, true, envelope.Data["openai_usage_debug_log_enabled"])
	require.Equal(t, "true", repo.values[service.SettingKeyCodexPassthroughUAVersion])
	require.Equal(t, "true", repo.values[service.SettingKeyOpenAIUsageDebugLogEnabled])
}

func TestSettingsCodexImageGenerationBridgeRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newTestSettingRepo()
	settingService := service.NewSettingService(repo, &config.Config{
		Gateway: config.GatewayConfig{CodexImageGenerationBridgeEnabled: true},
	})
	handler := NewSettingHandler(settingService, nil, nil, nil, nil, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
		c.Next()
	})
	router.GET("/settings", handler.GetSettings)
	router.PUT("/settings", handler.UpdateSettings)

	getReq := httptest.NewRequest(http.MethodGet, "/settings", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	require.Equal(t, http.StatusOK, getRec.Code)

	var getEnvelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getEnvelope))
	require.Equal(t, true, getEnvelope.Data["codex_image_generation_bridge_enabled"])

	body := bytes.NewBufferString(`{"codex_image_generation_bridge_enabled": false}`)
	putReq := httptest.NewRequest(http.MethodPut, "/settings", body)
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)
	require.Equal(t, http.StatusOK, putRec.Code)

	var putEnvelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &putEnvelope))
	require.Equal(t, false, putEnvelope.Data["codex_image_generation_bridge_enabled"])
	require.Equal(t, "false", repo.values[service.SettingKeyCodexImageGenerationBridgeEnabled])
}

func TestUpdateSettingsSuccessfulRequestRecordsRequiresEncryptionKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newTestSettingRepo()
	settingService := service.NewSettingService(repo, &config.Config{})
	handler := NewSettingHandler(settingService, nil, nil, nil, nil, nil)
	router := gin.New()
	router.PUT("/settings", handler.UpdateSettings)

	body := bytes.NewBufferString(`{
		"successful_request_records_enabled": true,
		"successful_request_records_max_body_bytes": 1048576
	}`)
	req := httptest.NewRequest(http.MethodPut, "/settings", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "TOTP_ENCRYPTION_KEY")
}

func TestSettingsSuccessfulRequestRecordsRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newTestSettingRepo()
	settingService := service.NewSettingService(repo, &config.Config{
		Totp: config.TotpConfig{EncryptionKeyConfigured: true},
	})
	handler := NewSettingHandler(settingService, nil, nil, nil, nil, nil)
	router := gin.New()
	router.PUT("/settings", handler.UpdateSettings)

	body := bytes.NewBufferString(`{
		"successful_request_records_enabled": true,
		"successful_request_records_max_body_bytes": 2097152
	}`)
	req := httptest.NewRequest(http.MethodPut, "/settings", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Equal(t, true, envelope.Data["successful_request_records_enabled"])
	require.Equal(t, float64(2097152), envelope.Data["successful_request_records_max_body_bytes"])
	require.Equal(t, "true", repo.values[service.SettingKeySuccessfulRequestRecordsEnabled])
	require.Equal(t, "2097152", repo.values[service.SettingKeySuccessfulRequestRecordsMaxBodyBytes])
}
