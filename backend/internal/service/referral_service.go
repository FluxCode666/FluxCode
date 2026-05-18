package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrReferralDisabled        = infraerrors.Forbidden("REFERRAL_DISABLED", "referral feature is disabled")
	ErrReferralCodeNotFound    = infraerrors.NotFound("REFERRAL_CODE_NOT_FOUND", "referral code not found")
	ErrSelfReferral            = infraerrors.BadRequest("SELF_REFERRAL", "cannot use your own referral code")
	ErrAlreadyReferred         = infraerrors.BadRequest("ALREADY_REFERRED", "user already has a referrer")
	ErrMaxInvitesReached       = infraerrors.BadRequest("MAX_INVITES_REACHED", "maximum number of invitations reached")
	ErrReferralCodeExists      = infraerrors.BadRequest("REFERRAL_CODE_EXISTS", "referral code already generated")
	ErrInvalidReferralCode     = infraerrors.BadRequest("INVALID_REFERRAL_CODE", "invalid referral code format")
	ErrGiftBalanceInsufficient = infraerrors.BadRequest("GIFT_BALANCE_INSUFFICIENT", "insufficient gift balance")
)

// ReferralService 推广奖励服务
type ReferralService struct {
	userRepo               UserRepository
	referralRepo           ReferralRepository
	giftBalanceRepo        GiftBalanceRepository
	configResolver         *ReferralConfigResolver
	salesCommissionService *SalesCommissionService
}

// NewReferralService 创建推广奖励服务
func NewReferralService(
	userRepo UserRepository,
	referralRepo ReferralRepository,
	giftBalanceRepo GiftBalanceRepository,
	configResolver *ReferralConfigResolver,
	salesCommissionService *SalesCommissionService,
) *ReferralService {
	return &ReferralService{
		userRepo:               userRepo,
		referralRepo:           referralRepo,
		giftBalanceRepo:        giftBalanceRepo,
		configResolver:         configResolver,
		salesCommissionService: salesCommissionService,
	}
}

// GenerateReferralCode 为用户生成推广码（首次访问推广中心时调用）
func (s *ReferralService) GenerateReferralCode(ctx context.Context, userID int64) (string, error) {
	globalCfg := s.configResolver.GetGlobalConfig(ctx)
	if !globalCfg.Enabled {
		return "", ErrReferralDisabled
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}
	if user.ReferralCode != "" {
		return user.ReferralCode, nil
	}

	// 生成唯一推广码
	code, err := s.generateUniqueCode(ctx)
	if err != nil {
		return "", fmt.Errorf("generate referral code: %w", err)
	}

	if err := s.userRepo.UpdateReferralCode(ctx, userID, code); err != nil {
		return "", fmt.Errorf("save referral code: %w", err)
	}
	return code, nil
}

// HandleReferralOnRegister 注册时处理推广关系（被邀请人触发）
// 在用户创建成功后调用
func (s *ReferralService) HandleReferralOnRegister(ctx context.Context, newUserID int64, referralCode string) {
	if referralCode == "" {
		return
	}

	globalCfg := s.configResolver.GetGlobalConfig(ctx)
	if !globalCfg.Enabled {
		return
	}

	// 根据推广码查找推广人
	referrer, err := s.userRepo.GetByReferralCode(ctx, referralCode)
	if err != nil || referrer == nil {
		slog.Warn("referral code not found during registration", "code", referralCode)
		return
	}

	// 防止自我邀请
	if referrer.ID == newUserID {
		slog.Warn("self-referral attempt", "userID", newUserID, "code", referralCode)
		return
	}

	// 检查被邀请人是否已有推广关系
	existing, _ := s.referralRepo.GetByRefereeID(ctx, newUserID)
	if existing != nil {
		slog.Warn("user already referred", "userID", newUserID)
		return
	}

	// 检查推广人邀请上限
	cfg := s.configResolver.Resolve(ctx, referrer.ID)
	if cfg.MaxInvites > 0 {
		count, _ := s.referralRepo.CountByReferrerID(ctx, referrer.ID)
		if count >= cfg.MaxInvites {
			slog.Warn("referrer max invites reached", "referrerID", referrer.ID, "count", count, "max", cfg.MaxInvites)
			return
		}
	}

	// 设置被邀请人的 referred_by
	if err := s.userRepo.UpdateReferredBy(ctx, newUserID, referrer.ID); err != nil {
		slog.Error("update referred_by", "userID", newUserID, "error", err)
		return
	}

	// 创建推广关系记录
	ref := &Referral{
		ReferrerID:          referrer.ID,
		RefereeID:           newUserID,
		ReferralCode:        referralCode,
		Status:              ReferralStatusPending,
		InviteeRewardAmount: cfg.InviteeRewardAmount,
	}
	if err := s.referralRepo.Create(ctx, ref); err != nil {
		slog.Error("create referral record", "error", err)
		return
	}

	// 发放被邀请人注册奖励（如果配置了）
	if cfg.InviteeRewardAmount > 0 {
		if s.grantGiftBalance(ctx, newUserID, cfg.InviteeRewardAmount, GiftBalanceSourceReferralInvitee, ref.ID, cfg.RewardExpiryDays,
			fmt.Sprintf("注册奖励（推广人: %s）", referrer.Email)) {
			if err := s.referralRepo.SetInviteeRewarded(ctx, ref.ID); err != nil {
				slog.Error("set invitee rewarded", "referralID", ref.ID, "error", err)
			}
		}
	}

	slog.Info("referral registered", "referrerID", referrer.ID, "refereeID", newUserID, "code", referralCode)
}

// HandleInviterRewardOnFirstRecharge 被邀请人首充后触发推广人奖励
func (s *ReferralService) HandleInviterRewardOnFirstRecharge(ctx context.Context, userID int64, rechargeAmount float64) {
	ref, err := s.referralRepo.GetByRefereeID(ctx, userID)
	if err != nil || ref == nil || ref.Status == ReferralStatusCompleted {
		return
	}
	if s.isSalesReferrer(ctx, ref.ReferrerID) {
		return
	}

	cfg := s.configResolver.Resolve(ctx, ref.ReferrerID)
	if !cfg.Enabled {
		return
	}

	// 发放推广人首充奖励
	if cfg.InviterRewardAmount > 0 && ref.InviterRewardedAt == nil {
		if s.grantGiftBalance(ctx, ref.ReferrerID, cfg.InviterRewardAmount, GiftBalanceSourceReferralInviter, ref.ID, cfg.RewardExpiryDays,
			fmt.Sprintf("被邀请人首充奖励（被邀请人ID: %d）", userID)) {
			if err := s.referralRepo.SetInviterRewarded(ctx, ref.ID, cfg.InviterRewardAmount); err != nil {
				slog.Error("set inviter rewarded", "referralID", ref.ID, "error", err)
			}
			slog.Info("inviter first-recharge reward granted", "referrerID", ref.ReferrerID, "refereeID", userID, "amount", cfg.InviterRewardAmount)
		}
	}
}

// HandleOngoingRewardOnRecharge 被邀请人每次充值时触发持续奖励
func (s *ReferralService) HandleOngoingRewardOnRecharge(ctx context.Context, userID int64, rechargeAmount float64, paymentOrderID int64) {
	ref, err := s.referralRepo.GetByRefereeID(ctx, userID)
	if err != nil || ref == nil {
		return
	}
	if s.isSalesReferrer(ctx, ref.ReferrerID) {
		return
	}

	cfg := s.configResolver.Resolve(ctx, ref.ReferrerID)
	if !cfg.Enabled || !cfg.OngoingRewardEnabled {
		return
	}

	// 检查持续奖励次数上限
	if cfg.OngoingRewardMaxCount > 0 && ref.OngoingRewardCount >= cfg.OngoingRewardMaxCount {
		return
	}

	// 检查持续奖励有效期（注册后 N 天内充值才触发）
	if cfg.OngoingRewardDurationDays > 0 {
		deadline := ref.CreatedAt.AddDate(0, 0, cfg.OngoingRewardDurationDays)
		if time.Now().After(deadline) {
			return
		}
	}

	// 计算持续奖励金额：根据 type 判断单位
	var rewardAmount float64
	switch cfg.OngoingRewardType {
	case "percentage":
		rewardAmount = rechargeAmount * cfg.OngoingRewardValue / 100
	default: // "fixed" 或未设置
		rewardAmount = cfg.OngoingRewardValue
	}
	if rewardAmount <= 0 {
		return
	}

	// Use payment order ID as the idempotency key so each successful recharge can
	// grant its own ongoing reward while duplicate fulfillment of the same order
	// remains harmless.
	if !s.grantGiftBalance(ctx, ref.ReferrerID, rewardAmount, GiftBalanceSourceReferralOngoing, paymentOrderID, cfg.RewardExpiryDays,
		fmt.Sprintf("持续充值奖励（被邀请人ID: %d, 充值: %.2f）", userID, rechargeAmount)) {
		return
	}

	if err := s.referralRepo.IncrementOngoingReward(ctx, ref.ID, rewardAmount); err != nil {
		slog.Error("increment ongoing reward", "referralID", ref.ID, "error", err)
	}
	slog.Info("ongoing reward granted", "referrerID", ref.ReferrerID, "refereeID", userID, "amount", rewardAmount)
}

// AdminGrantGiftBalance 管理员手动发放赠送余额
func (s *ReferralService) AdminGrantGiftBalance(ctx context.Context, userID int64, amount float64, expiryDays int, note string) error {
	if amount <= 0 {
		return infraerrors.BadRequest("INVALID_AMOUNT", "amount must be positive")
	}
	s.grantGiftBalance(ctx, userID, amount, GiftBalanceSourceAdminGrant, 0, expiryDays, note)
	return nil
}

// AdminMarkReferralCompleted 管理端手动完成推广记录，仅补发推广侧奖励。
func (s *ReferralService) AdminMarkReferralCompleted(ctx context.Context, referralID int64, note string, orderPayAmountCNY, orderCreditedAmount float64) error {
	ref, err := s.referralRepo.GetByID(ctx, referralID)
	if err != nil {
		return err
	}
	if ref == nil {
		return infraerrors.NotFound("REFERRAL_NOT_FOUND", "referral not found")
	}
	if ref.Status != ReferralStatusPending {
		return infraerrors.BadRequest("REFERRAL_STATUS_INVALID", "referral is not pending")
	}

	if s.isSalesReferrer(ctx, ref.ReferrerID) {
		if orderPayAmountCNY <= 0 || orderCreditedAmount <= 0 {
			return infraerrors.BadRequest("MANUAL_COMPLETION_ORDER_AMOUNT_REQUIRED", "order_pay_amount_cny and order_credited_amount must be greater than 0 for sales referrers")
		}
		if s.salesCommissionService != nil {
			if err := s.salesCommissionService.HandleReferralManualCompletion(ctx, ref, orderPayAmountCNY, orderCreditedAmount, strings.TrimSpace(note)); err != nil {
				return err
			}
		}
		return s.referralRepo.MarkCompleted(ctx, ref.ID, 0, strings.TrimSpace(note))
	}

	cfg := s.configResolver.Resolve(ctx, ref.ReferrerID)
	rewardAmount := cfg.InviterRewardAmount
	if rewardAmount > 0 && ref.InviterRewardedAt == nil {
		s.grantGiftBalance(
			ctx,
			ref.ReferrerID,
			rewardAmount,
			GiftBalanceSourceReferralInviter,
			ref.ID,
			cfg.RewardExpiryDays,
			strings.TrimSpace(note),
		)
	}

	return s.referralRepo.MarkCompleted(ctx, ref.ID, rewardAmount, strings.TrimSpace(note))
}

// GetUserReferralInfo 获取用户推广信息（推广中心页面数据，强类型）
func (s *ReferralService) GetUserReferralInfo(ctx context.Context, userID int64) (*ReferralUserInfo, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	globalCfg := s.configResolver.GetGlobalConfig(ctx)

	// 即使全局未启用，也读取配置以便 admin/UI 展示（Resolve 在 disabled 时返回空，故直接合并）
	var cfg *EffectiveReferralConfig
	if globalCfg.Enabled {
		cfg = s.configResolver.Resolve(ctx, userID)
	} else {
		cfg = mergeConfig(globalCfg, nil)
	}

	stats, _ := s.referralRepo.GetStatsByReferrerID(ctx, userID)
	if stats == nil {
		stats = &ReferralStats{}
	}

	giftSummary, _ := s.giftBalanceRepo.GetSummaryByUserID(ctx, userID)
	if giftSummary == nil {
		giftSummary = &GiftBalanceSummary{}
	}

	rewardType := cfg.OngoingRewardType
	if rewardType == "" {
		rewardType = "fixed"
	}

	return &ReferralUserInfo{
		ReferralCode:              user.ReferralCode,
		Enabled:                   globalCfg.Enabled,
		TotalInvites:              stats.TotalInvited,
		CompletedInvites:          stats.CompletedInvited,
		TotalEarned:               stats.TotalReward + stats.OngoingReward,
		GiftBalanceRemaining:      giftSummary.TotalRemaining,
		GiftBalanceTotalGranted:   giftSummary.TotalGranted,
		GiftBalanceTotalUsed:      giftSummary.TotalUsed,
		GiftBalanceTotalExpired:   giftSummary.TotalExpired,
		InviteeReward:             cfg.InviteeRewardAmount,
		InviterReward:             cfg.InviterRewardAmount,
		MaxInvites:                cfg.MaxInvites,
		OngoingRewardEnabled:      cfg.OngoingRewardEnabled,
		OngoingRewardType:         rewardType,
		OngoingRewardValue:        cfg.OngoingRewardValue,
		OngoingRewardMaxCount:     cfg.OngoingRewardMaxCount,
		OngoingRewardDurationDays: cfg.OngoingRewardDurationDays,
		RewardExpiryDays:          cfg.RewardExpiryDays,
	}, nil
}

// GetMyTrend 用户级邀请趋势数据
func (s *ReferralService) GetMyTrend(ctx context.Context, userID int64, days int) ([]ReferralTrendPoint, error) {
	return s.referralRepo.GetTrendByReferrerID(ctx, userID, days)
}

// GetGiftBalanceSummary 用户赠送余额汇总
func (s *ReferralService) GetGiftBalanceSummary(ctx context.Context, userID int64) (*GiftBalanceSummary, error) {
	summary, err := s.giftBalanceRepo.GetSummaryByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if summary == nil {
		summary = &GiftBalanceSummary{}
	}
	return summary, nil
}

// GetMyReferrals 获取用户的邀请列表
func (s *ReferralService) GetMyReferrals(ctx context.Context, userID int64, page, pageSize int) ([]Referral, int, error) {
	offset := (page - 1) * pageSize
	return s.referralRepo.GetByReferrerID(ctx, userID, offset, pageSize)
}

// GetMyGiftBalanceRecords 获取用户的赠送余额记录
func (s *ReferralService) GetMyGiftBalanceRecords(ctx context.Context, userID int64, page, pageSize int) ([]GiftBalanceRecord, int, error) {
	offset := (page - 1) * pageSize
	return s.giftBalanceRepo.GetByUserID(ctx, userID, offset, pageSize)
}

// GetGiftBalanceRemaining 获取用户赠送余额剩余
func (s *ReferralService) GetGiftBalanceRemaining(ctx context.Context, userID int64) (float64, error) {
	return s.giftBalanceRepo.GetTotalRemainingByUserID(ctx, userID)
}

// ValidateReferralCode 验证推广码是否有效
func (s *ReferralService) ValidateReferralCode(ctx context.Context, code string) (bool, error) {
	if code == "" {
		return false, nil
	}
	globalCfg := s.configResolver.GetGlobalConfig(ctx)
	if !globalCfg.Enabled {
		return false, nil
	}
	user, err := s.userRepo.GetByReferralCode(ctx, code)
	if err != nil {
		return false, err
	}
	return user != nil, nil
}

// ExpireGiftBalanceRecords 过期赠送余额清理（后台任务调用）
func (s *ReferralService) ExpireGiftBalanceRecords(ctx context.Context) (int, error) {
	return s.giftBalanceRepo.ExpireRecords(ctx)
}

// --- Admin Methods ---

// AdminGetStats 管理端获取推广总览统计
func (s *ReferralService) AdminGetStats(ctx context.Context) (*AdminReferralStats, error) {
	return s.giftBalanceRepo.GetAdminStats(ctx)
}

// AdminListReferrals 管理端列出推广关系
func (s *ReferralService) AdminListReferrals(ctx context.Context, status string, page, pageSize int) ([]Referral, int, error) {
	offset := (page - 1) * pageSize
	return s.referralRepo.ListAll(ctx, status, offset, pageSize)
}

// AdminGetLeaderboard 管理端获取推广排行榜
// period: all_time / this_month / this_week
func (s *ReferralService) AdminGetLeaderboard(ctx context.Context, period string, limit int) ([]ReferralLeaderboardEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.referralRepo.GetLeaderboard(ctx, period, limit)
}

// AdminGetDashboard 管理端推广数据看板：转化漏斗 + 趋势 + 概览
func (s *ReferralService) AdminGetDashboard(ctx context.Context, days int) (*AdminReferralDashboard, error) {
	if days <= 0 {
		days = 30
	}

	totalReferrals, err := s.referralRepo.CountAll(ctx)
	if err != nil {
		return nil, err
	}
	firstRecharges, err := s.referralRepo.CountFirstRecharges(ctx)
	if err != nil {
		return nil, err
	}

	var conversionRate float64
	if totalReferrals > 0 {
		conversionRate = float64(firstRecharges) / float64(totalReferrals) * 100
	}

	trend, err := s.referralRepo.GetGlobalTrend(ctx, days)
	if err != nil {
		return nil, err
	}

	summary, err := s.giftBalanceRepo.GetAdminStats(ctx)
	if err != nil {
		return nil, err
	}

	return &AdminReferralDashboard{
		Funnel: ReferralFunnel{
			TotalReferrals: totalReferrals,
			Registrations:  totalReferrals, // 当前实现：referral 关系即为注册
			FirstRecharges: firstRecharges,
			ConversionRate: conversionRate,
		},
		Trend:   trend,
		Summary: *summary,
	}, nil
}

// AdminBatchGrantGiftBalance 批量发放赠送余额
// target: "all" - 全站活跃用户, "selected" - 指定 user_ids
// 返回成功发放的数量
func (s *ReferralService) AdminBatchGrantGiftBalance(ctx context.Context, target string, userIDs []int64, amount float64, expiryDays int, note string) (int, error) {
	if amount <= 0 {
		return 0, infraerrors.BadRequest("INVALID_AMOUNT", "amount must be positive")
	}

	var targetIDs []int64
	switch target {
	case "all":
		ids, err := s.userRepo.ListActiveUserIDs(ctx)
		if err != nil {
			return 0, err
		}
		targetIDs = ids
	case "selected":
		targetIDs = userIDs
	default:
		return 0, infraerrors.BadRequest("INVALID_TARGET", "target must be 'all' or 'selected'")
	}

	count := 0
	for _, uid := range targetIDs {
		s.grantGiftBalance(ctx, uid, amount, GiftBalanceSourceAdminGrant, 0, expiryDays, note)
		count++
	}
	return count, nil
}

// --- Internal helpers ---

func (s *ReferralService) grantGiftBalance(ctx context.Context, userID int64, amount float64, source string, sourceRefID int64, expiryDays int, note string) bool {
	// 幂等性检查
	if sourceRefID > 0 {
		exists, _ := s.giftBalanceRepo.ExistsBySourceRef(ctx, source, sourceRefID)
		if exists {
			slog.Info("gift balance already granted (idempotent)", "source", source, "sourceRefID", sourceRefID)
			return false
		}
	}

	var expiresAt *time.Time
	if expiryDays > 0 {
		t := time.Now().AddDate(0, 0, expiryDays)
		expiresAt = &t
	}

	var refID *int64
	if sourceRefID > 0 {
		refID = &sourceRefID
	}

	record := &GiftBalanceRecord{
		UserID:      userID,
		Amount:      amount,
		Remaining:   amount,
		Source:      source,
		SourceRefID: refID,
		Note:        note,
		ExpiresAt:   expiresAt,
	}
	if err := s.giftBalanceRepo.Create(ctx, record); err != nil {
		slog.Error("grant gift balance", "userID", userID, "amount", amount, "source", source, "error", err)
		return false
	}
	return true
}

func (s *ReferralService) isSalesReferrer(ctx context.Context, userID int64) bool {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return false
	}
	return user.IsSales
}

// generateUniqueCode 生成 6-8 位唯一推广码
func (s *ReferralService) generateUniqueCode(ctx context.Context) (string, error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 排除易混淆字符 I,O,0,1
	const codeLen = 8

	for attempts := 0; attempts < 10; attempts++ {
		var sb strings.Builder
		for i := 0; i < codeLen; i++ {
			idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
			if err != nil {
				return "", err
			}
			sb.WriteByte(charset[idx.Int64()])
		}
		code := sb.String()

		// 检查唯一性
		existing, err := s.userRepo.GetByReferralCode(ctx, code)
		if err != nil {
			return "", err
		}
		if existing == nil {
			return code, nil
		}
	}
	return "", fmt.Errorf("failed to generate unique referral code after 10 attempts")
}
