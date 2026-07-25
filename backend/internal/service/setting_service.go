package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/imroc/req/v3"
	"golang.org/x/sync/singleflight"
)

var (
	ErrRegistrationDisabled   = infraerrors.Forbidden("REGISTRATION_DISABLED", "registration is currently disabled")
	ErrSettingNotFound        = infraerrors.NotFound("SETTING_NOT_FOUND", "setting not found")
	ErrDefaultSubGroupInvalid = infraerrors.BadRequest(
		"DEFAULT_SUBSCRIPTION_GROUP_INVALID",
		"default subscription group must exist and be subscription type",
	)
	ErrDefaultSubGroupDuplicate = infraerrors.BadRequest(
		"DEFAULT_SUBSCRIPTION_GROUP_DUPLICATE",
		"default subscription group cannot be duplicated",
	)
)

const DefaultOpenAIImageURLCacheTTLHours = 72
const DefaultQiniuPrefix = "openai/generated-images"
const DefaultQiniuUploadTimeoutSeconds = 30
const DefaultQiniuTokenTTLSeconds = 3600
const DefaultDashboardFireworksThreshold = 20.0

const (
	DefaultSuccessfulRequestRecordsMaxBodyBytes int64 = 1024 * 1024
	MinSuccessfulRequestRecordsMaxBodyBytes     int64 = 1024
	MaxSuccessfulRequestRecordsMaxBodyBytes     int64 = 16 * 1024 * 1024
)

// SuccessfulRequestRecordRuntimeSettings 是成功请求正文记录的动态运行时设置。
type SuccessfulRequestRecordRuntimeSettings struct {
	Enabled      bool
	MaxBodyBytes int64
}

type SettingRepository interface {
	Get(ctx context.Context, key string) (*Setting, error)
	GetValue(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	GetMultiple(ctx context.Context, keys []string) (map[string]string, error)
	SetMultiple(ctx context.Context, settings map[string]string) error
	GetAll(ctx context.Context) (map[string]string, error)
	Delete(ctx context.Context, key string) error
}

// cachedVersionBounds 缓存 Claude Code 版本号上下限（进程内缓存，60s TTL）
type cachedVersionBounds struct {
	min       string // 空字符串 = 不检查
	max       string // 空字符串 = 不检查
	expiresAt int64  // unix nano
}

// versionBoundsCache 版本号上下限进程内缓存
var versionBoundsCache atomic.Value // *cachedVersionBounds

// versionBoundsSF 防止缓存过期时 thundering herd
var versionBoundsSF singleflight.Group

// versionBoundsCacheTTL 缓存有效期
const versionBoundsCacheTTL = 60 * time.Second

// versionBoundsErrorTTL DB 错误时的短缓存，快速重试
const versionBoundsErrorTTL = 5 * time.Second

// versionBoundsDBTimeout singleflight 内 DB 查询超时，独立于请求 context
const versionBoundsDBTimeout = 5 * time.Second

// cachedBackendMode Backend Mode cache (in-process, 60s TTL)
type cachedBackendMode struct {
	value     bool
	expiresAt int64 // unix nano
}

var backendModeCache atomic.Value // *cachedBackendMode
var backendModeSF singleflight.Group

const backendModeCacheTTL = 60 * time.Second
const backendModeErrorTTL = 5 * time.Second
const backendModeDBTimeout = 5 * time.Second

// cachedGatewayForwardingSettings 缓存网关转发行为设置（进程内缓存，60s TTL）
type cachedGatewayForwardingSettings struct {
	fingerprintUnification bool
	metadataPassthrough    bool
	cchSigning             bool
	expiresAt              int64 // unix nano
}

var gatewayForwardingCache atomic.Value // *cachedGatewayForwardingSettings
var gatewayForwardingSF singleflight.Group

const gatewayForwardingCacheTTL = 60 * time.Second
const gatewayForwardingErrorTTL = 5 * time.Second
const gatewayForwardingDBTimeout = 5 * time.Second

// cachedCodexCLIConfig 缓存 Codex CLI UA 配置（进程内缓存，7 天 TTL，由 UpdateSettings 主动刷新）
type cachedCodexCLIConfig struct {
	userAgent            string
	version              string
	passthroughUAVersion bool
	usageDebugLogEnabled bool
	expiresAt            int64 // unix nano
}

var codexCLICfgCache atomic.Value // *cachedCodexCLIConfig
var codexCLICfgSF singleflight.Group

const codexCLICfgCacheTTL = 7 * 24 * time.Hour // 7 天；由 UpdateSettings 主动刷新

// cachedSystemPromptSettings 缓存平台级系统提示词配置（进程内缓存，7 天 TTL，由 UpdateSettings 主动刷新）
type cachedSystemPromptSettings struct {
	values    SystemPromptRuntimeSettings
	expiresAt int64 // unix nano
}

var systemPromptSettingsCache atomic.Value // *cachedSystemPromptSettings
var systemPromptSettingsSF singleflight.Group

const systemPromptSettingsCacheTTL = 7 * 24 * time.Hour
const systemPromptSettingsErrorTTL = 5 * time.Second
const systemPromptSettingsDBTimeout = 5 * time.Second

var systemPromptSettingKeys = []string{
	SettingKeySystemPromptAnthropic,
	SettingKeySystemPromptModeAnthropic,
	SettingKeySystemPromptOpenAI,
	SettingKeySystemPromptModeOpenAI,
	SettingKeySystemPromptGemini,
	SettingKeySystemPromptModeGemini,
	SettingKeySystemPromptAntigravity,
	SettingKeySystemPromptModeAntigravity,
	SettingKeySystemPromptUserScopeEnabled,
	SettingKeySystemPromptUserScopeMode,
	SettingKeySystemPromptUserScopeUserIDs,
}

// DefaultSubscriptionGroupReader validates group references used by default subscriptions.
type DefaultSubscriptionGroupReader interface {
	GetByID(ctx context.Context, id int64) (*Group, error)
}

// WebSearchManagerBuilder creates a websearch.Manager from config (injected by infra layer).
// proxyURLs maps proxy ID to resolved URL for provider-level proxy support.
type WebSearchManagerBuilder func(cfg *WebSearchEmulationConfig, proxyURLs map[int64]string)

// SettingService 系统设置服务
type SettingService struct {
	settingRepo             SettingRepository
	defaultSubGroupReader   DefaultSubscriptionGroupReader
	proxyRepo               ProxyRepository // for resolving websearch provider proxy URLs
	cfg                     *config.Config
	onUpdateCallbacks       []func() // Callbacks when settings are updated (cache invalidation, runtime refresh)
	onUpdateCallbacksMu     sync.RWMutex
	version                 string // Application version
	webSearchManagerBuilder WebSearchManagerBuilder
}

// NewSettingService 创建系统设置服务实例
func NewSettingService(settingRepo SettingRepository, cfg *config.Config) *SettingService {
	svc := &SettingService{
		settingRepo: settingRepo,
		cfg:         cfg,
	}
	// 启动时预热 Codex CLI UA 缓存，使 DB 值立即生效
	go svc.warmCodexCLIConfigCache()
	return svc
}

// warmCodexCLIConfigCache 在后台预热 Codex CLI UA 缓存，忽略错误（resolve 函数会回退到配置文件/默认值）。
func (s *SettingService) warmCodexCLIConfigCache() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.GetCodexCLIConfig(ctx)
}

// SetDefaultSubscriptionGroupReader injects an optional group reader for default subscription validation.
func (s *SettingService) SetDefaultSubscriptionGroupReader(reader DefaultSubscriptionGroupReader) {
	s.defaultSubGroupReader = reader
}

// SetProxyRepository injects a proxy repo for resolving websearch provider proxy URLs.
func (s *SettingService) SetProxyRepository(repo ProxyRepository) {
	s.proxyRepo = repo
}

// GetAllSettings 获取所有系统设置
func (s *SettingService) GetAllSettings(ctx context.Context) (*SystemSettings, error) {
	settings, err := s.settingRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("get all settings: %w", err)
	}

	return s.parseSettings(settings), nil
}

// GetFrontendURL 获取前端基础URL（数据库优先，fallback 到配置文件）
func (s *SettingService) GetFrontendURL(ctx context.Context) string {
	val, err := s.settingRepo.GetValue(ctx, SettingKeyFrontendURL)
	if err == nil && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return s.cfg.Server.FrontendURL
}

// GetPublicSettings 获取公开设置（无需登录）
func (s *SettingService) GetPublicSettings(ctx context.Context) (*PublicSettings, error) {
	keys := []string{
		SettingKeyRegistrationEnabled,
		SettingKeyEmailVerifyEnabled,
		SettingKeyRegistrationEmailSuffixWhitelist,
		SettingKeyPromoCodeEnabled,
		SettingKeyPasswordResetEnabled,
		SettingKeyInvitationCodeEnabled,
		SettingKeyTotpEnabled,
		SettingKeyChannelMonitorEnabled,
		SettingKeyTurnstileEnabled,
		SettingKeyTurnstileSiteKey,
		SettingKeySiteName,
		SettingKeySiteLogo,
		SettingKeySiteSubtitle,
		SettingKeyAPIBaseURL,
		SettingKeyContactInfo,
		SettingKeyDocURL,
		SettingKeyHomeContent,
		SettingKeyHideCcsImportButton,
		SettingKeyPurchaseSubscriptionEnabled,
		SettingKeyPurchaseSubscriptionURL,
		SettingKeyTableDefaultPageSize,
		SettingKeyTablePageSizeOptions,
		SettingKeyCustomMenuItems,
		SettingKeyCustomEndpoints,
		SettingKeyOpenAIUseKeyModelID,
		SettingKeyLinuxDoConnectEnabled,
		SettingKeyBackendModeEnabled,
		SettingKeyAttractPopupTitle,
		SettingKeyAttractPopupMarkdown,
		SettingKeyDashboardFireworksEnabled,
		SettingKeyDashboardFireworksThreshold,
		SettingPaymentEnabled,
		SettingKeyOIDCConnectEnabled,
		SettingKeyOIDCConnectProviderName,
		SettingKeyBalanceLowNotifyEnabled,
		SettingKeyBalanceLowNotifyThreshold,
		SettingKeyBalanceLowNotifyRechargeURL,
		SettingKeyAccountQuotaNotifyEnabled,
		SettingKeyReferralEnabled,
		SettingKeyReferralSalesEnabled,
	}

	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("get public settings: %w", err)
	}

	linuxDoEnabled := false
	if raw, ok := settings[SettingKeyLinuxDoConnectEnabled]; ok {
		linuxDoEnabled = raw == "true"
	} else {
		linuxDoEnabled = s.cfg != nil && s.cfg.LinuxDo.Enabled
	}
	oidcEnabled := false
	if raw, ok := settings[SettingKeyOIDCConnectEnabled]; ok {
		oidcEnabled = raw == "true"
	} else {
		oidcEnabled = s.cfg != nil && s.cfg.OIDC.Enabled
	}
	oidcProviderName := strings.TrimSpace(settings[SettingKeyOIDCConnectProviderName])
	if oidcProviderName == "" && s.cfg != nil {
		oidcProviderName = strings.TrimSpace(s.cfg.OIDC.ProviderName)
	}
	if oidcProviderName == "" {
		oidcProviderName = "OIDC"
	}

	// Password reset requires email verification to be enabled
	emailVerifyEnabled := settings[SettingKeyEmailVerifyEnabled] == "true"
	passwordResetEnabled := emailVerifyEnabled && settings[SettingKeyPasswordResetEnabled] == "true"
	registrationEmailSuffixWhitelist := ParseRegistrationEmailSuffixWhitelist(
		settings[SettingKeyRegistrationEmailSuffixWhitelist],
	)
	tableDefaultPageSize, tablePageSizeOptions := parseTablePreferences(
		settings[SettingKeyTableDefaultPageSize],
		settings[SettingKeyTablePageSizeOptions],
	)

	var balanceLowNotifyThreshold float64
	if v, err := strconv.ParseFloat(settings[SettingKeyBalanceLowNotifyThreshold], 64); err == nil && v >= 0 {
		balanceLowNotifyThreshold = v
	}

	return &PublicSettings{
		RegistrationEnabled:              settings[SettingKeyRegistrationEnabled] == "true",
		EmailVerifyEnabled:               emailVerifyEnabled,
		RegistrationEmailSuffixWhitelist: registrationEmailSuffixWhitelist,
		PromoCodeEnabled:                 settings[SettingKeyPromoCodeEnabled] != "false", // 默认启用
		PasswordResetEnabled:             passwordResetEnabled,
		InvitationCodeEnabled:            settings[SettingKeyInvitationCodeEnabled] == "true",
		TotpEnabled:                      settings[SettingKeyTotpEnabled] == "true",
		ChannelMonitorEnabled:            settings[SettingKeyChannelMonitorEnabled] == "true",
		TurnstileEnabled:                 settings[SettingKeyTurnstileEnabled] == "true",
		TurnstileSiteKey:                 settings[SettingKeyTurnstileSiteKey],
		SiteName:                         s.getStringOrDefault(settings, SettingKeySiteName, "FluxCode"),
		SiteLogo:                         settings[SettingKeySiteLogo],
		SiteSubtitle:                     s.getStringOrDefault(settings, SettingKeySiteSubtitle, "Subscription to API Conversion Platform"),
		APIBaseURL:                       settings[SettingKeyAPIBaseURL],
		ContactInfo:                      settings[SettingKeyContactInfo],
		DocURL:                           settings[SettingKeyDocURL],
		HomeContent:                      settings[SettingKeyHomeContent],
		HideCcsImportButton:              settings[SettingKeyHideCcsImportButton] == "true",
		PurchaseSubscriptionEnabled:      settings[SettingKeyPurchaseSubscriptionEnabled] == "true",
		PurchaseSubscriptionURL:          strings.TrimSpace(settings[SettingKeyPurchaseSubscriptionURL]),
		TableDefaultPageSize:             tableDefaultPageSize,
		TablePageSizeOptions:             tablePageSizeOptions,
		CustomMenuItems:                  settings[SettingKeyCustomMenuItems],
		CustomEndpoints:                  settings[SettingKeyCustomEndpoints],
		OpenAIUseKeyModelID:              s.getStringOrDefault(settings, SettingKeyOpenAIUseKeyModelID, "gpt-5.5"),
		LinuxDoOAuthEnabled:              linuxDoEnabled,
		BackendModeEnabled:               settings[SettingKeyBackendModeEnabled] == "true",
		AttractPopupTitle:                settings[SettingKeyAttractPopupTitle],
		AttractPopupMarkdown:             settings[SettingKeyAttractPopupMarkdown],
		DashboardFireworksEnabled:        settings[SettingKeyDashboardFireworksEnabled] != "false",
		DashboardFireworksThreshold:      parseDashboardFireworksThreshold(settings[SettingKeyDashboardFireworksThreshold]),
		PaymentEnabled:                   settings[SettingPaymentEnabled] == "true",
		OIDCOAuthEnabled:                 oidcEnabled,
		OIDCOAuthProviderName:            oidcProviderName,
		BalanceLowNotifyEnabled:          settings[SettingKeyBalanceLowNotifyEnabled] == "true",
		AccountQuotaNotifyEnabled:        settings[SettingKeyAccountQuotaNotifyEnabled] == "true",
		BalanceLowNotifyThreshold:        balanceLowNotifyThreshold,
		BalanceLowNotifyRechargeURL:      settings[SettingKeyBalanceLowNotifyRechargeURL],
		ReferralEnabled:                  settings[SettingKeyReferralEnabled] == "true",
		ReferralSalesEnabled:             settings[SettingKeyReferralSalesEnabled] == "true",
	}, nil
}

// SetOnUpdateCallback registers a callback function to be called when settings are updated.
// Multiple runtime components may register independently; callbacks are intentionally additive.
func (s *SettingService) SetOnUpdateCallback(callback func()) {
	if callback == nil {
		return
	}
	s.onUpdateCallbacksMu.Lock()
	s.onUpdateCallbacks = append(s.onUpdateCallbacks, callback)
	s.onUpdateCallbacksMu.Unlock()
}

// GetSuccessfulRequestRecordRuntimeSettings 读取成功请求正文记录的动态设置。
// 请求热路径不调用此方法；采集服务只在启动、设置变更和周期刷新时读取。
func (s *SettingService) GetSuccessfulRequestRecordRuntimeSettings(ctx context.Context) (SuccessfulRequestRecordRuntimeSettings, error) {
	result := SuccessfulRequestRecordRuntimeSettings{
		Enabled:      false,
		MaxBodyBytes: DefaultSuccessfulRequestRecordsMaxBodyBytes,
	}
	if s == nil || s.settingRepo == nil {
		return result, nil
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeySuccessfulRequestRecordsEnabled,
		SettingKeySuccessfulRequestRecordsMaxBodyBytes,
	})
	if err != nil {
		return result, fmt.Errorf("get successful request record settings: %w", err)
	}
	if raw, ok := values[SettingKeySuccessfulRequestRecordsEnabled]; ok {
		result.Enabled, _ = strconv.ParseBool(strings.TrimSpace(raw))
	}
	if raw, ok := values[SettingKeySuccessfulRequestRecordsMaxBodyBytes]; ok {
		if parsed, parseErr := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); parseErr == nil && parsed >= MinSuccessfulRequestRecordsMaxBodyBytes && parsed <= MaxSuccessfulRequestRecordsMaxBodyBytes {
			result.MaxBodyBytes = parsed
		}
	}
	return result, nil
}

// SetVersion sets the application version for injection into public settings
func (s *SettingService) SetVersion(version string) {
	s.version = version
}

// GetPublicSettingsForInjection returns public settings in a format suitable for HTML injection
// This implements the web.PublicSettingsProvider interface
func (s *SettingService) GetPublicSettingsForInjection(ctx context.Context) (any, error) {
	settings, err := s.GetPublicSettings(ctx)
	if err != nil {
		return nil, err
	}

	// Return a struct that matches the frontend's expected format
	return &struct {
		RegistrationEnabled              bool            `json:"registration_enabled"`
		EmailVerifyEnabled               bool            `json:"email_verify_enabled"`
		RegistrationEmailSuffixWhitelist []string        `json:"registration_email_suffix_whitelist"`
		PromoCodeEnabled                 bool            `json:"promo_code_enabled"`
		PasswordResetEnabled             bool            `json:"password_reset_enabled"`
		InvitationCodeEnabled            bool            `json:"invitation_code_enabled"`
		TotpEnabled                      bool            `json:"totp_enabled"`
		ChannelMonitorEnabled            bool            `json:"channel_monitor_enabled"`
		TurnstileEnabled                 bool            `json:"turnstile_enabled"`
		TurnstileSiteKey                 string          `json:"turnstile_site_key,omitempty"`
		SiteName                         string          `json:"site_name"`
		SiteLogo                         string          `json:"site_logo,omitempty"`
		SiteSubtitle                     string          `json:"site_subtitle,omitempty"`
		APIBaseURL                       string          `json:"api_base_url,omitempty"`
		ContactInfo                      string          `json:"contact_info,omitempty"`
		DocURL                           string          `json:"doc_url,omitempty"`
		HomeContent                      string          `json:"home_content,omitempty"`
		HideCcsImportButton              bool            `json:"hide_ccs_import_button"`
		PurchaseSubscriptionEnabled      bool            `json:"purchase_subscription_enabled"`
		PurchaseSubscriptionURL          string          `json:"purchase_subscription_url,omitempty"`
		TableDefaultPageSize             int             `json:"table_default_page_size"`
		TablePageSizeOptions             []int           `json:"table_page_size_options"`
		CustomMenuItems                  json.RawMessage `json:"custom_menu_items"`
		CustomEndpoints                  json.RawMessage `json:"custom_endpoints"`
		OpenAIUseKeyModelID              string          `json:"openai_use_key_model_id"`
		LinuxDoOAuthEnabled              bool            `json:"linuxdo_oauth_enabled"`
		BackendModeEnabled               bool            `json:"backend_mode_enabled"`
		AttractPopupTitle                string          `json:"attract_popup_title,omitempty"`
		AttractPopupMarkdown             string          `json:"attract_popup_markdown,omitempty"`
		DashboardFireworksEnabled        bool            `json:"dashboard_fireworks_enabled"`
		DashboardFireworksThreshold      float64         `json:"dashboard_fireworks_threshold"`
		PaymentEnabled                   bool            `json:"payment_enabled"`
		OIDCOAuthEnabled                 bool            `json:"oidc_oauth_enabled"`
		OIDCOAuthProviderName            string          `json:"oidc_oauth_provider_name"`
		Version                          string          `json:"version,omitempty"`
		BalanceLowNotifyEnabled          bool            `json:"balance_low_notify_enabled"`
		AccountQuotaNotifyEnabled        bool            `json:"account_quota_notify_enabled"`
		BalanceLowNotifyThreshold        float64         `json:"balance_low_notify_threshold"`
		BalanceLowNotifyRechargeURL      string          `json:"balance_low_notify_recharge_url"`
	}{
		RegistrationEnabled:              settings.RegistrationEnabled,
		EmailVerifyEnabled:               settings.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist: settings.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                 settings.PromoCodeEnabled,
		PasswordResetEnabled:             settings.PasswordResetEnabled,
		InvitationCodeEnabled:            settings.InvitationCodeEnabled,
		TotpEnabled:                      settings.TotpEnabled,
		ChannelMonitorEnabled:            settings.ChannelMonitorEnabled,
		TurnstileEnabled:                 settings.TurnstileEnabled,
		TurnstileSiteKey:                 settings.TurnstileSiteKey,
		SiteName:                         settings.SiteName,
		SiteLogo:                         settings.SiteLogo,
		SiteSubtitle:                     settings.SiteSubtitle,
		APIBaseURL:                       settings.APIBaseURL,
		ContactInfo:                      settings.ContactInfo,
		DocURL:                           settings.DocURL,
		HomeContent:                      settings.HomeContent,
		HideCcsImportButton:              settings.HideCcsImportButton,
		PurchaseSubscriptionEnabled:      settings.PurchaseSubscriptionEnabled,
		PurchaseSubscriptionURL:          settings.PurchaseSubscriptionURL,
		TableDefaultPageSize:             settings.TableDefaultPageSize,
		TablePageSizeOptions:             settings.TablePageSizeOptions,
		CustomMenuItems:                  filterUserVisibleMenuItems(settings.CustomMenuItems),
		CustomEndpoints:                  safeRawJSONArray(settings.CustomEndpoints),
		OpenAIUseKeyModelID:              settings.OpenAIUseKeyModelID,
		LinuxDoOAuthEnabled:              settings.LinuxDoOAuthEnabled,
		BackendModeEnabled:               settings.BackendModeEnabled,
		AttractPopupTitle:                settings.AttractPopupTitle,
		AttractPopupMarkdown:             settings.AttractPopupMarkdown,
		DashboardFireworksEnabled:        settings.DashboardFireworksEnabled,
		DashboardFireworksThreshold:      settings.DashboardFireworksThreshold,
		PaymentEnabled:                   settings.PaymentEnabled,
		OIDCOAuthEnabled:                 settings.OIDCOAuthEnabled,
		OIDCOAuthProviderName:            settings.OIDCOAuthProviderName,
		Version:                          s.version,
		BalanceLowNotifyEnabled:          settings.BalanceLowNotifyEnabled,
		AccountQuotaNotifyEnabled:        settings.AccountQuotaNotifyEnabled,
		BalanceLowNotifyThreshold:        settings.BalanceLowNotifyThreshold,
		BalanceLowNotifyRechargeURL:      settings.BalanceLowNotifyRechargeURL,
	}, nil
}

func (s *SettingService) GetChannelMonitorRuntime(ctx context.Context) ChannelMonitorRuntimeSettings {
	settings, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyChannelMonitorEnabled,
		SettingKeyChannelMonitorDefaultIntervalSeconds,
	})
	if err != nil {
		return ChannelMonitorRuntimeSettings{
			Enabled:                false,
			DefaultIntervalSeconds: ChannelMonitorFallbackIntervalSecond,
		}
	}
	return ChannelMonitorRuntimeSettings{
		Enabled: settings[SettingKeyChannelMonitorEnabled] == "true",
		DefaultIntervalSeconds: NormalizeChannelMonitorInterval(
			parseIntSetting(settings[SettingKeyChannelMonitorDefaultIntervalSeconds], ChannelMonitorFallbackIntervalSecond),
			ChannelMonitorFallbackIntervalSecond,
		),
	}
}

// filterUserVisibleMenuItems filters out admin-only menu items from a raw JSON
// array string, returning only items with visibility != "admin".
func filterUserVisibleMenuItems(raw string) json.RawMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return json.RawMessage("[]")
	}
	var items []struct {
		Visibility string `json:"visibility"`
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return json.RawMessage("[]")
	}

	// Parse full items to preserve all fields
	var fullItems []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fullItems); err != nil {
		return json.RawMessage("[]")
	}

	var filtered []json.RawMessage
	for i, item := range items {
		if item.Visibility != "admin" {
			filtered = append(filtered, fullItems[i])
		}
	}
	if len(filtered) == 0 {
		return json.RawMessage("[]")
	}
	result, err := json.Marshal(filtered)
	if err != nil {
		return json.RawMessage("[]")
	}
	return result
}

// safeRawJSONArray returns raw as json.RawMessage if it's valid JSON, otherwise "[]".
func safeRawJSONArray(raw string) json.RawMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return json.RawMessage("[]")
	}
	if json.Valid([]byte(raw)) {
		return json.RawMessage(raw)
	}
	return json.RawMessage("[]")
}

func parseIntSetting(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return value
}

func parseDashboardFireworksThreshold(raw string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return DefaultDashboardFireworksThreshold
	}
	return value
}

func normalizeOpenAIImageURLCacheTTLHours(hours int) int {
	if hours <= 0 {
		return DefaultOpenAIImageURLCacheTTLHours
	}
	return hours
}

func NormalizeGeneratedImageStorageSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "", GeneratedImageStorageSourceDB:
		return GeneratedImageStorageSourceDB
	case GeneratedImageStorageSourceQiniu:
		return GeneratedImageStorageSourceQiniu
	default:
		return ""
	}
}

func normalizeQiniuPrefix(prefix string) string {
	normalized := strings.Trim(strings.TrimSpace(prefix), "/")
	if normalized == "" {
		return DefaultQiniuPrefix
	}
	return normalized
}

func normalizeQiniuUploadTimeoutSeconds(seconds int) int {
	if seconds <= 0 {
		return DefaultQiniuUploadTimeoutSeconds
	}
	return seconds
}

func normalizeQiniuTokenTTLSeconds(seconds int) int {
	if seconds <= 0 {
		return DefaultQiniuTokenTTLSeconds
	}
	return seconds
}

func (s *SettingService) GetOpenAIImageURLCacheTTL(ctx context.Context) time.Duration {
	if s == nil || s.settingRepo == nil {
		return time.Duration(DefaultOpenAIImageURLCacheTTLHours) * time.Hour
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpenAIImageURLCacheTTLHours)
	if err != nil {
		return time.Duration(DefaultOpenAIImageURLCacheTTLHours) * time.Hour
	}
	return time.Duration(normalizeOpenAIImageURLCacheTTLHours(parseIntSetting(raw, DefaultOpenAIImageURLCacheTTLHours))) * time.Hour
}

type GeneratedImageStorageSettings struct {
	Source                    string
	ConfigSource              string
	QiniuAccessKey            string
	QiniuSecretKey            string
	QiniuBucket               string
	QiniuCDNDomain            string
	QiniuPrefix               string
	QiniuUseHTTPS             bool
	QiniuUploadTimeoutSeconds int
	QiniuTokenTTLSeconds      int
}

var generatedImageStorageSettingKeys = []string{
	SettingKeyGeneratedImageStorageSource,
	SettingKeyGeneratedImageStorageConfigSource,
	SettingKeyQiniuAccessKey,
	SettingKeyQiniuSecretKey,
	SettingKeyQiniuBucket,
	SettingKeyQiniuCDNDomain,
	SettingKeyQiniuPrefix,
	SettingKeyQiniuUseHTTPS,
	SettingKeyQiniuUploadTimeoutSeconds,
	SettingKeyQiniuTokenTTLSeconds,
}

func (s *SettingService) GetGeneratedImageStorageSettings(ctx context.Context) (*GeneratedImageStorageSettings, error) {
	if s == nil || s.settingRepo == nil {
		return parseGeneratedImageStorageSettings(nil), nil
	}
	values, err := s.settingRepo.GetMultiple(ctx, generatedImageStorageSettingKeys)
	if err != nil {
		return nil, fmt.Errorf("get generated image storage settings: %w", err)
	}
	return parseGeneratedImageStorageSettings(values), nil
}

func parseGeneratedImageStorageSettings(settings map[string]string) *GeneratedImageStorageSettings {
	if settings == nil {
		settings = map[string]string{}
	}
	source := NormalizeGeneratedImageStorageSource(settings[SettingKeyGeneratedImageStorageSource])
	if source == "" {
		source = GeneratedImageStorageSourceDB
	}
	rawConfigSource := strings.TrimSpace(settings[SettingKeyGeneratedImageStorageConfigSource])
	configSource := source
	if rawConfigSource != "" {
		configSource = NormalizeGeneratedImageStorageSource(rawConfigSource)
	}
	if configSource == "" {
		configSource = source
	}
	useHTTPS := true
	if raw, ok := settings[SettingKeyQiniuUseHTTPS]; ok {
		useHTTPS = raw == "true"
	}
	return &GeneratedImageStorageSettings{
		Source:                    source,
		ConfigSource:              configSource,
		QiniuAccessKey:            strings.TrimSpace(settings[SettingKeyQiniuAccessKey]),
		QiniuSecretKey:            strings.TrimSpace(settings[SettingKeyQiniuSecretKey]),
		QiniuBucket:               strings.TrimSpace(settings[SettingKeyQiniuBucket]),
		QiniuCDNDomain:            strings.TrimRight(strings.TrimSpace(settings[SettingKeyQiniuCDNDomain]), "/"),
		QiniuPrefix:               normalizeQiniuPrefix(settings[SettingKeyQiniuPrefix]),
		QiniuUseHTTPS:             useHTTPS,
		QiniuUploadTimeoutSeconds: normalizeQiniuUploadTimeoutSeconds(parseIntSetting(settings[SettingKeyQiniuUploadTimeoutSeconds], DefaultQiniuUploadTimeoutSeconds)),
		QiniuTokenTTLSeconds:      normalizeQiniuTokenTTLSeconds(parseIntSetting(settings[SettingKeyQiniuTokenTTLSeconds], DefaultQiniuTokenTTLSeconds)),
	}
}

func (s *SettingService) IsGeneratedImageCleanupEnabled(ctx context.Context) bool {
	if s == nil || s.settingRepo == nil {
		return false
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyGeneratedImageCleanupEnabled)
	if err != nil {
		return false
	}
	return raw == "true"
}

// GetFrameSrcOrigins returns deduplicated http(s) origins from home_content URL,
// purchase_subscription_url, and all custom_menu_items URLs. Used by the router layer for CSP frame-src injection.
func (s *SettingService) GetFrameSrcOrigins(ctx context.Context) ([]string, error) {
	settings, err := s.GetPublicSettings(ctx)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var origins []string

	addOrigin := func(rawURL string) {
		if origin := extractOriginFromURL(rawURL); origin != "" {
			if _, ok := seen[origin]; !ok {
				seen[origin] = struct{}{}
				origins = append(origins, origin)
			}
		}
	}

	// home content URL (when home_content is set to a URL for iframe embedding)
	addOrigin(settings.HomeContent)

	// purchase subscription URL
	if settings.PurchaseSubscriptionEnabled {
		addOrigin(settings.PurchaseSubscriptionURL)
	}

	// all custom menu items (including admin-only, since CSP must allow all iframes)
	for _, item := range parseCustomMenuItemURLs(settings.CustomMenuItems) {
		addOrigin(item)
	}

	return origins, nil
}

// extractOriginFromURL returns the scheme+host origin from rawURL.
// Only http and https schemes are accepted.
func extractOriginFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// parseCustomMenuItemURLs extracts URLs from a raw JSON array of custom menu items.
func parseCustomMenuItemURLs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	var items []struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	urls := make([]string, 0, len(items))
	for _, item := range items {
		if item.URL != "" {
			urls = append(urls, item.URL)
		}
	}
	return urls
}

// UpdateSettings 更新系统设置
func (s *SettingService) UpdateSettings(ctx context.Context, settings *SystemSettings) error {
	if settings == nil {
		return infraerrors.BadRequest("INVALID_SETTINGS", "settings must not be nil")
	}
	if settings.SuccessfulRequestRecordsEnabled {
		if !s.IsTotpEncryptionKeyConfigured() {
			return infraerrors.BadRequest("SUCCESSFUL_REQUEST_RECORDS_KEY_REQUIRED", "TOTP_ENCRYPTION_KEY environment variable must be configured before enabling successful request records")
		}
		if settings.SuccessfulRequestRecordsMaxBodyBytes < MinSuccessfulRequestRecordsMaxBodyBytes || settings.SuccessfulRequestRecordsMaxBodyBytes > MaxSuccessfulRequestRecordsMaxBodyBytes {
			return infraerrors.BadRequest("INVALID_SUCCESSFUL_REQUEST_RECORDS_MAX_BODY_BYTES", fmt.Sprintf("successful request record body limit must be between %d and %d bytes", MinSuccessfulRequestRecordsMaxBodyBytes, MaxSuccessfulRequestRecordsMaxBodyBytes))
		}
	} else if settings.SuccessfulRequestRecordsMaxBodyBytes <= 0 {
		settings.SuccessfulRequestRecordsMaxBodyBytes = DefaultSuccessfulRequestRecordsMaxBodyBytes
	}
	if err := s.validateDefaultSubscriptionGroups(ctx, settings.DefaultSubscriptions); err != nil {
		return err
	}
	normalizedWhitelist, err := NormalizeRegistrationEmailSuffixWhitelist(settings.RegistrationEmailSuffixWhitelist)
	if err != nil {
		return infraerrors.BadRequest("INVALID_REGISTRATION_EMAIL_SUFFIX_WHITELIST", err.Error())
	}
	if normalizedWhitelist == nil {
		normalizedWhitelist = []string{}
	}
	settings.RegistrationEmailSuffixWhitelist = normalizedWhitelist

	updates := make(map[string]string)

	// 注册设置
	updates[SettingKeyRegistrationEnabled] = strconv.FormatBool(settings.RegistrationEnabled)
	updates[SettingKeyEmailVerifyEnabled] = strconv.FormatBool(settings.EmailVerifyEnabled)
	registrationEmailSuffixWhitelistJSON, err := json.Marshal(settings.RegistrationEmailSuffixWhitelist)
	if err != nil {
		return fmt.Errorf("marshal registration email suffix whitelist: %w", err)
	}
	updates[SettingKeyRegistrationEmailSuffixWhitelist] = string(registrationEmailSuffixWhitelistJSON)
	updates[SettingKeyPromoCodeEnabled] = strconv.FormatBool(settings.PromoCodeEnabled)
	updates[SettingKeyPasswordResetEnabled] = strconv.FormatBool(settings.PasswordResetEnabled)
	updates[SettingKeyFrontendURL] = settings.FrontendURL
	updates[SettingKeyInvitationCodeEnabled] = strconv.FormatBool(settings.InvitationCodeEnabled)
	updates[SettingKeyTotpEnabled] = strconv.FormatBool(settings.TotpEnabled)
	updates[SettingKeyChannelMonitorEnabled] = strconv.FormatBool(settings.ChannelMonitorEnabled)
	updates[SettingKeyChannelMonitorDefaultIntervalSeconds] = strconv.Itoa(
		NormalizeChannelMonitorInterval(settings.ChannelMonitorDefaultIntervalSeconds, ChannelMonitorFallbackIntervalSecond),
	)

	// 邮件服务设置（只有非空才更新密码）
	updates[SettingKeyEmailProvider] = NormalizeEmailProvider(settings.EmailProvider)
	updates[SettingKeySMTPHost] = settings.SMTPHost
	updates[SettingKeySMTPPort] = strconv.Itoa(settings.SMTPPort)
	updates[SettingKeySMTPUsername] = settings.SMTPUsername
	if settings.SMTPPassword != "" {
		updates[SettingKeySMTPPassword] = settings.SMTPPassword
	}
	updates[SettingKeySMTPFrom] = settings.SMTPFrom
	updates[SettingKeySMTPFromName] = settings.SMTPFromName
	updates[SettingKeySMTPUseTLS] = strconv.FormatBool(settings.SMTPUseTLS)
	if settings.ResendAPIKey != "" {
		updates[SettingKeyResendAPIKey] = settings.ResendAPIKey
	}
	updates[SettingKeyResendFrom] = settings.ResendFrom
	updates[SettingKeyResendFromName] = settings.ResendFromName

	// Cloudflare Turnstile 设置（只有非空才更新密钥）
	updates[SettingKeyTurnstileEnabled] = strconv.FormatBool(settings.TurnstileEnabled)
	updates[SettingKeyTurnstileSiteKey] = settings.TurnstileSiteKey
	if settings.TurnstileSecretKey != "" {
		updates[SettingKeyTurnstileSecretKey] = settings.TurnstileSecretKey
	}

	// LinuxDo Connect OAuth 登录
	updates[SettingKeyLinuxDoConnectEnabled] = strconv.FormatBool(settings.LinuxDoConnectEnabled)
	updates[SettingKeyLinuxDoConnectClientID] = settings.LinuxDoConnectClientID
	updates[SettingKeyLinuxDoConnectRedirectURL] = settings.LinuxDoConnectRedirectURL
	if settings.LinuxDoConnectClientSecret != "" {
		updates[SettingKeyLinuxDoConnectClientSecret] = settings.LinuxDoConnectClientSecret
	}

	// Generic OIDC OAuth 登录
	updates[SettingKeyOIDCConnectEnabled] = strconv.FormatBool(settings.OIDCConnectEnabled)
	updates[SettingKeyOIDCConnectProviderName] = settings.OIDCConnectProviderName
	updates[SettingKeyOIDCConnectClientID] = settings.OIDCConnectClientID
	updates[SettingKeyOIDCConnectIssuerURL] = settings.OIDCConnectIssuerURL
	updates[SettingKeyOIDCConnectDiscoveryURL] = settings.OIDCConnectDiscoveryURL
	updates[SettingKeyOIDCConnectAuthorizeURL] = settings.OIDCConnectAuthorizeURL
	updates[SettingKeyOIDCConnectTokenURL] = settings.OIDCConnectTokenURL
	updates[SettingKeyOIDCConnectUserInfoURL] = settings.OIDCConnectUserInfoURL
	updates[SettingKeyOIDCConnectJWKSURL] = settings.OIDCConnectJWKSURL
	updates[SettingKeyOIDCConnectScopes] = settings.OIDCConnectScopes
	updates[SettingKeyOIDCConnectRedirectURL] = settings.OIDCConnectRedirectURL
	updates[SettingKeyOIDCConnectFrontendRedirectURL] = settings.OIDCConnectFrontendRedirectURL
	updates[SettingKeyOIDCConnectTokenAuthMethod] = settings.OIDCConnectTokenAuthMethod
	updates[SettingKeyOIDCConnectUsePKCE] = strconv.FormatBool(settings.OIDCConnectUsePKCE)
	updates[SettingKeyOIDCConnectValidateIDToken] = strconv.FormatBool(settings.OIDCConnectValidateIDToken)
	updates[SettingKeyOIDCConnectAllowedSigningAlgs] = settings.OIDCConnectAllowedSigningAlgs
	updates[SettingKeyOIDCConnectClockSkewSeconds] = strconv.Itoa(settings.OIDCConnectClockSkewSeconds)
	updates[SettingKeyOIDCConnectRequireEmailVerified] = strconv.FormatBool(settings.OIDCConnectRequireEmailVerified)
	updates[SettingKeyOIDCConnectUserInfoEmailPath] = settings.OIDCConnectUserInfoEmailPath
	updates[SettingKeyOIDCConnectUserInfoIDPath] = settings.OIDCConnectUserInfoIDPath
	updates[SettingKeyOIDCConnectUserInfoUsernamePath] = settings.OIDCConnectUserInfoUsernamePath
	if settings.OIDCConnectClientSecret != "" {
		updates[SettingKeyOIDCConnectClientSecret] = settings.OIDCConnectClientSecret
	}

	// OEM设置
	updates[SettingKeySiteName] = settings.SiteName
	updates[SettingKeySiteLogo] = settings.SiteLogo
	updates[SettingKeySiteSubtitle] = settings.SiteSubtitle
	updates[SettingKeyAPIBaseURL] = settings.APIBaseURL
	updates[SettingKeyContactInfo] = settings.ContactInfo
	updates[SettingKeyDocURL] = settings.DocURL
	updates[SettingKeyHomeContent] = settings.HomeContent
	updates[SettingKeyHideCcsImportButton] = strconv.FormatBool(settings.HideCcsImportButton)
	updates[SettingKeyPurchaseSubscriptionEnabled] = strconv.FormatBool(settings.PurchaseSubscriptionEnabled)
	updates[SettingKeyPurchaseSubscriptionURL] = strings.TrimSpace(settings.PurchaseSubscriptionURL)
	tableDefaultPageSize, tablePageSizeOptions := normalizeTablePreferences(
		settings.TableDefaultPageSize,
		settings.TablePageSizeOptions,
	)
	updates[SettingKeyTableDefaultPageSize] = strconv.Itoa(tableDefaultPageSize)
	tablePageSizeOptionsJSON, err := json.Marshal(tablePageSizeOptions)
	if err != nil {
		return fmt.Errorf("marshal table page size options: %w", err)
	}
	updates[SettingKeyTablePageSizeOptions] = string(tablePageSizeOptionsJSON)
	updates[SettingKeyCustomMenuItems] = settings.CustomMenuItems
	updates[SettingKeyCustomEndpoints] = settings.CustomEndpoints
	updates[SettingKeyOpenAIUseKeyModelID] = strings.TrimSpace(settings.OpenAIUseKeyModelID)
	updates[SettingKeyOpenAIImageURLCacheTTLHours] = strconv.Itoa(normalizeOpenAIImageURLCacheTTLHours(settings.OpenAIImageURLCacheTTLHours))
	generatedImageStorageSource := NormalizeGeneratedImageStorageSource(settings.GeneratedImageStorageSource)
	if generatedImageStorageSource == "" {
		return infraerrors.BadRequest("INVALID_GENERATED_IMAGE_STORAGE_SOURCE", "generated image storage source must be db or qiniu")
	}
	generatedImageStorageConfigSource := generatedImageStorageSource
	if rawConfigSource := strings.TrimSpace(settings.GeneratedImageStorageConfigSource); rawConfigSource != "" {
		generatedImageStorageConfigSource = NormalizeGeneratedImageStorageSource(rawConfigSource)
		if generatedImageStorageConfigSource == "" {
			return infraerrors.BadRequest("INVALID_GENERATED_IMAGE_STORAGE_CONFIG_SOURCE", "generated image storage config source must be db or qiniu")
		}
	}
	qiniuAccessKey := strings.TrimSpace(settings.QiniuAccessKey)
	qiniuSecretKey := strings.TrimSpace(settings.QiniuSecretKey)
	qiniuBucket := strings.TrimSpace(settings.QiniuBucket)
	qiniuCDNDomain := strings.TrimRight(strings.TrimSpace(settings.QiniuCDNDomain), "/")
	qiniuPrefix := normalizeQiniuPrefix(settings.QiniuPrefix)
	qiniuUploadTimeoutSeconds := normalizeQiniuUploadTimeoutSeconds(settings.QiniuUploadTimeoutSeconds)
	qiniuTokenTTLSeconds := normalizeQiniuTokenTTLSeconds(settings.QiniuTokenTTLSeconds)
	if generatedImageStorageSource == GeneratedImageStorageSourceQiniu {
		if qiniuAccessKey == "" {
			return infraerrors.BadRequest("QINIU_ACCESS_KEY_REQUIRED", "qiniu access key is required when generated image storage source is qiniu")
		}
		if qiniuSecretKey == "" {
			return infraerrors.BadRequest("QINIU_SECRET_KEY_REQUIRED", "qiniu secret key is required when generated image storage source is qiniu")
		}
		if qiniuBucket == "" {
			return infraerrors.BadRequest("QINIU_BUCKET_REQUIRED", "qiniu bucket is required when generated image storage source is qiniu")
		}
		if qiniuCDNDomain == "" {
			return infraerrors.BadRequest("QINIU_CDN_DOMAIN_REQUIRED", "qiniu cdn domain is required when generated image storage source is qiniu")
		}
	}
	updates[SettingKeyGeneratedImageStorageSource] = generatedImageStorageSource
	updates[SettingKeyGeneratedImageStorageConfigSource] = generatedImageStorageConfigSource
	updates[SettingKeyQiniuAccessKey] = qiniuAccessKey
	if qiniuSecretKey != "" {
		updates[SettingKeyQiniuSecretKey] = qiniuSecretKey
	}
	updates[SettingKeyQiniuBucket] = qiniuBucket
	updates[SettingKeyQiniuCDNDomain] = qiniuCDNDomain
	updates[SettingKeyQiniuPrefix] = qiniuPrefix
	updates[SettingKeyQiniuUseHTTPS] = strconv.FormatBool(settings.QiniuUseHTTPS)
	updates[SettingKeyQiniuUploadTimeoutSeconds] = strconv.Itoa(qiniuUploadTimeoutSeconds)
	updates[SettingKeyQiniuTokenTTLSeconds] = strconv.Itoa(qiniuTokenTTLSeconds)
	updates[SettingKeyGeneratedImageCleanupEnabled] = strconv.FormatBool(settings.GeneratedImageCleanupEnabled)

	// 默认配置
	updates[SettingKeyDefaultConcurrency] = strconv.Itoa(settings.DefaultConcurrency)
	updates[SettingKeyDefaultBalance] = strconv.FormatFloat(settings.DefaultBalance, 'f', 8, 64)
	defaultSubsJSON, err := json.Marshal(settings.DefaultSubscriptions)
	if err != nil {
		return fmt.Errorf("marshal default subscriptions: %w", err)
	}
	updates[SettingKeyDefaultSubscriptions] = string(defaultSubsJSON)

	// Model fallback configuration
	updates[SettingKeyEnableModelFallback] = strconv.FormatBool(settings.EnableModelFallback)
	updates[SettingKeyFallbackModelAnthropic] = settings.FallbackModelAnthropic
	updates[SettingKeyFallbackModelOpenAI] = settings.FallbackModelOpenAI
	updates[SettingKeyFallbackModelGemini] = settings.FallbackModelGemini
	updates[SettingKeyFallbackModelAntigravity] = settings.FallbackModelAntigravity

	// Identity patch configuration (Claude -> Gemini)
	updates[SettingKeyEnableIdentityPatch] = strconv.FormatBool(settings.EnableIdentityPatch)
	updates[SettingKeyIdentityPatchPrompt] = settings.IdentityPatchPrompt

	if err := normalizeSystemPromptSystemSettings(settings); err != nil {
		return err
	}
	settings.SystemPromptUserScopeMode = normalizeSystemPromptUserScopeMode(settings.SystemPromptUserScopeMode)
	settings.SystemPromptUserScopeUserIDs = normalizeSystemPromptUserScopeUserIDs(settings.SystemPromptUserScopeUserIDs)
	if settings.SystemPromptUserScopeMode == SystemPromptUserScopeAll {
		settings.SystemPromptUserScopeUserIDs = []int64{}
	}
	systemPromptUserScopeUserIDsJSON, err := marshalSystemPromptUserScopeUserIDs(settings.SystemPromptUserScopeUserIDs)
	if err != nil {
		return fmt.Errorf("marshal system prompt user scope user ids: %w", err)
	}
	updates[SettingKeySystemPromptAnthropic] = settings.SystemPromptAnthropic
	updates[SettingKeySystemPromptModeAnthropic] = settings.SystemPromptModeAnthropic
	updates[SettingKeySystemPromptOpenAI] = settings.SystemPromptOpenAI
	updates[SettingKeySystemPromptModeOpenAI] = settings.SystemPromptModeOpenAI
	updates[SettingKeySystemPromptGemini] = settings.SystemPromptGemini
	updates[SettingKeySystemPromptModeGemini] = settings.SystemPromptModeGemini
	updates[SettingKeySystemPromptAntigravity] = settings.SystemPromptAntigravity
	updates[SettingKeySystemPromptModeAntigravity] = settings.SystemPromptModeAntigravity
	updates[SettingKeySystemPromptUserScopeEnabled] = strconv.FormatBool(settings.SystemPromptUserScopeEnabled)
	updates[SettingKeySystemPromptUserScopeMode] = settings.SystemPromptUserScopeMode
	updates[SettingKeySystemPromptUserScopeUserIDs] = systemPromptUserScopeUserIDsJSON

	// Ops monitoring (vNext)
	updates[SettingKeyOpsMonitoringEnabled] = strconv.FormatBool(settings.OpsMonitoringEnabled)
	updates[SettingKeyOpsRealtimeMonitoringEnabled] = strconv.FormatBool(settings.OpsRealtimeMonitoringEnabled)
	updates[SettingKeyOpsQueryModeDefault] = string(ParseOpsQueryMode(settings.OpsQueryModeDefault))
	if settings.OpsMetricsIntervalSeconds > 0 {
		updates[SettingKeyOpsMetricsIntervalSeconds] = strconv.Itoa(settings.OpsMetricsIntervalSeconds)
	}

	// Claude Code version check
	updates[SettingKeyMinClaudeCodeVersion] = settings.MinClaudeCodeVersion
	updates[SettingKeyMaxClaudeCodeVersion] = settings.MaxClaudeCodeVersion

	// 分组隔离
	updates[SettingKeyAllowUngroupedKeyScheduling] = strconv.FormatBool(settings.AllowUngroupedKeyScheduling)

	// Backend Mode
	updates[SettingKeyBackendModeEnabled] = strconv.FormatBool(settings.BackendModeEnabled)

	// 兑换码发货文案 & 引流弹窗
	updates[SettingKeyRedeemDeliveryText] = settings.RedeemDeliveryText
	updates[SettingKeyAttractPopupTitle] = settings.AttractPopupTitle
	updates[SettingKeyAttractPopupMarkdown] = settings.AttractPopupMarkdown
	updates[SettingKeyDashboardFireworksEnabled] = strconv.FormatBool(settings.DashboardFireworksEnabled)
	updates[SettingKeyDashboardFireworksThreshold] = strconv.FormatFloat(
		parseDashboardFireworksThreshold(strconv.FormatFloat(settings.DashboardFireworksThreshold, 'f', -1, 64)),
		'f',
		-1,
		64,
	)

	// Gateway forwarding behavior
	updates[SettingKeyEnableFingerprintUnification] = strconv.FormatBool(settings.EnableFingerprintUnification)
	updates[SettingKeyEnableMetadataPassthrough] = strconv.FormatBool(settings.EnableMetadataPassthrough)
	updates[SettingKeyEnableCCHSigning] = strconv.FormatBool(settings.EnableCCHSigning)
	updates[SettingKeyCodexImageGenerationBridgeEnabled] = strconv.FormatBool(settings.CodexImageGenerationBridgeEnabled)

	// Balance low notification
	updates[SettingKeyBalanceLowNotifyEnabled] = strconv.FormatBool(settings.BalanceLowNotifyEnabled)
	updates[SettingKeyBalanceLowNotifyThreshold] = strconv.FormatFloat(settings.BalanceLowNotifyThreshold, 'f', 8, 64)
	updates[SettingKeyBalanceLowNotifyRechargeURL] = settings.BalanceLowNotifyRechargeURL
	updates[SettingKeyAccountQuotaNotifyEnabled] = strconv.FormatBool(settings.AccountQuotaNotifyEnabled)
	updates[SettingKeyAccountQuotaNotifyEmails] = MarshalNotifyEmails(settings.AccountQuotaNotifyEmails)

	// Codex CLI User-Agent 配置
	updates[SettingKeyCodexCLIUserAgent] = settings.CodexCLIUserAgent
	updates[SettingKeyCodexCLIVersion] = settings.CodexCLIVersion
	updates[SettingKeyCodexPassthroughUAVersion] = strconv.FormatBool(settings.CodexPassthroughUAVersion)
	updates[SettingKeyOpenAIUsageDebugLogEnabled] = strconv.FormatBool(settings.OpenAIUsageDebugLogEnabled)
	updates[SettingKeySuccessfulRequestRecordsEnabled] = strconv.FormatBool(settings.SuccessfulRequestRecordsEnabled)
	updates[SettingKeySuccessfulRequestRecordsMaxBodyBytes] = strconv.FormatInt(settings.SuccessfulRequestRecordsMaxBodyBytes, 10)

	err = s.settingRepo.SetMultiple(ctx, updates)
	if err == nil {
		// 先使 inflight singleflight 失效，再刷新缓存，缩小旧值覆盖新值的竞态窗口
		versionBoundsSF.Forget("version_bounds")
		versionBoundsCache.Store(&cachedVersionBounds{
			min:       settings.MinClaudeCodeVersion,
			max:       settings.MaxClaudeCodeVersion,
			expiresAt: time.Now().Add(versionBoundsCacheTTL).UnixNano(),
		})
		backendModeSF.Forget("backend_mode")
		backendModeCache.Store(&cachedBackendMode{
			value:     settings.BackendModeEnabled,
			expiresAt: time.Now().Add(backendModeCacheTTL).UnixNano(),
		})
		gatewayForwardingSF.Forget("gateway_forwarding")
		gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
			fingerprintUnification: settings.EnableFingerprintUnification,
			metadataPassthrough:    settings.EnableMetadataPassthrough,
			cchSigning:             settings.EnableCCHSigning,
			expiresAt:              time.Now().Add(gatewayForwardingCacheTTL).UnixNano(),
		})
		codexCLICfgSF.Forget("codex_cli_cfg")
		codexCLICfgCache.Store(&cachedCodexCLIConfig{
			userAgent:            settings.CodexCLIUserAgent,
			version:              settings.CodexCLIVersion,
			passthroughUAVersion: settings.CodexPassthroughUAVersion,
			usageDebugLogEnabled: settings.OpenAIUsageDebugLogEnabled,
			expiresAt:            time.Now().Add(codexCLICfgCacheTTL).UnixNano(),
		})
		refreshSystemPromptSettingsCache(settings)
		s.onUpdateCallbacksMu.RLock()
		callbacks := append([]func(){}, s.onUpdateCallbacks...)
		s.onUpdateCallbacksMu.RUnlock()
		for _, callback := range callbacks {
			callback() // Invalidate caches / refresh runtime state after settings update
		}
	}
	return err
}

func (s *SettingService) validateDefaultSubscriptionGroups(ctx context.Context, items []DefaultSubscriptionSetting) error {
	if len(items) == 0 {
		return nil
	}

	checked := make(map[int64]struct{}, len(items))
	for _, item := range items {
		if item.GroupID <= 0 {
			continue
		}
		if _, ok := checked[item.GroupID]; ok {
			return ErrDefaultSubGroupDuplicate.WithMetadata(map[string]string{
				"group_id": strconv.FormatInt(item.GroupID, 10),
			})
		}
		checked[item.GroupID] = struct{}{}
		if s.defaultSubGroupReader == nil {
			continue
		}

		group, err := s.defaultSubGroupReader.GetByID(ctx, item.GroupID)
		if err != nil {
			if errors.Is(err, ErrGroupNotFound) {
				return ErrDefaultSubGroupInvalid.WithMetadata(map[string]string{
					"group_id": strconv.FormatInt(item.GroupID, 10),
				})
			}
			return fmt.Errorf("get default subscription group %d: %w", item.GroupID, err)
		}
		if !group.IsSubscriptionType() {
			return ErrDefaultSubGroupInvalid.WithMetadata(map[string]string{
				"group_id": strconv.FormatInt(item.GroupID, 10),
			})
		}
	}

	return nil
}

func normalizeSystemPromptSystemSettings(settings *SystemSettings) error {
	var err error
	if settings.SystemPromptAnthropic, settings.SystemPromptModeAnthropic, err = NormalizeSystemPromptConfig(settings.SystemPromptAnthropic, settings.SystemPromptModeAnthropic); err != nil {
		return err
	}
	if settings.SystemPromptOpenAI, settings.SystemPromptModeOpenAI, err = NormalizeSystemPromptConfig(settings.SystemPromptOpenAI, settings.SystemPromptModeOpenAI); err != nil {
		return err
	}
	if settings.SystemPromptGemini, settings.SystemPromptModeGemini, err = NormalizeSystemPromptConfig(settings.SystemPromptGemini, settings.SystemPromptModeGemini); err != nil {
		return err
	}
	if settings.SystemPromptAntigravity, settings.SystemPromptModeAntigravity, err = NormalizeSystemPromptConfig(settings.SystemPromptAntigravity, settings.SystemPromptModeAntigravity); err != nil {
		return err
	}
	return nil
}

func normalizeSystemPromptUserScopeMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case SystemPromptUserScopeWhitelist, SystemPromptUserScopeBlacklist:
		return strings.TrimSpace(mode)
	default:
		return SystemPromptUserScopeAll
	}
}

func normalizeSystemPromptUserScopeUserIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func parseSystemPromptUserScopeUserIDs(raw string) []int64 {
	var ids []int64
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &ids); err != nil {
		return []int64{}
	}
	return normalizeSystemPromptUserScopeUserIDs(ids)
}

func marshalSystemPromptUserScopeUserIDs(ids []int64) (string, error) {
	normalized := normalizeSystemPromptUserScopeUserIDs(ids)
	b, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// IsRegistrationEnabled 检查是否开放注册
func (s *SettingService) IsRegistrationEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyRegistrationEnabled)
	if err != nil {
		// 安全默认：如果设置不存在或查询出错，默认关闭注册
		return false
	}
	return value == "true"
}

// IsBackendModeEnabled checks if backend mode is enabled
// Uses in-process atomic.Value cache with 60s TTL, zero-lock hot path
func (s *SettingService) IsBackendModeEnabled(ctx context.Context) bool {
	if cached, ok := backendModeCache.Load().(*cachedBackendMode); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.value
		}
	}
	result, _, _ := backendModeSF.Do("backend_mode", func() (any, error) {
		if cached, ok := backendModeCache.Load().(*cachedBackendMode); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.value, nil
			}
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backendModeDBTimeout)
		defer cancel()
		value, err := s.settingRepo.GetValue(dbCtx, SettingKeyBackendModeEnabled)
		if err != nil {
			if errors.Is(err, ErrSettingNotFound) {
				// Setting not yet created (fresh install) - default to disabled with full TTL
				backendModeCache.Store(&cachedBackendMode{
					value:     false,
					expiresAt: time.Now().Add(backendModeCacheTTL).UnixNano(),
				})
				return false, nil
			}
			slog.Warn("failed to get backend_mode_enabled setting", "error", err)
			backendModeCache.Store(&cachedBackendMode{
				value:     false,
				expiresAt: time.Now().Add(backendModeErrorTTL).UnixNano(),
			})
			return false, nil
		}
		enabled := value == "true"
		backendModeCache.Store(&cachedBackendMode{
			value:     enabled,
			expiresAt: time.Now().Add(backendModeCacheTTL).UnixNano(),
		})
		return enabled, nil
	})
	if val, ok := result.(bool); ok {
		return val
	}
	return false
}

// GetGatewayForwardingSettings returns cached gateway forwarding settings.
// Uses in-process atomic.Value cache with 60s TTL, zero-lock hot path.
// Returns (fingerprintUnification, metadataPassthrough, cchSigning).
func (s *SettingService) GetGatewayForwardingSettings(ctx context.Context) (fingerprintUnification, metadataPassthrough, cchSigning bool) {
	if cached, ok := gatewayForwardingCache.Load().(*cachedGatewayForwardingSettings); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.fingerprintUnification, cached.metadataPassthrough, cached.cchSigning
		}
	}
	type gwfResult struct {
		fp, mp, cch bool
	}
	val, _, _ := gatewayForwardingSF.Do("gateway_forwarding", func() (any, error) {
		if cached, ok := gatewayForwardingCache.Load().(*cachedGatewayForwardingSettings); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return gwfResult{cached.fingerprintUnification, cached.metadataPassthrough, cached.cchSigning}, nil
			}
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), gatewayForwardingDBTimeout)
		defer cancel()
		values, err := s.settingRepo.GetMultiple(dbCtx, []string{
			SettingKeyEnableFingerprintUnification,
			SettingKeyEnableMetadataPassthrough,
			SettingKeyEnableCCHSigning,
		})
		if err != nil {
			slog.Warn("failed to get gateway forwarding settings", "error", err)
			gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
				fingerprintUnification: true,
				metadataPassthrough:    false,
				cchSigning:             false,
				expiresAt:              time.Now().Add(gatewayForwardingErrorTTL).UnixNano(),
			})
			return gwfResult{true, false, false}, nil
		}
		fp := true
		if v, ok := values[SettingKeyEnableFingerprintUnification]; ok && v != "" {
			fp = v == "true"
		}
		mp := values[SettingKeyEnableMetadataPassthrough] == "true"
		cch := values[SettingKeyEnableCCHSigning] == "true"
		gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
			fingerprintUnification: fp,
			metadataPassthrough:    mp,
			cchSigning:             cch,
			expiresAt:              time.Now().Add(gatewayForwardingCacheTTL).UnixNano(),
		})
		return gwfResult{fp, mp, cch}, nil
	})
	if r, ok := val.(gwfResult); ok {
		return r.fp, r.mp, r.cch
	}
	return true, false, false // fail-open defaults
}

// GetCodexCLIConfig returns cached Codex CLI UA configuration.
// Uses in-process atomic.Value cache with 7-day TTL (refreshed by UpdateSettings).
// Returns (userAgent, version, passthroughUAVersion).
func (s *SettingService) GetCodexCLIConfig(ctx context.Context) (userAgent, version string, passthroughUAVersion bool) {
	if cached, ok := codexCLICfgCache.Load().(*cachedCodexCLIConfig); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.userAgent, cached.version, cached.passthroughUAVersion
		}
	}
	type cliResult struct {
		ua, ver     string
		passthrough bool
		debug       bool
	}
	val, _, _ := codexCLICfgSF.Do("codex_cli_cfg", func() (any, error) {
		if cached, ok := codexCLICfgCache.Load().(*cachedCodexCLIConfig); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cliResult{cached.userAgent, cached.version, cached.passthroughUAVersion, cached.usageDebugLogEnabled}, nil
			}
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		values, err := s.settingRepo.GetMultiple(dbCtx, []string{
			SettingKeyCodexCLIUserAgent,
			SettingKeyCodexCLIVersion,
			SettingKeyCodexPassthroughUAVersion,
			SettingKeyOpenAIUsageDebugLogEnabled,
		})
		if err != nil {
			slog.Warn("failed to get codex CLI config", "error", err)
			codexCLICfgCache.Store(&cachedCodexCLIConfig{
				passthroughUAVersion: true,
				expiresAt:            time.Now().Add(5 * time.Second).UnixNano(),
			})
			return cliResult{passthrough: true}, nil
		}
		ua := strings.TrimSpace(values[SettingKeyCodexCLIUserAgent])
		ver := strings.TrimSpace(values[SettingKeyCodexCLIVersion])
		passthrough := true
		if raw := strings.TrimSpace(values[SettingKeyCodexPassthroughUAVersion]); raw != "" {
			passthrough = raw == "true"
		}
		usageDebug := false
		if raw := strings.TrimSpace(values[SettingKeyOpenAIUsageDebugLogEnabled]); raw != "" {
			usageDebug = raw == "true"
		}
		codexCLICfgCache.Store(&cachedCodexCLIConfig{
			userAgent:            ua,
			version:              ver,
			passthroughUAVersion: passthrough,
			usageDebugLogEnabled: usageDebug,
			expiresAt:            time.Now().Add(codexCLICfgCacheTTL).UnixNano(),
		})
		return cliResult{ua, ver, passthrough, usageDebug}, nil
	})
	if r, ok := val.(cliResult); ok {
		return r.ua, r.ver, r.passthrough
	}
	return "", "", false
}

func (s *SettingService) IsCodexImageGenerationBridgeEnabled(ctx context.Context) bool {
	fallback := false
	if s != nil && s.cfg != nil {
		fallback = s.cfg.Gateway.CodexImageGenerationBridgeEnabled
	}
	if s == nil || s.settingRepo == nil {
		return fallback
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyCodexImageGenerationBridgeEnabled)
	if err != nil {
		return fallback
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

// GetSystemPromptSettings returns platform-level system prompt settings and user scope.
// It uses a 7-day in-process cache; UpdateSettings refreshes it immediately.
func (s *SettingService) GetSystemPromptSettings(ctx context.Context) SystemPromptRuntimeSettings {
	if cached, ok := systemPromptSettingsCache.Load().(*cachedSystemPromptSettings); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cloneSystemPromptSettings(cached.values)
		}
	}
	val, _, _ := systemPromptSettingsSF.Do("system_prompt_settings", func() (any, error) {
		if cached, ok := systemPromptSettingsCache.Load().(*cachedSystemPromptSettings); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cloneSystemPromptSettings(cached.values), nil
			}
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), systemPromptSettingsDBTimeout)
		defer cancel()
		values, err := s.settingRepo.GetMultiple(dbCtx, systemPromptSettingKeys)
		if err != nil {
			slog.Warn("failed to get system prompt settings", "error", err)
			fallback := defaultSystemPromptSettings()
			systemPromptSettingsCache.Store(&cachedSystemPromptSettings{
				values:    fallback,
				expiresAt: time.Now().Add(systemPromptSettingsErrorTTL).UnixNano(),
			})
			return cloneSystemPromptSettings(fallback), nil
		}
		settings := buildSystemPromptSettings(values)
		systemPromptSettingsCache.Store(&cachedSystemPromptSettings{
			values:    settings,
			expiresAt: time.Now().Add(systemPromptSettingsCacheTTL).UnixNano(),
		})
		return cloneSystemPromptSettings(settings), nil
	})
	if out, ok := val.(SystemPromptRuntimeSettings); ok {
		return out
	}
	return defaultSystemPromptSettings()
}

func (s *SettingService) CanUserConfigureSystemPrompt(ctx context.Context, userID int64) bool {
	if s == nil {
		return true
	}
	settings := s.GetSystemPromptSettings(ctx)
	return IsSystemPromptAllowedForUserID(userID, settings.UserScope)
}

func refreshSystemPromptSettingsCache(settings *SystemSettings) {
	if settings == nil {
		return
	}
	values := map[string]string{
		SettingKeySystemPromptAnthropic:        settings.SystemPromptAnthropic,
		SettingKeySystemPromptModeAnthropic:    settings.SystemPromptModeAnthropic,
		SettingKeySystemPromptOpenAI:           settings.SystemPromptOpenAI,
		SettingKeySystemPromptModeOpenAI:       settings.SystemPromptModeOpenAI,
		SettingKeySystemPromptGemini:           settings.SystemPromptGemini,
		SettingKeySystemPromptModeGemini:       settings.SystemPromptModeGemini,
		SettingKeySystemPromptAntigravity:      settings.SystemPromptAntigravity,
		SettingKeySystemPromptModeAntigravity:  settings.SystemPromptModeAntigravity,
		SettingKeySystemPromptUserScopeEnabled: strconv.FormatBool(settings.SystemPromptUserScopeEnabled),
		SettingKeySystemPromptUserScopeMode:    settings.SystemPromptUserScopeMode,
	}
	userIDsJSON, err := marshalSystemPromptUserScopeUserIDs(settings.SystemPromptUserScopeUserIDs)
	if err != nil {
		userIDsJSON = "[]"
	}
	values[SettingKeySystemPromptUserScopeUserIDs] = userIDsJSON
	systemPromptSettingsSF.Forget("system_prompt_settings")
	systemPromptSettingsCache.Store(&cachedSystemPromptSettings{
		values:    buildSystemPromptSettings(values),
		expiresAt: time.Now().Add(systemPromptSettingsCacheTTL).UnixNano(),
	})
}

func buildSystemPromptSettings(values map[string]string) SystemPromptRuntimeSettings {
	out := defaultSystemPromptSettings()
	apply := func(platform, promptKey, modeKey string) {
		prompt, mode := normalizeStoredSystemPromptConfig(values[promptKey], values[modeKey])
		out.Prompts[platform] = EffectiveSystemPrompt{
			Prompt: prompt,
			Mode:   mode,
			Source: SystemPromptSourceSystem,
		}
	}
	apply(PlatformAnthropic, SettingKeySystemPromptAnthropic, SettingKeySystemPromptModeAnthropic)
	apply(PlatformOpenAI, SettingKeySystemPromptOpenAI, SettingKeySystemPromptModeOpenAI)
	apply(PlatformGemini, SettingKeySystemPromptGemini, SettingKeySystemPromptModeGemini)
	apply(PlatformAntigravity, SettingKeySystemPromptAntigravity, SettingKeySystemPromptModeAntigravity)
	out.UserScope = SystemPromptUserScope{
		Enabled: values[SettingKeySystemPromptUserScopeEnabled] == "true",
		Mode:    normalizeSystemPromptUserScopeMode(values[SettingKeySystemPromptUserScopeMode]),
		UserIDs: parseSystemPromptUserScopeUserIDs(values[SettingKeySystemPromptUserScopeUserIDs]),
	}
	return out
}

func normalizeStoredSystemPromptConfig(prompt, mode string) (string, string) {
	normalizedPrompt, normalizedMode, err := NormalizeSystemPromptConfig(prompt, mode)
	if err != nil {
		return "", SystemPromptModeInherit
	}
	return normalizedPrompt, normalizedMode
}

func defaultSystemPromptSettings() SystemPromptRuntimeSettings {
	return SystemPromptRuntimeSettings{
		Prompts: map[string]EffectiveSystemPrompt{
			PlatformAnthropic:   {Mode: SystemPromptModeInherit, Source: SystemPromptSourceSystem},
			PlatformOpenAI:      {Mode: SystemPromptModeInherit, Source: SystemPromptSourceSystem},
			PlatformGemini:      {Mode: SystemPromptModeInherit, Source: SystemPromptSourceSystem},
			PlatformAntigravity: {Mode: SystemPromptModeInherit, Source: SystemPromptSourceSystem},
		},
		UserScope: SystemPromptUserScope{
			Enabled: false,
			Mode:    SystemPromptUserScopeAll,
			UserIDs: []int64{},
		},
	}
}

func cloneSystemPromptSettings(in SystemPromptRuntimeSettings) SystemPromptRuntimeSettings {
	prompts := make(map[string]EffectiveSystemPrompt, len(in.Prompts))
	for key, value := range in.Prompts {
		prompts[key] = value
	}
	return SystemPromptRuntimeSettings{
		Prompts: prompts,
		UserScope: SystemPromptUserScope{
			Enabled: in.UserScope.Enabled,
			Mode:    in.UserScope.Mode,
			UserIDs: append([]int64(nil), in.UserScope.UserIDs...),
		},
	}
}

// IsEmailVerifyEnabled 检查是否开启邮件验证
func (s *SettingService) IsEmailVerifyEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyEmailVerifyEnabled)
	if err != nil {
		return false
	}
	return value == "true"
}

// GetRegistrationEmailSuffixWhitelist returns normalized registration email suffix whitelist.
func (s *SettingService) GetRegistrationEmailSuffixWhitelist(ctx context.Context) []string {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyRegistrationEmailSuffixWhitelist)
	if err != nil {
		return []string{}
	}
	return ParseRegistrationEmailSuffixWhitelist(value)
}

// IsPromoCodeEnabled 检查是否启用优惠码功能
func (s *SettingService) IsPromoCodeEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyPromoCodeEnabled)
	if err != nil {
		return true // 默认启用
	}
	return value != "false"
}

// IsInvitationCodeEnabled 检查是否启用邀请码注册功能
func (s *SettingService) IsInvitationCodeEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyInvitationCodeEnabled)
	if err != nil {
		return false // 默认关闭
	}
	return value == "true"
}

// IsPasswordResetEnabled 检查是否启用密码重置功能
// 要求：必须同时开启邮件验证
func (s *SettingService) IsPasswordResetEnabled(ctx context.Context) bool {
	// Password reset requires email verification to be enabled
	if !s.IsEmailVerifyEnabled(ctx) {
		return false
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyPasswordResetEnabled)
	if err != nil {
		return false // 默认关闭
	}
	return value == "true"
}

// IsTotpEnabled 检查是否启用 TOTP 双因素认证功能
func (s *SettingService) IsTotpEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyTotpEnabled)
	if err != nil {
		return false // 默认关闭
	}
	return value == "true"
}

// IsTotpEncryptionKeyConfigured 检查 TOTP 加密密钥是否已手动配置
// 只有手动配置了密钥才允许在管理后台启用 TOTP 功能
func (s *SettingService) IsTotpEncryptionKeyConfigured() bool {
	return s != nil && s.cfg != nil && s.cfg.Totp.EncryptionKeyConfigured
}

// GetSiteName 获取网站名称
func (s *SettingService) GetSiteName(ctx context.Context) string {
	value, err := s.settingRepo.GetValue(ctx, SettingKeySiteName)
	if err != nil || value == "" {
		return "FluxCode"
	}
	return value
}

// GetDefaultConcurrency 获取默认并发量
func (s *SettingService) GetDefaultConcurrency(ctx context.Context) int {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyDefaultConcurrency)
	if err != nil {
		return s.cfg.Default.UserConcurrency
	}
	if v, err := strconv.Atoi(value); err == nil && v > 0 {
		return v
	}
	return s.cfg.Default.UserConcurrency
}

// GetDefaultBalance 获取默认余额
func (s *SettingService) GetDefaultBalance(ctx context.Context) float64 {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyDefaultBalance)
	if err != nil {
		return s.cfg.Default.UserBalance
	}
	if v, err := strconv.ParseFloat(value, 64); err == nil && v >= 0 {
		return v
	}
	return s.cfg.Default.UserBalance
}

// GetDefaultSubscriptions 获取新用户默认订阅配置列表。
func (s *SettingService) GetDefaultSubscriptions(ctx context.Context) []DefaultSubscriptionSetting {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyDefaultSubscriptions)
	if err != nil {
		return nil
	}
	return parseDefaultSubscriptions(value)
}

// InitializeDefaultSettings 初始化默认设置
func (s *SettingService) InitializeDefaultSettings(ctx context.Context) error {
	// 检查是否已有设置
	_, err := s.settingRepo.GetValue(ctx, SettingKeyRegistrationEnabled)
	if err == nil {
		// 已有设置，不需要初始化
		return nil
	}
	if !errors.Is(err, ErrSettingNotFound) {
		return fmt.Errorf("check existing settings: %w", err)
	}

	// 初始化默认设置
	defaults := map[string]string{
		SettingKeyRegistrationEnabled:                  "true",
		SettingKeyEmailVerifyEnabled:                   "false",
		SettingKeyRegistrationEmailSuffixWhitelist:     "[]",
		SettingKeyPromoCodeEnabled:                     "true", // 默认启用优惠码功能
		SettingKeySiteName:                             "FluxCode",
		SettingKeySiteLogo:                             "",
		SettingKeyPurchaseSubscriptionEnabled:          "false",
		SettingKeyPurchaseSubscriptionURL:              "",
		SettingKeyTableDefaultPageSize:                 "20",
		SettingKeyTablePageSizeOptions:                 "[10,20,50,100]",
		SettingKeyCustomMenuItems:                      "[]",
		SettingKeyCustomEndpoints:                      "[]",
		SettingKeyOpenAIUseKeyModelID:                  "gpt-5.5",
		SettingKeyOpenAIImageURLCacheTTLHours:          strconv.Itoa(DefaultOpenAIImageURLCacheTTLHours),
		SettingKeyGeneratedImageStorageSource:          GeneratedImageStorageSourceDB,
		SettingKeyGeneratedImageStorageConfigSource:    GeneratedImageStorageSourceDB,
		SettingKeyQiniuAccessKey:                       "",
		SettingKeyQiniuSecretKey:                       "",
		SettingKeyQiniuBucket:                          "",
		SettingKeyQiniuCDNDomain:                       "",
		SettingKeyQiniuPrefix:                          DefaultQiniuPrefix,
		SettingKeyQiniuUseHTTPS:                        "true",
		SettingKeyQiniuUploadTimeoutSeconds:            strconv.Itoa(DefaultQiniuUploadTimeoutSeconds),
		SettingKeyQiniuTokenTTLSeconds:                 strconv.Itoa(DefaultQiniuTokenTTLSeconds),
		SettingKeyGeneratedImageCleanupEnabled:         "false",
		SettingKeyOIDCConnectEnabled:                   "false",
		SettingKeyOIDCConnectProviderName:              "OIDC",
		SettingKeyDefaultConcurrency:                   strconv.Itoa(s.cfg.Default.UserConcurrency),
		SettingKeyDefaultBalance:                       strconv.FormatFloat(s.cfg.Default.UserBalance, 'f', 8, 64),
		SettingKeyDefaultSubscriptions:                 "[]",
		SettingKeyEmailProvider:                        EmailProviderSMTP,
		SettingKeySMTPPort:                             "587",
		SettingKeySMTPUseTLS:                           "false",
		SettingKeyChannelMonitorEnabled:                "false",
		SettingKeyChannelMonitorDefaultIntervalSeconds: strconv.Itoa(ChannelMonitorFallbackIntervalSecond),
		// Model fallback defaults
		SettingKeyEnableModelFallback:      "false",
		SettingKeyFallbackModelAnthropic:   "claude-3-5-sonnet-20241022",
		SettingKeyFallbackModelOpenAI:      "gpt-4o",
		SettingKeyFallbackModelGemini:      "gemini-2.5-pro",
		SettingKeyFallbackModelAntigravity: "gemini-2.5-pro",
		// Identity patch defaults
		SettingKeyEnableIdentityPatch:          "true",
		SettingKeyIdentityPatchPrompt:          "",
		SettingKeySystemPromptAnthropic:        "",
		SettingKeySystemPromptModeAnthropic:    SystemPromptModeInherit,
		SettingKeySystemPromptOpenAI:           "",
		SettingKeySystemPromptModeOpenAI:       SystemPromptModeInherit,
		SettingKeySystemPromptGemini:           "",
		SettingKeySystemPromptModeGemini:       SystemPromptModeInherit,
		SettingKeySystemPromptAntigravity:      "",
		SettingKeySystemPromptModeAntigravity:  SystemPromptModeInherit,
		SettingKeySystemPromptUserScopeEnabled: "false",
		SettingKeySystemPromptUserScopeMode:    SystemPromptUserScopeAll,
		SettingKeySystemPromptUserScopeUserIDs: "[]",

		// Ops monitoring defaults (vNext)
		SettingKeyOpsMonitoringEnabled:         "true",
		SettingKeyOpsRealtimeMonitoringEnabled: "true",
		SettingKeyOpsQueryModeDefault:          "auto",
		SettingKeyOpsMetricsIntervalSeconds:    "60",

		// Claude Code version check (default: empty = disabled)
		SettingKeyMinClaudeCodeVersion: "",
		SettingKeyMaxClaudeCodeVersion: "",

		// 分组隔离（默认不允许未分组 Key 调度）
		SettingKeyAllowUngroupedKeyScheduling: "false",

		// Dashboard fireworks
		SettingKeyDashboardFireworksEnabled:            "true",
		SettingKeyDashboardFireworksThreshold:          strconv.FormatFloat(DefaultDashboardFireworksThreshold, 'f', -1, 64),
		SettingKeySuccessfulRequestRecordsEnabled:      "false",
		SettingKeySuccessfulRequestRecordsMaxBodyBytes: strconv.FormatInt(DefaultSuccessfulRequestRecordsMaxBodyBytes, 10),
	}

	return s.settingRepo.SetMultiple(ctx, defaults)
}

// parseSettings 解析设置到结构体
func (s *SettingService) parseSettings(settings map[string]string) *SystemSettings {
	emailVerifyEnabled := settings[SettingKeyEmailVerifyEnabled] == "true"
	generatedImageStorageSettings := parseGeneratedImageStorageSettings(settings)
	result := &SystemSettings{
		RegistrationEnabled:              settings[SettingKeyRegistrationEnabled] == "true",
		EmailVerifyEnabled:               emailVerifyEnabled,
		RegistrationEmailSuffixWhitelist: ParseRegistrationEmailSuffixWhitelist(settings[SettingKeyRegistrationEmailSuffixWhitelist]),
		PromoCodeEnabled:                 settings[SettingKeyPromoCodeEnabled] != "false", // 默认启用
		PasswordResetEnabled:             emailVerifyEnabled && settings[SettingKeyPasswordResetEnabled] == "true",
		FrontendURL:                      settings[SettingKeyFrontendURL],
		InvitationCodeEnabled:            settings[SettingKeyInvitationCodeEnabled] == "true",
		TotpEnabled:                      settings[SettingKeyTotpEnabled] == "true",
		ChannelMonitorEnabled:            settings[SettingKeyChannelMonitorEnabled] == "true",
		ChannelMonitorDefaultIntervalSeconds: NormalizeChannelMonitorInterval(
			parseIntSetting(settings[SettingKeyChannelMonitorDefaultIntervalSeconds], ChannelMonitorFallbackIntervalSecond),
			ChannelMonitorFallbackIntervalSecond,
		),
		EmailProvider:                     NormalizeEmailProvider(settings[SettingKeyEmailProvider]),
		SMTPHost:                          settings[SettingKeySMTPHost],
		SMTPUsername:                      settings[SettingKeySMTPUsername],
		SMTPFrom:                          settings[SettingKeySMTPFrom],
		SMTPFromName:                      settings[SettingKeySMTPFromName],
		SMTPUseTLS:                        settings[SettingKeySMTPUseTLS] == "true",
		SMTPPasswordConfigured:            settings[SettingKeySMTPPassword] != "",
		ResendFrom:                        settings[SettingKeyResendFrom],
		ResendFromName:                    settings[SettingKeyResendFromName],
		ResendAPIKeyConfigured:            settings[SettingKeyResendAPIKey] != "",
		TurnstileEnabled:                  settings[SettingKeyTurnstileEnabled] == "true",
		TurnstileSiteKey:                  settings[SettingKeyTurnstileSiteKey],
		TurnstileSecretKeyConfigured:      settings[SettingKeyTurnstileSecretKey] != "",
		SiteName:                          s.getStringOrDefault(settings, SettingKeySiteName, "FluxCode"),
		SiteLogo:                          settings[SettingKeySiteLogo],
		SiteSubtitle:                      s.getStringOrDefault(settings, SettingKeySiteSubtitle, "Subscription to API Conversion Platform"),
		APIBaseURL:                        settings[SettingKeyAPIBaseURL],
		ContactInfo:                       settings[SettingKeyContactInfo],
		DocURL:                            settings[SettingKeyDocURL],
		HomeContent:                       settings[SettingKeyHomeContent],
		HideCcsImportButton:               settings[SettingKeyHideCcsImportButton] == "true",
		PurchaseSubscriptionEnabled:       settings[SettingKeyPurchaseSubscriptionEnabled] == "true",
		PurchaseSubscriptionURL:           strings.TrimSpace(settings[SettingKeyPurchaseSubscriptionURL]),
		CustomMenuItems:                   settings[SettingKeyCustomMenuItems],
		CustomEndpoints:                   settings[SettingKeyCustomEndpoints],
		OpenAIUseKeyModelID:               s.getStringOrDefault(settings, SettingKeyOpenAIUseKeyModelID, "gpt-5.5"),
		OpenAIImageURLCacheTTLHours:       normalizeOpenAIImageURLCacheTTLHours(parseIntSetting(settings[SettingKeyOpenAIImageURLCacheTTLHours], DefaultOpenAIImageURLCacheTTLHours)),
		GeneratedImageStorageSource:       generatedImageStorageSettings.Source,
		GeneratedImageStorageConfigSource: generatedImageStorageSettings.ConfigSource,
		QiniuAccessKey:                    generatedImageStorageSettings.QiniuAccessKey,
		QiniuSecretKey:                    generatedImageStorageSettings.QiniuSecretKey,
		QiniuSecretKeyConfigured:          generatedImageStorageSettings.QiniuSecretKey != "",
		QiniuBucket:                       generatedImageStorageSettings.QiniuBucket,
		QiniuCDNDomain:                    generatedImageStorageSettings.QiniuCDNDomain,
		QiniuPrefix:                       generatedImageStorageSettings.QiniuPrefix,
		QiniuUseHTTPS:                     generatedImageStorageSettings.QiniuUseHTTPS,
		QiniuUploadTimeoutSeconds:         generatedImageStorageSettings.QiniuUploadTimeoutSeconds,
		QiniuTokenTTLSeconds:              generatedImageStorageSettings.QiniuTokenTTLSeconds,
		GeneratedImageCleanupEnabled:      settings[SettingKeyGeneratedImageCleanupEnabled] == "true",
		BackendModeEnabled:                settings[SettingKeyBackendModeEnabled] == "true",
	}
	result.TableDefaultPageSize, result.TablePageSizeOptions = parseTablePreferences(
		settings[SettingKeyTableDefaultPageSize],
		settings[SettingKeyTablePageSizeOptions],
	)

	// 解析整数类型
	if port, err := strconv.Atoi(settings[SettingKeySMTPPort]); err == nil {
		result.SMTPPort = port
	} else {
		result.SMTPPort = 587
	}

	if concurrency, err := strconv.Atoi(settings[SettingKeyDefaultConcurrency]); err == nil {
		result.DefaultConcurrency = concurrency
	} else {
		result.DefaultConcurrency = s.cfg.Default.UserConcurrency
	}

	// 解析浮点数类型
	if balance, err := strconv.ParseFloat(settings[SettingKeyDefaultBalance], 64); err == nil {
		result.DefaultBalance = balance
	} else {
		result.DefaultBalance = s.cfg.Default.UserBalance
	}
	result.DefaultSubscriptions = parseDefaultSubscriptions(settings[SettingKeyDefaultSubscriptions])

	// 敏感信息直接返回，方便测试连接时使用
	result.SMTPPassword = settings[SettingKeySMTPPassword]
	result.ResendAPIKey = strings.TrimSpace(settings[SettingKeyResendAPIKey])
	result.TurnstileSecretKey = settings[SettingKeyTurnstileSecretKey]

	// LinuxDo Connect 设置：
	// - 兼容 config.yaml/env（避免老部署因为未迁移到数据库设置而被意外关闭）
	// - 支持在后台“系统设置”中覆盖并持久化（存储于 DB）
	linuxDoBase := config.LinuxDoConnectConfig{}
	if s.cfg != nil {
		linuxDoBase = s.cfg.LinuxDo
	}

	if raw, ok := settings[SettingKeyLinuxDoConnectEnabled]; ok {
		result.LinuxDoConnectEnabled = raw == "true"
	} else {
		result.LinuxDoConnectEnabled = linuxDoBase.Enabled
	}

	if v, ok := settings[SettingKeyLinuxDoConnectClientID]; ok && strings.TrimSpace(v) != "" {
		result.LinuxDoConnectClientID = strings.TrimSpace(v)
	} else {
		result.LinuxDoConnectClientID = linuxDoBase.ClientID
	}

	if v, ok := settings[SettingKeyLinuxDoConnectRedirectURL]; ok && strings.TrimSpace(v) != "" {
		result.LinuxDoConnectRedirectURL = strings.TrimSpace(v)
	} else {
		result.LinuxDoConnectRedirectURL = linuxDoBase.RedirectURL
	}

	result.LinuxDoConnectClientSecret = strings.TrimSpace(settings[SettingKeyLinuxDoConnectClientSecret])
	if result.LinuxDoConnectClientSecret == "" {
		result.LinuxDoConnectClientSecret = strings.TrimSpace(linuxDoBase.ClientSecret)
	}
	result.LinuxDoConnectClientSecretConfigured = result.LinuxDoConnectClientSecret != ""

	// Generic OIDC 设置：
	// - 兼容 config.yaml/env
	// - 支持后台系统设置覆盖并持久化（存储于 DB）
	oidcBase := config.OIDCConnectConfig{}
	if s.cfg != nil {
		oidcBase = s.cfg.OIDC
	}

	if raw, ok := settings[SettingKeyOIDCConnectEnabled]; ok {
		result.OIDCConnectEnabled = raw == "true"
	} else {
		result.OIDCConnectEnabled = oidcBase.Enabled
	}

	if v, ok := settings[SettingKeyOIDCConnectProviderName]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectProviderName = strings.TrimSpace(v)
	} else {
		result.OIDCConnectProviderName = strings.TrimSpace(oidcBase.ProviderName)
	}
	if result.OIDCConnectProviderName == "" {
		result.OIDCConnectProviderName = "OIDC"
	}

	if v, ok := settings[SettingKeyOIDCConnectClientID]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectClientID = strings.TrimSpace(v)
	} else {
		result.OIDCConnectClientID = strings.TrimSpace(oidcBase.ClientID)
	}
	if v, ok := settings[SettingKeyOIDCConnectIssuerURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectIssuerURL = strings.TrimSpace(v)
	} else {
		result.OIDCConnectIssuerURL = strings.TrimSpace(oidcBase.IssuerURL)
	}
	if v, ok := settings[SettingKeyOIDCConnectDiscoveryURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectDiscoveryURL = strings.TrimSpace(v)
	} else {
		result.OIDCConnectDiscoveryURL = strings.TrimSpace(oidcBase.DiscoveryURL)
	}
	if v, ok := settings[SettingKeyOIDCConnectAuthorizeURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectAuthorizeURL = strings.TrimSpace(v)
	} else {
		result.OIDCConnectAuthorizeURL = strings.TrimSpace(oidcBase.AuthorizeURL)
	}
	if v, ok := settings[SettingKeyOIDCConnectTokenURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectTokenURL = strings.TrimSpace(v)
	} else {
		result.OIDCConnectTokenURL = strings.TrimSpace(oidcBase.TokenURL)
	}
	if v, ok := settings[SettingKeyOIDCConnectUserInfoURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectUserInfoURL = strings.TrimSpace(v)
	} else {
		result.OIDCConnectUserInfoURL = strings.TrimSpace(oidcBase.UserInfoURL)
	}
	if v, ok := settings[SettingKeyOIDCConnectJWKSURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectJWKSURL = strings.TrimSpace(v)
	} else {
		result.OIDCConnectJWKSURL = strings.TrimSpace(oidcBase.JWKSURL)
	}
	if v, ok := settings[SettingKeyOIDCConnectScopes]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectScopes = strings.TrimSpace(v)
	} else {
		result.OIDCConnectScopes = strings.TrimSpace(oidcBase.Scopes)
	}
	if v, ok := settings[SettingKeyOIDCConnectRedirectURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectRedirectURL = strings.TrimSpace(v)
	} else {
		result.OIDCConnectRedirectURL = strings.TrimSpace(oidcBase.RedirectURL)
	}
	if v, ok := settings[SettingKeyOIDCConnectFrontendRedirectURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectFrontendRedirectURL = strings.TrimSpace(v)
	} else {
		result.OIDCConnectFrontendRedirectURL = strings.TrimSpace(oidcBase.FrontendRedirectURL)
	}
	if v, ok := settings[SettingKeyOIDCConnectTokenAuthMethod]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectTokenAuthMethod = strings.ToLower(strings.TrimSpace(v))
	} else {
		result.OIDCConnectTokenAuthMethod = strings.ToLower(strings.TrimSpace(oidcBase.TokenAuthMethod))
	}
	if raw, ok := settings[SettingKeyOIDCConnectUsePKCE]; ok {
		result.OIDCConnectUsePKCE = raw == "true"
	} else {
		result.OIDCConnectUsePKCE = oidcBase.UsePKCE
	}
	if raw, ok := settings[SettingKeyOIDCConnectValidateIDToken]; ok {
		result.OIDCConnectValidateIDToken = raw == "true"
	} else {
		result.OIDCConnectValidateIDToken = oidcBase.ValidateIDToken
	}
	if v, ok := settings[SettingKeyOIDCConnectAllowedSigningAlgs]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectAllowedSigningAlgs = strings.TrimSpace(v)
	} else {
		result.OIDCConnectAllowedSigningAlgs = strings.TrimSpace(oidcBase.AllowedSigningAlgs)
	}
	clockSkewSet := false
	if raw, ok := settings[SettingKeyOIDCConnectClockSkewSeconds]; ok && strings.TrimSpace(raw) != "" {
		if parsed, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
			result.OIDCConnectClockSkewSeconds = parsed
			clockSkewSet = true
		}
	}
	if !clockSkewSet {
		result.OIDCConnectClockSkewSeconds = oidcBase.ClockSkewSeconds
	}
	if !clockSkewSet && result.OIDCConnectClockSkewSeconds == 0 {
		result.OIDCConnectClockSkewSeconds = 120
	}
	if raw, ok := settings[SettingKeyOIDCConnectRequireEmailVerified]; ok {
		result.OIDCConnectRequireEmailVerified = raw == "true"
	} else {
		result.OIDCConnectRequireEmailVerified = oidcBase.RequireEmailVerified
	}
	if v, ok := settings[SettingKeyOIDCConnectUserInfoEmailPath]; ok {
		result.OIDCConnectUserInfoEmailPath = strings.TrimSpace(v)
	} else {
		result.OIDCConnectUserInfoEmailPath = strings.TrimSpace(oidcBase.UserInfoEmailPath)
	}
	if v, ok := settings[SettingKeyOIDCConnectUserInfoIDPath]; ok {
		result.OIDCConnectUserInfoIDPath = strings.TrimSpace(v)
	} else {
		result.OIDCConnectUserInfoIDPath = strings.TrimSpace(oidcBase.UserInfoIDPath)
	}
	if v, ok := settings[SettingKeyOIDCConnectUserInfoUsernamePath]; ok {
		result.OIDCConnectUserInfoUsernamePath = strings.TrimSpace(v)
	} else {
		result.OIDCConnectUserInfoUsernamePath = strings.TrimSpace(oidcBase.UserInfoUsernamePath)
	}
	result.OIDCConnectClientSecret = strings.TrimSpace(settings[SettingKeyOIDCConnectClientSecret])
	if result.OIDCConnectClientSecret == "" {
		result.OIDCConnectClientSecret = strings.TrimSpace(oidcBase.ClientSecret)
	}
	result.OIDCConnectClientSecretConfigured = result.OIDCConnectClientSecret != ""

	// Model fallback settings
	result.EnableModelFallback = settings[SettingKeyEnableModelFallback] == "true"
	result.FallbackModelAnthropic = s.getStringOrDefault(settings, SettingKeyFallbackModelAnthropic, "claude-3-5-sonnet-20241022")
	result.FallbackModelOpenAI = s.getStringOrDefault(settings, SettingKeyFallbackModelOpenAI, "gpt-4o")
	result.FallbackModelGemini = s.getStringOrDefault(settings, SettingKeyFallbackModelGemini, "gemini-2.5-pro")
	result.FallbackModelAntigravity = s.getStringOrDefault(settings, SettingKeyFallbackModelAntigravity, "gemini-2.5-pro")

	// Identity patch settings (default: enabled, to preserve existing behavior)
	if v, ok := settings[SettingKeyEnableIdentityPatch]; ok && v != "" {
		result.EnableIdentityPatch = v == "true"
	} else {
		result.EnableIdentityPatch = true
	}
	result.IdentityPatchPrompt = settings[SettingKeyIdentityPatchPrompt]

	result.SystemPromptAnthropic, result.SystemPromptModeAnthropic = normalizeStoredSystemPromptConfig(
		settings[SettingKeySystemPromptAnthropic],
		settings[SettingKeySystemPromptModeAnthropic],
	)
	result.SystemPromptOpenAI, result.SystemPromptModeOpenAI = normalizeStoredSystemPromptConfig(
		settings[SettingKeySystemPromptOpenAI],
		settings[SettingKeySystemPromptModeOpenAI],
	)
	result.SystemPromptGemini, result.SystemPromptModeGemini = normalizeStoredSystemPromptConfig(
		settings[SettingKeySystemPromptGemini],
		settings[SettingKeySystemPromptModeGemini],
	)
	result.SystemPromptAntigravity, result.SystemPromptModeAntigravity = normalizeStoredSystemPromptConfig(
		settings[SettingKeySystemPromptAntigravity],
		settings[SettingKeySystemPromptModeAntigravity],
	)
	result.SystemPromptUserScopeEnabled = settings[SettingKeySystemPromptUserScopeEnabled] == "true"
	result.SystemPromptUserScopeMode = normalizeSystemPromptUserScopeMode(settings[SettingKeySystemPromptUserScopeMode])
	result.SystemPromptUserScopeUserIDs = parseSystemPromptUserScopeUserIDs(settings[SettingKeySystemPromptUserScopeUserIDs])

	// Ops monitoring settings (default: enabled, fail-open)
	result.OpsMonitoringEnabled = !isFalseSettingValue(settings[SettingKeyOpsMonitoringEnabled])
	result.OpsRealtimeMonitoringEnabled = !isFalseSettingValue(settings[SettingKeyOpsRealtimeMonitoringEnabled])
	result.OpsQueryModeDefault = string(ParseOpsQueryMode(settings[SettingKeyOpsQueryModeDefault]))
	result.OpsMetricsIntervalSeconds = 60
	if raw := strings.TrimSpace(settings[SettingKeyOpsMetricsIntervalSeconds]); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			if v < 60 {
				v = 60
			}
			if v > 3600 {
				v = 3600
			}
			result.OpsMetricsIntervalSeconds = v
		}
	}

	// Claude Code version check
	result.MinClaudeCodeVersion = settings[SettingKeyMinClaudeCodeVersion]
	result.MaxClaudeCodeVersion = settings[SettingKeyMaxClaudeCodeVersion]

	// 分组隔离
	result.AllowUngroupedKeyScheduling = settings[SettingKeyAllowUngroupedKeyScheduling] == "true"

	// 兑换码发货文案 & 引流弹窗
	result.RedeemDeliveryText = s.getStringOrDefault(settings, SettingKeyRedeemDeliveryText, "${redeemCodes}")
	result.AttractPopupTitle = settings[SettingKeyAttractPopupTitle]
	result.AttractPopupMarkdown = settings[SettingKeyAttractPopupMarkdown]
	result.DashboardFireworksEnabled = settings[SettingKeyDashboardFireworksEnabled] != "false"
	result.DashboardFireworksThreshold = parseDashboardFireworksThreshold(settings[SettingKeyDashboardFireworksThreshold])

	// Gateway forwarding behavior (defaults: fingerprint=true, metadata_passthrough=false, cch_signing=false)
	if v, ok := settings[SettingKeyEnableFingerprintUnification]; ok && v != "" {
		result.EnableFingerprintUnification = v == "true"
	} else {
		result.EnableFingerprintUnification = true // default: enabled (current behavior)
	}
	result.EnableMetadataPassthrough = settings[SettingKeyEnableMetadataPassthrough] == "true"
	result.EnableCCHSigning = settings[SettingKeyEnableCCHSigning] == "true"
	if v, ok := settings[SettingKeyCodexImageGenerationBridgeEnabled]; ok && strings.TrimSpace(v) != "" {
		if parsed, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			result.CodexImageGenerationBridgeEnabled = parsed
		} else if s.cfg != nil {
			result.CodexImageGenerationBridgeEnabled = s.cfg.Gateway.CodexImageGenerationBridgeEnabled
		}
	} else if s.cfg != nil {
		result.CodexImageGenerationBridgeEnabled = s.cfg.Gateway.CodexImageGenerationBridgeEnabled
	}

	// Codex CLI User-Agent 配置
	result.CodexCLIUserAgent = settings[SettingKeyCodexCLIUserAgent]
	result.CodexCLIVersion = settings[SettingKeyCodexCLIVersion]
	result.CodexPassthroughUAVersion = true
	if raw := strings.TrimSpace(settings[SettingKeyCodexPassthroughUAVersion]); raw != "" {
		result.CodexPassthroughUAVersion = raw == "true"
	}
	if raw := strings.TrimSpace(settings[SettingKeyOpenAIUsageDebugLogEnabled]); raw != "" {
		result.OpenAIUsageDebugLogEnabled = raw == "true"
	}
	result.SuccessfulRequestRecordsEnabled = strings.TrimSpace(settings[SettingKeySuccessfulRequestRecordsEnabled]) == "true"
	result.SuccessfulRequestRecordsMaxBodyBytes = DefaultSuccessfulRequestRecordsMaxBodyBytes
	if raw := strings.TrimSpace(settings[SettingKeySuccessfulRequestRecordsMaxBodyBytes]); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed >= MinSuccessfulRequestRecordsMaxBodyBytes && parsed <= MaxSuccessfulRequestRecordsMaxBodyBytes {
			result.SuccessfulRequestRecordsMaxBodyBytes = parsed
		}
	}

	// Web search emulation: quick enabled check from the JSON config
	if raw := settings[SettingKeyWebSearchEmulationConfig]; raw != "" {
		var wsCfg WebSearchEmulationConfig
		if err := json.Unmarshal([]byte(raw), &wsCfg); err == nil {
			result.WebSearchEmulationEnabled = wsCfg.Enabled && len(wsCfg.Providers) > 0
		}
	}

	// Balance low notification
	result.BalanceLowNotifyEnabled = settings[SettingKeyBalanceLowNotifyEnabled] == "true"
	if v, err := strconv.ParseFloat(settings[SettingKeyBalanceLowNotifyThreshold], 64); err == nil && v >= 0 {
		result.BalanceLowNotifyThreshold = v
	}
	result.BalanceLowNotifyRechargeURL = settings[SettingKeyBalanceLowNotifyRechargeURL]

	// Account quota notification
	result.AccountQuotaNotifyEnabled = settings[SettingKeyAccountQuotaNotifyEnabled] == "true"
	if raw := strings.TrimSpace(settings[SettingKeyAccountQuotaNotifyEmails]); raw != "" {
		result.AccountQuotaNotifyEmails = ParseNotifyEmails(raw)
	}
	if result.AccountQuotaNotifyEmails == nil {
		result.AccountQuotaNotifyEmails = []NotifyEmailEntry{}
	}

	return result
}

func isFalseSettingValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "false", "0", "off", "disabled":
		return true
	default:
		return false
	}
}

func parseDefaultSubscriptions(raw string) []DefaultSubscriptionSetting {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var items []DefaultSubscriptionSetting
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}

	normalized := make([]DefaultSubscriptionSetting, 0, len(items))
	for _, item := range items {
		if item.GroupID <= 0 || item.ValidityDays <= 0 {
			continue
		}
		if item.ValidityDays > MaxValidityDays {
			item.ValidityDays = MaxValidityDays
		}
		normalized = append(normalized, item)
	}

	return normalized
}

func parseTablePreferences(defaultPageSizeRaw, optionsRaw string) (int, []int) {
	defaultPageSize := 20
	if v, err := strconv.Atoi(strings.TrimSpace(defaultPageSizeRaw)); err == nil {
		defaultPageSize = v
	}

	var options []int
	if strings.TrimSpace(optionsRaw) != "" {
		_ = json.Unmarshal([]byte(optionsRaw), &options)
	}

	return normalizeTablePreferences(defaultPageSize, options)
}

func normalizeTablePreferences(defaultPageSize int, options []int) (int, []int) {
	const minPageSize = 5
	const maxPageSize = 1000
	const fallbackPageSize = 20

	seen := make(map[int]struct{}, len(options))
	normalizedOptions := make([]int, 0, len(options))
	for _, option := range options {
		if option < minPageSize || option > maxPageSize {
			continue
		}
		if _, ok := seen[option]; ok {
			continue
		}
		seen[option] = struct{}{}
		normalizedOptions = append(normalizedOptions, option)
	}
	sort.Ints(normalizedOptions)

	if defaultPageSize < minPageSize || defaultPageSize > maxPageSize {
		defaultPageSize = fallbackPageSize
	}

	if len(normalizedOptions) == 0 {
		normalizedOptions = []int{10, 20, 50}
	}

	return defaultPageSize, normalizedOptions
}

// getStringOrDefault 获取字符串值或默认值
func (s *SettingService) getStringOrDefault(settings map[string]string, key, defaultValue string) string {
	if value, ok := settings[key]; ok && value != "" {
		return value
	}
	return defaultValue
}

// IsTurnstileEnabled 检查是否启用 Turnstile 验证
func (s *SettingService) IsTurnstileEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyTurnstileEnabled)
	if err != nil {
		return false
	}
	return value == "true"
}

// GetTurnstileSecretKey 获取 Turnstile Secret Key
func (s *SettingService) GetTurnstileSecretKey(ctx context.Context) string {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyTurnstileSecretKey)
	if err != nil {
		return ""
	}
	return value
}

// IsIdentityPatchEnabled 检查是否启用身份补丁（Claude -> Gemini systemInstruction 注入）
func (s *SettingService) IsIdentityPatchEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyEnableIdentityPatch)
	if err != nil {
		// 默认开启，保持兼容
		return true
	}
	return value == "true"
}

// GetIdentityPatchPrompt 获取自定义身份补丁提示词（为空表示使用内置默认模板）
func (s *SettingService) GetIdentityPatchPrompt(ctx context.Context) string {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyIdentityPatchPrompt)
	if err != nil {
		return ""
	}
	return value
}

// GenerateAdminAPIKey 生成新的管理员 API Key
func (s *SettingService) GenerateAdminAPIKey(ctx context.Context) (string, error) {
	// 生成 32 字节随机数 = 64 位十六进制字符
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}

	key := AdminAPIKeyPrefix + hex.EncodeToString(bytes)

	// 存储到 settings 表
	if err := s.settingRepo.Set(ctx, SettingKeyAdminAPIKey, key); err != nil {
		return "", fmt.Errorf("save admin api key: %w", err)
	}

	return key, nil
}

// GetAdminAPIKeyStatus 获取管理员 API Key 状态
// 返回脱敏的 key、是否存在、错误
func (s *SettingService) GetAdminAPIKeyStatus(ctx context.Context) (maskedKey string, exists bool, err error) {
	key, err := s.settingRepo.GetValue(ctx, SettingKeyAdminAPIKey)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	if key == "" {
		return "", false, nil
	}

	// 脱敏：显示前 10 位和后 4 位
	if len(key) > 14 {
		maskedKey = key[:10] + "..." + key[len(key)-4:]
	} else {
		maskedKey = key
	}

	return maskedKey, true, nil
}

// GetAdminAPIKey 获取完整的管理员 API Key（仅供内部验证使用）
// 如果未配置返回空字符串和 nil 错误，只有数据库错误时才返回 error
func (s *SettingService) GetAdminAPIKey(ctx context.Context) (string, error) {
	key, err := s.settingRepo.GetValue(ctx, SettingKeyAdminAPIKey)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return "", nil // 未配置，返回空字符串
		}
		return "", err // 数据库错误
	}
	return key, nil
}

// DeleteAdminAPIKey 删除管理员 API Key
func (s *SettingService) DeleteAdminAPIKey(ctx context.Context) error {
	return s.settingRepo.Delete(ctx, SettingKeyAdminAPIKey)
}

// IsModelFallbackEnabled 检查是否启用模型兜底机制
func (s *SettingService) IsModelFallbackEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyEnableModelFallback)
	if err != nil {
		return false // Default: disabled
	}
	return value == "true"
}

// GetFallbackModel 获取指定平台的兜底模型
func (s *SettingService) GetFallbackModel(ctx context.Context, platform string) string {
	var key string
	var defaultModel string

	switch platform {
	case PlatformAnthropic:
		key = SettingKeyFallbackModelAnthropic
		defaultModel = "claude-3-5-sonnet-20241022"
	case PlatformOpenAI:
		key = SettingKeyFallbackModelOpenAI
		defaultModel = "gpt-4o"
	case PlatformGemini:
		key = SettingKeyFallbackModelGemini
		defaultModel = "gemini-2.5-pro"
	case PlatformAntigravity:
		key = SettingKeyFallbackModelAntigravity
		defaultModel = "gemini-2.5-pro"
	default:
		return ""
	}

	value, err := s.settingRepo.GetValue(ctx, key)
	if err != nil || value == "" {
		return defaultModel
	}
	return value
}

// GetLinuxDoConnectOAuthConfig 返回用于登录的"最终生效" LinuxDo Connect 配置。
//
// 优先级：
// - 若对应系统设置键存在，则覆盖 config.yaml/env 的值
// - 否则回退到 config.yaml/env 的值
func (s *SettingService) GetLinuxDoConnectOAuthConfig(ctx context.Context) (config.LinuxDoConnectConfig, error) {
	if s == nil || s.cfg == nil {
		return config.LinuxDoConnectConfig{}, infraerrors.ServiceUnavailable("CONFIG_NOT_READY", "config not loaded")
	}

	effective := s.cfg.LinuxDo

	keys := []string{
		SettingKeyLinuxDoConnectEnabled,
		SettingKeyLinuxDoConnectClientID,
		SettingKeyLinuxDoConnectClientSecret,
		SettingKeyLinuxDoConnectRedirectURL,
	}
	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return config.LinuxDoConnectConfig{}, fmt.Errorf("get linuxdo connect settings: %w", err)
	}

	if raw, ok := settings[SettingKeyLinuxDoConnectEnabled]; ok {
		effective.Enabled = raw == "true"
	}
	if v, ok := settings[SettingKeyLinuxDoConnectClientID]; ok && strings.TrimSpace(v) != "" {
		effective.ClientID = strings.TrimSpace(v)
	}
	if v, ok := settings[SettingKeyLinuxDoConnectClientSecret]; ok && strings.TrimSpace(v) != "" {
		effective.ClientSecret = strings.TrimSpace(v)
	}
	if v, ok := settings[SettingKeyLinuxDoConnectRedirectURL]; ok && strings.TrimSpace(v) != "" {
		effective.RedirectURL = strings.TrimSpace(v)
	}

	if !effective.Enabled {
		return config.LinuxDoConnectConfig{}, infraerrors.NotFound("OAUTH_DISABLED", "oauth login is disabled")
	}

	// 基础健壮性校验（避免把用户重定向到一个必然失败或不安全的 OAuth 流程里）。
	if strings.TrimSpace(effective.ClientID) == "" {
		return config.LinuxDoConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth client id not configured")
	}
	if strings.TrimSpace(effective.AuthorizeURL) == "" {
		return config.LinuxDoConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth authorize url not configured")
	}
	if strings.TrimSpace(effective.TokenURL) == "" {
		return config.LinuxDoConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth token url not configured")
	}
	if strings.TrimSpace(effective.UserInfoURL) == "" {
		return config.LinuxDoConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth userinfo url not configured")
	}
	if strings.TrimSpace(effective.RedirectURL) == "" {
		return config.LinuxDoConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth redirect url not configured")
	}
	if strings.TrimSpace(effective.FrontendRedirectURL) == "" {
		return config.LinuxDoConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth frontend redirect url not configured")
	}

	if err := config.ValidateAbsoluteHTTPURL(effective.AuthorizeURL); err != nil {
		return config.LinuxDoConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth authorize url invalid")
	}
	if err := config.ValidateAbsoluteHTTPURL(effective.TokenURL); err != nil {
		return config.LinuxDoConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth token url invalid")
	}
	if err := config.ValidateAbsoluteHTTPURL(effective.UserInfoURL); err != nil {
		return config.LinuxDoConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth userinfo url invalid")
	}
	if err := config.ValidateAbsoluteHTTPURL(effective.RedirectURL); err != nil {
		return config.LinuxDoConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth redirect url invalid")
	}
	if err := config.ValidateFrontendRedirectURL(effective.FrontendRedirectURL); err != nil {
		return config.LinuxDoConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth frontend redirect url invalid")
	}

	method := strings.ToLower(strings.TrimSpace(effective.TokenAuthMethod))
	switch method {
	case "", "client_secret_post", "client_secret_basic":
		if strings.TrimSpace(effective.ClientSecret) == "" {
			return config.LinuxDoConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth client secret not configured")
		}
	case "none":
		if !effective.UsePKCE {
			return config.LinuxDoConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth pkce must be enabled when token_auth_method=none")
		}
	default:
		return config.LinuxDoConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth token_auth_method invalid")
	}

	return effective, nil
}

// GetOverloadCooldownSettings 获取529过载冷却配置
func (s *SettingService) GetOverloadCooldownSettings(ctx context.Context) (*OverloadCooldownSettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyOverloadCooldownSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultOverloadCooldownSettings(), nil
		}
		return nil, fmt.Errorf("get overload cooldown settings: %w", err)
	}
	if value == "" {
		return DefaultOverloadCooldownSettings(), nil
	}

	var settings OverloadCooldownSettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return DefaultOverloadCooldownSettings(), nil
	}

	// 修正配置值范围
	if settings.CooldownMinutes < 1 {
		settings.CooldownMinutes = 1
	}
	if settings.CooldownMinutes > 120 {
		settings.CooldownMinutes = 120
	}

	return &settings, nil
}

// SetOverloadCooldownSettings 设置529过载冷却配置
func (s *SettingService) SetOverloadCooldownSettings(ctx context.Context, settings *OverloadCooldownSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}

	// 禁用时修正为合法值即可，不拒绝请求
	if settings.CooldownMinutes < 1 || settings.CooldownMinutes > 120 {
		if settings.Enabled {
			return fmt.Errorf("cooldown_minutes must be between 1-120")
		}
		settings.CooldownMinutes = 10 // 禁用状态下归一化为默认值
	}

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal overload cooldown settings: %w", err)
	}

	return s.settingRepo.Set(ctx, SettingKeyOverloadCooldownSettings, string(data))
}

// GetOIDCConnectOAuthConfig 返回用于登录的“最终生效” OIDC 配置。
//
// 优先级：
// - 若对应系统设置键存在，则覆盖 config.yaml/env 的值
// - 否则回退到 config.yaml/env 的值
func (s *SettingService) GetOIDCConnectOAuthConfig(ctx context.Context) (config.OIDCConnectConfig, error) {
	if s == nil || s.cfg == nil {
		return config.OIDCConnectConfig{}, infraerrors.ServiceUnavailable("CONFIG_NOT_READY", "config not loaded")
	}

	effective := s.cfg.OIDC

	keys := []string{
		SettingKeyOIDCConnectEnabled,
		SettingKeyOIDCConnectProviderName,
		SettingKeyOIDCConnectClientID,
		SettingKeyOIDCConnectClientSecret,
		SettingKeyOIDCConnectIssuerURL,
		SettingKeyOIDCConnectDiscoveryURL,
		SettingKeyOIDCConnectAuthorizeURL,
		SettingKeyOIDCConnectTokenURL,
		SettingKeyOIDCConnectUserInfoURL,
		SettingKeyOIDCConnectJWKSURL,
		SettingKeyOIDCConnectScopes,
		SettingKeyOIDCConnectRedirectURL,
		SettingKeyOIDCConnectFrontendRedirectURL,
		SettingKeyOIDCConnectTokenAuthMethod,
		SettingKeyOIDCConnectUsePKCE,
		SettingKeyOIDCConnectValidateIDToken,
		SettingKeyOIDCConnectAllowedSigningAlgs,
		SettingKeyOIDCConnectClockSkewSeconds,
		SettingKeyOIDCConnectRequireEmailVerified,
		SettingKeyOIDCConnectUserInfoEmailPath,
		SettingKeyOIDCConnectUserInfoIDPath,
		SettingKeyOIDCConnectUserInfoUsernamePath,
	}
	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return config.OIDCConnectConfig{}, fmt.Errorf("get oidc connect settings: %w", err)
	}

	if raw, ok := settings[SettingKeyOIDCConnectEnabled]; ok {
		effective.Enabled = raw == "true"
	}
	if v, ok := settings[SettingKeyOIDCConnectProviderName]; ok && strings.TrimSpace(v) != "" {
		effective.ProviderName = strings.TrimSpace(v)
	}
	if v, ok := settings[SettingKeyOIDCConnectClientID]; ok && strings.TrimSpace(v) != "" {
		effective.ClientID = strings.TrimSpace(v)
	}
	if v, ok := settings[SettingKeyOIDCConnectClientSecret]; ok && strings.TrimSpace(v) != "" {
		effective.ClientSecret = strings.TrimSpace(v)
	}
	if v, ok := settings[SettingKeyOIDCConnectIssuerURL]; ok && strings.TrimSpace(v) != "" {
		effective.IssuerURL = strings.TrimSpace(v)
	}
	if v, ok := settings[SettingKeyOIDCConnectDiscoveryURL]; ok && strings.TrimSpace(v) != "" {
		effective.DiscoveryURL = strings.TrimSpace(v)
	}
	if v, ok := settings[SettingKeyOIDCConnectAuthorizeURL]; ok && strings.TrimSpace(v) != "" {
		effective.AuthorizeURL = strings.TrimSpace(v)
	}
	if v, ok := settings[SettingKeyOIDCConnectTokenURL]; ok && strings.TrimSpace(v) != "" {
		effective.TokenURL = strings.TrimSpace(v)
	}
	if v, ok := settings[SettingKeyOIDCConnectUserInfoURL]; ok && strings.TrimSpace(v) != "" {
		effective.UserInfoURL = strings.TrimSpace(v)
	}
	if v, ok := settings[SettingKeyOIDCConnectJWKSURL]; ok && strings.TrimSpace(v) != "" {
		effective.JWKSURL = strings.TrimSpace(v)
	}
	if v, ok := settings[SettingKeyOIDCConnectScopes]; ok && strings.TrimSpace(v) != "" {
		effective.Scopes = strings.TrimSpace(v)
	}
	if v, ok := settings[SettingKeyOIDCConnectRedirectURL]; ok && strings.TrimSpace(v) != "" {
		effective.RedirectURL = strings.TrimSpace(v)
	}
	if v, ok := settings[SettingKeyOIDCConnectFrontendRedirectURL]; ok && strings.TrimSpace(v) != "" {
		effective.FrontendRedirectURL = strings.TrimSpace(v)
	}
	if v, ok := settings[SettingKeyOIDCConnectTokenAuthMethod]; ok && strings.TrimSpace(v) != "" {
		effective.TokenAuthMethod = strings.ToLower(strings.TrimSpace(v))
	}
	if raw, ok := settings[SettingKeyOIDCConnectUsePKCE]; ok {
		effective.UsePKCE = raw == "true"
	}
	if raw, ok := settings[SettingKeyOIDCConnectValidateIDToken]; ok {
		effective.ValidateIDToken = raw == "true"
	}
	if v, ok := settings[SettingKeyOIDCConnectAllowedSigningAlgs]; ok && strings.TrimSpace(v) != "" {
		effective.AllowedSigningAlgs = strings.TrimSpace(v)
	}
	if raw, ok := settings[SettingKeyOIDCConnectClockSkewSeconds]; ok && strings.TrimSpace(raw) != "" {
		if parsed, parseErr := strconv.Atoi(strings.TrimSpace(raw)); parseErr == nil {
			effective.ClockSkewSeconds = parsed
		}
	}
	if raw, ok := settings[SettingKeyOIDCConnectRequireEmailVerified]; ok {
		effective.RequireEmailVerified = raw == "true"
	}
	if v, ok := settings[SettingKeyOIDCConnectUserInfoEmailPath]; ok {
		effective.UserInfoEmailPath = strings.TrimSpace(v)
	}
	if v, ok := settings[SettingKeyOIDCConnectUserInfoIDPath]; ok {
		effective.UserInfoIDPath = strings.TrimSpace(v)
	}
	if v, ok := settings[SettingKeyOIDCConnectUserInfoUsernamePath]; ok {
		effective.UserInfoUsernamePath = strings.TrimSpace(v)
	}

	if !effective.Enabled {
		return config.OIDCConnectConfig{}, infraerrors.NotFound("OAUTH_DISABLED", "oauth login is disabled")
	}
	if strings.TrimSpace(effective.ProviderName) == "" {
		effective.ProviderName = "OIDC"
	}
	if strings.TrimSpace(effective.ClientID) == "" {
		return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth client id not configured")
	}
	if strings.TrimSpace(effective.IssuerURL) == "" {
		return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth issuer url not configured")
	}
	if strings.TrimSpace(effective.RedirectURL) == "" {
		return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth redirect url not configured")
	}
	if strings.TrimSpace(effective.FrontendRedirectURL) == "" {
		return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth frontend redirect url not configured")
	}
	if !scopesContainOpenID(effective.Scopes) {
		return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth scopes must contain openid")
	}
	if effective.ClockSkewSeconds < 0 || effective.ClockSkewSeconds > 600 {
		return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth clock skew must be between 0 and 600")
	}

	if err := config.ValidateAbsoluteHTTPURL(effective.IssuerURL); err != nil {
		return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth issuer url invalid")
	}

	discoveryURL := strings.TrimSpace(effective.DiscoveryURL)
	if discoveryURL == "" {
		discoveryURL = oidcDefaultDiscoveryURL(effective.IssuerURL)
		effective.DiscoveryURL = discoveryURL
	}
	if discoveryURL != "" {
		if err := config.ValidateAbsoluteHTTPURL(discoveryURL); err != nil {
			return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth discovery url invalid")
		}
	}

	needsDiscovery := strings.TrimSpace(effective.AuthorizeURL) == "" ||
		strings.TrimSpace(effective.TokenURL) == "" ||
		(effective.ValidateIDToken && strings.TrimSpace(effective.JWKSURL) == "")
	if needsDiscovery && discoveryURL != "" {
		metadata, resolveErr := oidcResolveProviderMetadata(ctx, discoveryURL)
		if resolveErr != nil {
			return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth discovery resolve failed").WithCause(resolveErr)
		}
		if strings.TrimSpace(effective.AuthorizeURL) == "" {
			effective.AuthorizeURL = strings.TrimSpace(metadata.AuthorizationEndpoint)
		}
		if strings.TrimSpace(effective.TokenURL) == "" {
			effective.TokenURL = strings.TrimSpace(metadata.TokenEndpoint)
		}
		if strings.TrimSpace(effective.UserInfoURL) == "" {
			effective.UserInfoURL = strings.TrimSpace(metadata.UserInfoEndpoint)
		}
		if strings.TrimSpace(effective.JWKSURL) == "" {
			effective.JWKSURL = strings.TrimSpace(metadata.JWKSURI)
		}
	}

	if strings.TrimSpace(effective.AuthorizeURL) == "" {
		return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth authorize url not configured")
	}
	if strings.TrimSpace(effective.TokenURL) == "" {
		return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth token url not configured")
	}
	if err := config.ValidateAbsoluteHTTPURL(effective.AuthorizeURL); err != nil {
		return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth authorize url invalid")
	}
	if err := config.ValidateAbsoluteHTTPURL(effective.TokenURL); err != nil {
		return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth token url invalid")
	}
	if v := strings.TrimSpace(effective.UserInfoURL); v != "" {
		if err := config.ValidateAbsoluteHTTPURL(v); err != nil {
			return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth userinfo url invalid")
		}
	}
	if effective.ValidateIDToken {
		if strings.TrimSpace(effective.JWKSURL) == "" {
			return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth jwks url not configured")
		}
		if strings.TrimSpace(effective.AllowedSigningAlgs) == "" {
			return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth signing algs not configured")
		}
	}
	if v := strings.TrimSpace(effective.JWKSURL); v != "" {
		if err := config.ValidateAbsoluteHTTPURL(v); err != nil {
			return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth jwks url invalid")
		}
	}
	if err := config.ValidateAbsoluteHTTPURL(effective.RedirectURL); err != nil {
		return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth redirect url invalid")
	}
	if err := config.ValidateFrontendRedirectURL(effective.FrontendRedirectURL); err != nil {
		return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth frontend redirect url invalid")
	}

	method := strings.ToLower(strings.TrimSpace(effective.TokenAuthMethod))
	switch method {
	case "", "client_secret_post", "client_secret_basic":
		if strings.TrimSpace(effective.ClientSecret) == "" {
			return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth client secret not configured")
		}
	case "none":
		if !effective.UsePKCE {
			return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth pkce must be enabled when token_auth_method=none")
		}
	default:
		return config.OIDCConnectConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth token_auth_method invalid")
	}

	return effective, nil
}

func scopesContainOpenID(scopes string) bool {
	for _, scope := range strings.Fields(strings.ToLower(strings.TrimSpace(scopes))) {
		if scope == "openid" {
			return true
		}
	}
	return false
}

type oidcProviderMetadata struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

func oidcDefaultDiscoveryURL(issuerURL string) string {
	issuerURL = strings.TrimSpace(issuerURL)
	if issuerURL == "" {
		return ""
	}
	return strings.TrimRight(issuerURL, "/") + "/.well-known/openid-configuration"
}

func oidcResolveProviderMetadata(ctx context.Context, discoveryURL string) (*oidcProviderMetadata, error) {
	discoveryURL = strings.TrimSpace(discoveryURL)
	if discoveryURL == "" {
		return nil, fmt.Errorf("discovery url is empty")
	}

	resp, err := req.C().
		SetTimeout(15*time.Second).
		R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		Get(discoveryURL)
	if err != nil {
		return nil, fmt.Errorf("request discovery document: %w", err)
	}
	if !resp.IsSuccessState() {
		return nil, fmt.Errorf("discovery request failed: status=%d", resp.StatusCode)
	}

	metadata := &oidcProviderMetadata{}
	if err := json.Unmarshal(resp.Bytes(), metadata); err != nil {
		return nil, fmt.Errorf("parse discovery document: %w", err)
	}
	return metadata, nil
}

// GetStreamTimeoutSettings 获取流超时处理配置
func (s *SettingService) GetStreamTimeoutSettings(ctx context.Context) (*StreamTimeoutSettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyStreamTimeoutSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultStreamTimeoutSettings(), nil
		}
		return nil, fmt.Errorf("get stream timeout settings: %w", err)
	}
	if value == "" {
		return DefaultStreamTimeoutSettings(), nil
	}

	var settings StreamTimeoutSettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return DefaultStreamTimeoutSettings(), nil
	}

	// 验证并修正配置值
	if settings.TempUnschedMinutes < 1 {
		settings.TempUnschedMinutes = 1
	}
	if settings.TempUnschedMinutes > 60 {
		settings.TempUnschedMinutes = 60
	}
	if settings.ThresholdCount < 1 {
		settings.ThresholdCount = 1
	}
	if settings.ThresholdCount > 10 {
		settings.ThresholdCount = 10
	}
	if settings.ThresholdWindowMinutes < 1 {
		settings.ThresholdWindowMinutes = 1
	}
	if settings.ThresholdWindowMinutes > 60 {
		settings.ThresholdWindowMinutes = 60
	}

	// 验证 action
	switch settings.Action {
	case StreamTimeoutActionTempUnsched, StreamTimeoutActionError, StreamTimeoutActionNone:
		// valid
	default:
		settings.Action = StreamTimeoutActionTempUnsched
	}

	return &settings, nil
}

// IsUngroupedKeySchedulingAllowed 查询是否允许未分组 Key 调度
func (s *SettingService) IsUngroupedKeySchedulingAllowed(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyAllowUngroupedKeyScheduling)
	if err != nil {
		return false // fail-closed: 查询失败时默认不允许
	}
	return value == "true"
}

// GetClaudeCodeVersionBounds 获取 Claude Code 版本号上下限要求
// 使用进程内 atomic.Value 缓存，60 秒 TTL，热路径零锁开销
// singleflight 防止缓存过期时 thundering herd
// 返回空字符串表示不做对应方向的版本检查
func (s *SettingService) GetClaudeCodeVersionBounds(ctx context.Context) (min, max string) {
	if cached, ok := versionBoundsCache.Load().(*cachedVersionBounds); ok {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.min, cached.max
		}
	}
	// singleflight: 同一时刻只有一个 goroutine 查询 DB，其余复用结果
	type bounds struct{ min, max string }
	result, err, _ := versionBoundsSF.Do("version_bounds", func() (any, error) {
		// 二次检查，避免排队的 goroutine 重复查询
		if cached, ok := versionBoundsCache.Load().(*cachedVersionBounds); ok {
			if time.Now().UnixNano() < cached.expiresAt {
				return bounds{cached.min, cached.max}, nil
			}
		}
		// 使用独立 context：断开请求取消链，避免客户端断连导致空值被长期缓存
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), versionBoundsDBTimeout)
		defer cancel()
		values, err := s.settingRepo.GetMultiple(dbCtx, []string{
			SettingKeyMinClaudeCodeVersion,
			SettingKeyMaxClaudeCodeVersion,
		})
		if err != nil {
			// fail-open: DB 错误时不阻塞请求，但记录日志并使用短 TTL 快速重试
			slog.Warn("failed to get claude code version bounds setting, skipping version check", "error", err)
			versionBoundsCache.Store(&cachedVersionBounds{
				min:       "",
				max:       "",
				expiresAt: time.Now().Add(versionBoundsErrorTTL).UnixNano(),
			})
			return bounds{"", ""}, nil
		}
		b := bounds{
			min: values[SettingKeyMinClaudeCodeVersion],
			max: values[SettingKeyMaxClaudeCodeVersion],
		}
		versionBoundsCache.Store(&cachedVersionBounds{
			min:       b.min,
			max:       b.max,
			expiresAt: time.Now().Add(versionBoundsCacheTTL).UnixNano(),
		})
		return b, nil
	})
	if err != nil {
		return "", ""
	}
	b, ok := result.(bounds)
	if !ok {
		return "", ""
	}
	return b.min, b.max
}

// GetRectifierSettings 获取请求整流器配置
func (s *SettingService) GetRectifierSettings(ctx context.Context) (*RectifierSettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyRectifierSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultRectifierSettings(), nil
		}
		return nil, fmt.Errorf("get rectifier settings: %w", err)
	}
	if value == "" {
		return DefaultRectifierSettings(), nil
	}

	var settings RectifierSettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return DefaultRectifierSettings(), nil
	}

	return &settings, nil
}

// SetRectifierSettings 设置请求整流器配置
func (s *SettingService) SetRectifierSettings(ctx context.Context, settings *RectifierSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal rectifier settings: %w", err)
	}

	return s.settingRepo.Set(ctx, SettingKeyRectifierSettings, string(data))
}

// IsSignatureRectifierEnabled 判断签名整流是否启用（总开关 && 签名子开关）
func (s *SettingService) IsSignatureRectifierEnabled(ctx context.Context) bool {
	settings, err := s.GetRectifierSettings(ctx)
	if err != nil {
		return true // fail-open: 查询失败时默认启用
	}
	return settings.Enabled && settings.ThinkingSignatureEnabled
}

// IsBudgetRectifierEnabled 判断 Budget 整流是否启用（总开关 && Budget 子开关）
func (s *SettingService) IsBudgetRectifierEnabled(ctx context.Context) bool {
	settings, err := s.GetRectifierSettings(ctx)
	if err != nil {
		return true // fail-open: 查询失败时默认启用
	}
	return settings.Enabled && settings.ThinkingBudgetEnabled
}

// GetBetaPolicySettings 获取 Beta 策略配置
func (s *SettingService) GetBetaPolicySettings(ctx context.Context) (*BetaPolicySettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyBetaPolicySettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultBetaPolicySettings(), nil
		}
		return nil, fmt.Errorf("get beta policy settings: %w", err)
	}
	if value == "" {
		return DefaultBetaPolicySettings(), nil
	}

	var settings BetaPolicySettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return DefaultBetaPolicySettings(), nil
	}

	return &settings, nil
}

// SetBetaPolicySettings 设置 Beta 策略配置
func (s *SettingService) SetBetaPolicySettings(ctx context.Context, settings *BetaPolicySettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}

	validActions := map[string]bool{
		BetaPolicyActionPass: true, BetaPolicyActionFilter: true, BetaPolicyActionBlock: true,
	}
	validScopes := map[string]bool{
		BetaPolicyScopeAll: true, BetaPolicyScopeOAuth: true, BetaPolicyScopeAPIKey: true, BetaPolicyScopeBedrock: true,
	}

	for i, rule := range settings.Rules {
		if rule.BetaToken == "" {
			return fmt.Errorf("rule[%d]: beta_token cannot be empty", i)
		}
		if !validActions[rule.Action] {
			return fmt.Errorf("rule[%d]: invalid action %q", i, rule.Action)
		}
		if !validScopes[rule.Scope] {
			return fmt.Errorf("rule[%d]: invalid scope %q", i, rule.Scope)
		}
		// Validate model_whitelist patterns
		for j, pattern := range rule.ModelWhitelist {
			trimmed := strings.TrimSpace(pattern)
			if trimmed == "" {
				return fmt.Errorf("rule[%d]: model_whitelist[%d] cannot be empty", i, j)
			}
			settings.Rules[i].ModelWhitelist[j] = trimmed
		}
		// Validate fallback_action
		if rule.FallbackAction != "" && !validActions[rule.FallbackAction] {
			return fmt.Errorf("rule[%d]: invalid fallback_action %q", i, rule.FallbackAction)
		}
	}

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal beta policy settings: %w", err)
	}

	return s.settingRepo.Set(ctx, SettingKeyBetaPolicySettings, string(data))
}

// SetStreamTimeoutSettings 设置流超时处理配置
func (s *SettingService) SetStreamTimeoutSettings(ctx context.Context, settings *StreamTimeoutSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}

	// 验证配置值
	if settings.TempUnschedMinutes < 1 || settings.TempUnschedMinutes > 60 {
		return fmt.Errorf("temp_unsched_minutes must be between 1-60")
	}
	if settings.ThresholdCount < 1 || settings.ThresholdCount > 10 {
		return fmt.Errorf("threshold_count must be between 1-10")
	}
	if settings.ThresholdWindowMinutes < 1 || settings.ThresholdWindowMinutes > 60 {
		return fmt.Errorf("threshold_window_minutes must be between 1-60")
	}

	switch settings.Action {
	case StreamTimeoutActionTempUnsched, StreamTimeoutActionError, StreamTimeoutActionNone:
		// valid
	default:
		return fmt.Errorf("invalid action: %s", settings.Action)
	}

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal stream timeout settings: %w", err)
	}

	return s.settingRepo.Set(ctx, SettingKeyStreamTimeoutSettings, string(data))
}
