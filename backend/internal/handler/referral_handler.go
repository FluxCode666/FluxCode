package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
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
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	userID := subject.UserID
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
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	userID := subject.UserID
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
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	userID := subject.UserID
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
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	userID := subject.UserID
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
// GET /api/v1/referral/gift-balance/remaining
func (h *ReferralHandler) GetGiftBalanceRemaining(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	userID := subject.UserID
	remaining, err := h.referralService.GetGiftBalanceRemaining(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"remaining": remaining})
}

// GetMyGiftBalanceSummary 获取赠送余额汇总（已发/已用/剩余/已过期）
// GET /api/v1/referral/gift-balance/summary
func (h *ReferralHandler) GetMyGiftBalanceSummary(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	userID := subject.UserID
	summary, err := h.referralService.GetGiftBalanceSummary(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, summary)
}

// GetGiftBalanceOverview 获取赠送余额概览（Header 余额下拉用）
// GET /api/v1/referral/gift-balance/overview
func (h *ReferralHandler) GetGiftBalanceOverview(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	overview, err := h.referralService.GetGiftBalanceOverview(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, overview)
}

// GetMyStats 获取我的推广趋势数据
// GET /api/v1/referral/stats
func (h *ReferralHandler) GetMyStats(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	userID := subject.UserID
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	if days <= 0 || days > 365 {
		days = 30
	}
	trend, err := h.referralService.GetMyTrend(c.Request.Context(), userID, days)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"period": "daily",
		"data":   trend,
	})
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
