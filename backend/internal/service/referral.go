package service

import (
	"context"
	"time"
)

// GiftBalanceSource 赠送余额来源类型
const (
	GiftBalanceSourceReferralInvitee = "referral_invitee" // 被邀请人注册奖励
	GiftBalanceSourceReferralInviter = "referral_inviter" // 推广人首充奖励
	GiftBalanceSourceReferralOngoing = "referral_ongoing" // 推广人持续奖励
	GiftBalanceSourceAdminGrant      = "admin_grant"      // 管理员手动发放
)

// ReferralStatus 推广关系状态
const (
	ReferralStatusPending   = "pending"   // 待完成（被邀请人尚未首充）
	ReferralStatusCompleted = "completed" // 已完成（被邀请人已首充，推广人已获得奖励）
)

// GiftBalanceRecord 赠送余额记录
type GiftBalanceRecord struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	Amount      float64    `json:"amount"`
	Remaining   float64    `json:"remaining"`
	Source      string     `json:"source"`
	SourceRefID *int64     `json:"source_ref_id"`
	Note        string     `json:"note,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Referral 推广关系记录
type Referral struct {
	ID                  int64      `json:"id"`
	ReferrerID          int64      `json:"referrer_id"`
	RefereeID           int64      `json:"referee_id"`
	ReferralCode        string     `json:"referral_code"`
	Status              string     `json:"status"`
	InviteeRewardAmount float64    `json:"invitee_reward_amount"`
	InviterRewardAmount float64    `json:"inviter_reward_amount"`
	InviteeRewardedAt   *time.Time `json:"invitee_rewarded_at"`
	InviterRewardedAt   *time.Time `json:"inviter_rewarded_at"`
	OngoingRewardCount  int        `json:"ongoing_reward_count"`
	OngoingRewardTotal  float64    `json:"ongoing_reward_total"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`

	// 附加字段（列表查询时填充）
	RefereeEmail    string `json:"referee_email,omitempty"`
	RefereeUsername string `json:"referee_username,omitempty"`
}

// UserReferralConfig 用户级推广配置覆盖
//
// OngoingRewardType: "fixed" | "percentage"
// OngoingRewardValue: 当 type=fixed 时单位为美元，type=percentage 时单位为百分点（0-100）
type UserReferralConfig struct {
	ID                        int64
	UserID                    int64
	InviteeRewardAmount       *float64
	InviterRewardAmount       *float64
	MaxInvites                *int
	RewardExpiryDays          *int
	OngoingRewardEnabled      *bool
	OngoingRewardType         *string
	OngoingRewardValue        *float64
	OngoingRewardMaxCount     *int
	OngoingRewardDurationDays *int
	Notes                     string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

// ReferralGlobalConfig 全局推广配置（从 Settings 表读取）
//
// OngoingRewardType: "fixed" | "percentage"（空字符串视为 fixed）
// OngoingRewardValue: 当 type=fixed 时单位为美元，type=percentage 时单位为百分点（0-100）
type ReferralGlobalConfig struct {
	Enabled                   bool
	InviteeRewardAmount       float64
	InviterRewardAmount       float64
	MaxInvites                int
	RewardExpiryDays          int
	OngoingRewardEnabled      bool
	OngoingRewardType         string
	OngoingRewardValue        float64
	OngoingRewardMaxCount     int
	OngoingRewardDurationDays int
}

// EffectiveReferralConfig 最终生效的推广配置（全局 + 用户覆盖合并后）
type EffectiveReferralConfig struct {
	Enabled                   bool
	InviteeRewardAmount       float64
	InviterRewardAmount       float64
	MaxInvites                int
	RewardExpiryDays          int
	OngoingRewardEnabled      bool
	OngoingRewardType         string
	OngoingRewardValue        float64
	OngoingRewardMaxCount     int
	OngoingRewardDurationDays int
}

// ReferralStats 推广统计
type ReferralStats struct {
	TotalInvited     int     `json:"total_invited"`
	CompletedInvited int     `json:"completed_invited"`
	TotalReward      float64 `json:"total_reward"`
	OngoingReward    float64 `json:"ongoing_reward"`
}

// ReferralUserInfo 用户推广中心概览（前后端契约）
type ReferralUserInfo struct {
	ReferralCode              string  `json:"referral_code"`
	Enabled                   bool    `json:"enabled"`
	TotalInvites              int     `json:"total_invites"`
	CompletedInvites          int     `json:"completed_invites"`
	TotalEarned               float64 `json:"total_earned"`
	GiftBalanceRemaining      float64 `json:"gift_balance_remaining"`
	GiftBalanceTotalGranted   float64 `json:"gift_balance_total_granted"`
	GiftBalanceTotalUsed      float64 `json:"gift_balance_total_used"`
	GiftBalanceTotalExpired   float64 `json:"gift_balance_total_expired"`
	InviteeReward             float64 `json:"invitee_reward"`
	InviterReward             float64 `json:"inviter_reward"`
	MaxInvites                int     `json:"max_invites"`
	OngoingRewardEnabled      bool    `json:"ongoing_reward_enabled"`
	OngoingRewardType         string  `json:"ongoing_reward_type"`
	OngoingRewardValue        float64 `json:"ongoing_reward_value"`
	OngoingRewardMaxCount     int     `json:"ongoing_reward_max_count"`
	OngoingRewardDurationDays int     `json:"ongoing_reward_duration_days"`
	RewardExpiryDays          int     `json:"reward_expiry_days"`
}

// ReferralTrendPoint 趋势图单点数据
type ReferralTrendPoint struct {
	Date         string  `json:"date"`
	Invitations  int     `json:"invitations"`
	Completions  int     `json:"completions"`
	RewardsTotal float64 `json:"rewards_total"`
}

// ReferralFunnel 转化漏斗
type ReferralFunnel struct {
	TotalReferrals int     `json:"total_referrals"`
	Registrations  int     `json:"registrations"`
	FirstRecharges int     `json:"first_recharges"`
	ConversionRate float64 `json:"conversion_rate"`
}

// AdminReferralDashboard 管理端推广数据看板
type AdminReferralDashboard struct {
	Funnel  ReferralFunnel       `json:"funnel"`
	Trend   []ReferralTrendPoint `json:"trend"`
	Summary AdminReferralStats   `json:"summary"`
}

// GiftBalanceSummary 赠送余额汇总
type GiftBalanceSummary struct {
	TotalGranted   float64 `json:"total_granted"`
	TotalRemaining float64 `json:"total_remaining"`
	TotalUsed      float64 `json:"total_used"`
	TotalExpired   float64 `json:"total_expired"`
}

// AdminReferralStats 管理端推广总览统计
type AdminReferralStats struct {
	TotalReferrals       int     `json:"total_referrals"`
	CompletedReferrals   int     `json:"completed_referrals"`
	TotalGiftGranted     float64 `json:"total_gift_granted"`
	TotalGiftUsed        float64 `json:"total_gift_used"`
	TotalGiftRemaining   float64 `json:"total_gift_remaining"`
	ActiveReferrersCount int     `json:"active_referrers_count"`
}

// ReferralLeaderboardEntry 推广排行榜条目
type ReferralLeaderboardEntry struct {
	UserID       int64   `json:"user_id"`
	Email        string  `json:"email"`
	Username     string  `json:"username"`
	InviteCount  int     `json:"invite_count"`
	TotalReward  float64 `json:"total_reward"`
	ReferralCode string  `json:"referral_code"`
}

// --- Repository Interfaces ---

// GiftBalanceRepository 赠送余额记录仓储接口
type GiftBalanceRepository interface {
	// Create 创建赠送余额记录
	Create(ctx context.Context, record *GiftBalanceRecord) error
	// GetByUserID 获取用户的赠送余额记录（分页）
	GetByUserID(ctx context.Context, userID int64, offset, limit int) ([]GiftBalanceRecord, int, error)
	// GetAvailableByUserID 获取用户有效（remaining > 0 且未过期）的赠送余额记录，按创建时间升序（FIFO）
	GetAvailableByUserID(ctx context.Context, userID int64) ([]GiftBalanceRecord, error)
	// GetSummaryByUserID 获取用户赠送余额汇总
	GetSummaryByUserID(ctx context.Context, userID int64) (*GiftBalanceSummary, error)
	// DeductFIFO 按 FIFO 扣减指定用户的赠送余额，返回实际扣减金额
	DeductFIFO(ctx context.Context, userID int64, amount float64) (float64, error)
	// ExpireRecords 将已过期的记录 remaining 设为 0，返回清理数量
	ExpireRecords(ctx context.Context) (int, error)
	// GetTotalRemainingByUserID 获取用户赠送余额总剩余
	GetTotalRemainingByUserID(ctx context.Context, userID int64) (float64, error)
	// ExistsBySourceRef 检查指定来源+引用ID的记录是否存在（幂等性）
	ExistsBySourceRef(ctx context.Context, source string, sourceRefID int64) (bool, error)
	// GetAdminStats 获取管理端全局统计
	GetAdminStats(ctx context.Context) (*AdminReferralStats, error)
}

// ReferralRepository 推广关系仓储接口
type ReferralRepository interface {
	// Create 创建推广关系
	Create(ctx context.Context, referral *Referral) error
	// GetByRefereeID 根据被邀请人ID获取推广关系
	GetByRefereeID(ctx context.Context, refereeID int64) (*Referral, error)
	// GetByReferrerID 获取推广人的邀请列表（分页）
	GetByReferrerID(ctx context.Context, referrerID int64, offset, limit int) ([]Referral, int, error)
	// CountByReferrerID 获取推广人的邀请总数
	CountByReferrerID(ctx context.Context, referrerID int64) (int, error)
	// UpdateStatus 更新推广关系状态
	UpdateStatus(ctx context.Context, id int64, status string) error
	// SetInviteeRewarded 标记被邀请人已获得注册奖励
	SetInviteeRewarded(ctx context.Context, id int64) error
	// SetInviterRewarded 标记推广人已获得首充奖励
	SetInviterRewarded(ctx context.Context, id int64, rewardAmount float64) error
	// IncrementOngoingReward 增加持续奖励计数和金额
	IncrementOngoingReward(ctx context.Context, id int64, amount float64) error
	// GetStatsByReferrerID 获取推广人的推广统计
	GetStatsByReferrerID(ctx context.Context, referrerID int64) (*ReferralStats, error)
	// ListAll 管理端列表（分页，支持筛选）
	ListAll(ctx context.Context, status string, offset, limit int) ([]Referral, int, error)
	// GetLeaderboard 推广排行榜（period: all_time / this_month / this_week）
	GetLeaderboard(ctx context.Context, period string, limit int) ([]ReferralLeaderboardEntry, error)
	// GetTrendByReferrerID 用户级趋势数据（按日，最近 N 天）
	GetTrendByReferrerID(ctx context.Context, referrerID int64, days int) ([]ReferralTrendPoint, error)
	// GetGlobalTrend 全站趋势数据
	GetGlobalTrend(ctx context.Context, days int) ([]ReferralTrendPoint, error)
	// CountFirstRecharges 统计已完成（已首充）的推广关系数量
	CountFirstRecharges(ctx context.Context) (int, error)
	// CountAll 统计推广关系总数
	CountAll(ctx context.Context) (int, error)
}

// UserReferralConfigRepository 用户推广配置仓储接口
type UserReferralConfigRepository interface {
	// GetByUserID 获取用户推广配置
	GetByUserID(ctx context.Context, userID int64) (*UserReferralConfig, error)
	// Upsert 创建或更新用户推广配置
	Upsert(ctx context.Context, config *UserReferralConfig) error
	// Delete 删除用户推广配置
	Delete(ctx context.Context, userID int64) error
}
