package admin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// semverPattern 预编译 semver 格式校验正则
var semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// menuItemIDPattern validates custom menu item IDs: alphanumeric, hyphens, underscores only.
var menuItemIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// generateMenuItemID generates a short random hex ID for a custom menu item.
func generateMenuItemID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate menu item ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func scopesContainOpenID(scopes string) bool {
	for _, scope := range strings.Fields(strings.ToLower(strings.TrimSpace(scopes))) {
		if scope == "openid" {
			return true
		}
	}
	return false
}

// SettingHandler 系统设置处理器
type SettingHandler struct {
	settingService       *service.SettingService
	emailService         *service.EmailService
	turnstileService     *service.TurnstileService
	opsService           *service.OpsService
	paymentConfigService *service.PaymentConfigService
	paymentService       *service.PaymentService
}

// NewSettingHandler 创建系统设置处理器
func NewSettingHandler(settingService *service.SettingService, emailService *service.EmailService, turnstileService *service.TurnstileService, opsService *service.OpsService, paymentConfigService *service.PaymentConfigService, paymentService *service.PaymentService) *SettingHandler {
	return &SettingHandler{
		settingService:       settingService,
		emailService:         emailService,
		turnstileService:     turnstileService,
		opsService:           opsService,
		paymentConfigService: paymentConfigService,
		paymentService:       paymentService,
	}
}

// GetSettings 获取所有系统设置
// GET /api/v1/admin/settings
func (h *SettingHandler) GetSettings(c *gin.Context) {
	settings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// Check if ops monitoring is enabled (respects config.ops.enabled)
	opsEnabled := h.opsService != nil && h.opsService.IsMonitoringEnabled(c.Request.Context())
	defaultSubscriptions := make([]dto.DefaultSubscriptionSetting, 0, len(settings.DefaultSubscriptions))
	for _, sub := range settings.DefaultSubscriptions {
		defaultSubscriptions = append(defaultSubscriptions, dto.DefaultSubscriptionSetting{
			GroupID:      sub.GroupID,
			ValidityDays: sub.ValidityDays,
		})
	}

	// Load payment config
	var paymentCfg *service.PaymentConfig
	if h.paymentConfigService != nil {
		paymentCfg, _ = h.paymentConfigService.GetPaymentConfig(c.Request.Context())
	}
	if paymentCfg == nil {
		paymentCfg = &service.PaymentConfig{}
	}

	response.Success(c, dto.SystemSettings{
		RegistrationEnabled:                  settings.RegistrationEnabled,
		EmailVerifyEnabled:                   settings.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist:     settings.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                     settings.PromoCodeEnabled,
		PasswordResetEnabled:                 settings.PasswordResetEnabled,
		FrontendURL:                          settings.FrontendURL,
		InvitationCodeEnabled:                settings.InvitationCodeEnabled,
		TotpEnabled:                          settings.TotpEnabled,
		TotpEncryptionKeyConfigured:          h.settingService.IsTotpEncryptionKeyConfigured(),
		ChannelMonitorEnabled:                settings.ChannelMonitorEnabled,
		ChannelMonitorDefaultIntervalSeconds: settings.ChannelMonitorDefaultIntervalSeconds,
		SMTPHost:                             settings.SMTPHost,
		SMTPPort:                             settings.SMTPPort,
		SMTPUsername:                         settings.SMTPUsername,
		SMTPPasswordConfigured:               settings.SMTPPasswordConfigured,
		SMTPFrom:                             settings.SMTPFrom,
		SMTPFromName:                         settings.SMTPFromName,
		SMTPUseTLS:                           settings.SMTPUseTLS,
		EmailProvider:                        settings.EmailProvider,
		ResendAPIKeyConfigured:               settings.ResendAPIKeyConfigured,
		ResendFrom:                           settings.ResendFrom,
		ResendFromName:                       settings.ResendFromName,
		TurnstileEnabled:                     settings.TurnstileEnabled,
		TurnstileSiteKey:                     settings.TurnstileSiteKey,
		TurnstileSecretKeyConfigured:         settings.TurnstileSecretKeyConfigured,
		LinuxDoConnectEnabled:                settings.LinuxDoConnectEnabled,
		LinuxDoConnectClientID:               settings.LinuxDoConnectClientID,
		LinuxDoConnectClientSecretConfigured: settings.LinuxDoConnectClientSecretConfigured,
		LinuxDoConnectRedirectURL:            settings.LinuxDoConnectRedirectURL,
		OIDCConnectEnabled:                   settings.OIDCConnectEnabled,
		OIDCConnectProviderName:              settings.OIDCConnectProviderName,
		OIDCConnectClientID:                  settings.OIDCConnectClientID,
		OIDCConnectClientSecretConfigured:    settings.OIDCConnectClientSecretConfigured,
		OIDCConnectIssuerURL:                 settings.OIDCConnectIssuerURL,
		OIDCConnectDiscoveryURL:              settings.OIDCConnectDiscoveryURL,
		OIDCConnectAuthorizeURL:              settings.OIDCConnectAuthorizeURL,
		OIDCConnectTokenURL:                  settings.OIDCConnectTokenURL,
		OIDCConnectUserInfoURL:               settings.OIDCConnectUserInfoURL,
		OIDCConnectJWKSURL:                   settings.OIDCConnectJWKSURL,
		OIDCConnectScopes:                    settings.OIDCConnectScopes,
		OIDCConnectRedirectURL:               settings.OIDCConnectRedirectURL,
		OIDCConnectFrontendRedirectURL:       settings.OIDCConnectFrontendRedirectURL,
		OIDCConnectTokenAuthMethod:           settings.OIDCConnectTokenAuthMethod,
		OIDCConnectUsePKCE:                   settings.OIDCConnectUsePKCE,
		OIDCConnectValidateIDToken:           settings.OIDCConnectValidateIDToken,
		OIDCConnectAllowedSigningAlgs:        settings.OIDCConnectAllowedSigningAlgs,
		OIDCConnectClockSkewSeconds:          settings.OIDCConnectClockSkewSeconds,
		OIDCConnectRequireEmailVerified:      settings.OIDCConnectRequireEmailVerified,
		OIDCConnectUserInfoEmailPath:         settings.OIDCConnectUserInfoEmailPath,
		OIDCConnectUserInfoIDPath:            settings.OIDCConnectUserInfoIDPath,
		OIDCConnectUserInfoUsernamePath:      settings.OIDCConnectUserInfoUsernamePath,
		SiteName:                             settings.SiteName,
		SiteLogo:                             settings.SiteLogo,
		SiteSubtitle:                         settings.SiteSubtitle,
		APIBaseURL:                           settings.APIBaseURL,
		ContactInfo:                          settings.ContactInfo,
		DocURL:                               settings.DocURL,
		HomeContent:                          settings.HomeContent,
		HideCcsImportButton:                  settings.HideCcsImportButton,
		PurchaseSubscriptionEnabled:          settings.PurchaseSubscriptionEnabled,
		PurchaseSubscriptionURL:              settings.PurchaseSubscriptionURL,
		TableDefaultPageSize:                 settings.TableDefaultPageSize,
		TablePageSizeOptions:                 settings.TablePageSizeOptions,
		CustomMenuItems:                      dto.ParseCustomMenuItems(settings.CustomMenuItems),
		CustomEndpoints:                      dto.ParseCustomEndpoints(settings.CustomEndpoints),
		OpenAIUseKeyModelID:                  settings.OpenAIUseKeyModelID,
		OpenAIImageURLCacheTTLHours:          settings.OpenAIImageURLCacheTTLHours,
		GeneratedImageStorageSource:          settings.GeneratedImageStorageSource,
		GeneratedImageStorageConfigSource:    settings.GeneratedImageStorageConfigSource,
		QiniuAccessKey:                       settings.QiniuAccessKey,
		QiniuSecretKeyConfigured:             settings.QiniuSecretKeyConfigured,
		QiniuBucket:                          settings.QiniuBucket,
		QiniuCDNDomain:                       settings.QiniuCDNDomain,
		QiniuPrefix:                          settings.QiniuPrefix,
		QiniuUseHTTPS:                        settings.QiniuUseHTTPS,
		QiniuUploadTimeoutSeconds:            settings.QiniuUploadTimeoutSeconds,
		QiniuTokenTTLSeconds:                 settings.QiniuTokenTTLSeconds,
		GeneratedImageCleanupEnabled:         settings.GeneratedImageCleanupEnabled,
		MediaSyncWaitTimeoutSeconds:          settings.MediaSyncWaitTimeoutSeconds,
		MediaSyncTimeoutFallbackAsyncEnabled: settings.MediaSyncTimeoutFallbackAsyncEnabled,
		MediaSyncTimeoutBillingPolicy:        settings.MediaSyncTimeoutBillingPolicy,
		MediaSyncTimeoutPenaltyRatio:         settings.MediaSyncTimeoutPenaltyRatio,
		MediaVideoStorageMode:                settings.MediaVideoStorageMode,
		MediaVideoProxyFallbackEnabled:       settings.MediaVideoProxyFallbackEnabled,
		DefaultConcurrency:                   settings.DefaultConcurrency,
		DefaultBalance:                       settings.DefaultBalance,
		DefaultSubscriptions:                 defaultSubscriptions,
		EnableModelFallback:                  settings.EnableModelFallback,
		FallbackModelAnthropic:               settings.FallbackModelAnthropic,
		FallbackModelOpenAI:                  settings.FallbackModelOpenAI,
		FallbackModelGemini:                  settings.FallbackModelGemini,
		FallbackModelAntigravity:             settings.FallbackModelAntigravity,
		EnableIdentityPatch:                  settings.EnableIdentityPatch,
		IdentityPatchPrompt:                  settings.IdentityPatchPrompt,
		SystemPromptAnthropic:                settings.SystemPromptAnthropic,
		SystemPromptModeAnthropic:            settings.SystemPromptModeAnthropic,
		SystemPromptOpenAI:                   settings.SystemPromptOpenAI,
		SystemPromptModeOpenAI:               settings.SystemPromptModeOpenAI,
		SystemPromptGemini:                   settings.SystemPromptGemini,
		SystemPromptModeGemini:               settings.SystemPromptModeGemini,
		SystemPromptAntigravity:              settings.SystemPromptAntigravity,
		SystemPromptModeAntigravity:          settings.SystemPromptModeAntigravity,
		SystemPromptUserScopeEnabled:         settings.SystemPromptUserScopeEnabled,
		SystemPromptUserScopeMode:            settings.SystemPromptUserScopeMode,
		SystemPromptUserScopeUserIDs:         settings.SystemPromptUserScopeUserIDs,
		OpsMonitoringEnabled:                 opsEnabled && settings.OpsMonitoringEnabled,
		OpsRealtimeMonitoringEnabled:         settings.OpsRealtimeMonitoringEnabled,
		OpsQueryModeDefault:                  settings.OpsQueryModeDefault,
		OpsMetricsIntervalSeconds:            settings.OpsMetricsIntervalSeconds,
		MinClaudeCodeVersion:                 settings.MinClaudeCodeVersion,
		MaxClaudeCodeVersion:                 settings.MaxClaudeCodeVersion,
		AllowUngroupedKeyScheduling:          settings.AllowUngroupedKeyScheduling,
		BackendModeEnabled:                   settings.BackendModeEnabled,
		RedeemDeliveryText:                   settings.RedeemDeliveryText,
		AttractPopupTitle:                    settings.AttractPopupTitle,
		AttractPopupMarkdown:                 settings.AttractPopupMarkdown,
		DashboardFireworksEnabled:            settings.DashboardFireworksEnabled,
		DashboardFireworksThreshold:          settings.DashboardFireworksThreshold,
		EnableFingerprintUnification:         settings.EnableFingerprintUnification,
		EnableMetadataPassthrough:            settings.EnableMetadataPassthrough,
		EnableCCHSigning:                     settings.EnableCCHSigning,
		CodexImageGenerationBridgeEnabled:    settings.CodexImageGenerationBridgeEnabled,
		CodexCLIUserAgent:                    settings.CodexCLIUserAgent,
		CodexCLIVersion:                      settings.CodexCLIVersion,
		CodexPassthroughUAVersion:            settings.CodexPassthroughUAVersion,
		OpenAIUsageDebugLogEnabled:           settings.OpenAIUsageDebugLogEnabled,
		WebSearchEmulationEnabled:            settings.WebSearchEmulationEnabled,
		BalanceLowNotifyEnabled:              settings.BalanceLowNotifyEnabled,
		BalanceLowNotifyThreshold:            settings.BalanceLowNotifyThreshold,
		BalanceLowNotifyRechargeURL:          settings.BalanceLowNotifyRechargeURL,
		AccountQuotaNotifyEnabled:            settings.AccountQuotaNotifyEnabled,
		AccountQuotaNotifyEmails:             dto.NotifyEmailEntriesFromService(settings.AccountQuotaNotifyEmails),
		PaymentEnabled:                       paymentCfg.Enabled,
		PaymentMinAmount:                     paymentCfg.MinAmount,
		PaymentMaxAmount:                     paymentCfg.MaxAmount,
		PaymentDailyLimit:                    paymentCfg.DailyLimit,
		PaymentOrderTimeoutMin:               paymentCfg.OrderTimeoutMin,
		PaymentMaxPendingOrders:              paymentCfg.MaxPendingOrders,
		PaymentEnabledTypes:                  paymentCfg.EnabledTypes,
		PaymentBalanceDisabled:               paymentCfg.BalanceDisabled,
		PaymentBalanceRechargeMultiplier:     paymentCfg.BalanceRechargeMultiplier,
		PaymentRechargeFeeRate:               paymentCfg.RechargeFeeRate,
		PaymentOrderIDPrefix:                 paymentCfg.OrderIDPrefix,
		PaymentLoadBalanceStrat:              paymentCfg.LoadBalanceStrategy,
		PaymentProductNamePrefix:             paymentCfg.ProductNamePrefix,
		PaymentProductNameSuffix:             paymentCfg.ProductNameSuffix,
		PaymentHelpImageURL:                  paymentCfg.HelpImageURL,
		PaymentHelpText:                      paymentCfg.HelpText,
		PaymentCancelRateLimitEnabled:        paymentCfg.CancelRateLimitEnabled,
		PaymentCancelRateLimitMax:            paymentCfg.CancelRateLimitMax,
		PaymentCancelRateLimitWindow:         paymentCfg.CancelRateLimitWindow,
		PaymentCancelRateLimitUnit:           paymentCfg.CancelRateLimitUnit,
		PaymentCancelRateLimitMode:           paymentCfg.CancelRateLimitMode,
	})
}

// UpdateSettingsRequest 更新设置请求
type UpdateSettingsRequest struct {
	// 注册设置
	RegistrationEnabled                  bool     `json:"registration_enabled"`
	EmailVerifyEnabled                   bool     `json:"email_verify_enabled"`
	RegistrationEmailSuffixWhitelist     []string `json:"registration_email_suffix_whitelist"`
	PromoCodeEnabled                     bool     `json:"promo_code_enabled"`
	PasswordResetEnabled                 bool     `json:"password_reset_enabled"`
	FrontendURL                          string   `json:"frontend_url"`
	InvitationCodeEnabled                bool     `json:"invitation_code_enabled"`
	TotpEnabled                          bool     `json:"totp_enabled"` // TOTP 双因素认证
	ChannelMonitorEnabled                bool     `json:"channel_monitor_enabled"`
	ChannelMonitorDefaultIntervalSeconds int      `json:"channel_monitor_default_interval_seconds"`

	// 邮件服务设置
	SMTPHost       string  `json:"smtp_host"`
	SMTPPort       int     `json:"smtp_port"`
	SMTPUsername   string  `json:"smtp_username"`
	SMTPPassword   string  `json:"smtp_password"`
	SMTPFrom       string  `json:"smtp_from_email"`
	SMTPFromName   string  `json:"smtp_from_name"`
	SMTPUseTLS     bool    `json:"smtp_use_tls"`
	EmailProvider  *string `json:"email_provider"`
	ResendAPIKey   string  `json:"resend_api_key"`
	ResendFrom     *string `json:"resend_from_email"`
	ResendFromName *string `json:"resend_from_name"`

	// Cloudflare Turnstile 设置
	TurnstileEnabled   bool   `json:"turnstile_enabled"`
	TurnstileSiteKey   string `json:"turnstile_site_key"`
	TurnstileSecretKey string `json:"turnstile_secret_key"`

	// LinuxDo Connect OAuth 登录
	LinuxDoConnectEnabled      bool   `json:"linuxdo_connect_enabled"`
	LinuxDoConnectClientID     string `json:"linuxdo_connect_client_id"`
	LinuxDoConnectClientSecret string `json:"linuxdo_connect_client_secret"`
	LinuxDoConnectRedirectURL  string `json:"linuxdo_connect_redirect_url"`

	// Generic OIDC OAuth 登录
	OIDCConnectEnabled              bool   `json:"oidc_connect_enabled"`
	OIDCConnectProviderName         string `json:"oidc_connect_provider_name"`
	OIDCConnectClientID             string `json:"oidc_connect_client_id"`
	OIDCConnectClientSecret         string `json:"oidc_connect_client_secret"`
	OIDCConnectIssuerURL            string `json:"oidc_connect_issuer_url"`
	OIDCConnectDiscoveryURL         string `json:"oidc_connect_discovery_url"`
	OIDCConnectAuthorizeURL         string `json:"oidc_connect_authorize_url"`
	OIDCConnectTokenURL             string `json:"oidc_connect_token_url"`
	OIDCConnectUserInfoURL          string `json:"oidc_connect_userinfo_url"`
	OIDCConnectJWKSURL              string `json:"oidc_connect_jwks_url"`
	OIDCConnectScopes               string `json:"oidc_connect_scopes"`
	OIDCConnectRedirectURL          string `json:"oidc_connect_redirect_url"`
	OIDCConnectFrontendRedirectURL  string `json:"oidc_connect_frontend_redirect_url"`
	OIDCConnectTokenAuthMethod      string `json:"oidc_connect_token_auth_method"`
	OIDCConnectUsePKCE              bool   `json:"oidc_connect_use_pkce"`
	OIDCConnectValidateIDToken      bool   `json:"oidc_connect_validate_id_token"`
	OIDCConnectAllowedSigningAlgs   string `json:"oidc_connect_allowed_signing_algs"`
	OIDCConnectClockSkewSeconds     int    `json:"oidc_connect_clock_skew_seconds"`
	OIDCConnectRequireEmailVerified bool   `json:"oidc_connect_require_email_verified"`
	OIDCConnectUserInfoEmailPath    string `json:"oidc_connect_userinfo_email_path"`
	OIDCConnectUserInfoIDPath       string `json:"oidc_connect_userinfo_id_path"`
	OIDCConnectUserInfoUsernamePath string `json:"oidc_connect_userinfo_username_path"`

	// OEM设置
	SiteName                             string                `json:"site_name"`
	SiteLogo                             string                `json:"site_logo"`
	SiteSubtitle                         string                `json:"site_subtitle"`
	APIBaseURL                           string                `json:"api_base_url"`
	ContactInfo                          string                `json:"contact_info"`
	DocURL                               string                `json:"doc_url"`
	HomeContent                          string                `json:"home_content"`
	HideCcsImportButton                  bool                  `json:"hide_ccs_import_button"`
	PurchaseSubscriptionEnabled          *bool                 `json:"purchase_subscription_enabled"`
	PurchaseSubscriptionURL              *string               `json:"purchase_subscription_url"`
	TableDefaultPageSize                 int                   `json:"table_default_page_size"`
	TablePageSizeOptions                 []int                 `json:"table_page_size_options"`
	CustomMenuItems                      *[]dto.CustomMenuItem `json:"custom_menu_items"`
	CustomEndpoints                      *[]dto.CustomEndpoint `json:"custom_endpoints"`
	OpenAIUseKeyModelID                  *string               `json:"openai_use_key_model_id"`
	OpenAIImageURLCacheTTLHours          *int                  `json:"openai_image_url_cache_ttl_hours"`
	GeneratedImageStorageSource          *string               `json:"generated_image_storage_source"`
	GeneratedImageStorageConfigSource    *string               `json:"generated_image_storage_config_source"`
	QiniuAccessKey                       *string               `json:"qiniu_access_key"`
	QiniuSecretKey                       string                `json:"qiniu_secret_key"`
	QiniuBucket                          *string               `json:"qiniu_bucket"`
	QiniuCDNDomain                       *string               `json:"qiniu_cdn_domain"`
	QiniuPrefix                          *string               `json:"qiniu_prefix"`
	QiniuUseHTTPS                        *bool                 `json:"qiniu_use_https"`
	QiniuUploadTimeoutSeconds            *int                  `json:"qiniu_upload_timeout_seconds"`
	QiniuTokenTTLSeconds                 *int                  `json:"qiniu_token_ttl_seconds"`
	GeneratedImageCleanupEnabled         *bool                 `json:"generated_image_cleanup_enabled"`
	MediaSyncWaitTimeoutSeconds          *int                  `json:"media_sync_wait_timeout_seconds"`
	MediaSyncTimeoutFallbackAsyncEnabled *bool                 `json:"media_sync_timeout_fallback_async_enabled"`
	MediaSyncTimeoutBillingPolicy        *string               `json:"media_sync_timeout_billing_policy"`
	MediaSyncTimeoutPenaltyRatio         *float64              `json:"media_sync_timeout_penalty_ratio"`
	MediaVideoStorageMode                *string               `json:"media_video_storage_mode"`
	MediaVideoProxyFallbackEnabled       *bool                 `json:"media_video_proxy_fallback_enabled"`

	// 默认配置
	DefaultConcurrency   int                              `json:"default_concurrency"`
	DefaultBalance       float64                          `json:"default_balance"`
	DefaultSubscriptions []dto.DefaultSubscriptionSetting `json:"default_subscriptions"`

	// Model fallback configuration
	EnableModelFallback      bool   `json:"enable_model_fallback"`
	FallbackModelAnthropic   string `json:"fallback_model_anthropic"`
	FallbackModelOpenAI      string `json:"fallback_model_openai"`
	FallbackModelGemini      string `json:"fallback_model_gemini"`
	FallbackModelAntigravity string `json:"fallback_model_antigravity"`

	// Identity patch configuration (Claude -> Gemini)
	EnableIdentityPatch bool   `json:"enable_identity_patch"`
	IdentityPatchPrompt string `json:"identity_patch_prompt"`

	// System prompt injection configuration
	SystemPromptAnthropic        string  `json:"system_prompt_anthropic"`
	SystemPromptModeAnthropic    string  `json:"system_prompt_mode_anthropic"`
	SystemPromptOpenAI           string  `json:"system_prompt_openai"`
	SystemPromptModeOpenAI       string  `json:"system_prompt_mode_openai"`
	SystemPromptGemini           string  `json:"system_prompt_gemini"`
	SystemPromptModeGemini       string  `json:"system_prompt_mode_gemini"`
	SystemPromptAntigravity      string  `json:"system_prompt_antigravity"`
	SystemPromptModeAntigravity  string  `json:"system_prompt_mode_antigravity"`
	SystemPromptUserScopeEnabled bool    `json:"system_prompt_user_scope_enabled"`
	SystemPromptUserScopeMode    string  `json:"system_prompt_user_scope_mode"`
	SystemPromptUserScopeUserIDs []int64 `json:"system_prompt_user_scope_user_ids"`

	// Ops monitoring (vNext)
	OpsMonitoringEnabled         *bool   `json:"ops_monitoring_enabled"`
	OpsRealtimeMonitoringEnabled *bool   `json:"ops_realtime_monitoring_enabled"`
	OpsQueryModeDefault          *string `json:"ops_query_mode_default"`
	OpsMetricsIntervalSeconds    *int    `json:"ops_metrics_interval_seconds"`

	MinClaudeCodeVersion string `json:"min_claude_code_version"`
	MaxClaudeCodeVersion string `json:"max_claude_code_version"`

	// 分组隔离
	AllowUngroupedKeyScheduling bool `json:"allow_ungrouped_key_scheduling"`

	// Backend Mode
	BackendModeEnabled bool `json:"backend_mode_enabled"`

	// 兑换码发货文案 & 引流弹窗
	RedeemDeliveryText   string `json:"redeem_delivery_text"`
	AttractPopupTitle    string `json:"attract_popup_title"`
	AttractPopupMarkdown string `json:"attract_popup_markdown"`

	DashboardFireworksEnabled   *bool    `json:"dashboard_fireworks_enabled"`
	DashboardFireworksThreshold *float64 `json:"dashboard_fireworks_threshold"`

	// Gateway forwarding behavior
	EnableFingerprintUnification      *bool `json:"enable_fingerprint_unification"`
	EnableMetadataPassthrough         *bool `json:"enable_metadata_passthrough"`
	EnableCCHSigning                  *bool `json:"enable_cch_signing"`
	CodexImageGenerationBridgeEnabled *bool `json:"codex_image_generation_bridge_enabled"`

	// Codex CLI User-Agent
	CodexCLIUserAgent          *string `json:"codex_cli_user_agent"`
	CodexCLIVersion            *string `json:"codex_cli_version"`
	CodexPassthroughUAVersion  *bool   `json:"codex_official_client_passthrough_ua_version"`
	OpenAIUsageDebugLogEnabled *bool   `json:"openai_usage_debug_log_enabled"`

	// Balance low notification
	BalanceLowNotifyEnabled     *bool                   `json:"balance_low_notify_enabled"`
	BalanceLowNotifyThreshold   *float64                `json:"balance_low_notify_threshold"`
	BalanceLowNotifyRechargeURL *string                 `json:"balance_low_notify_recharge_url"`
	AccountQuotaNotifyEnabled   *bool                   `json:"account_quota_notify_enabled"`
	AccountQuotaNotifyEmails    *[]dto.NotifyEmailEntry `json:"account_quota_notify_emails"`

	// Payment configuration (integrated into settings, full replace)
	PaymentEnabled                   *bool    `json:"payment_enabled"`
	PaymentMinAmount                 *float64 `json:"payment_min_amount"`
	PaymentMaxAmount                 *float64 `json:"payment_max_amount"`
	PaymentDailyLimit                *float64 `json:"payment_daily_limit"`
	PaymentOrderTimeoutMin           *int     `json:"payment_order_timeout_minutes"`
	PaymentMaxPendingOrders          *int     `json:"payment_max_pending_orders"`
	PaymentEnabledTypes              []string `json:"payment_enabled_types"`
	PaymentBalanceDisabled           *bool    `json:"payment_balance_disabled"`
	PaymentBalanceRechargeMultiplier *float64 `json:"payment_balance_recharge_multiplier"`
	PaymentRechargeFeeRate           *float64 `json:"payment_recharge_fee_rate"`
	PaymentOrderIDPrefix             *string  `json:"payment_order_id_prefix"`
	PaymentLoadBalanceStrat          *string  `json:"payment_load_balance_strategy"`
	PaymentProductNamePrefix         *string  `json:"payment_product_name_prefix"`
	PaymentProductNameSuffix         *string  `json:"payment_product_name_suffix"`
	PaymentHelpImageURL              *string  `json:"payment_help_image_url"`
	PaymentHelpText                  *string  `json:"payment_help_text"`

	// Cancel rate limit
	PaymentCancelRateLimitEnabled *bool   `json:"payment_cancel_rate_limit_enabled"`
	PaymentCancelRateLimitMax     *int    `json:"payment_cancel_rate_limit_max"`
	PaymentCancelRateLimitWindow  *int    `json:"payment_cancel_rate_limit_window"`
	PaymentCancelRateLimitUnit    *string `json:"payment_cancel_rate_limit_unit"`
	PaymentCancelRateLimitMode    *string `json:"payment_cancel_rate_limit_window_mode"`
}

// UpdateSettings 更新系统设置
// PUT /api/v1/admin/settings
func (h *SettingHandler) UpdateSettings(c *gin.Context) {
	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	previousSettings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// 验证参数
	if req.DefaultConcurrency < 1 {
		req.DefaultConcurrency = 1
	}
	if req.DefaultBalance < 0 {
		req.DefaultBalance = 0
	}
	// 通用表格配置：兼容旧客户端未传字段时保留当前值。
	if req.TableDefaultPageSize <= 0 {
		req.TableDefaultPageSize = previousSettings.TableDefaultPageSize
	}
	if req.TablePageSizeOptions == nil {
		req.TablePageSizeOptions = previousSettings.TablePageSizeOptions
	}
	req.SMTPHost = strings.TrimSpace(req.SMTPHost)
	req.SMTPUsername = strings.TrimSpace(req.SMTPUsername)
	req.SMTPPassword = strings.TrimSpace(req.SMTPPassword)
	req.SMTPFrom = strings.TrimSpace(req.SMTPFrom)
	req.SMTPFromName = strings.TrimSpace(req.SMTPFromName)
	req.ResendAPIKey = strings.TrimSpace(req.ResendAPIKey)
	if req.SMTPPort <= 0 {
		req.SMTPPort = 587
	}
	req.DefaultSubscriptions = normalizeDefaultSubscriptions(req.DefaultSubscriptions)

	emailProvider := previousSettings.EmailProvider
	if emailProvider == "" {
		emailProvider = service.EmailProviderSMTP
	}
	if req.EmailProvider != nil {
		emailProvider = strings.ToLower(strings.TrimSpace(*req.EmailProvider))
		if !service.IsSupportedEmailProvider(emailProvider) {
			response.BadRequest(c, "Unsupported email provider")
			return
		}
	}
	resendFrom := previousSettings.ResendFrom
	if req.ResendFrom != nil {
		resendFrom = strings.TrimSpace(*req.ResendFrom)
	}
	resendFromName := previousSettings.ResendFromName
	if req.ResendFromName != nil {
		resendFromName = strings.TrimSpace(*req.ResendFromName)
	}
	if emailProvider == service.EmailProviderResend {
		if resendFrom == "" {
			response.BadRequest(c, "Resend from email is required when Resend is selected")
			return
		}
		if req.ResendAPIKey == "" && previousSettings.ResendAPIKey == "" {
			response.BadRequest(c, "Resend API key is required when Resend is selected")
			return
		}
	}

	// SMTP 配置保护：如果请求中 smtp_host 为空但数据库中已有配置，则保留已有 SMTP 配置
	// 防止前端加载设置失败时空表单覆盖已保存的 SMTP 配置
	if req.SMTPHost == "" && previousSettings.SMTPHost != "" {
		req.SMTPHost = previousSettings.SMTPHost
		req.SMTPPort = previousSettings.SMTPPort
		req.SMTPUsername = previousSettings.SMTPUsername
		req.SMTPFrom = previousSettings.SMTPFrom
		req.SMTPFromName = previousSettings.SMTPFromName
		req.SMTPUseTLS = previousSettings.SMTPUseTLS
	}

	// Turnstile 参数验证
	if req.TurnstileEnabled {
		// 检查必填字段
		if req.TurnstileSiteKey == "" {
			response.BadRequest(c, "Turnstile Site Key is required when enabled")
			return
		}
		// 如果未提供 secret key，使用已保存的值（留空保留当前值）
		if req.TurnstileSecretKey == "" {
			if previousSettings.TurnstileSecretKey == "" {
				response.BadRequest(c, "Turnstile Secret Key is required when enabled")
				return
			}
			req.TurnstileSecretKey = previousSettings.TurnstileSecretKey
		}

		// 当 site_key 或 secret_key 任一变化时验证（避免配置错误导致无法登录）
		siteKeyChanged := previousSettings.TurnstileSiteKey != req.TurnstileSiteKey
		secretKeyChanged := previousSettings.TurnstileSecretKey != req.TurnstileSecretKey
		if siteKeyChanged || secretKeyChanged {
			if err := h.turnstileService.ValidateSecretKey(c.Request.Context(), req.TurnstileSecretKey); err != nil {
				response.ErrorFrom(c, err)
				return
			}
		}
	}

	// TOTP 双因素认证参数验证
	// 只有手动配置了加密密钥才允许启用 TOTP 功能
	if req.TotpEnabled && !previousSettings.TotpEnabled {
		// 尝试启用 TOTP，检查加密密钥是否已手动配置
		if !h.settingService.IsTotpEncryptionKeyConfigured() {
			response.BadRequest(c, "Cannot enable TOTP: TOTP_ENCRYPTION_KEY environment variable must be configured first. Generate a key with 'openssl rand -hex 32' and set it in your environment.")
			return
		}
	}

	// LinuxDo Connect 参数验证
	if req.LinuxDoConnectEnabled {
		req.LinuxDoConnectClientID = strings.TrimSpace(req.LinuxDoConnectClientID)
		req.LinuxDoConnectClientSecret = strings.TrimSpace(req.LinuxDoConnectClientSecret)
		req.LinuxDoConnectRedirectURL = strings.TrimSpace(req.LinuxDoConnectRedirectURL)

		if req.LinuxDoConnectClientID == "" {
			response.BadRequest(c, "LinuxDo Client ID is required when enabled")
			return
		}
		if req.LinuxDoConnectRedirectURL == "" {
			response.BadRequest(c, "LinuxDo Redirect URL is required when enabled")
			return
		}
		if err := config.ValidateAbsoluteHTTPURL(req.LinuxDoConnectRedirectURL); err != nil {
			response.BadRequest(c, "LinuxDo Redirect URL must be an absolute http(s) URL")
			return
		}

		// 如果未提供 client_secret，则保留现有值（如有）。
		if req.LinuxDoConnectClientSecret == "" {
			if previousSettings.LinuxDoConnectClientSecret == "" {
				response.BadRequest(c, "LinuxDo Client Secret is required when enabled")
				return
			}
			req.LinuxDoConnectClientSecret = previousSettings.LinuxDoConnectClientSecret
		}
	}

	// Generic OIDC 参数验证
	if req.OIDCConnectEnabled {
		req.OIDCConnectProviderName = strings.TrimSpace(req.OIDCConnectProviderName)
		req.OIDCConnectClientID = strings.TrimSpace(req.OIDCConnectClientID)
		req.OIDCConnectClientSecret = strings.TrimSpace(req.OIDCConnectClientSecret)
		req.OIDCConnectIssuerURL = strings.TrimSpace(req.OIDCConnectIssuerURL)
		req.OIDCConnectDiscoveryURL = strings.TrimSpace(req.OIDCConnectDiscoveryURL)
		req.OIDCConnectAuthorizeURL = strings.TrimSpace(req.OIDCConnectAuthorizeURL)
		req.OIDCConnectTokenURL = strings.TrimSpace(req.OIDCConnectTokenURL)
		req.OIDCConnectUserInfoURL = strings.TrimSpace(req.OIDCConnectUserInfoURL)
		req.OIDCConnectJWKSURL = strings.TrimSpace(req.OIDCConnectJWKSURL)
		req.OIDCConnectScopes = strings.TrimSpace(req.OIDCConnectScopes)
		req.OIDCConnectRedirectURL = strings.TrimSpace(req.OIDCConnectRedirectURL)
		req.OIDCConnectFrontendRedirectURL = strings.TrimSpace(req.OIDCConnectFrontendRedirectURL)
		req.OIDCConnectTokenAuthMethod = strings.ToLower(strings.TrimSpace(req.OIDCConnectTokenAuthMethod))
		req.OIDCConnectAllowedSigningAlgs = strings.TrimSpace(req.OIDCConnectAllowedSigningAlgs)
		req.OIDCConnectUserInfoEmailPath = strings.TrimSpace(req.OIDCConnectUserInfoEmailPath)
		req.OIDCConnectUserInfoIDPath = strings.TrimSpace(req.OIDCConnectUserInfoIDPath)
		req.OIDCConnectUserInfoUsernamePath = strings.TrimSpace(req.OIDCConnectUserInfoUsernamePath)

		if req.OIDCConnectProviderName == "" {
			req.OIDCConnectProviderName = "OIDC"
		}
		if req.OIDCConnectClientID == "" {
			response.BadRequest(c, "OIDC Client ID is required when enabled")
			return
		}
		if req.OIDCConnectIssuerURL == "" {
			response.BadRequest(c, "OIDC Issuer URL is required when enabled")
			return
		}
		if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectIssuerURL); err != nil {
			response.BadRequest(c, "OIDC Issuer URL must be an absolute http(s) URL")
			return
		}
		if req.OIDCConnectDiscoveryURL != "" {
			if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectDiscoveryURL); err != nil {
				response.BadRequest(c, "OIDC Discovery URL must be an absolute http(s) URL")
				return
			}
		}
		if req.OIDCConnectAuthorizeURL != "" {
			if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectAuthorizeURL); err != nil {
				response.BadRequest(c, "OIDC Authorize URL must be an absolute http(s) URL")
				return
			}
		}
		if req.OIDCConnectTokenURL != "" {
			if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectTokenURL); err != nil {
				response.BadRequest(c, "OIDC Token URL must be an absolute http(s) URL")
				return
			}
		}
		if req.OIDCConnectUserInfoURL != "" {
			if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectUserInfoURL); err != nil {
				response.BadRequest(c, "OIDC UserInfo URL must be an absolute http(s) URL")
				return
			}
		}
		if req.OIDCConnectRedirectURL == "" {
			response.BadRequest(c, "OIDC Redirect URL is required when enabled")
			return
		}
		if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectRedirectURL); err != nil {
			response.BadRequest(c, "OIDC Redirect URL must be an absolute http(s) URL")
			return
		}
		if req.OIDCConnectFrontendRedirectURL == "" {
			response.BadRequest(c, "OIDC Frontend Redirect URL is required when enabled")
			return
		}
		if err := config.ValidateFrontendRedirectURL(req.OIDCConnectFrontendRedirectURL); err != nil {
			response.BadRequest(c, "OIDC Frontend Redirect URL is invalid")
			return
		}
		if !scopesContainOpenID(req.OIDCConnectScopes) {
			response.BadRequest(c, "OIDC scopes must contain openid")
			return
		}
		switch req.OIDCConnectTokenAuthMethod {
		case "", "client_secret_post", "client_secret_basic", "none":
		default:
			response.BadRequest(c, "OIDC Token Auth Method must be one of client_secret_post/client_secret_basic/none")
			return
		}
		if req.OIDCConnectTokenAuthMethod == "none" && !req.OIDCConnectUsePKCE {
			response.BadRequest(c, "OIDC PKCE must be enabled when token_auth_method=none")
			return
		}
		if req.OIDCConnectClockSkewSeconds < 0 || req.OIDCConnectClockSkewSeconds > 600 {
			response.BadRequest(c, "OIDC clock skew seconds must be between 0 and 600")
			return
		}
		if req.OIDCConnectValidateIDToken {
			if req.OIDCConnectAllowedSigningAlgs == "" {
				response.BadRequest(c, "OIDC Allowed Signing Algs is required when validate_id_token=true")
				return
			}
		}
		if req.OIDCConnectJWKSURL != "" {
			if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectJWKSURL); err != nil {
				response.BadRequest(c, "OIDC JWKS URL must be an absolute http(s) URL")
				return
			}
		}
		if req.OIDCConnectTokenAuthMethod == "" || req.OIDCConnectTokenAuthMethod == "client_secret_post" || req.OIDCConnectTokenAuthMethod == "client_secret_basic" {
			if req.OIDCConnectClientSecret == "" {
				if previousSettings.OIDCConnectClientSecret == "" {
					response.BadRequest(c, "OIDC Client Secret is required when enabled")
					return
				}
				req.OIDCConnectClientSecret = previousSettings.OIDCConnectClientSecret
			}
		}
	}

	// “购买订阅”页面配置验证
	purchaseEnabled := previousSettings.PurchaseSubscriptionEnabled
	if req.PurchaseSubscriptionEnabled != nil {
		purchaseEnabled = *req.PurchaseSubscriptionEnabled
	}
	purchaseURL := previousSettings.PurchaseSubscriptionURL
	if req.PurchaseSubscriptionURL != nil {
		purchaseURL = strings.TrimSpace(*req.PurchaseSubscriptionURL)
	}

	// - 启用时要求 URL 合法且非空
	// - 禁用时允许为空；若提供了 URL 也做基本校验，避免误配置
	if purchaseEnabled {
		if purchaseURL == "" {
			response.BadRequest(c, "Purchase Subscription URL is required when enabled")
			return
		}
		if err := config.ValidateAbsoluteHTTPURL(purchaseURL); err != nil {
			response.BadRequest(c, "Purchase Subscription URL must be an absolute http(s) URL")
			return
		}
	} else if purchaseURL != "" {
		if err := config.ValidateAbsoluteHTTPURL(purchaseURL); err != nil {
			response.BadRequest(c, "Purchase Subscription URL must be an absolute http(s) URL")
			return
		}
	}

	// Frontend URL 验证
	req.FrontendURL = strings.TrimSpace(req.FrontendURL)
	if req.FrontendURL != "" {
		if err := config.ValidateAbsoluteHTTPURL(req.FrontendURL); err != nil {
			response.BadRequest(c, "Frontend URL must be an absolute http(s) URL")
			return
		}
	}

	// 自定义菜单项验证
	const (
		maxCustomMenuItems    = 20
		maxMenuItemLabelLen   = 50
		maxMenuItemURLLen     = 2048
		maxMenuItemIconSVGLen = 10 * 1024 // 10KB
		maxMenuItemIDLen      = 32
	)

	customMenuJSON := previousSettings.CustomMenuItems
	if req.CustomMenuItems != nil {
		items := *req.CustomMenuItems
		if len(items) > maxCustomMenuItems {
			response.BadRequest(c, "Too many custom menu items (max 20)")
			return
		}
		for i, item := range items {
			if strings.TrimSpace(item.Label) == "" {
				response.BadRequest(c, "Custom menu item label is required")
				return
			}
			if len(item.Label) > maxMenuItemLabelLen {
				response.BadRequest(c, "Custom menu item label is too long (max 50 characters)")
				return
			}
			if strings.TrimSpace(item.URL) == "" {
				response.BadRequest(c, "Custom menu item URL is required")
				return
			}
			if len(item.URL) > maxMenuItemURLLen {
				response.BadRequest(c, "Custom menu item URL is too long (max 2048 characters)")
				return
			}
			if err := config.ValidateAbsoluteHTTPURL(strings.TrimSpace(item.URL)); err != nil {
				response.BadRequest(c, "Custom menu item URL must be an absolute http(s) URL")
				return
			}
			if item.Visibility != "user" && item.Visibility != "admin" {
				response.BadRequest(c, "Custom menu item visibility must be 'user' or 'admin'")
				return
			}
			if len(item.IconSVG) > maxMenuItemIconSVGLen {
				response.BadRequest(c, "Custom menu item icon SVG is too large (max 10KB)")
				return
			}
			// Auto-generate ID if missing
			if strings.TrimSpace(item.ID) == "" {
				id, err := generateMenuItemID()
				if err != nil {
					response.Error(c, http.StatusInternalServerError, "Failed to generate menu item ID")
					return
				}
				items[i].ID = id
			} else if len(item.ID) > maxMenuItemIDLen {
				response.BadRequest(c, "Custom menu item ID is too long (max 32 characters)")
				return
			} else if !menuItemIDPattern.MatchString(item.ID) {
				response.BadRequest(c, "Custom menu item ID contains invalid characters (only a-z, A-Z, 0-9, - and _ are allowed)")
				return
			}
		}
		// ID uniqueness check
		seen := make(map[string]struct{}, len(items))
		for _, item := range items {
			if _, exists := seen[item.ID]; exists {
				response.BadRequest(c, "Duplicate custom menu item ID: "+item.ID)
				return
			}
			seen[item.ID] = struct{}{}
		}
		menuBytes, err := json.Marshal(items)
		if err != nil {
			response.BadRequest(c, "Failed to serialize custom menu items")
			return
		}
		customMenuJSON = string(menuBytes)
	}

	// 自定义端点验证
	const (
		maxCustomEndpoints        = 10
		maxEndpointNameLen        = 50
		maxEndpointURLLen         = 2048
		maxEndpointDescriptionLen = 200
	)

	customEndpointsJSON := previousSettings.CustomEndpoints
	if req.CustomEndpoints != nil {
		endpoints := *req.CustomEndpoints
		if len(endpoints) > maxCustomEndpoints {
			response.BadRequest(c, "Too many custom endpoints (max 10)")
			return
		}
		for _, ep := range endpoints {
			if strings.TrimSpace(ep.Name) == "" {
				response.BadRequest(c, "Custom endpoint name is required")
				return
			}
			if len(ep.Name) > maxEndpointNameLen {
				response.BadRequest(c, "Custom endpoint name is too long (max 50 characters)")
				return
			}
			if strings.TrimSpace(ep.Endpoint) == "" {
				response.BadRequest(c, "Custom endpoint URL is required")
				return
			}
			if len(ep.Endpoint) > maxEndpointURLLen {
				response.BadRequest(c, "Custom endpoint URL is too long (max 2048 characters)")
				return
			}
			if err := config.ValidateAbsoluteHTTPURL(strings.TrimSpace(ep.Endpoint)); err != nil {
				response.BadRequest(c, "Custom endpoint URL must be an absolute http(s) URL")
				return
			}
			if len(ep.Description) > maxEndpointDescriptionLen {
				response.BadRequest(c, "Custom endpoint description is too long (max 200 characters)")
				return
			}
		}
		endpointBytes, err := json.Marshal(endpoints)
		if err != nil {
			response.BadRequest(c, "Failed to serialize custom endpoints")
			return
		}
		customEndpointsJSON = string(endpointBytes)
	}

	generatedImageStorageSource := previousSettings.GeneratedImageStorageSource
	if generatedImageStorageSource == "" {
		generatedImageStorageSource = service.GeneratedImageStorageSourceDB
	}
	if req.GeneratedImageStorageSource != nil {
		generatedImageStorageSource = service.NormalizeGeneratedImageStorageSource(*req.GeneratedImageStorageSource)
		if generatedImageStorageSource == "" {
			response.BadRequest(c, "Generated image storage source must be db or qiniu")
			return
		}
	}
	generatedImageStorageConfigSource := previousSettings.GeneratedImageStorageConfigSource
	if generatedImageStorageConfigSource == "" {
		generatedImageStorageConfigSource = generatedImageStorageSource
	}
	if req.GeneratedImageStorageConfigSource != nil {
		generatedImageStorageConfigSource = service.NormalizeGeneratedImageStorageSource(*req.GeneratedImageStorageConfigSource)
		if generatedImageStorageConfigSource == "" {
			response.BadRequest(c, "Generated image storage config source must be db or qiniu")
			return
		}
	}
	qiniuAccessKey := previousSettings.QiniuAccessKey
	if req.QiniuAccessKey != nil {
		qiniuAccessKey = strings.TrimSpace(*req.QiniuAccessKey)
	}
	qiniuSecretKey := strings.TrimSpace(req.QiniuSecretKey)
	if qiniuSecretKey == "" {
		qiniuSecretKey = previousSettings.QiniuSecretKey
	}
	qiniuBucket := previousSettings.QiniuBucket
	if req.QiniuBucket != nil {
		qiniuBucket = strings.TrimSpace(*req.QiniuBucket)
	}
	qiniuCDNDomain := previousSettings.QiniuCDNDomain
	if req.QiniuCDNDomain != nil {
		qiniuCDNDomain = strings.TrimRight(strings.TrimSpace(*req.QiniuCDNDomain), "/")
	}
	qiniuPrefix := previousSettings.QiniuPrefix
	if req.QiniuPrefix != nil {
		qiniuPrefix = strings.Trim(strings.TrimSpace(*req.QiniuPrefix), "/")
	}
	qiniuUseHTTPS := previousSettings.QiniuUseHTTPS
	if req.QiniuUseHTTPS != nil {
		qiniuUseHTTPS = *req.QiniuUseHTTPS
	}
	qiniuUploadTimeoutSeconds := previousSettings.QiniuUploadTimeoutSeconds
	if req.QiniuUploadTimeoutSeconds != nil {
		qiniuUploadTimeoutSeconds = *req.QiniuUploadTimeoutSeconds
	}
	qiniuTokenTTLSeconds := previousSettings.QiniuTokenTTLSeconds
	if req.QiniuTokenTTLSeconds != nil {
		qiniuTokenTTLSeconds = *req.QiniuTokenTTLSeconds
	}
	if qiniuUploadTimeoutSeconds <= 0 {
		qiniuUploadTimeoutSeconds = service.DefaultQiniuUploadTimeoutSeconds
	}
	if qiniuTokenTTLSeconds <= 0 {
		qiniuTokenTTLSeconds = service.DefaultQiniuTokenTTLSeconds
	}
	if qiniuPrefix == "" {
		qiniuPrefix = service.DefaultQiniuPrefix
	}
	if generatedImageStorageSource == service.GeneratedImageStorageSourceQiniu {
		if qiniuAccessKey == "" {
			response.BadRequest(c, "Qiniu Access Key is required when generated image storage source is Qiniu")
			return
		}
		if qiniuSecretKey == "" {
			response.BadRequest(c, "Qiniu Secret Key is required when generated image storage source is Qiniu")
			return
		}
		if qiniuBucket == "" {
			response.BadRequest(c, "Qiniu Bucket is required when generated image storage source is Qiniu")
			return
		}
		if qiniuCDNDomain == "" {
			response.BadRequest(c, "Qiniu CDN domain is required when generated image storage source is Qiniu")
			return
		}
		cdnDomainForValidation := qiniuCDNDomain
		if !strings.Contains(cdnDomainForValidation, "://") {
			cdnDomainForValidation = "https://" + cdnDomainForValidation
		}
		if err := config.ValidateAbsoluteHTTPURL(cdnDomainForValidation); err != nil {
			response.BadRequest(c, "Qiniu CDN domain must be a valid http(s) URL or domain")
			return
		}
	}

	// Ops metrics collector interval validation (seconds).
	if req.OpsMetricsIntervalSeconds != nil {
		v := *req.OpsMetricsIntervalSeconds
		if v < 60 {
			v = 60
		}
		if v > 3600 {
			v = 3600
		}
		req.OpsMetricsIntervalSeconds = &v
	}
	defaultSubscriptions := make([]service.DefaultSubscriptionSetting, 0, len(req.DefaultSubscriptions))
	for _, sub := range req.DefaultSubscriptions {
		defaultSubscriptions = append(defaultSubscriptions, service.DefaultSubscriptionSetting{
			GroupID:      sub.GroupID,
			ValidityDays: sub.ValidityDays,
		})
	}

	// 验证最低版本号格式（空字符串=禁用，或合法 semver）
	if req.MinClaudeCodeVersion != "" {
		if !semverPattern.MatchString(req.MinClaudeCodeVersion) {
			response.Error(c, http.StatusBadRequest, "min_claude_code_version must be empty or a valid semver (e.g. 2.1.63)")
			return
		}
	}

	// 验证最高版本号格式（空字符串=禁用，或合法 semver）
	if req.MaxClaudeCodeVersion != "" {
		if !semverPattern.MatchString(req.MaxClaudeCodeVersion) {
			response.Error(c, http.StatusBadRequest, "max_claude_code_version must be empty or a valid semver (e.g. 3.0.0)")
			return
		}
	}

	// 交叉验证：如果同时设置了最低和最高版本号，最高版本号必须 >= 最低版本号
	if req.MinClaudeCodeVersion != "" && req.MaxClaudeCodeVersion != "" {
		if service.CompareVersions(req.MaxClaudeCodeVersion, req.MinClaudeCodeVersion) < 0 {
			response.Error(c, http.StatusBadRequest, "max_claude_code_version must be greater than or equal to min_claude_code_version")
			return
		}
	}

	settings := &service.SystemSettings{
		RegistrationEnabled:              req.RegistrationEnabled,
		EmailVerifyEnabled:               req.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist: req.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                 req.PromoCodeEnabled,
		PasswordResetEnabled:             req.PasswordResetEnabled,
		FrontendURL:                      req.FrontendURL,
		InvitationCodeEnabled:            req.InvitationCodeEnabled,
		TotpEnabled:                      req.TotpEnabled,
		ChannelMonitorEnabled:            req.ChannelMonitorEnabled,
		ChannelMonitorDefaultIntervalSeconds: service.NormalizeChannelMonitorInterval(
			req.ChannelMonitorDefaultIntervalSeconds,
			service.NormalizeChannelMonitorInterval(
				previousSettings.ChannelMonitorDefaultIntervalSeconds,
				service.ChannelMonitorFallbackIntervalSecond,
			),
		),
		SMTPHost:                        req.SMTPHost,
		SMTPPort:                        req.SMTPPort,
		SMTPUsername:                    req.SMTPUsername,
		SMTPPassword:                    req.SMTPPassword,
		SMTPFrom:                        req.SMTPFrom,
		SMTPFromName:                    req.SMTPFromName,
		SMTPUseTLS:                      req.SMTPUseTLS,
		EmailProvider:                   emailProvider,
		ResendAPIKey:                    req.ResendAPIKey,
		ResendFrom:                      resendFrom,
		ResendFromName:                  resendFromName,
		TurnstileEnabled:                req.TurnstileEnabled,
		TurnstileSiteKey:                req.TurnstileSiteKey,
		TurnstileSecretKey:              req.TurnstileSecretKey,
		LinuxDoConnectEnabled:           req.LinuxDoConnectEnabled,
		LinuxDoConnectClientID:          req.LinuxDoConnectClientID,
		LinuxDoConnectClientSecret:      req.LinuxDoConnectClientSecret,
		LinuxDoConnectRedirectURL:       req.LinuxDoConnectRedirectURL,
		OIDCConnectEnabled:              req.OIDCConnectEnabled,
		OIDCConnectProviderName:         req.OIDCConnectProviderName,
		OIDCConnectClientID:             req.OIDCConnectClientID,
		OIDCConnectClientSecret:         req.OIDCConnectClientSecret,
		OIDCConnectIssuerURL:            req.OIDCConnectIssuerURL,
		OIDCConnectDiscoveryURL:         req.OIDCConnectDiscoveryURL,
		OIDCConnectAuthorizeURL:         req.OIDCConnectAuthorizeURL,
		OIDCConnectTokenURL:             req.OIDCConnectTokenURL,
		OIDCConnectUserInfoURL:          req.OIDCConnectUserInfoURL,
		OIDCConnectJWKSURL:              req.OIDCConnectJWKSURL,
		OIDCConnectScopes:               req.OIDCConnectScopes,
		OIDCConnectRedirectURL:          req.OIDCConnectRedirectURL,
		OIDCConnectFrontendRedirectURL:  req.OIDCConnectFrontendRedirectURL,
		OIDCConnectTokenAuthMethod:      req.OIDCConnectTokenAuthMethod,
		OIDCConnectUsePKCE:              req.OIDCConnectUsePKCE,
		OIDCConnectValidateIDToken:      req.OIDCConnectValidateIDToken,
		OIDCConnectAllowedSigningAlgs:   req.OIDCConnectAllowedSigningAlgs,
		OIDCConnectClockSkewSeconds:     req.OIDCConnectClockSkewSeconds,
		OIDCConnectRequireEmailVerified: req.OIDCConnectRequireEmailVerified,
		OIDCConnectUserInfoEmailPath:    req.OIDCConnectUserInfoEmailPath,
		OIDCConnectUserInfoIDPath:       req.OIDCConnectUserInfoIDPath,
		OIDCConnectUserInfoUsernamePath: req.OIDCConnectUserInfoUsernamePath,
		SiteName:                        req.SiteName,
		SiteLogo:                        req.SiteLogo,
		SiteSubtitle:                    req.SiteSubtitle,
		APIBaseURL:                      req.APIBaseURL,
		ContactInfo:                     req.ContactInfo,
		DocURL:                          req.DocURL,
		HomeContent:                     req.HomeContent,
		HideCcsImportButton:             req.HideCcsImportButton,
		PurchaseSubscriptionEnabled:     purchaseEnabled,
		PurchaseSubscriptionURL:         purchaseURL,
		TableDefaultPageSize:            req.TableDefaultPageSize,
		TablePageSizeOptions:            req.TablePageSizeOptions,
		CustomMenuItems:                 customMenuJSON,
		CustomEndpoints:                 customEndpointsJSON,
		OpenAIUseKeyModelID: func() string {
			if req.OpenAIUseKeyModelID != nil {
				return strings.TrimSpace(*req.OpenAIUseKeyModelID)
			}
			return previousSettings.OpenAIUseKeyModelID
		}(),
		OpenAIImageURLCacheTTLHours: func() int {
			if req.OpenAIImageURLCacheTTLHours != nil {
				return *req.OpenAIImageURLCacheTTLHours
			}
			return previousSettings.OpenAIImageURLCacheTTLHours
		}(),
		GeneratedImageStorageSource:       generatedImageStorageSource,
		GeneratedImageStorageConfigSource: generatedImageStorageConfigSource,
		QiniuAccessKey:                    qiniuAccessKey,
		QiniuSecretKey:                    qiniuSecretKey,
		QiniuBucket:                       qiniuBucket,
		QiniuCDNDomain:                    qiniuCDNDomain,
		QiniuPrefix:                       qiniuPrefix,
		QiniuUseHTTPS:                     qiniuUseHTTPS,
		QiniuUploadTimeoutSeconds:         qiniuUploadTimeoutSeconds,
		QiniuTokenTTLSeconds:              qiniuTokenTTLSeconds,
		GeneratedImageCleanupEnabled: func() bool {
			if req.GeneratedImageCleanupEnabled != nil {
				return *req.GeneratedImageCleanupEnabled
			}
			return previousSettings.GeneratedImageCleanupEnabled
		}(),
		MediaSyncWaitTimeoutSeconds: func() int {
			if req.MediaSyncWaitTimeoutSeconds != nil {
				return *req.MediaSyncWaitTimeoutSeconds
			}
			return previousSettings.MediaSyncWaitTimeoutSeconds
		}(),
		MediaSyncTimeoutFallbackAsyncEnabled: func() bool {
			if req.MediaSyncTimeoutFallbackAsyncEnabled != nil {
				return *req.MediaSyncTimeoutFallbackAsyncEnabled
			}
			return previousSettings.MediaSyncTimeoutFallbackAsyncEnabled
		}(),
		MediaSyncTimeoutBillingPolicy: func() string {
			if req.MediaSyncTimeoutBillingPolicy != nil {
				return *req.MediaSyncTimeoutBillingPolicy
			}
			return previousSettings.MediaSyncTimeoutBillingPolicy
		}(),
		MediaSyncTimeoutPenaltyRatio: func() float64 {
			if req.MediaSyncTimeoutPenaltyRatio != nil {
				return *req.MediaSyncTimeoutPenaltyRatio
			}
			return previousSettings.MediaSyncTimeoutPenaltyRatio
		}(),
		MediaVideoStorageMode: func() string {
			if req.MediaVideoStorageMode != nil {
				return *req.MediaVideoStorageMode
			}
			return previousSettings.MediaVideoStorageMode
		}(),
		MediaVideoProxyFallbackEnabled: func() bool {
			if req.MediaVideoProxyFallbackEnabled != nil {
				return *req.MediaVideoProxyFallbackEnabled
			}
			return previousSettings.MediaVideoProxyFallbackEnabled
		}(),
		DefaultConcurrency:           req.DefaultConcurrency,
		DefaultBalance:               req.DefaultBalance,
		DefaultSubscriptions:         defaultSubscriptions,
		EnableModelFallback:          req.EnableModelFallback,
		FallbackModelAnthropic:       req.FallbackModelAnthropic,
		FallbackModelOpenAI:          req.FallbackModelOpenAI,
		FallbackModelGemini:          req.FallbackModelGemini,
		FallbackModelAntigravity:     req.FallbackModelAntigravity,
		EnableIdentityPatch:          req.EnableIdentityPatch,
		IdentityPatchPrompt:          req.IdentityPatchPrompt,
		SystemPromptAnthropic:        req.SystemPromptAnthropic,
		SystemPromptModeAnthropic:    req.SystemPromptModeAnthropic,
		SystemPromptOpenAI:           req.SystemPromptOpenAI,
		SystemPromptModeOpenAI:       req.SystemPromptModeOpenAI,
		SystemPromptGemini:           req.SystemPromptGemini,
		SystemPromptModeGemini:       req.SystemPromptModeGemini,
		SystemPromptAntigravity:      req.SystemPromptAntigravity,
		SystemPromptModeAntigravity:  req.SystemPromptModeAntigravity,
		SystemPromptUserScopeEnabled: req.SystemPromptUserScopeEnabled,
		SystemPromptUserScopeMode:    req.SystemPromptUserScopeMode,
		SystemPromptUserScopeUserIDs: req.SystemPromptUserScopeUserIDs,
		MinClaudeCodeVersion:         req.MinClaudeCodeVersion,
		MaxClaudeCodeVersion:         req.MaxClaudeCodeVersion,
		AllowUngroupedKeyScheduling:  req.AllowUngroupedKeyScheduling,
		BackendModeEnabled:           req.BackendModeEnabled,
		RedeemDeliveryText:           req.RedeemDeliveryText,
		AttractPopupTitle:            req.AttractPopupTitle,
		AttractPopupMarkdown:         req.AttractPopupMarkdown,
		DashboardFireworksEnabled: func() bool {
			if req.DashboardFireworksEnabled != nil {
				return *req.DashboardFireworksEnabled
			}
			return previousSettings.DashboardFireworksEnabled
		}(),
		DashboardFireworksThreshold: func() float64 {
			if req.DashboardFireworksThreshold != nil {
				return *req.DashboardFireworksThreshold
			}
			return previousSettings.DashboardFireworksThreshold
		}(),
		OpsMonitoringEnabled: func() bool {
			if req.OpsMonitoringEnabled != nil {
				return *req.OpsMonitoringEnabled
			}
			return previousSettings.OpsMonitoringEnabled
		}(),
		OpsRealtimeMonitoringEnabled: func() bool {
			if req.OpsRealtimeMonitoringEnabled != nil {
				return *req.OpsRealtimeMonitoringEnabled
			}
			return previousSettings.OpsRealtimeMonitoringEnabled
		}(),
		OpsQueryModeDefault: func() string {
			if req.OpsQueryModeDefault != nil {
				return *req.OpsQueryModeDefault
			}
			return previousSettings.OpsQueryModeDefault
		}(),
		OpsMetricsIntervalSeconds: func() int {
			if req.OpsMetricsIntervalSeconds != nil {
				return *req.OpsMetricsIntervalSeconds
			}
			return previousSettings.OpsMetricsIntervalSeconds
		}(),
		EnableFingerprintUnification: func() bool {
			if req.EnableFingerprintUnification != nil {
				return *req.EnableFingerprintUnification
			}
			return previousSettings.EnableFingerprintUnification
		}(),
		EnableMetadataPassthrough: func() bool {
			if req.EnableMetadataPassthrough != nil {
				return *req.EnableMetadataPassthrough
			}
			return previousSettings.EnableMetadataPassthrough
		}(),
		EnableCCHSigning: func() bool {
			if req.EnableCCHSigning != nil {
				return *req.EnableCCHSigning
			}
			return previousSettings.EnableCCHSigning
		}(),
		CodexImageGenerationBridgeEnabled: func() bool {
			if req.CodexImageGenerationBridgeEnabled != nil {
				return *req.CodexImageGenerationBridgeEnabled
			}
			return previousSettings.CodexImageGenerationBridgeEnabled
		}(),
		CodexCLIUserAgent: func() string {
			if req.CodexCLIUserAgent != nil {
				return strings.TrimSpace(*req.CodexCLIUserAgent)
			}
			return previousSettings.CodexCLIUserAgent
		}(),
		CodexCLIVersion: func() string {
			if req.CodexCLIVersion != nil {
				return strings.TrimSpace(*req.CodexCLIVersion)
			}
			return previousSettings.CodexCLIVersion
		}(),
		CodexPassthroughUAVersion: func() bool {
			if req.CodexPassthroughUAVersion != nil {
				return *req.CodexPassthroughUAVersion
			}
			return previousSettings.CodexPassthroughUAVersion
		}(),
		OpenAIUsageDebugLogEnabled: func() bool {
			if req.OpenAIUsageDebugLogEnabled != nil {
				return *req.OpenAIUsageDebugLogEnabled
			}
			return previousSettings.OpenAIUsageDebugLogEnabled
		}(),
		BalanceLowNotifyEnabled: func() bool {
			if req.BalanceLowNotifyEnabled != nil {
				return *req.BalanceLowNotifyEnabled
			}
			return previousSettings.BalanceLowNotifyEnabled
		}(),
		BalanceLowNotifyThreshold: func() float64 {
			if req.BalanceLowNotifyThreshold != nil {
				return *req.BalanceLowNotifyThreshold
			}
			return previousSettings.BalanceLowNotifyThreshold
		}(),
		BalanceLowNotifyRechargeURL: func() string {
			if req.BalanceLowNotifyRechargeURL != nil {
				return *req.BalanceLowNotifyRechargeURL
			}
			return previousSettings.BalanceLowNotifyRechargeURL
		}(),
		AccountQuotaNotifyEnabled: func() bool {
			if req.AccountQuotaNotifyEnabled != nil {
				return *req.AccountQuotaNotifyEnabled
			}
			return previousSettings.AccountQuotaNotifyEnabled
		}(),
		AccountQuotaNotifyEmails: func() []service.NotifyEmailEntry {
			if req.AccountQuotaNotifyEmails != nil {
				return dto.NotifyEmailEntriesToService(*req.AccountQuotaNotifyEmails)
			}
			return previousSettings.AccountQuotaNotifyEmails
		}(),
	}

	if err := h.settingService.UpdateSettings(c.Request.Context(), settings); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// Update payment configuration (integrated into system settings).
	// Skip if no payment fields were provided (prevents accidental wipe).
	if h.paymentConfigService != nil && hasPaymentFields(req) {
		paymentReq := service.UpdatePaymentConfigRequest{
			Enabled:                   req.PaymentEnabled,
			MinAmount:                 req.PaymentMinAmount,
			MaxAmount:                 req.PaymentMaxAmount,
			DailyLimit:                req.PaymentDailyLimit,
			OrderTimeoutMin:           req.PaymentOrderTimeoutMin,
			MaxPendingOrders:          req.PaymentMaxPendingOrders,
			EnabledTypes:              req.PaymentEnabledTypes,
			BalanceDisabled:           req.PaymentBalanceDisabled,
			BalanceRechargeMultiplier: req.PaymentBalanceRechargeMultiplier,
			RechargeFeeRate:           req.PaymentRechargeFeeRate,
			OrderIDPrefix:             req.PaymentOrderIDPrefix,
			LoadBalanceStrategy:       req.PaymentLoadBalanceStrat,
			ProductNamePrefix:         req.PaymentProductNamePrefix,
			ProductNameSuffix:         req.PaymentProductNameSuffix,
			HelpImageURL:              req.PaymentHelpImageURL,
			HelpText:                  req.PaymentHelpText,
			CancelRateLimitEnabled:    req.PaymentCancelRateLimitEnabled,
			CancelRateLimitMax:        req.PaymentCancelRateLimitMax,
			CancelRateLimitWindow:     req.PaymentCancelRateLimitWindow,
			CancelRateLimitUnit:       req.PaymentCancelRateLimitUnit,
			CancelRateLimitMode:       req.PaymentCancelRateLimitMode,
		}
		if err := h.paymentConfigService.UpdatePaymentConfig(c.Request.Context(), paymentReq); err != nil {
			response.ErrorFrom(c, err)
			return
		}
		// Refresh in-memory provider registry so config changes take effect immediately
		if h.paymentService != nil {
			h.paymentService.RefreshProviders(c.Request.Context())
		}
	}

	h.auditSettingsUpdate(c, previousSettings, settings, req)

	// 重新获取设置返回
	updatedSettings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	updatedDefaultSubscriptions := make([]dto.DefaultSubscriptionSetting, 0, len(updatedSettings.DefaultSubscriptions))
	for _, sub := range updatedSettings.DefaultSubscriptions {
		updatedDefaultSubscriptions = append(updatedDefaultSubscriptions, dto.DefaultSubscriptionSetting{
			GroupID:      sub.GroupID,
			ValidityDays: sub.ValidityDays,
		})
	}

	// Reload payment config for response
	var updatedPaymentCfg *service.PaymentConfig
	if h.paymentConfigService != nil {
		updatedPaymentCfg, _ = h.paymentConfigService.GetPaymentConfig(c.Request.Context())
	}
	if updatedPaymentCfg == nil {
		updatedPaymentCfg = &service.PaymentConfig{}
	}

	response.Success(c, dto.SystemSettings{
		RegistrationEnabled:                  updatedSettings.RegistrationEnabled,
		EmailVerifyEnabled:                   updatedSettings.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist:     updatedSettings.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                     updatedSettings.PromoCodeEnabled,
		PasswordResetEnabled:                 updatedSettings.PasswordResetEnabled,
		FrontendURL:                          updatedSettings.FrontendURL,
		InvitationCodeEnabled:                updatedSettings.InvitationCodeEnabled,
		TotpEnabled:                          updatedSettings.TotpEnabled,
		TotpEncryptionKeyConfigured:          h.settingService.IsTotpEncryptionKeyConfigured(),
		ChannelMonitorEnabled:                updatedSettings.ChannelMonitorEnabled,
		ChannelMonitorDefaultIntervalSeconds: updatedSettings.ChannelMonitorDefaultIntervalSeconds,
		SMTPHost:                             updatedSettings.SMTPHost,
		SMTPPort:                             updatedSettings.SMTPPort,
		SMTPUsername:                         updatedSettings.SMTPUsername,
		SMTPPasswordConfigured:               updatedSettings.SMTPPasswordConfigured,
		SMTPFrom:                             updatedSettings.SMTPFrom,
		SMTPFromName:                         updatedSettings.SMTPFromName,
		SMTPUseTLS:                           updatedSettings.SMTPUseTLS,
		EmailProvider:                        updatedSettings.EmailProvider,
		ResendAPIKeyConfigured:               updatedSettings.ResendAPIKeyConfigured,
		ResendFrom:                           updatedSettings.ResendFrom,
		ResendFromName:                       updatedSettings.ResendFromName,
		TurnstileEnabled:                     updatedSettings.TurnstileEnabled,
		TurnstileSiteKey:                     updatedSettings.TurnstileSiteKey,
		TurnstileSecretKeyConfigured:         updatedSettings.TurnstileSecretKeyConfigured,
		LinuxDoConnectEnabled:                updatedSettings.LinuxDoConnectEnabled,
		LinuxDoConnectClientID:               updatedSettings.LinuxDoConnectClientID,
		LinuxDoConnectClientSecretConfigured: updatedSettings.LinuxDoConnectClientSecretConfigured,
		LinuxDoConnectRedirectURL:            updatedSettings.LinuxDoConnectRedirectURL,
		OIDCConnectEnabled:                   updatedSettings.OIDCConnectEnabled,
		OIDCConnectProviderName:              updatedSettings.OIDCConnectProviderName,
		OIDCConnectClientID:                  updatedSettings.OIDCConnectClientID,
		OIDCConnectClientSecretConfigured:    updatedSettings.OIDCConnectClientSecretConfigured,
		OIDCConnectIssuerURL:                 updatedSettings.OIDCConnectIssuerURL,
		OIDCConnectDiscoveryURL:              updatedSettings.OIDCConnectDiscoveryURL,
		OIDCConnectAuthorizeURL:              updatedSettings.OIDCConnectAuthorizeURL,
		OIDCConnectTokenURL:                  updatedSettings.OIDCConnectTokenURL,
		OIDCConnectUserInfoURL:               updatedSettings.OIDCConnectUserInfoURL,
		OIDCConnectJWKSURL:                   updatedSettings.OIDCConnectJWKSURL,
		OIDCConnectScopes:                    updatedSettings.OIDCConnectScopes,
		OIDCConnectRedirectURL:               updatedSettings.OIDCConnectRedirectURL,
		OIDCConnectFrontendRedirectURL:       updatedSettings.OIDCConnectFrontendRedirectURL,
		OIDCConnectTokenAuthMethod:           updatedSettings.OIDCConnectTokenAuthMethod,
		OIDCConnectUsePKCE:                   updatedSettings.OIDCConnectUsePKCE,
		OIDCConnectValidateIDToken:           updatedSettings.OIDCConnectValidateIDToken,
		OIDCConnectAllowedSigningAlgs:        updatedSettings.OIDCConnectAllowedSigningAlgs,
		OIDCConnectClockSkewSeconds:          updatedSettings.OIDCConnectClockSkewSeconds,
		OIDCConnectRequireEmailVerified:      updatedSettings.OIDCConnectRequireEmailVerified,
		OIDCConnectUserInfoEmailPath:         updatedSettings.OIDCConnectUserInfoEmailPath,
		OIDCConnectUserInfoIDPath:            updatedSettings.OIDCConnectUserInfoIDPath,
		OIDCConnectUserInfoUsernamePath:      updatedSettings.OIDCConnectUserInfoUsernamePath,
		SiteName:                             updatedSettings.SiteName,
		SiteLogo:                             updatedSettings.SiteLogo,
		SiteSubtitle:                         updatedSettings.SiteSubtitle,
		APIBaseURL:                           updatedSettings.APIBaseURL,
		ContactInfo:                          updatedSettings.ContactInfo,
		DocURL:                               updatedSettings.DocURL,
		HomeContent:                          updatedSettings.HomeContent,
		HideCcsImportButton:                  updatedSettings.HideCcsImportButton,
		PurchaseSubscriptionEnabled:          updatedSettings.PurchaseSubscriptionEnabled,
		PurchaseSubscriptionURL:              updatedSettings.PurchaseSubscriptionURL,
		TableDefaultPageSize:                 updatedSettings.TableDefaultPageSize,
		TablePageSizeOptions:                 updatedSettings.TablePageSizeOptions,
		CustomMenuItems:                      dto.ParseCustomMenuItems(updatedSettings.CustomMenuItems),
		CustomEndpoints:                      dto.ParseCustomEndpoints(updatedSettings.CustomEndpoints),
		OpenAIUseKeyModelID:                  updatedSettings.OpenAIUseKeyModelID,
		OpenAIImageURLCacheTTLHours:          updatedSettings.OpenAIImageURLCacheTTLHours,
		GeneratedImageStorageSource:          updatedSettings.GeneratedImageStorageSource,
		GeneratedImageStorageConfigSource:    updatedSettings.GeneratedImageStorageConfigSource,
		QiniuAccessKey:                       updatedSettings.QiniuAccessKey,
		QiniuSecretKeyConfigured:             updatedSettings.QiniuSecretKeyConfigured,
		QiniuBucket:                          updatedSettings.QiniuBucket,
		QiniuCDNDomain:                       updatedSettings.QiniuCDNDomain,
		QiniuPrefix:                          updatedSettings.QiniuPrefix,
		QiniuUseHTTPS:                        updatedSettings.QiniuUseHTTPS,
		QiniuUploadTimeoutSeconds:            updatedSettings.QiniuUploadTimeoutSeconds,
		QiniuTokenTTLSeconds:                 updatedSettings.QiniuTokenTTLSeconds,
		GeneratedImageCleanupEnabled:         updatedSettings.GeneratedImageCleanupEnabled,
		MediaSyncWaitTimeoutSeconds:          updatedSettings.MediaSyncWaitTimeoutSeconds,
		MediaSyncTimeoutFallbackAsyncEnabled: updatedSettings.MediaSyncTimeoutFallbackAsyncEnabled,
		MediaSyncTimeoutBillingPolicy:        updatedSettings.MediaSyncTimeoutBillingPolicy,
		MediaSyncTimeoutPenaltyRatio:         updatedSettings.MediaSyncTimeoutPenaltyRatio,
		MediaVideoStorageMode:                updatedSettings.MediaVideoStorageMode,
		MediaVideoProxyFallbackEnabled:       updatedSettings.MediaVideoProxyFallbackEnabled,
		DefaultConcurrency:                   updatedSettings.DefaultConcurrency,
		DefaultBalance:                       updatedSettings.DefaultBalance,
		DefaultSubscriptions:                 updatedDefaultSubscriptions,
		EnableModelFallback:                  updatedSettings.EnableModelFallback,
		FallbackModelAnthropic:               updatedSettings.FallbackModelAnthropic,
		FallbackModelOpenAI:                  updatedSettings.FallbackModelOpenAI,
		FallbackModelGemini:                  updatedSettings.FallbackModelGemini,
		FallbackModelAntigravity:             updatedSettings.FallbackModelAntigravity,
		EnableIdentityPatch:                  updatedSettings.EnableIdentityPatch,
		IdentityPatchPrompt:                  updatedSettings.IdentityPatchPrompt,
		SystemPromptAnthropic:                updatedSettings.SystemPromptAnthropic,
		SystemPromptModeAnthropic:            updatedSettings.SystemPromptModeAnthropic,
		SystemPromptOpenAI:                   updatedSettings.SystemPromptOpenAI,
		SystemPromptModeOpenAI:               updatedSettings.SystemPromptModeOpenAI,
		SystemPromptGemini:                   updatedSettings.SystemPromptGemini,
		SystemPromptModeGemini:               updatedSettings.SystemPromptModeGemini,
		SystemPromptAntigravity:              updatedSettings.SystemPromptAntigravity,
		SystemPromptModeAntigravity:          updatedSettings.SystemPromptModeAntigravity,
		SystemPromptUserScopeEnabled:         updatedSettings.SystemPromptUserScopeEnabled,
		SystemPromptUserScopeMode:            updatedSettings.SystemPromptUserScopeMode,
		SystemPromptUserScopeUserIDs:         updatedSettings.SystemPromptUserScopeUserIDs,
		OpsMonitoringEnabled:                 updatedSettings.OpsMonitoringEnabled,
		OpsRealtimeMonitoringEnabled:         updatedSettings.OpsRealtimeMonitoringEnabled,
		OpsQueryModeDefault:                  updatedSettings.OpsQueryModeDefault,
		OpsMetricsIntervalSeconds:            updatedSettings.OpsMetricsIntervalSeconds,
		MinClaudeCodeVersion:                 updatedSettings.MinClaudeCodeVersion,
		MaxClaudeCodeVersion:                 updatedSettings.MaxClaudeCodeVersion,
		AllowUngroupedKeyScheduling:          updatedSettings.AllowUngroupedKeyScheduling,
		BackendModeEnabled:                   updatedSettings.BackendModeEnabled,
		RedeemDeliveryText:                   updatedSettings.RedeemDeliveryText,
		AttractPopupTitle:                    updatedSettings.AttractPopupTitle,
		AttractPopupMarkdown:                 updatedSettings.AttractPopupMarkdown,
		DashboardFireworksEnabled:            updatedSettings.DashboardFireworksEnabled,
		DashboardFireworksThreshold:          updatedSettings.DashboardFireworksThreshold,
		EnableFingerprintUnification:         updatedSettings.EnableFingerprintUnification,
		EnableMetadataPassthrough:            updatedSettings.EnableMetadataPassthrough,
		EnableCCHSigning:                     updatedSettings.EnableCCHSigning,
		CodexImageGenerationBridgeEnabled:    updatedSettings.CodexImageGenerationBridgeEnabled,
		CodexCLIUserAgent:                    updatedSettings.CodexCLIUserAgent,
		CodexCLIVersion:                      updatedSettings.CodexCLIVersion,
		CodexPassthroughUAVersion:            updatedSettings.CodexPassthroughUAVersion,
		OpenAIUsageDebugLogEnabled:           updatedSettings.OpenAIUsageDebugLogEnabled,
		BalanceLowNotifyEnabled:              updatedSettings.BalanceLowNotifyEnabled,
		BalanceLowNotifyThreshold:            updatedSettings.BalanceLowNotifyThreshold,
		BalanceLowNotifyRechargeURL:          updatedSettings.BalanceLowNotifyRechargeURL,
		AccountQuotaNotifyEnabled:            updatedSettings.AccountQuotaNotifyEnabled,
		AccountQuotaNotifyEmails:             dto.NotifyEmailEntriesFromService(updatedSettings.AccountQuotaNotifyEmails),
		PaymentEnabled:                       updatedPaymentCfg.Enabled,
		PaymentMinAmount:                     updatedPaymentCfg.MinAmount,
		PaymentMaxAmount:                     updatedPaymentCfg.MaxAmount,
		PaymentDailyLimit:                    updatedPaymentCfg.DailyLimit,
		PaymentOrderTimeoutMin:               updatedPaymentCfg.OrderTimeoutMin,
		PaymentMaxPendingOrders:              updatedPaymentCfg.MaxPendingOrders,
		PaymentEnabledTypes:                  updatedPaymentCfg.EnabledTypes,
		PaymentBalanceDisabled:               updatedPaymentCfg.BalanceDisabled,
		PaymentBalanceRechargeMultiplier:     updatedPaymentCfg.BalanceRechargeMultiplier,
		PaymentRechargeFeeRate:               updatedPaymentCfg.RechargeFeeRate,
		PaymentOrderIDPrefix:                 updatedPaymentCfg.OrderIDPrefix,
		PaymentLoadBalanceStrat:              updatedPaymentCfg.LoadBalanceStrategy,
		PaymentProductNamePrefix:             updatedPaymentCfg.ProductNamePrefix,
		PaymentProductNameSuffix:             updatedPaymentCfg.ProductNameSuffix,
		PaymentHelpImageURL:                  updatedPaymentCfg.HelpImageURL,
		PaymentHelpText:                      updatedPaymentCfg.HelpText,
		PaymentCancelRateLimitEnabled:        updatedPaymentCfg.CancelRateLimitEnabled,
		PaymentCancelRateLimitMax:            updatedPaymentCfg.CancelRateLimitMax,
		PaymentCancelRateLimitWindow:         updatedPaymentCfg.CancelRateLimitWindow,
		PaymentCancelRateLimitUnit:           updatedPaymentCfg.CancelRateLimitUnit,
		PaymentCancelRateLimitMode:           updatedPaymentCfg.CancelRateLimitMode,
	})
}

// hasPaymentFields returns true if any payment-related field was explicitly provided.
func hasPaymentFields(req UpdateSettingsRequest) bool {
	return req.PaymentEnabled != nil || req.PaymentMinAmount != nil ||
		req.PaymentMaxAmount != nil || req.PaymentDailyLimit != nil ||
		req.PaymentOrderTimeoutMin != nil || req.PaymentMaxPendingOrders != nil ||
		req.PaymentEnabledTypes != nil || req.PaymentBalanceDisabled != nil ||
		req.PaymentBalanceRechargeMultiplier != nil || req.PaymentRechargeFeeRate != nil ||
		req.PaymentOrderIDPrefix != nil || req.PaymentLoadBalanceStrat != nil || req.PaymentProductNamePrefix != nil ||
		req.PaymentProductNameSuffix != nil || req.PaymentHelpImageURL != nil ||
		req.PaymentHelpText != nil || req.PaymentCancelRateLimitEnabled != nil ||
		req.PaymentCancelRateLimitMax != nil || req.PaymentCancelRateLimitWindow != nil ||
		req.PaymentCancelRateLimitUnit != nil || req.PaymentCancelRateLimitMode != nil
}

func (h *SettingHandler) auditSettingsUpdate(c *gin.Context, before *service.SystemSettings, after *service.SystemSettings, req UpdateSettingsRequest) {
	if before == nil || after == nil {
		return
	}

	changed := diffSettings(before, after, req)
	if len(changed) == 0 {
		return
	}

	subject, _ := middleware.GetAuthSubjectFromContext(c)
	role, _ := middleware.GetUserRoleFromContext(c)
	slog.Info("settings updated",
		"audit", true,
		"user_id", subject.UserID,
		"role", role,
		"changed", changed,
	)
}

func diffSettings(before *service.SystemSettings, after *service.SystemSettings, req UpdateSettingsRequest) []string {
	changed := make([]string, 0, 20)
	if before.RegistrationEnabled != after.RegistrationEnabled {
		changed = append(changed, "registration_enabled")
	}
	if before.EmailVerifyEnabled != after.EmailVerifyEnabled {
		changed = append(changed, "email_verify_enabled")
	}
	if !equalStringSlice(before.RegistrationEmailSuffixWhitelist, after.RegistrationEmailSuffixWhitelist) {
		changed = append(changed, "registration_email_suffix_whitelist")
	}
	if before.PromoCodeEnabled != after.PromoCodeEnabled {
		changed = append(changed, "promo_code_enabled")
	}
	if before.InvitationCodeEnabled != after.InvitationCodeEnabled {
		changed = append(changed, "invitation_code_enabled")
	}
	if before.PasswordResetEnabled != after.PasswordResetEnabled {
		changed = append(changed, "password_reset_enabled")
	}
	if before.FrontendURL != after.FrontendURL {
		changed = append(changed, "frontend_url")
	}
	if before.TotpEnabled != after.TotpEnabled {
		changed = append(changed, "totp_enabled")
	}
	if before.SMTPHost != after.SMTPHost {
		changed = append(changed, "smtp_host")
	}
	if before.SMTPPort != after.SMTPPort {
		changed = append(changed, "smtp_port")
	}
	if before.SMTPUsername != after.SMTPUsername {
		changed = append(changed, "smtp_username")
	}
	if req.SMTPPassword != "" {
		changed = append(changed, "smtp_password")
	}
	if before.SMTPFrom != after.SMTPFrom {
		changed = append(changed, "smtp_from_email")
	}
	if before.SMTPFromName != after.SMTPFromName {
		changed = append(changed, "smtp_from_name")
	}
	if before.SMTPUseTLS != after.SMTPUseTLS {
		changed = append(changed, "smtp_use_tls")
	}
	if before.EmailProvider != after.EmailProvider {
		changed = append(changed, "email_provider")
	}
	if req.ResendAPIKey != "" {
		changed = append(changed, "resend_api_key")
	}
	if before.ResendFrom != after.ResendFrom {
		changed = append(changed, "resend_from_email")
	}
	if before.ResendFromName != after.ResendFromName {
		changed = append(changed, "resend_from_name")
	}
	if before.TurnstileEnabled != after.TurnstileEnabled {
		changed = append(changed, "turnstile_enabled")
	}
	if before.TurnstileSiteKey != after.TurnstileSiteKey {
		changed = append(changed, "turnstile_site_key")
	}
	if req.TurnstileSecretKey != "" {
		changed = append(changed, "turnstile_secret_key")
	}
	if before.LinuxDoConnectEnabled != after.LinuxDoConnectEnabled {
		changed = append(changed, "linuxdo_connect_enabled")
	}
	if before.LinuxDoConnectClientID != after.LinuxDoConnectClientID {
		changed = append(changed, "linuxdo_connect_client_id")
	}
	if req.LinuxDoConnectClientSecret != "" {
		changed = append(changed, "linuxdo_connect_client_secret")
	}
	if before.LinuxDoConnectRedirectURL != after.LinuxDoConnectRedirectURL {
		changed = append(changed, "linuxdo_connect_redirect_url")
	}
	if before.OIDCConnectEnabled != after.OIDCConnectEnabled {
		changed = append(changed, "oidc_connect_enabled")
	}
	if before.OIDCConnectProviderName != after.OIDCConnectProviderName {
		changed = append(changed, "oidc_connect_provider_name")
	}
	if before.OIDCConnectClientID != after.OIDCConnectClientID {
		changed = append(changed, "oidc_connect_client_id")
	}
	if req.OIDCConnectClientSecret != "" {
		changed = append(changed, "oidc_connect_client_secret")
	}
	if before.OIDCConnectIssuerURL != after.OIDCConnectIssuerURL {
		changed = append(changed, "oidc_connect_issuer_url")
	}
	if before.OIDCConnectDiscoveryURL != after.OIDCConnectDiscoveryURL {
		changed = append(changed, "oidc_connect_discovery_url")
	}
	if before.OIDCConnectAuthorizeURL != after.OIDCConnectAuthorizeURL {
		changed = append(changed, "oidc_connect_authorize_url")
	}
	if before.OIDCConnectTokenURL != after.OIDCConnectTokenURL {
		changed = append(changed, "oidc_connect_token_url")
	}
	if before.OIDCConnectUserInfoURL != after.OIDCConnectUserInfoURL {
		changed = append(changed, "oidc_connect_userinfo_url")
	}
	if before.OIDCConnectJWKSURL != after.OIDCConnectJWKSURL {
		changed = append(changed, "oidc_connect_jwks_url")
	}
	if before.OIDCConnectScopes != after.OIDCConnectScopes {
		changed = append(changed, "oidc_connect_scopes")
	}
	if before.OIDCConnectRedirectURL != after.OIDCConnectRedirectURL {
		changed = append(changed, "oidc_connect_redirect_url")
	}
	if before.OIDCConnectFrontendRedirectURL != after.OIDCConnectFrontendRedirectURL {
		changed = append(changed, "oidc_connect_frontend_redirect_url")
	}
	if before.OIDCConnectTokenAuthMethod != after.OIDCConnectTokenAuthMethod {
		changed = append(changed, "oidc_connect_token_auth_method")
	}
	if before.OIDCConnectUsePKCE != after.OIDCConnectUsePKCE {
		changed = append(changed, "oidc_connect_use_pkce")
	}
	if before.OIDCConnectValidateIDToken != after.OIDCConnectValidateIDToken {
		changed = append(changed, "oidc_connect_validate_id_token")
	}
	if before.OIDCConnectAllowedSigningAlgs != after.OIDCConnectAllowedSigningAlgs {
		changed = append(changed, "oidc_connect_allowed_signing_algs")
	}
	if before.OIDCConnectClockSkewSeconds != after.OIDCConnectClockSkewSeconds {
		changed = append(changed, "oidc_connect_clock_skew_seconds")
	}
	if before.OIDCConnectRequireEmailVerified != after.OIDCConnectRequireEmailVerified {
		changed = append(changed, "oidc_connect_require_email_verified")
	}
	if before.OIDCConnectUserInfoEmailPath != after.OIDCConnectUserInfoEmailPath {
		changed = append(changed, "oidc_connect_userinfo_email_path")
	}
	if before.OIDCConnectUserInfoIDPath != after.OIDCConnectUserInfoIDPath {
		changed = append(changed, "oidc_connect_userinfo_id_path")
	}
	if before.OIDCConnectUserInfoUsernamePath != after.OIDCConnectUserInfoUsernamePath {
		changed = append(changed, "oidc_connect_userinfo_username_path")
	}
	if before.SiteName != after.SiteName {
		changed = append(changed, "site_name")
	}
	if before.SiteLogo != after.SiteLogo {
		changed = append(changed, "site_logo")
	}
	if before.SiteSubtitle != after.SiteSubtitle {
		changed = append(changed, "site_subtitle")
	}
	if before.APIBaseURL != after.APIBaseURL {
		changed = append(changed, "api_base_url")
	}
	if before.ContactInfo != after.ContactInfo {
		changed = append(changed, "contact_info")
	}
	if before.DocURL != after.DocURL {
		changed = append(changed, "doc_url")
	}
	if before.HomeContent != after.HomeContent {
		changed = append(changed, "home_content")
	}
	if before.HideCcsImportButton != after.HideCcsImportButton {
		changed = append(changed, "hide_ccs_import_button")
	}
	if before.DefaultConcurrency != after.DefaultConcurrency {
		changed = append(changed, "default_concurrency")
	}
	if before.DefaultBalance != after.DefaultBalance {
		changed = append(changed, "default_balance")
	}
	if !equalDefaultSubscriptions(before.DefaultSubscriptions, after.DefaultSubscriptions) {
		changed = append(changed, "default_subscriptions")
	}
	if before.EnableModelFallback != after.EnableModelFallback {
		changed = append(changed, "enable_model_fallback")
	}
	if before.FallbackModelAnthropic != after.FallbackModelAnthropic {
		changed = append(changed, "fallback_model_anthropic")
	}
	if before.FallbackModelOpenAI != after.FallbackModelOpenAI {
		changed = append(changed, "fallback_model_openai")
	}
	if before.FallbackModelGemini != after.FallbackModelGemini {
		changed = append(changed, "fallback_model_gemini")
	}
	if before.FallbackModelAntigravity != after.FallbackModelAntigravity {
		changed = append(changed, "fallback_model_antigravity")
	}
	if before.OpenAIUseKeyModelID != after.OpenAIUseKeyModelID {
		changed = append(changed, "openai_use_key_model_id")
	}
	if before.GeneratedImageStorageSource != after.GeneratedImageStorageSource {
		changed = append(changed, "generated_image_storage_source")
	}
	if before.GeneratedImageStorageConfigSource != after.GeneratedImageStorageConfigSource {
		changed = append(changed, "generated_image_storage_config_source")
	}
	if before.QiniuAccessKey != after.QiniuAccessKey {
		changed = append(changed, "qiniu_access_key")
	}
	if req.QiniuSecretKey != "" {
		changed = append(changed, "qiniu_secret_key")
	}
	if before.QiniuBucket != after.QiniuBucket {
		changed = append(changed, "qiniu_bucket")
	}
	if before.QiniuCDNDomain != after.QiniuCDNDomain {
		changed = append(changed, "qiniu_cdn_domain")
	}
	if before.QiniuPrefix != after.QiniuPrefix {
		changed = append(changed, "qiniu_prefix")
	}
	if before.QiniuUseHTTPS != after.QiniuUseHTTPS {
		changed = append(changed, "qiniu_use_https")
	}
	if before.QiniuUploadTimeoutSeconds != after.QiniuUploadTimeoutSeconds {
		changed = append(changed, "qiniu_upload_timeout_seconds")
	}
	if before.QiniuTokenTTLSeconds != after.QiniuTokenTTLSeconds {
		changed = append(changed, "qiniu_token_ttl_seconds")
	}
	if before.MediaSyncWaitTimeoutSeconds != after.MediaSyncWaitTimeoutSeconds {
		changed = append(changed, "media_sync_wait_timeout_seconds")
	}
	if before.MediaSyncTimeoutFallbackAsyncEnabled != after.MediaSyncTimeoutFallbackAsyncEnabled {
		changed = append(changed, "media_sync_timeout_fallback_async_enabled")
	}
	if before.MediaSyncTimeoutBillingPolicy != after.MediaSyncTimeoutBillingPolicy {
		changed = append(changed, "media_sync_timeout_billing_policy")
	}
	if before.MediaSyncTimeoutPenaltyRatio != after.MediaSyncTimeoutPenaltyRatio {
		changed = append(changed, "media_sync_timeout_penalty_ratio")
	}
	if before.MediaVideoStorageMode != after.MediaVideoStorageMode {
		changed = append(changed, "media_video_storage_mode")
	}
	if before.MediaVideoProxyFallbackEnabled != after.MediaVideoProxyFallbackEnabled {
		changed = append(changed, "media_video_proxy_fallback_enabled")
	}
	if before.EnableIdentityPatch != after.EnableIdentityPatch {
		changed = append(changed, "enable_identity_patch")
	}
	if before.IdentityPatchPrompt != after.IdentityPatchPrompt {
		changed = append(changed, "identity_patch_prompt")
	}
	if before.SystemPromptAnthropic != after.SystemPromptAnthropic || before.SystemPromptModeAnthropic != after.SystemPromptModeAnthropic {
		changed = append(changed, "system_prompt_anthropic")
	}
	if before.SystemPromptOpenAI != after.SystemPromptOpenAI || before.SystemPromptModeOpenAI != after.SystemPromptModeOpenAI {
		changed = append(changed, "system_prompt_openai")
	}
	if before.SystemPromptGemini != after.SystemPromptGemini || before.SystemPromptModeGemini != after.SystemPromptModeGemini {
		changed = append(changed, "system_prompt_gemini")
	}
	if before.SystemPromptAntigravity != after.SystemPromptAntigravity || before.SystemPromptModeAntigravity != after.SystemPromptModeAntigravity {
		changed = append(changed, "system_prompt_antigravity")
	}
	if before.SystemPromptUserScopeEnabled != after.SystemPromptUserScopeEnabled ||
		before.SystemPromptUserScopeMode != after.SystemPromptUserScopeMode ||
		!reflect.DeepEqual(before.SystemPromptUserScopeUserIDs, after.SystemPromptUserScopeUserIDs) {
		changed = append(changed, "system_prompt_user_scope")
	}
	if before.OpsMonitoringEnabled != after.OpsMonitoringEnabled {
		changed = append(changed, "ops_monitoring_enabled")
	}
	if before.OpsRealtimeMonitoringEnabled != after.OpsRealtimeMonitoringEnabled {
		changed = append(changed, "ops_realtime_monitoring_enabled")
	}
	if before.OpsQueryModeDefault != after.OpsQueryModeDefault {
		changed = append(changed, "ops_query_mode_default")
	}
	if before.OpsMetricsIntervalSeconds != after.OpsMetricsIntervalSeconds {
		changed = append(changed, "ops_metrics_interval_seconds")
	}
	if before.MinClaudeCodeVersion != after.MinClaudeCodeVersion {
		changed = append(changed, "min_claude_code_version")
	}
	if before.MaxClaudeCodeVersion != after.MaxClaudeCodeVersion {
		changed = append(changed, "max_claude_code_version")
	}
	if before.AllowUngroupedKeyScheduling != after.AllowUngroupedKeyScheduling {
		changed = append(changed, "allow_ungrouped_key_scheduling")
	}
	if before.BackendModeEnabled != after.BackendModeEnabled {
		changed = append(changed, "backend_mode_enabled")
	}
	if before.PurchaseSubscriptionEnabled != after.PurchaseSubscriptionEnabled {
		changed = append(changed, "purchase_subscription_enabled")
	}
	if before.PurchaseSubscriptionURL != after.PurchaseSubscriptionURL {
		changed = append(changed, "purchase_subscription_url")
	}
	if before.TableDefaultPageSize != after.TableDefaultPageSize {
		changed = append(changed, "table_default_page_size")
	}
	if !equalIntSlice(before.TablePageSizeOptions, after.TablePageSizeOptions) {
		changed = append(changed, "table_page_size_options")
	}
	if before.CustomMenuItems != after.CustomMenuItems {
		changed = append(changed, "custom_menu_items")
	}
	if before.RedeemDeliveryText != after.RedeemDeliveryText {
		changed = append(changed, "redeem_delivery_text")
	}
	if before.AttractPopupTitle != after.AttractPopupTitle {
		changed = append(changed, "attract_popup_title")
	}
	if before.AttractPopupMarkdown != after.AttractPopupMarkdown {
		changed = append(changed, "attract_popup_markdown")
	}
	if before.DashboardFireworksEnabled != after.DashboardFireworksEnabled {
		changed = append(changed, "dashboard_fireworks_enabled")
	}
	if before.DashboardFireworksThreshold != after.DashboardFireworksThreshold {
		changed = append(changed, "dashboard_fireworks_threshold")
	}
	if before.CustomEndpoints != after.CustomEndpoints {
		changed = append(changed, "custom_endpoints")
	}
	if before.EnableFingerprintUnification != after.EnableFingerprintUnification {
		changed = append(changed, "enable_fingerprint_unification")
	}
	if before.EnableMetadataPassthrough != after.EnableMetadataPassthrough {
		changed = append(changed, "enable_metadata_passthrough")
	}
	if before.EnableCCHSigning != after.EnableCCHSigning {
		changed = append(changed, "enable_cch_signing")
	}
	// Balance & quota notification
	if before.BalanceLowNotifyEnabled != after.BalanceLowNotifyEnabled {
		changed = append(changed, "balance_low_notify_enabled")
	}
	if before.BalanceLowNotifyThreshold != after.BalanceLowNotifyThreshold {
		changed = append(changed, "balance_low_notify_threshold")
	}
	if before.BalanceLowNotifyRechargeURL != after.BalanceLowNotifyRechargeURL {
		changed = append(changed, "balance_low_notify_recharge_url")
	}
	if before.AccountQuotaNotifyEnabled != after.AccountQuotaNotifyEnabled {
		changed = append(changed, "account_quota_notify_enabled")
	}
	if !equalNotifyEmailEntries(before.AccountQuotaNotifyEmails, after.AccountQuotaNotifyEmails) {
		changed = append(changed, "account_quota_notify_emails")
	}
	return changed
}

func normalizeDefaultSubscriptions(input []dto.DefaultSubscriptionSetting) []dto.DefaultSubscriptionSetting {
	if len(input) == 0 {
		return nil
	}
	normalized := make([]dto.DefaultSubscriptionSetting, 0, len(input))
	for _, item := range input {
		if item.GroupID <= 0 || item.ValidityDays <= 0 {
			continue
		}
		if item.ValidityDays > service.MaxValidityDays {
			item.ValidityDays = service.MaxValidityDays
		}
		normalized = append(normalized, item)
	}
	return normalized
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalDefaultSubscriptions(a, b []service.DefaultSubscriptionSetting) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].GroupID != b[i].GroupID || a[i].ValidityDays != b[i].ValidityDays {
			return false
		}
	}
	return true
}

func equalIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalNotifyEmailEntries(a, b []service.NotifyEmailEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Email != b[i].Email || a[i].Verified != b[i].Verified || a[i].Disabled != b[i].Disabled {
			return false
		}
	}
	return true
}

// TestSMTPRequest 测试SMTP连接请求
type TestSMTPRequest struct {
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	SMTPUseTLS   bool   `json:"smtp_use_tls"`
}

// TestSMTPConnection 测试SMTP连接
// POST /api/v1/admin/settings/test-smtp
func (h *SettingHandler) TestSMTPConnection(c *gin.Context) {
	var req TestSMTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	req.SMTPHost = strings.TrimSpace(req.SMTPHost)
	req.SMTPUsername = strings.TrimSpace(req.SMTPUsername)

	var savedConfig *service.SMTPConfig
	if cfg, err := h.emailService.GetSMTPConfig(c.Request.Context()); err == nil && cfg != nil {
		savedConfig = cfg
	}

	if req.SMTPHost == "" && savedConfig != nil {
		req.SMTPHost = savedConfig.Host
	}
	if req.SMTPPort <= 0 {
		if savedConfig != nil && savedConfig.Port > 0 {
			req.SMTPPort = savedConfig.Port
		} else {
			req.SMTPPort = 587
		}
	}
	if req.SMTPUsername == "" && savedConfig != nil {
		req.SMTPUsername = savedConfig.Username
	}
	password := strings.TrimSpace(req.SMTPPassword)
	if password == "" && savedConfig != nil {
		password = savedConfig.Password
	}
	if req.SMTPHost == "" {
		response.BadRequest(c, "SMTP host is required")
		return
	}

	config := &service.SMTPConfig{
		Host:     req.SMTPHost,
		Port:     req.SMTPPort,
		Username: req.SMTPUsername,
		Password: password,
		UseTLS:   req.SMTPUseTLS,
	}

	err := h.emailService.TestSMTPConnectionWithConfig(config)
	if err != nil {
		response.BadRequest(c, "SMTP connection test failed: "+err.Error())
		return
	}

	response.Success(c, gin.H{"message": "SMTP connection successful"})
}

// SendTestEmailRequest 发送测试邮件请求
type SendTestEmailRequest struct {
	Email          string `json:"email" binding:"required,email"`
	SMTPHost       string `json:"smtp_host"`
	SMTPPort       int    `json:"smtp_port"`
	SMTPUsername   string `json:"smtp_username"`
	SMTPPassword   string `json:"smtp_password"`
	SMTPFrom       string `json:"smtp_from_email"`
	SMTPFromName   string `json:"smtp_from_name"`
	SMTPUseTLS     bool   `json:"smtp_use_tls"`
	EmailProvider  string `json:"email_provider"`
	ResendAPIKey   string `json:"resend_api_key"`
	ResendFrom     string `json:"resend_from_email"`
	ResendFromName string `json:"resend_from_name"`
}

// SendTestEmail 发送测试邮件
// POST /api/v1/admin/settings/send-test-email
func (h *SettingHandler) SendTestEmail(c *gin.Context) {
	var req SendTestEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	req.SMTPHost = strings.TrimSpace(req.SMTPHost)
	req.SMTPUsername = strings.TrimSpace(req.SMTPUsername)
	req.SMTPFrom = strings.TrimSpace(req.SMTPFrom)
	req.SMTPFromName = strings.TrimSpace(req.SMTPFromName)
	req.EmailProvider = strings.ToLower(strings.TrimSpace(req.EmailProvider))
	req.ResendAPIKey = strings.TrimSpace(req.ResendAPIKey)
	req.ResendFrom = strings.TrimSpace(req.ResendFrom)
	req.ResendFromName = strings.TrimSpace(req.ResendFromName)

	savedSettings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	provider := req.EmailProvider
	if provider == "" {
		provider = savedSettings.EmailProvider
	}
	if provider == "" {
		provider = service.EmailProviderSMTP
	}
	if !service.IsSupportedEmailProvider(provider) {
		response.BadRequest(c, "Unsupported email provider")
		return
	}
	if provider == service.EmailProviderResend {
		apiKey := req.ResendAPIKey
		if apiKey == "" {
			apiKey = savedSettings.ResendAPIKey
		}
		from := req.ResendFrom
		if from == "" {
			from = savedSettings.ResendFrom
		}
		fromName := req.ResendFromName
		if fromName == "" {
			fromName = savedSettings.ResendFromName
		}
		if apiKey == "" || from == "" {
			response.BadRequest(c, "Resend API key and from email are required")
			return
		}

		siteName := h.settingService.GetSiteName(c.Request.Context())
		subject := "[" + siteName + "] Test Email"
		body := `<html><body><h2>Email Configuration Successful!</h2><p>This is a test email to verify your Resend settings are working correctly.</p></body></html>`
		if err := h.emailService.SendEmailWithResendConfig(c.Request.Context(), &service.ResendConfig{
			APIKey:   apiKey,
			From:     from,
			FromName: fromName,
		}, req.Email, subject, body); err != nil {
			response.BadRequest(c, "Failed to send test email: "+err.Error())
			return
		}
		response.Success(c, gin.H{"message": "Test email sent successfully"})
		return
	}

	var savedConfig *service.SMTPConfig
	if cfg, err := h.emailService.GetSMTPConfig(c.Request.Context()); err == nil && cfg != nil {
		savedConfig = cfg
	}

	if req.SMTPHost == "" && savedConfig != nil {
		req.SMTPHost = savedConfig.Host
	}
	if req.SMTPPort <= 0 {
		if savedConfig != nil && savedConfig.Port > 0 {
			req.SMTPPort = savedConfig.Port
		} else {
			req.SMTPPort = 587
		}
	}
	if req.SMTPUsername == "" && savedConfig != nil {
		req.SMTPUsername = savedConfig.Username
	}
	password := strings.TrimSpace(req.SMTPPassword)
	if password == "" && savedConfig != nil {
		password = savedConfig.Password
	}
	if req.SMTPFrom == "" && savedConfig != nil {
		req.SMTPFrom = savedConfig.From
	}
	if req.SMTPFromName == "" && savedConfig != nil {
		req.SMTPFromName = savedConfig.FromName
	}
	if req.SMTPHost == "" {
		response.BadRequest(c, "SMTP host is required")
		return
	}

	config := &service.SMTPConfig{
		Host:     req.SMTPHost,
		Port:     req.SMTPPort,
		Username: req.SMTPUsername,
		Password: password,
		From:     req.SMTPFrom,
		FromName: req.SMTPFromName,
		UseTLS:   req.SMTPUseTLS,
	}

	siteName := h.settingService.GetSiteName(c.Request.Context())
	subject := "[" + siteName + "] Test Email"
	body := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background-color: #f5f5f5; margin: 0; padding: 20px; }
        .container { max-width: 600px; margin: 0 auto; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; text-align: center; }
        .content { padding: 40px 30px; text-align: center; }
        .success { color: #10b981; font-size: 48px; margin-bottom: 20px; }
        .footer { background-color: #f8f9fa; padding: 20px; text-align: center; color: #999; font-size: 12px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>` + siteName + `</h1>
        </div>
        <div class="content">
            <div class="success">✓</div>
            <h2>Email Configuration Successful!</h2>
            <p>This is a test email to verify your SMTP settings are working correctly.</p>
        </div>
        <div class="footer">
            <p>This is an automated test message.</p>
        </div>
    </div>
</body>
</html>
`

	if err := h.emailService.SendEmailWithConfig(config, req.Email, subject, body); err != nil {
		response.BadRequest(c, "Failed to send test email: "+err.Error())
		return
	}

	response.Success(c, gin.H{"message": "Test email sent successfully"})
}

// GetAdminAPIKey 获取管理员 API Key 状态
// GET /api/v1/admin/settings/admin-api-key
func (h *SettingHandler) GetAdminAPIKey(c *gin.Context) {
	maskedKey, exists, err := h.settingService.GetAdminAPIKeyStatus(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"exists":     exists,
		"masked_key": maskedKey,
	})
}

// RegenerateAdminAPIKey 生成/重新生成管理员 API Key
// POST /api/v1/admin/settings/admin-api-key/regenerate
func (h *SettingHandler) RegenerateAdminAPIKey(c *gin.Context) {
	key, err := h.settingService.GenerateAdminAPIKey(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"key": key, // 完整 key 只在生成时返回一次
	})
}

// DeleteAdminAPIKey 删除管理员 API Key
// DELETE /api/v1/admin/settings/admin-api-key
func (h *SettingHandler) DeleteAdminAPIKey(c *gin.Context) {
	if err := h.settingService.DeleteAdminAPIKey(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Admin API key deleted"})
}

// GetOverloadCooldownSettings 获取529过载冷却配置
// GET /api/v1/admin/settings/overload-cooldown
func (h *SettingHandler) GetOverloadCooldownSettings(c *gin.Context) {
	settings, err := h.settingService.GetOverloadCooldownSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.OverloadCooldownSettings{
		Enabled:         settings.Enabled,
		CooldownMinutes: settings.CooldownMinutes,
	})
}

// UpdateOverloadCooldownSettingsRequest 更新529过载冷却配置请求
type UpdateOverloadCooldownSettingsRequest struct {
	Enabled         bool `json:"enabled"`
	CooldownMinutes int  `json:"cooldown_minutes"`
}

// UpdateOverloadCooldownSettings 更新529过载冷却配置
// PUT /api/v1/admin/settings/overload-cooldown
func (h *SettingHandler) UpdateOverloadCooldownSettings(c *gin.Context) {
	var req UpdateOverloadCooldownSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	settings := &service.OverloadCooldownSettings{
		Enabled:         req.Enabled,
		CooldownMinutes: req.CooldownMinutes,
	}

	if err := h.settingService.SetOverloadCooldownSettings(c.Request.Context(), settings); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	updatedSettings, err := h.settingService.GetOverloadCooldownSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.OverloadCooldownSettings{
		Enabled:         updatedSettings.Enabled,
		CooldownMinutes: updatedSettings.CooldownMinutes,
	})
}

// GetStreamTimeoutSettings 获取流超时处理配置
// GET /api/v1/admin/settings/stream-timeout
func (h *SettingHandler) GetStreamTimeoutSettings(c *gin.Context) {
	settings, err := h.settingService.GetStreamTimeoutSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.StreamTimeoutSettings{
		Enabled:                settings.Enabled,
		Action:                 settings.Action,
		TempUnschedMinutes:     settings.TempUnschedMinutes,
		ThresholdCount:         settings.ThresholdCount,
		ThresholdWindowMinutes: settings.ThresholdWindowMinutes,
	})
}

// GetRectifierSettings 获取请求整流器配置
// GET /api/v1/admin/settings/rectifier
func (h *SettingHandler) GetRectifierSettings(c *gin.Context) {
	settings, err := h.settingService.GetRectifierSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	patterns := settings.APIKeySignaturePatterns
	if patterns == nil {
		patterns = []string{}
	}
	response.Success(c, dto.RectifierSettings{
		Enabled:                  settings.Enabled,
		ThinkingSignatureEnabled: settings.ThinkingSignatureEnabled,
		ThinkingBudgetEnabled:    settings.ThinkingBudgetEnabled,
		APIKeySignatureEnabled:   settings.APIKeySignatureEnabled,
		APIKeySignaturePatterns:  patterns,
	})
}

// UpdateRectifierSettingsRequest 更新整流器配置请求
type UpdateRectifierSettingsRequest struct {
	Enabled                  bool     `json:"enabled"`
	ThinkingSignatureEnabled bool     `json:"thinking_signature_enabled"`
	ThinkingBudgetEnabled    bool     `json:"thinking_budget_enabled"`
	APIKeySignatureEnabled   bool     `json:"apikey_signature_enabled"`
	APIKeySignaturePatterns  []string `json:"apikey_signature_patterns"`
}

// UpdateRectifierSettings 更新请求整流器配置
// PUT /api/v1/admin/settings/rectifier
func (h *SettingHandler) UpdateRectifierSettings(c *gin.Context) {
	var req UpdateRectifierSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 校验并清理自定义匹配关键词
	const maxPatterns = 50
	const maxPatternLen = 500
	if len(req.APIKeySignaturePatterns) > maxPatterns {
		response.BadRequest(c, "Too many signature patterns (max 50)")
		return
	}
	var cleanedPatterns []string
	for _, p := range req.APIKeySignaturePatterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len(p) > maxPatternLen {
			response.BadRequest(c, "Signature pattern too long (max 500 characters)")
			return
		}
		cleanedPatterns = append(cleanedPatterns, p)
	}

	settings := &service.RectifierSettings{
		Enabled:                  req.Enabled,
		ThinkingSignatureEnabled: req.ThinkingSignatureEnabled,
		ThinkingBudgetEnabled:    req.ThinkingBudgetEnabled,
		APIKeySignatureEnabled:   req.APIKeySignatureEnabled,
		APIKeySignaturePatterns:  cleanedPatterns,
	}

	if err := h.settingService.SetRectifierSettings(c.Request.Context(), settings); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 重新获取设置返回
	updatedSettings, err := h.settingService.GetRectifierSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	updatedPatterns := updatedSettings.APIKeySignaturePatterns
	if updatedPatterns == nil {
		updatedPatterns = []string{}
	}
	response.Success(c, dto.RectifierSettings{
		Enabled:                  updatedSettings.Enabled,
		ThinkingSignatureEnabled: updatedSettings.ThinkingSignatureEnabled,
		ThinkingBudgetEnabled:    updatedSettings.ThinkingBudgetEnabled,
		APIKeySignatureEnabled:   updatedSettings.APIKeySignatureEnabled,
		APIKeySignaturePatterns:  updatedPatterns,
	})
}

// GetBetaPolicySettings 获取 Beta 策略配置
// GET /api/v1/admin/settings/beta-policy
func (h *SettingHandler) GetBetaPolicySettings(c *gin.Context) {
	settings, err := h.settingService.GetBetaPolicySettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	rules := make([]dto.BetaPolicyRule, len(settings.Rules))
	for i, r := range settings.Rules {
		rules[i] = dto.BetaPolicyRule(r)
	}
	response.Success(c, dto.BetaPolicySettings{Rules: rules})
}

// UpdateBetaPolicySettingsRequest 更新 Beta 策略配置请求
type UpdateBetaPolicySettingsRequest struct {
	Rules []dto.BetaPolicyRule `json:"rules"`
}

// UpdateBetaPolicySettings 更新 Beta 策略配置
// PUT /api/v1/admin/settings/beta-policy
func (h *SettingHandler) UpdateBetaPolicySettings(c *gin.Context) {
	var req UpdateBetaPolicySettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	rules := make([]service.BetaPolicyRule, len(req.Rules))
	for i, r := range req.Rules {
		rules[i] = service.BetaPolicyRule(r)
	}

	settings := &service.BetaPolicySettings{Rules: rules}
	if err := h.settingService.SetBetaPolicySettings(c.Request.Context(), settings); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// Re-fetch to return updated settings
	updated, err := h.settingService.GetBetaPolicySettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	outRules := make([]dto.BetaPolicyRule, len(updated.Rules))
	for i, r := range updated.Rules {
		outRules[i] = dto.BetaPolicyRule(r)
	}
	response.Success(c, dto.BetaPolicySettings{Rules: outRules})
}

// UpdateStreamTimeoutSettingsRequest 更新流超时配置请求
type UpdateStreamTimeoutSettingsRequest struct {
	Enabled                bool   `json:"enabled"`
	Action                 string `json:"action"`
	TempUnschedMinutes     int    `json:"temp_unsched_minutes"`
	ThresholdCount         int    `json:"threshold_count"`
	ThresholdWindowMinutes int    `json:"threshold_window_minutes"`
}

// UpdateStreamTimeoutSettings 更新流超时处理配置
// PUT /api/v1/admin/settings/stream-timeout
func (h *SettingHandler) UpdateStreamTimeoutSettings(c *gin.Context) {
	var req UpdateStreamTimeoutSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	settings := &service.StreamTimeoutSettings{
		Enabled:                req.Enabled,
		Action:                 req.Action,
		TempUnschedMinutes:     req.TempUnschedMinutes,
		ThresholdCount:         req.ThresholdCount,
		ThresholdWindowMinutes: req.ThresholdWindowMinutes,
	}

	if err := h.settingService.SetStreamTimeoutSettings(c.Request.Context(), settings); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 重新获取设置返回
	updatedSettings, err := h.settingService.GetStreamTimeoutSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.StreamTimeoutSettings{
		Enabled:                updatedSettings.Enabled,
		Action:                 updatedSettings.Action,
		TempUnschedMinutes:     updatedSettings.TempUnschedMinutes,
		ThresholdCount:         updatedSettings.ThresholdCount,
		ThresholdWindowMinutes: updatedSettings.ThresholdWindowMinutes,
	})
}

// GetWebSearchEmulationConfig 获取 Web Search 模拟配置
// GET /api/v1/admin/settings/web-search-emulation
func (h *SettingHandler) GetWebSearchEmulationConfig(c *gin.Context) {
	cfg, err := h.settingService.GetWebSearchEmulationConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, service.PopulateWebSearchUsage(c.Request.Context(), cfg))
}

// UpdateWebSearchEmulationConfig 更新 Web Search 模拟配置
// PUT /api/v1/admin/settings/web-search-emulation
func (h *SettingHandler) UpdateWebSearchEmulationConfig(c *gin.Context) {
	var cfg service.WebSearchEmulationConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := h.settingService.SaveWebSearchEmulationConfig(c.Request.Context(), &cfg); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// Re-read (with sanitized api keys) to return current state
	updated, err := h.settingService.GetWebSearchEmulationConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, service.PopulateWebSearchUsage(c.Request.Context(), updated))
}

// ResetWebSearchUsage 重置指定 provider 的配额用量
// POST /api/v1/admin/settings/web-search-emulation/reset-usage
func (h *SettingHandler) ResetWebSearchUsage(c *gin.Context) {
	var req struct {
		ProviderType string `json:"provider_type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.ProviderType == "" {
		response.BadRequest(c, "provider_type is required")
		return
	}
	if err := service.ResetWebSearchUsage(c.Request.Context(), req.ProviderType); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

// TestWebSearchEmulation 测试 Web Search 搜索
// POST /api/v1/admin/settings/web-search-emulation/test
func (h *SettingHandler) TestWebSearchEmulation(c *gin.Context) {
	var req struct {
		Query string `json:"query"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		req.Query = "搜索今年世界大事件"
	}

	result, err := service.TestWebSearch(c.Request.Context(), req.Query)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
