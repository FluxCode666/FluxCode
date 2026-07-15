package admin

import (
	"bytes"
	"context"
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

func TestSettingHandlerRoundTripsMediaSettings(t *testing.T) {
	router, repo := newSettingHandlerMediaTestRouter(t, nil)
	body := `{"media_sync_wait_timeout_seconds":0,"media_sync_timeout_fallback_async_enabled":true,"media_sync_timeout_billing_policy":"refund","media_sync_timeout_penalty_ratio":0.8,"media_video_storage_mode":"hybrid","media_video_proxy_fallback_enabled":false}`

	rec := performAdminJSONRequest(router, http.MethodPut, "/settings", body)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "0", repo.values[service.SettingKeyMediaSyncWaitTimeoutSeconds])
	require.Equal(t, "true", repo.values[service.SettingKeyMediaSyncTimeoutFallbackAsyncEnabled])
	require.Equal(t, service.MediaTimeoutBillingPolicyRefund, repo.values[service.SettingKeyMediaSyncTimeoutBillingPolicy])
	require.Equal(t, "0.8", repo.values[service.SettingKeyMediaSyncTimeoutPenaltyRatio])
	require.Equal(t, service.MediaVideoStorageModeHybrid, repo.values[service.SettingKeyMediaVideoStorageMode])
	require.Equal(t, "false", repo.values[service.SettingKeyMediaVideoProxyFallbackEnabled])

	data := decodeSettingsResponse(t, rec)
	require.Equal(t, float64(0), data["media_sync_wait_timeout_seconds"])
	require.Equal(t, true, data["media_sync_timeout_fallback_async_enabled"])
	require.Equal(t, "refund", data["media_sync_timeout_billing_policy"])
	require.Equal(t, 0.8, data["media_sync_timeout_penalty_ratio"])
	require.Equal(t, "hybrid", data["media_video_storage_mode"])
	require.Equal(t, false, data["media_video_proxy_fallback_enabled"])

	getRec := performAdminJSONRequest(router, http.MethodGet, "/settings", "")
	require.Equal(t, http.StatusOK, getRec.Code)
	getData := decodeSettingsResponse(t, getRec)
	require.Equal(t, float64(0), getData["media_sync_wait_timeout_seconds"])
	require.Equal(t, true, getData["media_sync_timeout_fallback_async_enabled"])
	require.Equal(t, "refund", getData["media_sync_timeout_billing_policy"])
	require.Equal(t, 0.8, getData["media_sync_timeout_penalty_ratio"])
	require.Equal(t, "hybrid", getData["media_video_storage_mode"])
	require.Equal(t, false, getData["media_video_proxy_fallback_enabled"])
}

func TestSettingHandlerMediaPartialUpdatePreservesOmittedMediaSettings(t *testing.T) {
	router, repo := newSettingHandlerMediaTestRouter(t, map[string]string{
		service.SettingKeyMediaSyncWaitTimeoutSeconds:          "17",
		service.SettingKeyMediaSyncTimeoutFallbackAsyncEnabled: "true",
		service.SettingKeyMediaSyncTimeoutBillingPolicy:        service.MediaTimeoutBillingPolicyRefund,
		service.SettingKeyMediaSyncTimeoutPenaltyRatio:         "0.35",
		service.SettingKeyMediaVideoStorageMode:                service.MediaVideoStorageModeHybrid,
		service.SettingKeyMediaVideoProxyFallbackEnabled:       "true",
	})

	rec := performAdminJSONRequest(router, http.MethodPut, "/settings", `{"media_video_proxy_fallback_enabled":false}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "17", repo.values[service.SettingKeyMediaSyncWaitTimeoutSeconds])
	require.Equal(t, "true", repo.values[service.SettingKeyMediaSyncTimeoutFallbackAsyncEnabled])
	require.Equal(t, service.MediaTimeoutBillingPolicyRefund, repo.values[service.SettingKeyMediaSyncTimeoutBillingPolicy])
	require.Equal(t, "0.35", repo.values[service.SettingKeyMediaSyncTimeoutPenaltyRatio])
	require.Equal(t, service.MediaVideoStorageModeHybrid, repo.values[service.SettingKeyMediaVideoStorageMode])
	require.Equal(t, "false", repo.values[service.SettingKeyMediaVideoProxyFallbackEnabled])
}

func TestSettingHandlerRejectsInvalidMediaSettings(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "ratio above one", body: `{"media_sync_timeout_penalty_ratio":1.2}`},
		{name: "empty policy", body: `{"media_sync_timeout_billing_policy":""}`},
		{name: "blank policy", body: `{"media_sync_timeout_billing_policy":" \t\n "}`},
		{name: "empty storage", body: `{"media_video_storage_mode":""}`},
		{name: "blank storage", body: `{"media_video_storage_mode":" \t\n "}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, repo := newSettingHandlerMediaTestRouter(t, nil)

			rec := performAdminJSONRequest(router, http.MethodPut, "/settings", tt.body)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Zero(t, repo.setMultipleCalls)
		})
	}
}

func TestDiffSettingsIncludesMediaChanges(t *testing.T) {
	before := validHandlerMediaSettings()
	after := *before
	after.MediaSyncWaitTimeoutSeconds = 0
	after.MediaSyncTimeoutFallbackAsyncEnabled = true
	after.MediaSyncTimeoutBillingPolicy = service.MediaTimeoutBillingPolicyRefund
	after.MediaSyncTimeoutPenaltyRatio = 0.5
	after.MediaVideoProxyFallbackEnabled = false

	changed := diffSettings(before, &after, UpdateSettingsRequest{})

	require.Subset(t, changed, []string{
		"media_sync_wait_timeout_seconds",
		"media_sync_timeout_fallback_async_enabled",
		"media_sync_timeout_billing_policy",
		"media_sync_timeout_penalty_ratio",
		"media_video_proxy_fallback_enabled",
	})
}

type mediaHandlerSettingRepo struct {
	*testSettingRepo
	setMultipleCalls int
}

func (r *mediaHandlerSettingRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	r.setMultipleCalls++
	return r.testSettingRepo.SetMultiple(ctx, settings)
}

func newSettingHandlerMediaTestRouter(t *testing.T, values map[string]string) (*gin.Engine, *mediaHandlerSettingRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := &mediaHandlerSettingRepo{testSettingRepo: newTestSettingRepo()}
	for key, value := range values {
		repo.values[key] = value
	}
	settingService := service.NewSettingService(repo, &config.Config{})
	handler := NewSettingHandler(settingService, nil, nil, nil, nil, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
		c.Next()
	})
	router.PUT("/settings", handler.UpdateSettings)
	router.GET("/settings", handler.GetSettings)
	return router, repo
}

func performAdminJSONRequest(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeSettingsResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	return envelope.Data
}

func validHandlerMediaSettings() *service.SystemSettings {
	return &service.SystemSettings{
		MediaSyncWaitTimeoutSeconds:    240,
		MediaSyncTimeoutBillingPolicy:  service.MediaTimeoutBillingPolicyPenalty,
		MediaSyncTimeoutPenaltyRatio:   0.8,
		MediaVideoStorageMode:          service.MediaVideoStorageModeHybrid,
		MediaVideoProxyFallbackEnabled: true,
	}
}
