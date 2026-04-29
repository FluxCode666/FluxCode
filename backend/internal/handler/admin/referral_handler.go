package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ReferralHandler 管理端推广管理 handler
type ReferralHandler struct {
	referralService *service.ReferralService
	configResolver  *service.ReferralConfigResolver
	settingRepo     service.SettingRepository
	userConfigRepo  service.UserReferralConfigRepository
}

func NewReferralHandler(
	referralService *service.ReferralService,
	configResolver *service.ReferralConfigResolver,
	settingRepo service.SettingRepository,
	userConfigRepo service.UserReferralConfigRepository,
) *ReferralHandler {
	return &ReferralHandler{
		referralService: referralService,
		configResolver:  configResolver,
		settingRepo:     settingRepo,
		userConfigRepo:  userConfigRepo,
	}
}

// GetStats 获取推广总览统计
// GET /api/v1/admin/referral/stats
func (h *ReferralHandler) GetStats(c *gin.Context) {
	stats, err := h.referralService.AdminGetStats(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}

// GetConfig 获取全局推广配置
// GET /api/v1/admin/referral/config
func (h *ReferralHandler) GetConfig(c *gin.Context) {
	cfg := h.configResolver.GetGlobalConfig(c.Request.Context())
	response.Success(c, cfg)
}

// updateConfigRequest 更新全局推广配置请求
type updateConfigRequest struct {
	ReferralEnabled              *string `json:"referral_enabled"`
	ReferralInviteeReward        *string `json:"referral_invitee_reward"`
	ReferralInviterReward        *string `json:"referral_inviter_reward"`
	ReferralMaxInvites           *string `json:"referral_max_invites"`
	ReferralRewardExpiryDays     *string `json:"referral_reward_expiry_days"`
	ReferralOngoingRewardEnabled *string `json:"referral_ongoing_reward_enabled"`
	ReferralOngoingRewardAmount  *string `json:"referral_ongoing_reward_amount"`
	ReferralOngoingRewardPercent *string `json:"referral_ongoing_reward_percent"`
}

// UpdateConfig 更新全局推广配置
// PUT /api/v1/admin/referral/config
func (h *ReferralHandler) UpdateConfig(c *gin.Context) {
	var req updateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	settings := make(map[string]string)
	if req.ReferralEnabled != nil {
		settings[service.SettingKeyReferralEnabled] = *req.ReferralEnabled
	}
	if req.ReferralInviteeReward != nil {
		settings[service.SettingKeyReferralInviteeReward] = *req.ReferralInviteeReward
	}
	if req.ReferralInviterReward != nil {
		settings[service.SettingKeyReferralInviterReward] = *req.ReferralInviterReward
	}
	if req.ReferralMaxInvites != nil {
		settings[service.SettingKeyReferralMaxInvites] = *req.ReferralMaxInvites
	}
	if req.ReferralRewardExpiryDays != nil {
		settings[service.SettingKeyReferralRewardExpiryDays] = *req.ReferralRewardExpiryDays
	}
	if req.ReferralOngoingRewardEnabled != nil {
		settings[service.SettingKeyReferralOngoingRewardEnabled] = *req.ReferralOngoingRewardEnabled
	}
	if req.ReferralOngoingRewardAmount != nil {
		settings[service.SettingKeyReferralOngoingRewardAmount] = *req.ReferralOngoingRewardAmount
	}
	if req.ReferralOngoingRewardPercent != nil {
		settings[service.SettingKeyReferralOngoingRewardPercent] = *req.ReferralOngoingRewardPercent
	}

	if len(settings) > 0 {
		if err := h.settingRepo.SetMultiple(c.Request.Context(), settings); err != nil {
			response.ErrorFrom(c, err)
			return
		}
		// 失效全局缓存
		h.configResolver.InvalidateGlobalCache()
	}

	response.Success(c, gin.H{"message": "ok"})
}

// ListReferrals 列出推广关系
// GET /api/v1/admin/referral/list
func (h *ReferralHandler) ListReferrals(c *gin.Context) {
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	referrals, total, err := h.referralService.AdminListReferrals(c.Request.Context(), status, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"items": referrals,
		"total": total,
		"page":  page,
	})
}

// GetLeaderboard 获取推广排行榜
// GET /api/v1/admin/referral/leaderboard
func (h *ReferralHandler) GetLeaderboard(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	entries, err := h.referralService.AdminGetLeaderboard(c.Request.Context(), limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, entries)
}

// grantGiftBalanceRequest 手动发放赠送余额请求
type grantGiftBalanceRequest struct {
	UserID     int64   `json:"user_id" binding:"required"`
	Amount     float64 `json:"amount" binding:"required,gt=0"`
	ExpiryDays int     `json:"expiry_days"`
	Note       string  `json:"note"`
}

// GrantGiftBalance 手动发放赠送余额
// POST /api/v1/admin/referral/grant-gift-balance
func (h *ReferralHandler) GrantGiftBalance(c *gin.Context) {
	var req grantGiftBalanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := h.referralService.AdminGrantGiftBalance(c.Request.Context(), req.UserID, req.Amount, req.ExpiryDays, req.Note); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

// GetUserConfig 获取用户个性化推广配置
// GET /api/v1/admin/referral/user-config/:userId
func (h *ReferralHandler) GetUserConfig(c *gin.Context) {
	userID, _ := strconv.ParseInt(c.Param("userId"), 10, 64)
	if userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	cfg, err := h.userConfigRepo.GetByUserID(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if cfg == nil {
		response.Success(c, gin.H{"config": nil})
		return
	}
	response.Success(c, gin.H{"config": cfg})
}

// UpsertUserConfig 设置用户个性化推广配置
// PUT /api/v1/admin/referral/user-config/:userId
func (h *ReferralHandler) UpsertUserConfig(c *gin.Context) {
	userID, _ := strconv.ParseInt(c.Param("userId"), 10, 64)
	if userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	var cfg service.UserReferralConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg.UserID = userID

	if err := h.userConfigRepo.Upsert(c.Request.Context(), &cfg); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// 失效该用户缓存
	h.configResolver.InvalidateUserCache(userID)
	response.Success(c, gin.H{"message": "ok"})
}

// DeleteUserConfig 删除用户个性化推广配置
// DELETE /api/v1/admin/referral/user-config/:userId
func (h *ReferralHandler) DeleteUserConfig(c *gin.Context) {
	userID, _ := strconv.ParseInt(c.Param("userId"), 10, 64)
	if userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	if err := h.userConfigRepo.Delete(c.Request.Context(), userID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.configResolver.InvalidateUserCache(userID)
	response.Success(c, gin.H{"message": "ok"})
}
