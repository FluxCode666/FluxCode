package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// PoolMonitorHandler handles account pool monitor configurations.
type PoolMonitorHandler struct {
	poolMonitorService *service.PoolMonitorService
}

func NewPoolMonitorHandler(poolMonitorService *service.PoolMonitorService) *PoolMonitorHandler {
	return &PoolMonitorHandler{poolMonitorService: poolMonitorService}
}

type poolMonitorConfigDTO struct {
	Platform                  string   `json:"platform"`
	PoolThresholdEnabled      bool     `json:"pool_threshold_enabled"`
	ProxyFailureEnabled       bool     `json:"proxy_failure_enabled"`
	ProxyActiveProbeEnabled   bool     `json:"proxy_active_probe_enabled"`
	DisabledProxyScheduleMode string   `json:"disabled_proxy_schedule_mode"`
	AvailableCountThreshold   int      `json:"available_count_threshold"`
	AvailableRatioThreshold   int      `json:"available_ratio_threshold"`
	CheckIntervalMinutes      int      `json:"check_interval_minutes"`
	ProxyProbeIntervalMinutes int      `json:"proxy_probe_interval_minutes"`
	ProxyFailureWindowMinutes int      `json:"proxy_failure_window_minutes"`
	ProxyFailureThreshold     int      `json:"proxy_failure_threshold"`
	AlertEmails               []string `json:"alert_emails"`
	AlertCooldownMinutes      int      `json:"alert_cooldown_minutes"`
}

type updatePoolMonitorConfigRequest struct {
	PoolThresholdEnabled      *bool     `json:"pool_threshold_enabled,omitempty"`
	ProxyFailureEnabled       *bool     `json:"proxy_failure_enabled,omitempty"`
	ProxyActiveProbeEnabled   *bool     `json:"proxy_active_probe_enabled,omitempty"`
	DisabledProxyScheduleMode *string   `json:"disabled_proxy_schedule_mode,omitempty"`
	AvailableCountThreshold   *int      `json:"available_count_threshold,omitempty"`
	AvailableRatioThreshold   *int      `json:"available_ratio_threshold,omitempty"`
	CheckIntervalMinutes      *int      `json:"check_interval_minutes,omitempty"`
	ProxyProbeIntervalMinutes *int      `json:"proxy_probe_interval_minutes,omitempty"`
	ProxyFailureWindowMinutes *int      `json:"proxy_failure_window_minutes,omitempty"`
	ProxyFailureThreshold     *int      `json:"proxy_failure_threshold,omitempty"`
	AlertEmails               *[]string `json:"alert_emails,omitempty"`
	AlertCooldownMinutes      *int      `json:"alert_cooldown_minutes,omitempty"`
}

// GetConfig handles GET /api/v1/admin/pool-monitor/:platform.
func (h *PoolMonitorHandler) GetConfig(c *gin.Context) {
	platform := strings.TrimSpace(c.Param("platform"))
	cfg, err := h.poolMonitorService.GetConfig(c.Request.Context(), platform)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toPoolMonitorConfigDTO(cfg))
}

// UpdateConfig handles PUT /api/v1/admin/pool-monitor/:platform.
func (h *PoolMonitorHandler) UpdateConfig(c *gin.Context) {
	platform := strings.TrimSpace(c.Param("platform"))

	var req updatePoolMonitorConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	current, err := h.poolMonitorService.GetConfig(c.Request.Context(), platform)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	merged := &service.AccountPoolAlertConfig{
		Platform:                  current.Platform,
		PoolThresholdEnabled:      current.PoolThresholdEnabled,
		ProxyFailureEnabled:       current.ProxyFailureEnabled,
		ProxyActiveProbeEnabled:   current.ProxyActiveProbeEnabled,
		DisabledProxyScheduleMode: current.DisabledProxyScheduleMode,
		AvailableCountThreshold:   current.AvailableCountThreshold,
		AvailableRatioThreshold:   current.AvailableRatioThreshold,
		CheckIntervalMinutes:      current.CheckIntervalMinutes,
		ProxyProbeIntervalMinutes: current.ProxyProbeIntervalMinutes,
		ProxyFailureWindowMinutes: current.ProxyFailureWindowMinutes,
		ProxyFailureThreshold:     current.ProxyFailureThreshold,
		AlertEmails:               append([]string(nil), current.AlertEmails...),
		AlertCooldownMinutes:      current.AlertCooldownMinutes,
	}

	if req.PoolThresholdEnabled != nil {
		merged.PoolThresholdEnabled = *req.PoolThresholdEnabled
	}
	if req.ProxyFailureEnabled != nil {
		merged.ProxyFailureEnabled = *req.ProxyFailureEnabled
	}
	if req.ProxyActiveProbeEnabled != nil {
		merged.ProxyActiveProbeEnabled = *req.ProxyActiveProbeEnabled
	}
	if req.DisabledProxyScheduleMode != nil {
		merged.DisabledProxyScheduleMode = strings.TrimSpace(*req.DisabledProxyScheduleMode)
	}
	if req.AvailableCountThreshold != nil {
		merged.AvailableCountThreshold = *req.AvailableCountThreshold
	}
	if req.AvailableRatioThreshold != nil {
		merged.AvailableRatioThreshold = *req.AvailableRatioThreshold
	}
	if req.CheckIntervalMinutes != nil {
		merged.CheckIntervalMinutes = *req.CheckIntervalMinutes
	}
	if req.ProxyProbeIntervalMinutes != nil {
		merged.ProxyProbeIntervalMinutes = *req.ProxyProbeIntervalMinutes
	}
	if req.ProxyFailureWindowMinutes != nil {
		merged.ProxyFailureWindowMinutes = *req.ProxyFailureWindowMinutes
	}
	if req.ProxyFailureThreshold != nil {
		merged.ProxyFailureThreshold = *req.ProxyFailureThreshold
	}
	if req.AlertEmails != nil {
		merged.AlertEmails = append([]string(nil), *req.AlertEmails...)
	}
	if req.AlertCooldownMinutes != nil {
		merged.AlertCooldownMinutes = *req.AlertCooldownMinutes
	}

	updated, err := h.poolMonitorService.UpdateConfig(c.Request.Context(), merged)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, toPoolMonitorConfigDTO(updated))
}

func toPoolMonitorConfigDTO(cfg *service.AccountPoolAlertConfig) poolMonitorConfigDTO {
	if cfg == nil {
		return poolMonitorConfigDTO{}
	}
	alertEmails := append([]string(nil), cfg.AlertEmails...)
	if alertEmails == nil {
		alertEmails = []string{}
	}
	return poolMonitorConfigDTO{
		Platform:                  cfg.Platform,
		PoolThresholdEnabled:      cfg.PoolThresholdEnabled,
		ProxyFailureEnabled:       cfg.ProxyFailureEnabled,
		ProxyActiveProbeEnabled:   cfg.ProxyActiveProbeEnabled,
		DisabledProxyScheduleMode: cfg.DisabledProxyScheduleMode,
		AvailableCountThreshold:   cfg.AvailableCountThreshold,
		AvailableRatioThreshold:   cfg.AvailableRatioThreshold,
		CheckIntervalMinutes:      cfg.CheckIntervalMinutes,
		ProxyProbeIntervalMinutes: cfg.ProxyProbeIntervalMinutes,
		ProxyFailureWindowMinutes: cfg.ProxyFailureWindowMinutes,
		ProxyFailureThreshold:     cfg.ProxyFailureThreshold,
		AlertEmails:               alertEmails,
		AlertCooldownMinutes:      cfg.AlertCooldownMinutes,
	}
}
