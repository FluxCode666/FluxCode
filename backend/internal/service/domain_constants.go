package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

// Status constants
const (
	StatusActive   = domain.StatusActive
	StatusDisabled = domain.StatusDisabled
	StatusError    = domain.StatusError
	StatusBanned   = domain.StatusBanned
	StatusUnused   = domain.StatusUnused
	StatusUsed     = domain.StatusUsed
	StatusExpired  = domain.StatusExpired
)

// Role constants
const (
	RoleAdmin = domain.RoleAdmin
	RoleUser  = domain.RoleUser
)

// Platform constants
const (
	PlatformAnthropic   = domain.PlatformAnthropic
	PlatformOpenAI      = domain.PlatformOpenAI
	PlatformCodex2API   = domain.PlatformCodex2API
	PlatformGemini      = domain.PlatformGemini
	PlatformAntigravity = domain.PlatformAntigravity
)

const (
	SystemPromptModeInherit     = "inherit"
	SystemPromptModePassthrough = "passthrough"
	SystemPromptModeOverride    = "override"
	SystemPromptModeAppend      = "append"

	SystemPromptSourceNone   = "none"
	SystemPromptSourceAPIKey = "api_key"
	SystemPromptSourceGroup  = "group"
	SystemPromptSourceSystem = "system"
)

const (
	SystemPromptUserScopeAll       = "all"
	SystemPromptUserScopeWhitelist = "whitelist"
	SystemPromptUserScopeBlacklist = "blacklist"
)

const (
	GeneratedImageStorageSourceDB    = "db"
	GeneratedImageStorageSourceQiniu = "qiniu"
)

func IsOpenAICompatiblePlatform(platform string) bool {
	return platform == PlatformOpenAI || platform == PlatformCodex2API
}

func AccountPlatformGroupPlatform(platform string) string {
	if platform == PlatformCodex2API {
		return PlatformOpenAI
	}
	return platform
}

func AccountCanBelongToGroupPlatform(accountPlatform, groupPlatform string) bool {
	if accountPlatform == PlatformAntigravity && (groupPlatform == PlatformAnthropic || groupPlatform == PlatformGemini) {
		return true
	}
	return AccountPlatformGroupPlatform(accountPlatform) == groupPlatform
}

// Account type constants
const (
	AccountTypeOAuth      = domain.AccountTypeOAuth      // OAuth类型账号（full scope: profile + inference）
	AccountTypeSetupToken = domain.AccountTypeSetupToken // Setup Token类型账号（inference only scope）
	AccountTypeAPIKey     = domain.AccountTypeAPIKey     // API Key类型账号
	AccountTypeUpstream   = domain.AccountTypeUpstream   // 上游透传类型账号（通过 Base URL + API Key 连接上游）
	AccountTypeBedrock    = domain.AccountTypeBedrock    // AWS Bedrock 类型账号（通过 SigV4 签名或 API Key 连接 Bedrock，由 credentials.auth_mode 区分）
)

// Redeem type constants
const (
	RedeemTypeBalance      = domain.RedeemTypeBalance
	RedeemTypeConcurrency  = domain.RedeemTypeConcurrency
	RedeemTypeSubscription = domain.RedeemTypeSubscription
	RedeemTypeInvitation   = domain.RedeemTypeInvitation
)

// PromoCode status constants
const (
	PromoCodeStatusActive   = domain.PromoCodeStatusActive
	PromoCodeStatusDisabled = domain.PromoCodeStatusDisabled
)

// Admin adjustment type constants
const (
	AdjustmentTypeAdminBalance     = domain.AdjustmentTypeAdminBalance     // 管理员调整余额
	AdjustmentTypeAdminConcurrency = domain.AdjustmentTypeAdminConcurrency // 管理员调整并发数
)

// Group subscription type constants
const (
	SubscriptionTypeStandard     = domain.SubscriptionTypeStandard     // 标准计费模式（按余额扣费）
	SubscriptionTypeSubscription = domain.SubscriptionTypeSubscription // 订阅模式（按限额控制）
)

// Subscription status constants
const (
	SubscriptionStatusActive    = domain.SubscriptionStatusActive
	SubscriptionStatusExpired   = domain.SubscriptionStatusExpired
	SubscriptionStatusSuspended = domain.SubscriptionStatusSuspended
)

// LinuxDoConnectSyntheticEmailDomain 是 LinuxDo Connect 用户的合成邮箱后缀（RFC 保留域名）。
const LinuxDoConnectSyntheticEmailDomain = "@linuxdo-connect.invalid"

// OIDCConnectSyntheticEmailDomain 是 OIDC 用户的合成邮箱后缀（RFC 保留域名）。
const OIDCConnectSyntheticEmailDomain = "@oidc-connect.invalid"

// Setting keys
const (
	// 注册设置
	SettingKeyRegistrationEnabled              = "registration_enabled"                // 是否开放注册
	SettingKeyEmailVerifyEnabled               = "email_verify_enabled"                // 是否开启邮件验证
	SettingKeyRegistrationEmailSuffixWhitelist = "registration_email_suffix_whitelist" // 注册邮箱后缀白名单（JSON 数组）
	SettingKeyPromoCodeEnabled                 = "promo_code_enabled"                  // 是否启用优惠码功能
	SettingKeyPasswordResetEnabled             = "password_reset_enabled"              // 是否启用忘记密码功能（需要先开启邮件验证）
	SettingKeyFrontendURL                      = "frontend_url"                        // 前端基础URL，用于生成邮件中的重置密码链接
	SettingKeyInvitationCodeEnabled            = "invitation_code_enabled"             // 是否启用邀请码注册

	// 邮件服务设置
	SettingKeyEmailProvider  = "email_provider"   // 邮件发送提供商: smtp/resend
	SettingKeySMTPHost       = "smtp_host"        // SMTP服务器地址
	SettingKeySMTPPort       = "smtp_port"        // SMTP端口
	SettingKeySMTPUsername   = "smtp_username"    // SMTP用户名
	SettingKeySMTPPassword   = "smtp_password"    // SMTP密码（加密存储）
	SettingKeySMTPFrom       = "smtp_from"        // 发件人地址
	SettingKeySMTPFromName   = "smtp_from_name"   // 发件人名称
	SettingKeySMTPUseTLS     = "smtp_use_tls"     // 是否使用TLS
	SettingKeyResendAPIKey   = "resend_api_key"   // Resend API Key
	SettingKeyResendFrom     = "resend_from"      // Resend 发件人地址
	SettingKeyResendFromName = "resend_from_name" // Resend 发件人名称

	// Cloudflare Turnstile 设置
	SettingKeyTurnstileEnabled   = "turnstile_enabled"    // 是否启用 Turnstile 验证
	SettingKeyTurnstileSiteKey   = "turnstile_site_key"   // Turnstile Site Key
	SettingKeyTurnstileSecretKey = "turnstile_secret_key" // Turnstile Secret Key

	// TOTP 双因素认证设置
	SettingKeyTotpEnabled = "totp_enabled" // 是否启用 TOTP 2FA 功能

	// Channel Monitor 渠道状态监控设置
	SettingKeyChannelMonitorEnabled                = "channel_monitor_enabled"
	SettingKeyChannelMonitorDefaultIntervalSeconds = "channel_monitor_default_interval_seconds"

	// LinuxDo Connect OAuth 登录设置
	SettingKeyLinuxDoConnectEnabled      = "linuxdo_connect_enabled"
	SettingKeyLinuxDoConnectClientID     = "linuxdo_connect_client_id"
	SettingKeyLinuxDoConnectClientSecret = "linuxdo_connect_client_secret"
	SettingKeyLinuxDoConnectRedirectURL  = "linuxdo_connect_redirect_url"

	// Generic OIDC OAuth 登录设置
	SettingKeyOIDCConnectEnabled              = "oidc_connect_enabled"
	SettingKeyOIDCConnectProviderName         = "oidc_connect_provider_name"
	SettingKeyOIDCConnectClientID             = "oidc_connect_client_id"
	SettingKeyOIDCConnectClientSecret         = "oidc_connect_client_secret"
	SettingKeyOIDCConnectIssuerURL            = "oidc_connect_issuer_url"
	SettingKeyOIDCConnectDiscoveryURL         = "oidc_connect_discovery_url"
	SettingKeyOIDCConnectAuthorizeURL         = "oidc_connect_authorize_url"
	SettingKeyOIDCConnectTokenURL             = "oidc_connect_token_url"
	SettingKeyOIDCConnectUserInfoURL          = "oidc_connect_userinfo_url"
	SettingKeyOIDCConnectJWKSURL              = "oidc_connect_jwks_url"
	SettingKeyOIDCConnectScopes               = "oidc_connect_scopes"
	SettingKeyOIDCConnectRedirectURL          = "oidc_connect_redirect_url"
	SettingKeyOIDCConnectFrontendRedirectURL  = "oidc_connect_frontend_redirect_url"
	SettingKeyOIDCConnectTokenAuthMethod      = "oidc_connect_token_auth_method"
	SettingKeyOIDCConnectUsePKCE              = "oidc_connect_use_pkce"
	SettingKeyOIDCConnectValidateIDToken      = "oidc_connect_validate_id_token"
	SettingKeyOIDCConnectAllowedSigningAlgs   = "oidc_connect_allowed_signing_algs"
	SettingKeyOIDCConnectClockSkewSeconds     = "oidc_connect_clock_skew_seconds"
	SettingKeyOIDCConnectRequireEmailVerified = "oidc_connect_require_email_verified"
	SettingKeyOIDCConnectUserInfoEmailPath    = "oidc_connect_userinfo_email_path"
	SettingKeyOIDCConnectUserInfoIDPath       = "oidc_connect_userinfo_id_path"
	SettingKeyOIDCConnectUserInfoUsernamePath = "oidc_connect_userinfo_username_path"

	// OEM设置
	SettingKeySiteName                     = "site_name"                     // 网站名称
	SettingKeySiteLogo                     = "site_logo"                     // 网站Logo (base64)
	SettingKeySiteSubtitle                 = "site_subtitle"                 // 网站副标题
	SettingKeyAPIBaseURL                   = "api_base_url"                  // API端点地址（用于客户端配置和导入）
	SettingKeyContactInfo                  = "contact_info"                  // 客服联系方式
	SettingKeyDocURL                       = "doc_url"                       // 文档链接
	SettingKeyHomeContent                  = "home_content"                  // 首页内容（支持 Markdown/HTML，或 URL 作为 iframe src）
	SettingKeyHideCcsImportButton          = "hide_ccs_import_button"        // 是否隐藏 API Keys 页面的导入 CCS 按钮
	SettingKeyPurchaseSubscriptionEnabled  = "purchase_subscription_enabled" // 是否展示"购买订阅"页面入口
	SettingKeyPurchaseSubscriptionURL      = "purchase_subscription_url"     // "购买订阅"页面 URL（作为 iframe src）
	SettingKeyTableDefaultPageSize         = "table_default_page_size"       // 表格默认每页条数
	SettingKeyTablePageSizeOptions         = "table_page_size_options"       // 表格可选每页条数（JSON 数组）
	SettingKeyCustomMenuItems              = "custom_menu_items"             // 自定义菜单项（JSON 数组）
	SettingKeyCustomEndpoints              = "custom_endpoints"              // 自定义端点列表（JSON 数组）
	SettingKeyOpenAIUseKeyModelID          = "openai_use_key_model_id"       // 用户侧 OpenAI/Codex 一键配置默认模型 ID
	SettingKeyOpenAIImageURLCacheTTLHours  = "openai_image_url_cache_ttl_hours"
	SettingKeyGeneratedImageCleanupEnabled = "generated_image_cleanup_enabled"

	// 媒体运行时设置
	SettingKeyMediaSyncWaitTimeoutSeconds          = "media_sync_wait_timeout_seconds"
	SettingKeyMediaSyncTimeoutFallbackAsyncEnabled = "media_sync_timeout_fallback_async_enabled"
	SettingKeyMediaSyncTimeoutBillingPolicy        = "media_sync_timeout_billing_policy"
	SettingKeyMediaSyncTimeoutPenaltyRatio         = "media_sync_timeout_penalty_ratio"
	SettingKeyMediaVideoStorageMode                = "media_video_storage_mode"
	SettingKeyMediaVideoProxyFallbackEnabled       = "media_video_proxy_fallback_enabled"

	// 生图存储设置
	SettingKeyGeneratedImageStorageSource       = "generated_image_storage_source"        // db/qiniu, actual storage source
	SettingKeyGeneratedImageStorageConfigSource = "generated_image_storage_config_source" // db/qiniu, settings panel source
	SettingKeyQiniuAccessKey                    = "qiniu_access_key"
	SettingKeyQiniuSecretKey                    = "qiniu_secret_key"
	SettingKeyQiniuBucket                       = "qiniu_bucket"
	SettingKeyQiniuCDNDomain                    = "qiniu_cdn_domain"
	SettingKeyQiniuPrefix                       = "qiniu_prefix"
	SettingKeyQiniuUseHTTPS                     = "qiniu_use_https"
	SettingKeyQiniuUploadTimeoutSeconds         = "qiniu_upload_timeout_seconds"
	SettingKeyQiniuTokenTTLSeconds              = "qiniu_token_ttl_seconds"

	// 默认配置
	SettingKeyDefaultConcurrency   = "default_concurrency"   // 新用户默认并发量
	SettingKeyDefaultBalance       = "default_balance"       // 新用户默认余额
	SettingKeyDefaultSubscriptions = "default_subscriptions" // 新用户默认订阅列表（JSON）

	// 管理员 API Key
	SettingKeyAdminAPIKey = "admin_api_key" // 全局管理员 API Key（用于外部系统集成）

	// Gemini 配额策略（JSON）
	SettingKeyGeminiQuotaPolicy = "gemini_quota_policy"

	// Model fallback settings
	SettingKeyEnableModelFallback      = "enable_model_fallback"
	SettingKeyFallbackModelAnthropic   = "fallback_model_anthropic"
	SettingKeyFallbackModelOpenAI      = "fallback_model_openai"
	SettingKeyFallbackModelGemini      = "fallback_model_gemini"
	SettingKeyFallbackModelAntigravity = "fallback_model_antigravity"

	// Request identity patch (Claude -> Gemini systemInstruction injection)
	SettingKeyEnableIdentityPatch = "enable_identity_patch"
	SettingKeyIdentityPatchPrompt = "identity_patch_prompt"

	// System prompt injection settings (platform scoped)
	SettingKeySystemPromptAnthropic        = "system_prompt_anthropic"
	SettingKeySystemPromptModeAnthropic    = "system_prompt_mode_anthropic"
	SettingKeySystemPromptOpenAI           = "system_prompt_openai"
	SettingKeySystemPromptModeOpenAI       = "system_prompt_mode_openai"
	SettingKeySystemPromptGemini           = "system_prompt_gemini"
	SettingKeySystemPromptModeGemini       = "system_prompt_mode_gemini"
	SettingKeySystemPromptAntigravity      = "system_prompt_antigravity"
	SettingKeySystemPromptModeAntigravity  = "system_prompt_mode_antigravity"
	SettingKeySystemPromptUserScopeEnabled = "system_prompt_user_scope_enabled"
	SettingKeySystemPromptUserScopeMode    = "system_prompt_user_scope_mode"
	SettingKeySystemPromptUserScopeUserIDs = "system_prompt_user_scope_user_ids"

	// =========================
	// Ops Monitoring (vNext)
	// =========================

	// SettingKeyOpsMonitoringEnabled is a DB-backed soft switch to enable/disable ops module at runtime.
	SettingKeyOpsMonitoringEnabled = "ops_monitoring_enabled"

	// SettingKeyOpsRealtimeMonitoringEnabled controls realtime features (e.g. WS/QPS push).
	SettingKeyOpsRealtimeMonitoringEnabled = "ops_realtime_monitoring_enabled"

	// SettingKeyOpsQueryModeDefault controls the default query mode for ops dashboard (auto/raw/preagg).
	SettingKeyOpsQueryModeDefault = "ops_query_mode_default"

	// SettingKeyOpsEmailNotificationConfig stores JSON config for ops email notifications.
	SettingKeyOpsEmailNotificationConfig = "ops_email_notification_config"

	// SettingKeyOpsAlertRuntimeSettings stores JSON config for ops alert evaluator runtime settings.
	SettingKeyOpsAlertRuntimeSettings = "ops_alert_runtime_settings"

	// SettingKeyOpsMetricsIntervalSeconds controls the ops metrics collector interval (>=60).
	SettingKeyOpsMetricsIntervalSeconds = "ops_metrics_interval_seconds"

	// SettingKeyOpsAdvancedSettings stores JSON config for ops advanced settings (data retention, aggregation).
	SettingKeyOpsAdvancedSettings = "ops_advanced_settings"

	// SettingKeyOpsRuntimeLogConfig stores JSON config for runtime log settings.
	SettingKeyOpsRuntimeLogConfig = "ops_runtime_log_config"

	// =========================
	// Overload Cooldown (529)
	// =========================

	// SettingKeyOverloadCooldownSettings stores JSON config for 529 overload cooldown handling.
	SettingKeyOverloadCooldownSettings = "overload_cooldown_settings"

	// =========================
	// Stream Timeout Handling
	// =========================

	// SettingKeyStreamTimeoutSettings stores JSON config for stream timeout handling.
	SettingKeyStreamTimeoutSettings = "stream_timeout_settings"

	// =========================
	// Request Rectifier (请求整流器)
	// =========================

	// SettingKeyRectifierSettings stores JSON config for rectifier settings (thinking signature + budget).
	SettingKeyRectifierSettings = "rectifier_settings"

	// =========================
	// Beta Policy Settings
	// =========================

	// SettingKeyBetaPolicySettings stores JSON config for beta policy rules.
	SettingKeyBetaPolicySettings = "beta_policy_settings"

	// =========================
	// Claude Code Version Check
	// =========================

	// SettingKeyMinClaudeCodeVersion 最低 Claude Code 版本号要求 (semver, 如 "2.1.0"，空值=不检查)
	SettingKeyMinClaudeCodeVersion = "min_claude_code_version"

	// SettingKeyMaxClaudeCodeVersion 最高 Claude Code 版本号限制 (semver, 如 "3.0.0"，空值=不检查)
	SettingKeyMaxClaudeCodeVersion = "max_claude_code_version"

	// SettingKeyAllowUngroupedKeyScheduling 允许未分组 API Key 调度（默认 false：未分组 Key 返回 403）
	SettingKeyAllowUngroupedKeyScheduling = "allow_ungrouped_key_scheduling"

	// SettingKeyBackendModeEnabled Backend 模式：禁用用户注册和自助服务，仅管理员可登录
	SettingKeyBackendModeEnabled = "backend_mode_enabled"

	// =========================
	// 兑换码发货文案 & 引流弹窗
	// =========================

	// SettingKeyRedeemDeliveryText 兑换码发货文案模板（支持 ${redeemCodes} 占位符）
	SettingKeyRedeemDeliveryText = "redeem_delivery_text"

	// SettingKeyAttractPopupTitle 引流弹窗标题
	SettingKeyAttractPopupTitle = "attract_popup_title"

	// SettingKeyAttractPopupMarkdown 引流弹窗 Markdown 内容
	SettingKeyAttractPopupMarkdown = "attract_popup_markdown"

	// Dashboard fireworks
	SettingKeyDashboardFireworksEnabled   = "dashboard_fireworks_enabled"
	SettingKeyDashboardFireworksThreshold = "dashboard_fireworks_threshold"

	// Gateway Forwarding Behavior
	// SettingKeyEnableFingerprintUnification 是否统一 OAuth 账号的 X-Stainless-* 指纹头（默认 true）
	SettingKeyEnableFingerprintUnification = "enable_fingerprint_unification"
	// SettingKeyEnableMetadataPassthrough 是否透传客户端原始 metadata.user_id（默认 false）
	SettingKeyEnableMetadataPassthrough = "enable_metadata_passthrough"
	// SettingKeyEnableCCHSigning 是否对 billing header 中的 cch 进行 xxHash64 签名（默认 false）
	SettingKeyEnableCCHSigning = "enable_cch_signing"
	// SettingKeyCodexImageGenerationBridgeEnabled 是否为 Codex /v1/responses 自动注入 image_generation bridge（默认 false）
	SettingKeyCodexImageGenerationBridgeEnabled = "codex_image_generation_bridge_enabled"

	// Balance Low Notification
	SettingKeyBalanceLowNotifyEnabled     = "balance_low_notify_enabled"      // 全局开关
	SettingKeyBalanceLowNotifyThreshold   = "balance_low_notify_threshold"    // 默认阈值（USD）
	SettingKeyBalanceLowNotifyRechargeURL = "balance_low_notify_recharge_url" // 充值页面 URL

	// Account Quota Notification
	SettingKeyAccountQuotaNotifyEnabled = "account_quota_notify_enabled" // 全局开关
	SettingKeyAccountQuotaNotifyEmails  = "account_quota_notify_emails"  // 管理员通知邮箱列表（JSON 数组）

	// Web Search Emulation
	SettingKeyWebSearchEmulationConfig = "web_search_emulation_config" // JSON 配置

	// =========================
	// 推广奖励系统 (Referral Reward)
	// =========================

	// --- 普通用户推广（非销售）---
	SettingKeyReferralEnabled                   = "referral_enabled"                      // 普通推广开关：控制非销售用户的推广功能
	SettingKeyReferralInviteeRewardEnabled      = "referral_invitee_reward_enabled"       // 普通推广：是否启用被邀请人注册奖励
	SettingKeyReferralInviteeReward             = "referral_invitee_reward"               // 被邀请人注册奖励金额（普通推广人邀请的新用户）
	SettingKeyReferralInviterReward             = "referral_inviter_reward"               // 普通推广人首充奖励金额（gift balance）
	SettingKeyReferralMaxInvites                = "referral_max_invites"                  // 普通推广人最大邀请数（0=无限）
	SettingKeyReferralRewardExpiryDays          = "referral_reward_expiry_days"           // 赠送余额过期天数（0=永不过期）
	SettingKeyReferralOngoingRewardEnabled      = "referral_ongoing_reward_enabled"       // 普通推广人持续奖励开关
	SettingKeyReferralOngoingRewardType         = "referral_ongoing_reward_type"          // 普通推广人持续奖励类型：fixed=固定金额，percentage=按充值百分比
	SettingKeyReferralOngoingRewardValue        = "referral_ongoing_reward_value"         // 普通推广人持续奖励数值：fixed 时单位为美元，percentage 时单位为百分点（0-100）
	SettingKeyReferralOngoingRewardMaxCount     = "referral_ongoing_reward_max_count"     // 普通推广人每条推广关系最多触发持续奖励次数（0=不限）
	SettingKeyReferralOngoingRewardDurationDays = "referral_ongoing_reward_duration_days" // 普通推广人持续奖励有效期：注册后N天内充值才触发（0=永久有效）

	// 普通推广：邀请人首充奖励
	SettingKeyReferralInviterFirstChargeRewardEnabled = "referral_inviter_first_charge_reward_enabled" // 邀请人首充奖励开关
	SettingKeyReferralInviterFirstChargeRewardType    = "referral_inviter_first_charge_reward_type"    // 邀请人首充奖励类型：fixed=固定金额，percentage=按充值百分比
	SettingKeyReferralInviterFirstChargeRewardValue   = "referral_inviter_first_charge_reward_value"   // 邀请人首充奖励数值

	// 普通推广：被邀请人首充奖励（全新）
	SettingKeyReferralInviteeFirstChargeRewardEnabled = "referral_invitee_first_charge_reward_enabled" // 被邀请人首充奖励开关
	SettingKeyReferralInviteeFirstChargeRewardType    = "referral_invitee_first_charge_reward_type"    // 被邀请人首充奖励类型 fixed/percentage
	SettingKeyReferralInviteeFirstChargeRewardValue   = "referral_invitee_first_charge_reward_value"   // 被邀请人首充奖励数值

	// 普通推广：被邀请人持续充值奖励（发给被邀请人自己）
	SettingKeyReferralInviteeOngoingRewardEnabled      = "referral_invitee_ongoing_reward_enabled"       // 普通推广：被邀请人持续充值奖励开关
	SettingKeyReferralInviteeOngoingRewardType         = "referral_invitee_ongoing_reward_type"          // 普通推广：被邀请人持续奖励类型 fixed/percentage
	SettingKeyReferralInviteeOngoingRewardValue        = "referral_invitee_ongoing_reward_value"         // 普通推广：被邀请人持续奖励数值
	SettingKeyReferralInviteeOngoingRewardMaxCount     = "referral_invitee_ongoing_reward_max_count"     // 普通推广：被邀请人持续奖励最大次数（0=不限）
	SettingKeyReferralInviteeOngoingRewardDurationDays = "referral_invitee_ongoing_reward_duration_days" // 普通推广：被邀请人持续奖励有效期天数（0=永久）

	// --- 销售用户推广 ---
	SettingKeyReferralSalesEnabled              = "referral_sales_enabled"                // 销售推广开关：控制销售用户的推广功能（佣金路径）
	SettingKeyReferralSalesInviteeRewardEnabled = "referral_sales_invitee_reward_enabled" // 销售推广：是否启用被邀请人注册奖励
	SettingKeyReferralSalesInviteeReward        = "referral_sales_invitee_reward"         // 被邀请人注册奖励金额（销售用户邀请的新用户）
	// 销售推广：被邀请人首充奖励
	SettingKeyReferralSalesInviteeFirstChargeRewardEnabled = "referral_sales_invitee_first_charge_reward_enabled" // 销售推广：被邀请人首充奖励开关
	SettingKeyReferralSalesInviteeFirstChargeRewardType    = "referral_sales_invitee_first_charge_reward_type"    // 销售推广：被邀请人首充奖励类型 fixed/percentage
	SettingKeyReferralSalesInviteeFirstChargeRewardValue   = "referral_sales_invitee_first_charge_reward_value"   // 销售推广：被邀请人首充奖励数值

	SettingKeyReferralSalesInviteeOngoingRewardEnabled      = "referral_sales_invitee_ongoing_reward_enabled"       // 销售推广：被邀请人持续充值奖励开关
	SettingKeyReferralSalesInviteeOngoingRewardType         = "referral_sales_invitee_ongoing_reward_type"          // 销售推广：被邀请人持续奖励类型 fixed/percentage
	SettingKeyReferralSalesInviteeOngoingRewardValue        = "referral_sales_invitee_ongoing_reward_value"         // 销售推广：被邀请人持续奖励数值
	SettingKeyReferralSalesInviteeOngoingRewardMaxCount     = "referral_sales_invitee_ongoing_reward_max_count"     // 销售推广：被邀请人持续奖励最大次数（0=不限）
	SettingKeyReferralSalesInviteeOngoingRewardDurationDays = "referral_sales_invitee_ongoing_reward_duration_days" // 销售推广：被邀请人持续奖励有效期天数（0=永久）

	// Codex CLI User-Agent 配置
	// SettingKeyCodexCLIUserAgent 发往 OpenAI 上游的 User-Agent 头（默认 "codex_cli_rs/0.144.1"）
	SettingKeyCodexCLIUserAgent = "codex_cli_user_agent"
	// SettingKeyCodexCLIVersion 发往 OpenAI compact 端点的 Version 头（默认 "0.144.1"）
	SettingKeyCodexCLIVersion = "codex_cli_version"
	// SettingKeyCodexPassthroughUAVersion 开启后官方 Codex 客户端请求保留入站 UA/Version
	SettingKeyCodexPassthroughUAVersion = "codex_official_client_passthrough_ua_version"
	// SettingKeyOpenAIUsageDebugLogEnabled 开启后打印 OpenAI 上游 usage 原始响应和计费拆分调试日志（默认 false）
	SettingKeyOpenAIUsageDebugLogEnabled = "openai_usage_debug_log_enabled"
)

const (
	MediaTimeoutBillingPolicyRefund  = "refund"
	MediaTimeoutBillingPolicyPenalty = "penalty"
	MediaVideoStorageModeHybrid      = "hybrid"

	DefaultMediaSyncWaitTimeoutSeconds          = 240
	DefaultMediaSyncTimeoutFallbackAsyncEnabled = false
	DefaultMediaSyncTimeoutPenaltyRatio         = 0.8
	DefaultMediaVideoProxyFallbackEnabled       = true
)

// AdminAPIKeyPrefix is the prefix for admin API keys (distinct from user "sk-" keys).
const AdminAPIKeyPrefix = "admin-"
