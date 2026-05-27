package admin

import (
	"strconv"
	"strings"

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

// adminConfigDTO 管理端推广配置（API 契约：使用 type+value 模式）
type adminConfigDTO struct {
	// --- 普通用户推广 ---
	ReferralEnabled               bool    `json:"referral_enabled"`
	ReferralInviteeRewardEnabled  bool    `json:"referral_invitee_reward_enabled"`
	ReferralInviteeReward         float64 `json:"referral_invitee_reward"`
	ReferralInviterReward         float64 `json:"referral_inviter_reward"`
	ReferralMaxInvites            int     `json:"referral_max_invites"`
	ReferralRewardExpiryDays      int     `json:"referral_reward_expiry_days"`
	ReferralGiftBalanceExpiryDays int     `json:"referral_gift_balance_expiry_days"` // 别名
	// --- 普通推广：首充奖励 ---
	ReferralInviterFirstChargeRewardEnabled bool    `json:"referral_inviter_first_charge_reward_enabled"`
	ReferralInviterFirstChargeRewardType    string  `json:"referral_inviter_first_charge_reward_type"`
	ReferralInviterFirstChargeRewardValue   float64 `json:"referral_inviter_first_charge_reward_value"`
	ReferralInviteeFirstChargeRewardEnabled bool    `json:"referral_invitee_first_charge_reward_enabled"`
	ReferralInviteeFirstChargeRewardType    string  `json:"referral_invitee_first_charge_reward_type"`
	ReferralInviteeFirstChargeRewardValue   float64 `json:"referral_invitee_first_charge_reward_value"`
	ReferralOngoingRewardEnabled            bool    `json:"referral_ongoing_reward_enabled"`
	ReferralOngoingRewardType               string  `json:"referral_ongoing_reward_type"`
	ReferralOngoingRewardValue              float64 `json:"referral_ongoing_reward_value"`
	ReferralOngoingRewardMaxCount           int     `json:"referral_ongoing_reward_max_count"`
	ReferralOngoingRewardDurationDays       int     `json:"referral_ongoing_reward_duration_days"`
	// --- 普通推广：被邀请人持续奖励 ---
	ReferralInviteeOngoingRewardEnabled      bool    `json:"referral_invitee_ongoing_reward_enabled"`
	ReferralInviteeOngoingRewardType         string  `json:"referral_invitee_ongoing_reward_type"`
	ReferralInviteeOngoingRewardValue        float64 `json:"referral_invitee_ongoing_reward_value"`
	ReferralInviteeOngoingRewardMaxCount     int     `json:"referral_invitee_ongoing_reward_max_count"`
	ReferralInviteeOngoingRewardDurationDays int     `json:"referral_invitee_ongoing_reward_duration_days"`
	// --- 销售用户推广 ---
	ReferralSalesEnabled                          bool    `json:"referral_sales_enabled"`
	ReferralSalesInviteeRewardEnabled             bool    `json:"referral_sales_invitee_reward_enabled"`
	ReferralSalesInviteeReward                    float64 `json:"referral_sales_invitee_reward"`
	ReferralSalesInviteeFirstChargeRewardEnabled  bool    `json:"referral_sales_invitee_first_charge_reward_enabled"`
	ReferralSalesInviteeFirstChargeRewardType     string  `json:"referral_sales_invitee_first_charge_reward_type"`
	ReferralSalesInviteeFirstChargeRewardValue    float64 `json:"referral_sales_invitee_first_charge_reward_value"`
	ReferralSalesInviteeOngoingRewardEnabled      bool    `json:"referral_sales_invitee_ongoing_reward_enabled"`
	ReferralSalesInviteeOngoingRewardType         string  `json:"referral_sales_invitee_ongoing_reward_type"`
	ReferralSalesInviteeOngoingRewardValue        float64 `json:"referral_sales_invitee_ongoing_reward_value"`
	ReferralSalesInviteeOngoingRewardMaxCount     int     `json:"referral_sales_invitee_ongoing_reward_max_count"`
	ReferralSalesInviteeOngoingRewardDurationDays int     `json:"referral_sales_invitee_ongoing_reward_duration_days"`
}

// GetConfig 获取全局推广配置
// GET /api/v1/admin/referral/config
func (h *ReferralHandler) GetConfig(c *gin.Context) {
	cfg := h.configResolver.GetGlobalConfig(c.Request.Context())

	rewardType := cfg.OngoingRewardType
	if rewardType == "" {
		rewardType = "fixed"
	}

	salesOngoingType := cfg.SalesInviteeOngoingRewardType
	if salesOngoingType == "" {
		salesOngoingType = "fixed"
	}

	inviteeOngoingType := cfg.InviteeOngoingRewardType
	if inviteeOngoingType == "" {
		inviteeOngoingType = "fixed"
	}

	inviterFirstChargeType := cfg.InviterFirstChargeRewardType
	if inviterFirstChargeType == "" {
		inviterFirstChargeType = "fixed"
	}
	inviteeFirstChargeType := cfg.InviteeFirstChargeRewardType
	if inviteeFirstChargeType == "" {
		inviteeFirstChargeType = "fixed"
	}

	response.Success(c, adminConfigDTO{
		ReferralEnabled:                               cfg.Enabled,
		ReferralInviteeRewardEnabled:                  cfg.InviteeRewardEnabled,
		ReferralInviteeReward:                         cfg.InviteeRewardAmount,
		ReferralInviterReward:                         cfg.InviterRewardAmount,
		ReferralMaxInvites:                            cfg.MaxInvites,
		ReferralRewardExpiryDays:                      cfg.RewardExpiryDays,
		ReferralGiftBalanceExpiryDays:                 cfg.RewardExpiryDays,
		ReferralInviterFirstChargeRewardEnabled:       cfg.InviterFirstChargeRewardEnabled,
		ReferralInviterFirstChargeRewardType:          inviterFirstChargeType,
		ReferralInviterFirstChargeRewardValue:         cfg.InviterFirstChargeRewardValue,
		ReferralInviteeFirstChargeRewardEnabled:       cfg.InviteeFirstChargeRewardEnabled,
		ReferralInviteeFirstChargeRewardType:          inviteeFirstChargeType,
		ReferralInviteeFirstChargeRewardValue:         cfg.InviteeFirstChargeRewardValue,
		ReferralOngoingRewardEnabled:                  cfg.OngoingRewardEnabled,
		ReferralOngoingRewardType:                     rewardType,
		ReferralOngoingRewardValue:                    cfg.OngoingRewardValue,
		ReferralOngoingRewardMaxCount:                 cfg.OngoingRewardMaxCount,
		ReferralOngoingRewardDurationDays:             cfg.OngoingRewardDurationDays,
		ReferralInviteeOngoingRewardEnabled:           cfg.InviteeOngoingRewardEnabled,
		ReferralInviteeOngoingRewardType:              inviteeOngoingType,
		ReferralInviteeOngoingRewardValue:             cfg.InviteeOngoingRewardValue,
		ReferralInviteeOngoingRewardMaxCount:          cfg.InviteeOngoingRewardMaxCount,
		ReferralInviteeOngoingRewardDurationDays:      cfg.InviteeOngoingRewardDurationDays,
		ReferralSalesEnabled:                          cfg.SalesEnabled,
		ReferralSalesInviteeRewardEnabled:             cfg.SalesInviteeRewardEnabled,
		ReferralSalesInviteeReward:                    cfg.SalesInviteeRewardAmount,
		ReferralSalesInviteeFirstChargeRewardEnabled:  cfg.SalesInviteeFirstChargeRewardEnabled,
		ReferralSalesInviteeFirstChargeRewardType:     cfg.SalesInviteeFirstChargeRewardType,
		ReferralSalesInviteeFirstChargeRewardValue:    cfg.SalesInviteeFirstChargeRewardValue,
		ReferralSalesInviteeOngoingRewardEnabled:      cfg.SalesInviteeOngoingRewardEnabled,
		ReferralSalesInviteeOngoingRewardType:         salesOngoingType,
		ReferralSalesInviteeOngoingRewardValue:        cfg.SalesInviteeOngoingRewardValue,
		ReferralSalesInviteeOngoingRewardMaxCount:     cfg.SalesInviteeOngoingRewardMaxCount,
		ReferralSalesInviteeOngoingRewardDurationDays: cfg.SalesInviteeOngoingRewardDurationDays,
	})
}

// updateConfigRequest 更新全局推广配置请求
// 使用 *T 指针字段：nil = 不修改对应配置
type updateConfigRequest struct {
	// --- 普通用户推广 ---
	ReferralEnabled               *bool    `json:"referral_enabled"`
	ReferralInviteeRewardEnabled  *bool    `json:"referral_invitee_reward_enabled"`
	ReferralInviteeReward         *float64 `json:"referral_invitee_reward"`
	ReferralInviterReward         *float64 `json:"referral_inviter_reward"`
	ReferralMaxInvites            *int     `json:"referral_max_invites"`
	ReferralRewardExpiryDays      *int     `json:"referral_reward_expiry_days"`
	ReferralGiftBalanceExpiryDays *int     `json:"referral_gift_balance_expiry_days"` // 别名
	// --- 普通推广：首充奖励 ---
	ReferralInviterFirstChargeRewardEnabled *bool    `json:"referral_inviter_first_charge_reward_enabled"`
	ReferralInviterFirstChargeRewardType    *string  `json:"referral_inviter_first_charge_reward_type"`
	ReferralInviterFirstChargeRewardValue   *float64 `json:"referral_inviter_first_charge_reward_value"`
	ReferralInviteeFirstChargeRewardEnabled *bool    `json:"referral_invitee_first_charge_reward_enabled"`
	ReferralInviteeFirstChargeRewardType    *string  `json:"referral_invitee_first_charge_reward_type"`
	ReferralInviteeFirstChargeRewardValue   *float64 `json:"referral_invitee_first_charge_reward_value"`
	ReferralOngoingRewardEnabled            *bool    `json:"referral_ongoing_reward_enabled"`
	ReferralOngoingRewardType               *string  `json:"referral_ongoing_reward_type"`
	ReferralOngoingRewardValue              *float64 `json:"referral_ongoing_reward_value"`
	ReferralOngoingRewardMaxCount           *int     `json:"referral_ongoing_reward_max_count"`
	ReferralOngoingRewardDurationDays       *int     `json:"referral_ongoing_reward_duration_days"`
	// --- 普通推广：被邀请人持续奖励 ---
	ReferralInviteeOngoingRewardEnabled      *bool    `json:"referral_invitee_ongoing_reward_enabled"`
	ReferralInviteeOngoingRewardType         *string  `json:"referral_invitee_ongoing_reward_type"`
	ReferralInviteeOngoingRewardValue        *float64 `json:"referral_invitee_ongoing_reward_value"`
	ReferralInviteeOngoingRewardMaxCount     *int     `json:"referral_invitee_ongoing_reward_max_count"`
	ReferralInviteeOngoingRewardDurationDays *int     `json:"referral_invitee_ongoing_reward_duration_days"`
	// --- 销售用户推广 ---
	ReferralSalesEnabled                          *bool    `json:"referral_sales_enabled"`
	ReferralSalesInviteeRewardEnabled             *bool    `json:"referral_sales_invitee_reward_enabled"`
	ReferralSalesInviteeReward                    *float64 `json:"referral_sales_invitee_reward"`
	ReferralSalesInviteeFirstChargeRewardEnabled  *bool    `json:"referral_sales_invitee_first_charge_reward_enabled"`
	ReferralSalesInviteeFirstChargeRewardType     *string  `json:"referral_sales_invitee_first_charge_reward_type"`
	ReferralSalesInviteeFirstChargeRewardValue    *float64 `json:"referral_sales_invitee_first_charge_reward_value"`
	ReferralSalesInviteeOngoingRewardEnabled      *bool    `json:"referral_sales_invitee_ongoing_reward_enabled"`
	ReferralSalesInviteeOngoingRewardType         *string  `json:"referral_sales_invitee_ongoing_reward_type"`
	ReferralSalesInviteeOngoingRewardValue        *float64 `json:"referral_sales_invitee_ongoing_reward_value"`
	ReferralSalesInviteeOngoingRewardMaxCount     *int     `json:"referral_sales_invitee_ongoing_reward_max_count"`
	ReferralSalesInviteeOngoingRewardDurationDays *int     `json:"referral_sales_invitee_ongoing_reward_duration_days"`
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
		settings[service.SettingKeyReferralEnabled] = strconv.FormatBool(*req.ReferralEnabled)
	}
	if req.ReferralInviteeReward != nil {
		settings[service.SettingKeyReferralInviteeReward] = strconv.FormatFloat(*req.ReferralInviteeReward, 'f', -1, 64)
	}
	if req.ReferralInviterReward != nil {
		settings[service.SettingKeyReferralInviterReward] = strconv.FormatFloat(*req.ReferralInviterReward, 'f', -1, 64)
	}
	if req.ReferralMaxInvites != nil {
		settings[service.SettingKeyReferralMaxInvites] = strconv.Itoa(*req.ReferralMaxInvites)
	}
	// 优先 reward_expiry_days，没传则用 gift_balance_expiry_days 别名
	if req.ReferralRewardExpiryDays != nil {
		settings[service.SettingKeyReferralRewardExpiryDays] = strconv.Itoa(*req.ReferralRewardExpiryDays)
	} else if req.ReferralGiftBalanceExpiryDays != nil {
		settings[service.SettingKeyReferralRewardExpiryDays] = strconv.Itoa(*req.ReferralGiftBalanceExpiryDays)
	}
	// --- 首充奖励 ---
	if req.ReferralInviterFirstChargeRewardEnabled != nil {
		settings[service.SettingKeyReferralInviterFirstChargeRewardEnabled] = strconv.FormatBool(*req.ReferralInviterFirstChargeRewardEnabled)
	}
	if req.ReferralInviterFirstChargeRewardType != nil {
		t := strings.ToLower(strings.TrimSpace(*req.ReferralInviterFirstChargeRewardType))
		if t != "percentage" {
			t = "fixed"
		}
		settings[service.SettingKeyReferralInviterFirstChargeRewardType] = t
	}
	if req.ReferralInviterFirstChargeRewardValue != nil {
		settings[service.SettingKeyReferralInviterFirstChargeRewardValue] = strconv.FormatFloat(*req.ReferralInviterFirstChargeRewardValue, 'f', -1, 64)
	}
	if req.ReferralInviteeFirstChargeRewardEnabled != nil {
		settings[service.SettingKeyReferralInviteeFirstChargeRewardEnabled] = strconv.FormatBool(*req.ReferralInviteeFirstChargeRewardEnabled)
	}
	if req.ReferralInviteeFirstChargeRewardType != nil {
		t := strings.ToLower(strings.TrimSpace(*req.ReferralInviteeFirstChargeRewardType))
		if t != "percentage" {
			t = "fixed"
		}
		settings[service.SettingKeyReferralInviteeFirstChargeRewardType] = t
	}
	if req.ReferralInviteeFirstChargeRewardValue != nil {
		settings[service.SettingKeyReferralInviteeFirstChargeRewardValue] = strconv.FormatFloat(*req.ReferralInviteeFirstChargeRewardValue, 'f', -1, 64)
	}
	if req.ReferralOngoingRewardEnabled != nil {
		settings[service.SettingKeyReferralOngoingRewardEnabled] = strconv.FormatBool(*req.ReferralOngoingRewardEnabled)
	}
	if req.ReferralOngoingRewardMaxCount != nil {
		settings[service.SettingKeyReferralOngoingRewardMaxCount] = strconv.Itoa(*req.ReferralOngoingRewardMaxCount)
	}
	if req.ReferralOngoingRewardDurationDays != nil {
		settings[service.SettingKeyReferralOngoingRewardDurationDays] = strconv.Itoa(*req.ReferralOngoingRewardDurationDays)
	}

	if req.ReferralOngoingRewardType != nil {
		rewardType := strings.ToLower(strings.TrimSpace(*req.ReferralOngoingRewardType))
		if rewardType != "percentage" {
			rewardType = "fixed"
		}
		settings[service.SettingKeyReferralOngoingRewardType] = rewardType
	}
	if req.ReferralOngoingRewardValue != nil {
		settings[service.SettingKeyReferralOngoingRewardValue] = strconv.FormatFloat(*req.ReferralOngoingRewardValue, 'f', -1, 64)
	}
	// --- 普通推广注册奖励开关 ---
	if req.ReferralInviteeRewardEnabled != nil {
		settings[service.SettingKeyReferralInviteeRewardEnabled] = strconv.FormatBool(*req.ReferralInviteeRewardEnabled)
	}
	// --- 普通推广：被邀请人持续奖励 ---
	if req.ReferralInviteeOngoingRewardEnabled != nil {
		settings[service.SettingKeyReferralInviteeOngoingRewardEnabled] = strconv.FormatBool(*req.ReferralInviteeOngoingRewardEnabled)
	}
	if req.ReferralInviteeOngoingRewardType != nil {
		t := strings.ToLower(strings.TrimSpace(*req.ReferralInviteeOngoingRewardType))
		if t != "percentage" {
			t = "fixed"
		}
		settings[service.SettingKeyReferralInviteeOngoingRewardType] = t
	}
	if req.ReferralInviteeOngoingRewardValue != nil {
		settings[service.SettingKeyReferralInviteeOngoingRewardValue] = strconv.FormatFloat(*req.ReferralInviteeOngoingRewardValue, 'f', -1, 64)
	}
	if req.ReferralInviteeOngoingRewardMaxCount != nil {
		settings[service.SettingKeyReferralInviteeOngoingRewardMaxCount] = strconv.Itoa(*req.ReferralInviteeOngoingRewardMaxCount)
	}
	if req.ReferralInviteeOngoingRewardDurationDays != nil {
		settings[service.SettingKeyReferralInviteeOngoingRewardDurationDays] = strconv.Itoa(*req.ReferralInviteeOngoingRewardDurationDays)
	}
	// --- 销售用户推广 ---
	if req.ReferralSalesEnabled != nil {
		settings[service.SettingKeyReferralSalesEnabled] = strconv.FormatBool(*req.ReferralSalesEnabled)
	}
	if req.ReferralSalesInviteeRewardEnabled != nil {
		settings[service.SettingKeyReferralSalesInviteeRewardEnabled] = strconv.FormatBool(*req.ReferralSalesInviteeRewardEnabled)
	}
	if req.ReferralSalesInviteeReward != nil {
		settings[service.SettingKeyReferralSalesInviteeReward] = strconv.FormatFloat(*req.ReferralSalesInviteeReward, 'f', -1, 64)
	}
	if req.ReferralSalesInviteeFirstChargeRewardEnabled != nil {
		settings[service.SettingKeyReferralSalesInviteeFirstChargeRewardEnabled] = strconv.FormatBool(*req.ReferralSalesInviteeFirstChargeRewardEnabled)
	}
	if req.ReferralSalesInviteeFirstChargeRewardType != nil {
		st := strings.ToLower(strings.TrimSpace(*req.ReferralSalesInviteeFirstChargeRewardType))
		if st != "percentage" {
			st = "fixed"
		}
		settings[service.SettingKeyReferralSalesInviteeFirstChargeRewardType] = st
	}
	if req.ReferralSalesInviteeFirstChargeRewardValue != nil {
		settings[service.SettingKeyReferralSalesInviteeFirstChargeRewardValue] = strconv.FormatFloat(*req.ReferralSalesInviteeFirstChargeRewardValue, 'f', -1, 64)
	}
	if req.ReferralSalesInviteeOngoingRewardEnabled != nil {
		settings[service.SettingKeyReferralSalesInviteeOngoingRewardEnabled] = strconv.FormatBool(*req.ReferralSalesInviteeOngoingRewardEnabled)
	}
	if req.ReferralSalesInviteeOngoingRewardType != nil {
		st := strings.ToLower(strings.TrimSpace(*req.ReferralSalesInviteeOngoingRewardType))
		if st != "percentage" {
			st = "fixed"
		}
		settings[service.SettingKeyReferralSalesInviteeOngoingRewardType] = st
	}
	if req.ReferralSalesInviteeOngoingRewardValue != nil {
		settings[service.SettingKeyReferralSalesInviteeOngoingRewardValue] = strconv.FormatFloat(*req.ReferralSalesInviteeOngoingRewardValue, 'f', -1, 64)
	}
	if req.ReferralSalesInviteeOngoingRewardMaxCount != nil {
		settings[service.SettingKeyReferralSalesInviteeOngoingRewardMaxCount] = strconv.Itoa(*req.ReferralSalesInviteeOngoingRewardMaxCount)
	}
	if req.ReferralSalesInviteeOngoingRewardDurationDays != nil {
		settings[service.SettingKeyReferralSalesInviteeOngoingRewardDurationDays] = strconv.Itoa(*req.ReferralSalesInviteeOngoingRewardDurationDays)
	}

	if len(settings) > 0 {
		if err := h.settingRepo.SetMultiple(c.Request.Context(), settings); err != nil {
			response.ErrorFrom(c, err)
			return
		}
		h.configResolver.InvalidateGlobalCache()
	}

	response.Success(c, gin.H{"message": "ok"})
}

// ListReferrals 列出推广关系
// GET /api/v1/admin/referral/list
func (h *ReferralHandler) ListReferrals(c *gin.Context) {
	status := c.Query("status")
	referrerID, _ := strconv.ParseInt(c.Query("referrer_id"), 10, 64)
	refereeID, _ := strconv.ParseInt(c.Query("referee_id"), 10, 64)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	referrals, total, err := h.referralService.AdminListReferrals(c.Request.Context(), status, referrerID, refereeID, page, pageSize)
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
	period := c.DefaultQuery("period", "all_time")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	entries, err := h.referralService.AdminGetLeaderboard(c.Request.Context(), period, startDate, endDate, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, entries)
}

// GetDashboard 获取推广数据看板（转化漏斗 + 趋势）
// GET /api/v1/admin/referral/dashboard
func (h *ReferralHandler) GetDashboard(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	if days <= 0 || days > 365 {
		days = 30
	}
	dashboard, err := h.referralService.AdminGetDashboard(c.Request.Context(), days)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dashboard)
}

// grantGiftBalanceRequest 手动发放赠送余额请求
type grantGiftBalanceRequest struct {
	UserID     int64   `json:"user_id" binding:"required"`
	Amount     float64 `json:"amount" binding:"required,gt=0"`
	ExpiryDays int     `json:"expiry_days"`
	Notes      string  `json:"notes"`
}

type markCompletedRequest struct {
	Notes               string  `json:"notes"`
	OrderPayAmountCNY   float64 `json:"order_pay_amount_cny"`
	OrderCreditedAmount float64 `json:"order_credited_amount"`
}

// GrantGiftBalance 手动发放赠送余额
// POST /api/v1/admin/referral/grant-gift-balance
func (h *ReferralHandler) GrantGiftBalance(c *gin.Context) {
	var req grantGiftBalanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := h.referralService.AdminGrantGiftBalance(c.Request.Context(), req.UserID, req.Amount, req.ExpiryDays, req.Notes); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

// MarkCompleted 手动完成推广关系
// POST /api/v1/admin/referral/:id/mark-completed
func (h *ReferralHandler) MarkCompleted(c *gin.Context) {
	referralID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if referralID <= 0 {
		response.BadRequest(c, "Invalid referral ID")
		return
	}

	var req markCompletedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Notes) == "" {
		response.BadRequest(c, "notes is required")
		return
	}

	if err := h.referralService.AdminMarkReferralCompleted(
		c.Request.Context(),
		referralID,
		req.Notes,
		req.OrderPayAmountCNY,
		req.OrderCreditedAmount,
	); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

// batchGrantRequest 批量发放赠送余额请求
type batchGrantRequest struct {
	Target     string  `json:"target" binding:"required"` // "all" | "selected"
	UserIDs    []int64 `json:"user_ids"`
	Amount     float64 `json:"amount" binding:"required,gt=0"`
	ExpiryDays int     `json:"expiry_days"`
	Notes      string  `json:"notes"`
}

// BatchGrantGiftBalance 批量发放赠送余额
// POST /api/v1/admin/referral/grant-batch
func (h *ReferralHandler) BatchGrantGiftBalance(c *gin.Context) {
	var req batchGrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	target := strings.ToLower(strings.TrimSpace(req.Target))
	if target != "all" && target != "selected" {
		response.BadRequest(c, "target must be 'all' or 'selected'")
		return
	}
	if target == "selected" && len(req.UserIDs) == 0 {
		response.BadRequest(c, "user_ids required when target=selected")
		return
	}

	count, err := h.referralService.AdminBatchGrantGiftBalance(c.Request.Context(), target, req.UserIDs, req.Amount, req.ExpiryDays, req.Notes)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"granted_count": count})
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

	effective := h.configResolver.Resolve(c.Request.Context(), userID)
	hasCustom := cfg != nil

	response.Success(c, gin.H{
		"has_custom_config": hasCustom,
		"config":            cfg,
		"effective":         effective,
	})
}

// userConfigRequest 用户级推广配置请求（type+value 模式）
type userConfigRequest struct {
	InviteeReward                   *float64 `json:"invitee_reward"`
	InviterReward                   *float64 `json:"inviter_reward"`
	MaxInvites                      *int     `json:"max_invites"`
	RewardExpiryDays                *int     `json:"reward_expiry_days"`
	InviterFirstChargeRewardEnabled *bool    `json:"inviter_first_charge_reward_enabled"`
	InviterFirstChargeRewardType    *string  `json:"inviter_first_charge_reward_type"`
	InviterFirstChargeRewardValue   *float64 `json:"inviter_first_charge_reward_value"`
	InviteeFirstChargeRewardEnabled *bool    `json:"invitee_first_charge_reward_enabled"`
	InviteeFirstChargeRewardType    *string  `json:"invitee_first_charge_reward_type"`
	InviteeFirstChargeRewardValue   *float64 `json:"invitee_first_charge_reward_value"`
	OngoingRewardEnabled            *bool    `json:"ongoing_reward_enabled"`
	OngoingRewardType               *string  `json:"ongoing_reward_type"`
	OngoingRewardValue              *float64 `json:"ongoing_reward_value"`
	OngoingRewardMaxCount           *int     `json:"ongoing_reward_max_count"`
	OngoingRewardDurationDays       *int     `json:"ongoing_reward_duration_days"`
	Notes                           string   `json:"notes"`
}

// UpsertUserConfig 设置用户个性化推广配置
// PUT /api/v1/admin/referral/user-config/:userId
func (h *ReferralHandler) UpsertUserConfig(c *gin.Context) {
	userID, _ := strconv.ParseInt(c.Param("userId"), 10, 64)
	if userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	var req userConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	cfg := &service.UserReferralConfig{
		UserID:                          userID,
		InviteeRewardAmount:             req.InviteeReward,
		InviterRewardAmount:             req.InviterReward,
		MaxInvites:                      req.MaxInvites,
		RewardExpiryDays:                req.RewardExpiryDays,
		InviterFirstChargeRewardEnabled: req.InviterFirstChargeRewardEnabled,
		InviterFirstChargeRewardType:    req.InviterFirstChargeRewardType,
		InviterFirstChargeRewardValue:   req.InviterFirstChargeRewardValue,
		InviteeFirstChargeRewardEnabled: req.InviteeFirstChargeRewardEnabled,
		InviteeFirstChargeRewardType:    req.InviteeFirstChargeRewardType,
		InviteeFirstChargeRewardValue:   req.InviteeFirstChargeRewardValue,
		OngoingRewardEnabled:            req.OngoingRewardEnabled,
		OngoingRewardMaxCount:           req.OngoingRewardMaxCount,
		OngoingRewardDurationDays:       req.OngoingRewardDurationDays,
		Notes:                           req.Notes,
	}

	if req.OngoingRewardType != nil {
		rewardType := strings.ToLower(strings.TrimSpace(*req.OngoingRewardType))
		if rewardType != "percentage" {
			rewardType = "fixed"
		}
		cfg.OngoingRewardType = &rewardType
	}
	if req.OngoingRewardValue != nil {
		v := *req.OngoingRewardValue
		cfg.OngoingRewardValue = &v
	}

	if err := h.userConfigRepo.Upsert(c.Request.Context(), cfg); err != nil {
		response.ErrorFrom(c, err)
		return
	}
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

// offlineRechargeRequest 录入私账充值请求
type offlineRechargeRequest struct {
	UserID         int64   `json:"user_id" binding:"required,min=1"`
	PayAmountCNY   float64 `json:"pay_amount_cny" binding:"required,gt=0"`
	CreditedAmount float64 `json:"credited_amount" binding:"required,gt=0"`
	CreditBalance  bool    `json:"credit_balance"`
	Note           string  `json:"note"`
}

// RecordOfflineRecharge 录入私账充值
// POST /api/v1/admin/referral/offline-recharge
func (h *ReferralHandler) RecordOfflineRecharge(c *gin.Context) {
	var req offlineRechargeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	result, err := h.referralService.RecordOfflineRecharge(c.Request.Context(), &service.OfflineRechargeInput{
		UserID:         req.UserID,
		PayAmountCNY:   req.PayAmountCNY,
		CreditedAmount: req.CreditedAmount,
		CreditBalance:  req.CreditBalance,
		Note:           req.Note,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
