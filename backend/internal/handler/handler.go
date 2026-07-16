package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
)

// AdminHandlers contains all admin-related HTTP handlers
type AdminHandlers struct {
	Dashboard              *admin.DashboardHandler
	User                   *admin.UserHandler
	Group                  *admin.GroupHandler
	Account                *admin.AccountHandler
	Announcement           *admin.AnnouncementHandler
	DataManagement         *admin.DataManagementHandler
	Backup                 *admin.BackupHandler
	OAuth                  *admin.OAuthHandler
	OpenAIOAuth            *admin.OpenAIOAuthHandler
	GeminiOAuth            *admin.GeminiOAuthHandler
	AntigravityOAuth       *admin.AntigravityOAuthHandler
	Proxy                  *admin.ProxyHandler
	Redeem                 *admin.RedeemHandler
	Promo                  *admin.PromoHandler
	Setting                *admin.SettingHandler
	Ops                    *admin.OpsHandler
	System                 *admin.SystemHandler
	Subscription           *admin.SubscriptionHandler
	Usage                  *admin.UsageHandler
	GeneratedImage         *admin.GeneratedImageHandler
	UserAttribute          *admin.UserAttributeHandler
	ErrorPassthrough       *admin.ErrorPassthroughHandler
	TLSFingerprintProfile  *admin.TLSFingerprintProfileHandler
	APIKey                 *admin.AdminAPIKeyHandler
	ScheduledTest          *admin.ScheduledTestHandler
	PoolMonitor            *admin.PoolMonitorHandler
	Channel                *admin.ChannelHandler
	ChannelMonitor         *admin.ChannelMonitorHandler
	ChannelMonitorTemplate *admin.ChannelMonitorRequestTemplateHandler
	Payment                *admin.PaymentHandler
	Referral               *admin.ReferralHandler
	SalesCommission        *admin.SalesCommissionHandler
	Promotion              *admin.PromotionHandler
}

// Handlers contains all HTTP handlers
type Handlers struct {
	Auth            *AuthHandler
	User            *UserHandler
	APIKey          *APIKeyHandler
	Usage           *UsageHandler
	Redeem          *RedeemHandler
	Subscription    *SubscriptionHandler
	Announcement    *AnnouncementHandler
	Admin           *AdminHandlers
	Gateway         *GatewayHandler
	OpenAIGateway   *OpenAIGatewayHandler
	Setting         *SettingHandler
	Totp            *TotpHandler
	ModelPricing    *ModelPricingHandler
	Payment         *PaymentHandler
	PaymentWebhook  *PaymentWebhookHandler
	Referral        *ReferralHandler
	SalesCommission *SalesCommissionHandler
	ChannelMonitor  *ChannelMonitorUserHandler
	MediaTask       *MediaTaskHandler
}

// BuildInfo contains build-time information
type BuildInfo struct {
	Version   string
	BuildType string // "source" for manual builds, "release" for CI builds
}
