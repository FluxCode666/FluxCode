package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ReferralHandler 用户端推广相关 handler
type ReferralHandler struct {
	referralService *service.ReferralService
}

func NewReferralHandler(referralService *service.ReferralService) *ReferralHandler {
	return &ReferralHandler{referralService: referralService}
}

// GetReferralInfo 获取推广中心信息
// GET /api/v1/user/referral/info
func (h *ReferralHandler) GetReferralInfo(c *gin.Context) {
	userID := c.GetInt64("user_id")
	info, err := h.referralService.GetUserReferralInfo(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, info)
}

// GenerateReferralCode 生成推广码
// POST /api/v1/user/referral/generate-code
func (h *ReferralHandler) GenerateReferralCode(c *gin.Context) {
	userID := c.GetInt64("user_id")
	code, err := h.referralService.GenerateReferralCode(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"referral_code": code})
}

// GetMyReferrals 获取我的邀请列表
// GET /api/v1/user/referral/invites
func (h *ReferralHandler) GetMyReferrals(c *gin.Context) {
	userID := c.GetInt64("user_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	referrals, total, err := h.referralService.GetMyReferrals(c.Request.Context(), userID, page, pageSize)
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

// GetMyGiftBalanceRecords 获取赠送余额记录
// GET /api/v1/user/referral/gift-balance
func (h *ReferralHandler) GetMyGiftBalanceRecords(c *gin.Context) {
	userID := c.GetInt64("user_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	records, total, err := h.referralService.GetMyGiftBalanceRecords(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"items": records,
		"total": total,
		"page":  page,
	})
}

// GetGiftBalanceRemaining 获取赠送余额剩余
// GET /api/v1/user/referral/gift-balance/remaining
func (h *ReferralHandler) GetGiftBalanceRemaining(c *gin.Context) {
	userID := c.GetInt64("user_id")
	remaining, err := h.referralService.GetGiftBalanceRemaining(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"remaining": remaining})
}

// ValidateReferralCode 验证推广码
// GET /api/v1/auth/validate-referral-code
func (h *ReferralHandler) ValidateReferralCode(c *gin.Context) {
	code := c.Query("code")
	valid, err := h.referralService.ValidateReferralCode(c.Request.Context(), code)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"valid": valid})
}
