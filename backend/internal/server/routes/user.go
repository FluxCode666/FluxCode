package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterUserRoutes 注册用户相关路由，并挂载官网需要的公开只读路由。
func RegisterUserRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
	settingService *service.SettingService,
	userAccessKeyAuth ...middleware.UserAccessKeyAuthMiddleware,
) {
	// 官网渠道状态页使用这组只读接口，允许未登录访问。
	monitors := v1.Group("/channel-monitors")
	{
		monitors.GET("", h.ChannelMonitor.List)
		monitors.GET("/:id/status", h.ChannelMonitor.GetStatus)
	}

	authenticated := v1.Group("")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	authenticated.Use(middleware.BackendModeUserGuard(settingService))
	{
		// 用户接口
		user := authenticated.Group("/user")
		{
			user.GET("/profile", h.User.GetProfile)
			user.GET("/access-key", h.UserAccessKey.GetAccessKey)
			user.POST("/access-key", h.UserAccessKey.CreateAccessKey)
			user.PUT("/password", h.User.ChangePassword)
			user.PUT("", h.User.UpdateProfile)
			user.POST("/legal-consent", h.User.AcceptLegalTerms)
			user.GET("/ui-preferences", h.User.GetUIPreferences)
			user.PUT("/ui-preferences", h.User.UpdateUIPreferences)

			// 通知邮箱管理
			notifyEmail := user.Group("/notify-email")
			{
				notifyEmail.POST("/send-code", h.User.SendNotifyEmailCode)
				notifyEmail.POST("/verify", h.User.VerifyNotifyEmail)
				notifyEmail.PUT("/toggle", h.User.ToggleNotifyEmail)
				notifyEmail.DELETE("", h.User.RemoveNotifyEmail)
			}

			// TOTP 双因素认证
			totp := user.Group("/totp")
			{
				totp.GET("/status", h.Totp.GetStatus)
				totp.GET("/verification-method", h.Totp.GetVerificationMethod)
				totp.POST("/send-code", h.Totp.SendVerifyCode)
				totp.POST("/setup", h.Totp.InitiateSetup)
				totp.POST("/enable", h.Totp.Enable)
				totp.POST("/disable", h.Totp.Disable)
			}

		}

		// 推广中心（与设计文档对齐：/api/v1/referral/*）
		if h.Referral != nil {
			referral := authenticated.Group("/referral")
			{
				referral.GET("/info", h.Referral.GetReferralInfo)
				referral.POST("/generate-code", h.Referral.GenerateReferralCode)
				referral.GET("/invites", h.Referral.GetMyReferrals)
				referral.GET("/stats", h.Referral.GetMyStats)
				referral.GET("/gift-balance", h.Referral.GetMyGiftBalanceRecords)
				referral.GET("/gift-balance/remaining", h.Referral.GetGiftBalanceRemaining)
				referral.GET("/gift-balance/overview", h.Referral.GetGiftBalanceOverview)
				referral.GET("/gift-balance/summary", h.Referral.GetMyGiftBalanceSummary)
			}
		}

		if h.SalesCommission != nil {
			salesCommissions := authenticated.Group("/sales-commissions")
			{
				salesCommissions.GET("/summary", h.SalesCommission.GetSummary)
				salesCommissions.GET("/records", h.SalesCommission.ListRecords)
				salesCommissions.GET("/monthly-progress", h.SalesCommission.GetMonthlyProgress)
			}
		}

		// API Key管理
		keys := authenticated.Group("/keys")
		{
			keys.GET("", h.APIKey.List)
			keys.GET("/:id", h.APIKey.GetByID)
			keys.POST("", h.APIKey.Create)
			keys.PUT("/:id", h.APIKey.Update)
			keys.DELETE("/:id", h.APIKey.Delete)
		}

		// 用户可用分组（非管理员接口）
		groups := authenticated.Group("/groups")
		{
			groups.GET("/available", h.APIKey.GetAvailableGroups)
			groups.GET("/rates", h.APIKey.GetUserGroupRates)
		}

		// 使用记录
		usage := authenticated.Group("/usage")
		{
			usage.GET("", h.Usage.List)
			usage.GET("/:id", h.Usage.GetByID)
			usage.GET("/stats", h.Usage.Stats)
			// User dashboard endpoints
			usage.GET("/dashboard/stats", h.Usage.DashboardStats)
			usage.GET("/dashboard/trend", h.Usage.DashboardTrend)
			usage.GET("/dashboard/models", h.Usage.DashboardModels)
			usage.POST("/dashboard/api-keys-usage", h.Usage.DashboardAPIKeysUsage)
		}

		// 公告（用户可见）
		announcements := authenticated.Group("/announcements")
		{
			announcements.GET("", h.Announcement.List)
			announcements.POST("/:id/read", h.Announcement.MarkRead)
		}

		// 卡密兑换
		redeem := authenticated.Group("/redeem")
		{
			redeem.POST("", h.Redeem.Redeem)
			redeem.GET("/history", h.Redeem.GetHistory)
		}

		// 用户订阅
		subscriptions := authenticated.Group("/subscriptions")
		{
			subscriptions.GET("", h.Subscription.List)
			subscriptions.GET("/active", h.Subscription.GetActive)
			subscriptions.GET("/progress", h.Subscription.GetProgress)
			subscriptions.GET("/summary", h.Subscription.GetSummary)
			subscriptions.GET("/:id/grants", h.Subscription.GetMyGrants)
		}
	}

	// 用户级开发者接口：仅接受专用用户访问密钥，不复用 JWT 或模型 API Key。
	// 保持该组最小权限，只公开当前用户自己的资源。
	if len(userAccessKeyAuth) > 0 && userAccessKeyAuth[0] != nil && h.UserAccessKey != nil && h.APIKey != nil {
		developer := v1.Group("/openapi")
		developer.Use(gin.HandlerFunc(userAccessKeyAuth[0]))
		developer.Use(middleware.BackendModeUserGuard(settingService))
		{
			developer.GET("/balance", h.UserAccessKey.GetBalance)
			developer.GET("/keys", h.APIKey.List)
			developer.GET("/keys/:id", h.APIKey.GetByID)
			developer.POST("/keys", h.APIKey.Create)
			developer.PUT("/keys/:id", h.APIKey.Update)
			developer.DELETE("/keys/:id", h.APIKey.Delete)
			developer.GET("/groups/available", h.APIKey.GetAvailableGroups)

			// 使用记录沿用用户端同一处理器，确保分页、筛选和所有权校验完全一致。
			if h.Usage != nil {
				usage := developer.Group("/usage")
				{
					usage.GET("", h.Usage.List)
					usage.GET("/stats", h.Usage.Stats)
					usage.GET("/:id", h.Usage.GetByID)
				}
			}
		}
	}
}
