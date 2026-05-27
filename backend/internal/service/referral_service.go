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
	paymentService         *PaymentService
}

// SetPaymentService 注入支付服务（避免循环依赖）
func (s *ReferralService) SetPaymentService(svc *PaymentService) {
	s.paymentService = svc
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
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}
	if user.ReferralCode != "" {
		return user.ReferralCode, nil
	}

	// 根据用户类型检查对应开关
	globalCfg := s.configResolver.GetGlobalConfig(ctx)
	if SalesCommissionUserEligible(user) {
		if !globalCfg.SalesEnabled {
			return "", ErrReferralDisabled
		}
	} else {
		if !globalCfg.Enabled {
			return "", ErrReferralDisabled
		}
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

	// 根据推广码查找推广人（需要先确定推广人类型再决定检查哪个开关）
	referrer, err := s.userRepo.GetByReferralCode(ctx, referralCode)
	if err != nil || referrer == nil {
		slog.Warn("referral code not found during registration", "code", referralCode)
		return
	}

	globalCfg := s.configResolver.GetGlobalConfig(ctx)
	isSalesReferrer := SalesCommissionUserEligible(referrer)

	// 根据推广人类型检查对应开关
	if isSalesReferrer {
		if !globalCfg.SalesEnabled {
			slog.Info("sales referral disabled, skip registration", "referrerID", referrer.ID, "refereeID", newUserID)
			return
		}
	} else {
		if !globalCfg.Enabled {
			slog.Info("regular referral disabled, skip registration", "referrerID", referrer.ID, "refereeID", newUserID)
			return
		}
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

	// 检查推广人邀请上限（仅普通推广人受限）
	if !isSalesReferrer {
		cfg := s.configResolver.Resolve(ctx, referrer.ID)
		if cfg.MaxInvites > 0 {
			count, _ := s.referralRepo.CountByReferrerID(ctx, referrer.ID)
			if count >= cfg.MaxInvites {
				slog.Warn("referrer max invites reached", "referrerID", referrer.ID, "count", count, "max", cfg.MaxInvites)
				return
			}
		}
	}

	// 确定被邀请人注册奖励金额（根据推广人类型选不同配置 + 检查开关）
	var inviteeRewardAmount float64
	var rewardExpiryDays int
	if isSalesReferrer {
		if globalCfg.SalesInviteeRewardEnabled {
			inviteeRewardAmount = globalCfg.SalesInviteeRewardAmount
		}
		rewardExpiryDays = 0 // 销售推广的被邀请人奖励不过期
	} else {
		if globalCfg.InviteeRewardEnabled {
			cfg := s.configResolver.Resolve(ctx, referrer.ID)
			inviteeRewardAmount = cfg.InviteeRewardAmount
			rewardExpiryDays = cfg.RewardExpiryDays
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
		InviteeRewardAmount: inviteeRewardAmount,
	}
	if err := s.referralRepo.Create(ctx, ref); err != nil {
		slog.Error("create referral record", "error", err)
		return
	}

	// 发放被邀请人注册奖励（如果配置了）
	if inviteeRewardAmount > 0 {
		if s.grantGiftBalance(ctx, newUserID, inviteeRewardAmount, GiftBalanceSourceReferralInvitee, ref.ID, rewardExpiryDays,
			fmt.Sprintf("推广注册赠送余额（推广人: %s）", referrer.Email)) {
			if err := s.referralRepo.SetInviteeRewarded(ctx, ref.ID); err != nil {
				slog.Error("set invitee rewarded", "referralID", ref.ID, "error", err)
			}
		}
	}

	slog.Info("referral registered", "referrerID", referrer.ID, "refereeID", newUserID, "code", referralCode, "isSales", isSalesReferrer)
}

// HandleInviterRewardOnFirstRecharge 被邀请人首充后触发推广人奖励
//
// 语义：referrals.status 与奖励发放解耦。
//   - 任意被邀请人首充事件都让 referrals.status: pending → completed
//     （管理界面据此判定 "已达首充要求"）。
//   - 销售推广人：奖励走 sales_commission_records，本函数不发 gift。
//   - 普通推广人：根据全局 / 用户级配置发 gift；奖励为 0 或全局禁用时不发 gift，
//     但仍要标 completed。
func (s *ReferralService) HandleInviterRewardOnFirstRecharge(ctx context.Context, userID int64, rechargeAmount float64) {
	ref, err := s.referralRepo.GetByRefereeID(ctx, userID)
	if err != nil || ref == nil || ref.Status == ReferralStatusCompleted {
		return
	}

	// 销售推广人：佣金路径在 sales_commission_service 里写入，本函数仅负责状态机收尾。
	if s.isSalesReferrer(ctx, ref.ReferrerID) {
		s.markReferralCompletedOnFirstRecharge(ctx, ref.ID)
		return
	}

	cfg := s.configResolver.Resolve(ctx, ref.ReferrerID)

	if !cfg.Enabled || !cfg.InviterFirstChargeRewardEnabled || ref.InviterRewardedAt != nil {
		// 没有 gift 奖励要发，但首充事件已经发生，仍要让 referral 状态完成。
		s.markReferralCompletedOnFirstRecharge(ctx, ref.ID)
		return
	}

	// 计算邀请人首充奖励金额：根据 type 判断单位
	var inviterRewardAmount float64
	switch cfg.InviterFirstChargeRewardType {
	case "percentage":
		inviterRewardAmount = rechargeAmount * cfg.InviterFirstChargeRewardValue / 100
	default: // "fixed" 或未设置
		inviterRewardAmount = cfg.InviterFirstChargeRewardValue
	}

	if inviterRewardAmount <= 0 {
		// 没有 gift 奖励要发，但首充事件已经发生，仍要让 referral 状态完成。
		s.markReferralCompletedOnFirstRecharge(ctx, ref.ID)
		return
	}

	// 发放推广人首充 gift 奖励；SetInviterRewarded 内部已经把 status 设为 completed。
	if !s.grantGiftBalance(ctx, ref.ReferrerID, inviterRewardAmount, GiftBalanceSourceReferralInviter, ref.ID, cfg.RewardExpiryDays,
		s.buildRewardNote(ctx, "下线首充赠送余额", userID, rechargeAmount, 0)) {
		// gift 创建失败时仍标 completed，避免管理界面停在 pending 误导运维。
		s.markReferralCompletedOnFirstRecharge(ctx, ref.ID)
		return
	}
	if err := s.referralRepo.SetInviterRewarded(ctx, ref.ID, inviterRewardAmount); err != nil {
		slog.Error("set inviter rewarded", "referralID", ref.ID, "error", err)
	}
	slog.Info("inviter first-recharge reward granted", "referrerID", ref.ReferrerID, "refereeID", userID, "amount", inviterRewardAmount)
}

// markReferralCompletedOnFirstRecharge 仅修改 referrals.status，
// 不动 inviter_reward_amount / inviter_rewarded_at（这些字段表示 "gift 奖励是否真实发放"，
// 销售路径与无奖励路径不应当写入它们）。
func (s *ReferralService) markReferralCompletedOnFirstRecharge(ctx context.Context, referralID int64) {
	if err := s.referralRepo.UpdateStatus(ctx, referralID, ReferralStatusCompleted); err != nil {
		slog.Error("mark referral completed on first recharge", "referralID", referralID, "error", err)
	}
}

// HandleInviteeRewardOnFirstRecharge 被邀请人首充后触发被邀请人首充奖励
func (s *ReferralService) HandleInviteeRewardOnFirstRecharge(ctx context.Context, userID int64, rechargeAmount float64, paymentOrderID int64) {
	ref, err := s.referralRepo.GetByRefereeID(ctx, userID)
	if err != nil || ref == nil {
		return
	}

	// 销售推广人不走普通首充被邀请人奖励路径
	if s.isSalesReferrer(ctx, ref.ReferrerID) {
		return
	}

	cfg := s.configResolver.Resolve(ctx, ref.ReferrerID)
	if !cfg.Enabled || !cfg.InviteeFirstChargeRewardEnabled {
		return
	}

	// 计算被邀请人首充奖励金额
	var rewardAmount float64
	switch cfg.InviteeFirstChargeRewardType {
	case "percentage":
		rewardAmount = rechargeAmount * cfg.InviteeFirstChargeRewardValue / 100
	default:
		rewardAmount = cfg.InviteeFirstChargeRewardValue
	}
	if rewardAmount <= 0 {
		return
	}

	// 使用负值 paymentOrderID 作为幂等键，避免与推广人奖励冲突
	idempotencyKey := -paymentOrderID
	if idempotencyKey == 0 {
		idempotencyKey = -2
	}
	if !s.grantGiftBalance(ctx, userID, rewardAmount, GiftBalanceSourceReferralInviteeFirstCharge, idempotencyKey, cfg.RewardExpiryDays,
		s.buildRewardNote(ctx, "首充赠送余额 - 被邀请人首充奖励", userID, rechargeAmount, ref.ReferrerID)) {
		return
	}
	slog.Info("invitee first-recharge reward granted", "refereeID", userID, "amount", rewardAmount, "referrerID", ref.ReferrerID)
}

// HandleOngoingRewardOnRecharge 被邀请人每次充值时触发持续奖励
// isFirstRecharge=true 时，普通推广的持续奖励被跳过（由首充专用方法处理），
// 但销售推广人的被邀请人持续奖励不受影响。
func (s *ReferralService) HandleOngoingRewardOnRecharge(ctx context.Context, userID int64, rechargeAmount float64, paymentOrderID int64, isFirstRecharge bool) {
	ref, err := s.referralRepo.GetByRefereeID(ctx, userID)
	if err != nil || ref == nil {
		return
	}

	isSales := s.isSalesReferrer(ctx, ref.ReferrerID)

	if isSales {
		if isFirstRecharge {
			// 销售推广：被邀请人首充奖励
			s.handleSalesInviteeFirstChargeReward(ctx, ref, userID, rechargeAmount, paymentOrderID)
		} else {
			// 销售推广：被邀请人持续充值奖励（首充走首充路径，不重复发放持续奖励）
			s.handleSalesInviteeOngoingReward(ctx, ref, userID, rechargeAmount, paymentOrderID)
		}
		return
	}

	// 普通推广：首充时跳过持续奖励，首充奖励由 HandleInviter/InviteeRewardOnFirstRecharge 处理
	if isFirstRecharge {
		return
	}

	cfg := s.configResolver.Resolve(ctx, ref.ReferrerID)
	if !cfg.Enabled {
		return
	}

	// 1) 推广人持续奖励
	s.handleInviterOngoingReward(ctx, cfg, ref, userID, rechargeAmount, paymentOrderID)

	// 2) 被邀请人持续奖励
	s.handleInviteeOngoingReward(ctx, cfg, ref, userID, rechargeAmount, paymentOrderID)
}

// handleInviterOngoingReward 普通推广：给推广人发放持续充值奖励
func (s *ReferralService) handleInviterOngoingReward(ctx context.Context, cfg *EffectiveReferralConfig, ref *Referral, userID int64, rechargeAmount float64, paymentOrderID int64) {
	if !cfg.OngoingRewardEnabled {
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
		s.buildRewardNote(ctx, "下线充值赠送余额 - 推广人持续奖励", userID, rechargeAmount, 0)) {
		return
	}

	if err := s.referralRepo.IncrementOngoingReward(ctx, ref.ID, rewardAmount); err != nil {
		slog.Error("increment ongoing reward", "referralID", ref.ID, "error", err)
	}
	slog.Info("inviter ongoing reward granted", "referrerID", ref.ReferrerID, "refereeID", userID, "amount", rewardAmount)
}

// handleInviteeOngoingReward 普通推广：给被邀请人发放持续充值奖励
func (s *ReferralService) handleInviteeOngoingReward(ctx context.Context, cfg *EffectiveReferralConfig, ref *Referral, userID int64, rechargeAmount float64, paymentOrderID int64) {
	if !cfg.InviteeOngoingRewardEnabled {
		return
	}

	// 检查持续奖励次数上限（复用 ref.InviteeOngoingRewardCount）
	if cfg.InviteeOngoingRewardMaxCount > 0 && ref.InviteeOngoingRewardCount >= cfg.InviteeOngoingRewardMaxCount {
		return
	}

	// 检查持续奖励有效期
	if cfg.InviteeOngoingRewardDurationDays > 0 {
		deadline := ref.CreatedAt.AddDate(0, 0, cfg.InviteeOngoingRewardDurationDays)
		if time.Now().After(deadline) {
			return
		}
	}

	// 计算奖励金额
	var rewardAmount float64
	switch cfg.InviteeOngoingRewardType {
	case "percentage":
		rewardAmount = rechargeAmount * cfg.InviteeOngoingRewardValue / 100
	default:
		rewardAmount = cfg.InviteeOngoingRewardValue
	}
	if rewardAmount <= 0 {
		return
	}

	// 使用不同的 idempotency 源ID 避免与推广人奖励冲突：paymentOrderID 取反
	inviteeIdempotencyKey := -paymentOrderID
	if inviteeIdempotencyKey == 0 {
		inviteeIdempotencyKey = -1
	}
	if !s.grantGiftBalance(ctx, userID, rewardAmount, GiftBalanceSourceReferralOngoing, inviteeIdempotencyKey, cfg.RewardExpiryDays,
		s.buildRewardNote(ctx, "下线充值赠送余额 - 被邀请人持续奖励", userID, rechargeAmount, ref.ReferrerID)) {
		return
	}

	if err := s.referralRepo.IncrementInviteeOngoingReward(ctx, ref.ID, rewardAmount); err != nil {
		slog.Error("increment invitee ongoing reward", "referralID", ref.ID, "error", err)
	}
	slog.Info("invitee ongoing reward granted", "refereeID", userID, "amount", rewardAmount)
}

// handleSalesInviteeFirstChargeReward 销售推广：被邀请人首充时获得首充奖励
func (s *ReferralService) handleSalesInviteeFirstChargeReward(ctx context.Context, ref *Referral, userID int64, rechargeAmount float64, paymentOrderID int64) {
	globalCfg := s.configResolver.GetGlobalConfig(ctx)
	if !globalCfg.SalesInviteeFirstChargeRewardEnabled {
		return
	}

	var rewardAmount float64
	switch globalCfg.SalesInviteeFirstChargeRewardType {
	case "percentage":
		rewardAmount = rechargeAmount * globalCfg.SalesInviteeFirstChargeRewardValue / 100
	default:
		rewardAmount = globalCfg.SalesInviteeFirstChargeRewardValue
	}
	if rewardAmount <= 0 {
		return
	}

	// 使用负值 paymentOrderID 做幂等，source 区分
	idempotencyKey := -paymentOrderID
	if idempotencyKey == 0 {
		idempotencyKey = -2
	}
	if !s.grantGiftBalance(ctx, userID, rewardAmount, GiftBalanceSourceReferralInviteeFirstCharge, idempotencyKey, 0,
		s.buildRewardNote(ctx, "首充赠送余额 - 销售推广被邀请人首充奖励", userID, rechargeAmount, ref.ReferrerID)) {
		return
	}
	slog.Info("sales invitee first-recharge reward granted", "referrerID", ref.ReferrerID, "refereeID", userID, "amount", rewardAmount)
}

// handleSalesInviteeOngoingReward 销售推广：被邀请人每次充值时获得持续奖励（发放给被邀请人自己）
func (s *ReferralService) handleSalesInviteeOngoingReward(ctx context.Context, ref *Referral, userID int64, rechargeAmount float64, paymentOrderID int64) {
	globalCfg := s.configResolver.GetGlobalConfig(ctx)
	if !globalCfg.SalesInviteeOngoingRewardEnabled {
		return
	}

	// 检查持续奖励次数上限
	if globalCfg.SalesInviteeOngoingRewardMaxCount > 0 && ref.OngoingRewardCount >= globalCfg.SalesInviteeOngoingRewardMaxCount {
		return
	}

	// 检查持续奖励有效期
	if globalCfg.SalesInviteeOngoingRewardDurationDays > 0 {
		deadline := ref.CreatedAt.AddDate(0, 0, globalCfg.SalesInviteeOngoingRewardDurationDays)
		if time.Now().After(deadline) {
			return
		}
	}

	// 计算奖励金额
	var rewardAmount float64
	switch globalCfg.SalesInviteeOngoingRewardType {
	case "percentage":
		rewardAmount = rechargeAmount * globalCfg.SalesInviteeOngoingRewardValue / 100
	default:
		rewardAmount = globalCfg.SalesInviteeOngoingRewardValue
	}
	if rewardAmount <= 0 {
		return
	}

	// 给被邀请人自己发放奖励（使用 paymentOrderID 做幂等）
	if !s.grantGiftBalance(ctx, userID, rewardAmount, GiftBalanceSourceReferralOngoing, paymentOrderID, 0,
		s.buildRewardNote(ctx, "下线充值赠送余额 - 销售推广持续奖励", userID, rechargeAmount, ref.ReferrerID)) {
		return
	}

	if err := s.referralRepo.IncrementOngoingReward(ctx, ref.ID, rewardAmount); err != nil {
		slog.Error("increment sales invitee ongoing reward", "referralID", ref.ID, "error", err)
	}
	slog.Info("sales invitee ongoing reward granted", "referrerID", ref.ReferrerID, "refereeID", userID, "amount", rewardAmount)
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

// OfflineRechargeInput 录入私账充值的请求参数
type OfflineRechargeInput struct {
	UserID         int64   // 被推广用户（充值用户）
	PayAmountCNY   float64 // 实付金额
	CreditedAmount float64 // 到账金额（加到用户余额的金额）
	CreditBalance  bool    // 是否同时给用户加余额
	Note           string  // 管理员备注
}

// OfflineRechargeResult 录入私账充值的结果
type OfflineRechargeResult struct {
	OrderID          int64   `json:"order_id"`
	UserID           int64   `json:"user_id"`
	ReferrerID       int64   `json:"referrer_id"`
	ReferrerEmail    string  `json:"referrer_email"`
	IsSalesReferrer  bool    `json:"is_sales_referrer"`
	BalanceCredited  bool    `json:"balance_credited"`
	CreditedAmount   float64 `json:"credited_amount"`
	RewardsTriggered bool    `json:"rewards_triggered"`
}

// RecordOfflineRecharge 管理员录入私账充值，自动查出推广关系并触发推广奖励。
//
// 流程：
//  1. 查出被充值用户的推广关系（referral）
//  2. 通过 PaymentService 创建私账充值订单（含加余额 + 审计日志）
//  3. 根据推广人类型触发奖励：
//     - 普通推广人：HandleInviterRewardOnFirstRecharge（如果是首充）+ HandleOngoingRewardOnRecharge
//     - 销售推广人：HandleBalanceRechargeCompleted（佣金记录）+ 持续被邀请人奖励
func (s *ReferralService) RecordOfflineRecharge(ctx context.Context, input *OfflineRechargeInput) (*OfflineRechargeResult, error) {
	if input == nil || input.UserID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_INPUT", "user_id is required")
	}
	if input.PayAmountCNY <= 0 || input.CreditedAmount <= 0 {
		return nil, infraerrors.BadRequest("INVALID_AMOUNT", "pay_amount_cny and credited_amount must be positive")
	}

	// 1. 查推广关系
	ref, err := s.referralRepo.GetByRefereeID(ctx, input.UserID)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		return nil, infraerrors.BadRequest("NO_REFERRAL", "this user has no referral relationship")
	}

	// 获取推广人信息
	referrer, err := s.userRepo.GetByID(ctx, ref.ReferrerID)
	if err != nil {
		return nil, err
	}
	if referrer == nil {
		return nil, infraerrors.NotFound("REFERRER_NOT_FOUND", "referrer user not found")
	}

	isSales := s.isSalesReferrer(ctx, ref.ReferrerID)

	// 2. 创建私账充值订单（含加余额 + 审计日志）
	// 在创建订单前判断是否首充（CreateOfflineRechargeOrder 内部会增加 TotalRecharged）
	isFirstRecharge, _ := s.userRepo.IsFirstRecharge(ctx, input.UserID)

	var orderID int64
	if s.paymentService != nil {
		oid, err := s.paymentService.CreateOfflineRechargeOrder(ctx, &OfflineRechargeRequest{
			UserID:         input.UserID,
			PayAmountCNY:   input.PayAmountCNY,
			CreditedAmount: input.CreditedAmount,
			CreditBalance:  input.CreditBalance,
			Note:           input.Note,
		})
		if err != nil {
			return nil, fmt.Errorf("create offline recharge order: %w", err)
		}
		orderID = oid
	} else {
		// fallback: 无 PaymentService 时直接加余额
		if input.CreditBalance {
			if err := s.userRepo.UpdateBalance(ctx, input.UserID, input.CreditedAmount); err != nil {
				return nil, fmt.Errorf("credit balance: %w", err)
			}
		}
		orderID = time.Now().UnixNano()
	}

	// 3. 触发推广奖励
	if isFirstRecharge {
		s.HandleInviterRewardOnFirstRecharge(ctx, input.UserID, input.CreditedAmount)
		s.HandleInviteeRewardOnFirstRecharge(ctx, input.UserID, input.CreditedAmount, orderID)
	}

	s.HandleOngoingRewardOnRecharge(ctx, input.UserID, input.CreditedAmount, orderID, isFirstRecharge)

	// 4. 销售佣金（仅当推广人是销售时触发）
	if isSales && s.salesCommissionService != nil {
		if err := s.salesCommissionService.HandleReferralManualCompletion(ctx, ref, input.PayAmountCNY, input.CreditedAmount, "offline recharge: "+input.Note); err != nil {
			slog.Warn("[RecordOfflineRecharge] sales commission creation failed", "userID", input.UserID, "referrerID", ref.ReferrerID, "error", err)
		}
	}

	referrerEmail := ""
	if referrer != nil {
		referrerEmail = referrer.Email
	}

	return &OfflineRechargeResult{
		OrderID:          orderID,
		UserID:           input.UserID,
		ReferrerID:       ref.ReferrerID,
		ReferrerEmail:    referrerEmail,
		IsSalesReferrer:  isSales,
		BalanceCredited:  input.CreditBalance,
		CreditedAmount:   input.CreditedAmount,
		RewardsTriggered: true,
	}, nil
}

// GetUserReferralInfo 获取用户推广信息（推广中心页面数据，强类型）
func (s *ReferralService) GetUserReferralInfo(ctx context.Context, userID int64) (*ReferralUserInfo, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	globalCfg := s.configResolver.GetGlobalConfig(ctx)

	// 根据用户类型判断推广功能是否启用
	isSales := SalesCommissionUserEligible(user)
	enabled := globalCfg.Enabled
	if isSales {
		enabled = globalCfg.SalesEnabled
	}

	// 即使全局未启用，也读取配置以便 admin/UI 展示
	var cfg *EffectiveReferralConfig
	if enabled {
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
		Enabled:                   enabled,
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

// GetGiftBalanceOverview 获取用户赠送余额概览（Header 下拉用）
func (s *ReferralService) GetGiftBalanceOverview(ctx context.Context, userID int64) (*GiftBalanceOverview, error) {
	remaining, err := s.giftBalanceRepo.GetTotalRemainingByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	nextAt, nextAmt, err := s.giftBalanceRepo.GetNextExpiry(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &GiftBalanceOverview{
		GiftBalanceRemaining: remaining,
		NextExpiryAt:         nextAt,
		NextExpiryAmount:     nextAmt,
	}, nil
}

// ValidateReferralCode 验证推广码是否有效
func (s *ReferralService) ValidateReferralCode(ctx context.Context, code string) (bool, error) {
	if code == "" {
		return false, nil
	}
	user, err := s.userRepo.GetByReferralCode(ctx, code)
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, nil
	}
	// 根据推广码所有者类型检查对应开关
	globalCfg := s.configResolver.GetGlobalConfig(ctx)
	if SalesCommissionUserEligible(user) {
		return globalCfg.SalesEnabled, nil
	}
	return globalCfg.Enabled, nil
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
func (s *ReferralService) AdminListReferrals(ctx context.Context, status string, referrerID, refereeID int64, page, pageSize int) ([]Referral, int, error) {
	offset := (page - 1) * pageSize
	return s.referralRepo.ListAll(ctx, status, referrerID, refereeID, offset, pageSize)
}

// AdminGetLeaderboard 管理端获取推广排行榜
// period: all_time / this_month / this_week / custom
func (s *ReferralService) AdminGetLeaderboard(ctx context.Context, period, startDate, endDate string, limit int) ([]ReferralLeaderboardEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.referralRepo.GetLeaderboard(ctx, period, startDate, endDate, limit)
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

// isSalesReferrer 仅当 referrer 同时满足 IsSales 且销售返佣配置完整时返回 true。
//
// 这与 SalesCommissionUserEligible 保持一致，避免 "IsSales=true 但 fixed 模式 rate=0
// 或 tiered 模式 tiers=[]" 的销售身份不完整用户走入 \"普通推广跳过 + 销售返佣跳过\"
// 的双不发缝隙——这种用户应该回退到普通推广奖励路径，确保被邀请人首充/持续充值时
// referrer 仍然能拿到 gift_balance 奖励。
func (s *ReferralService) isSalesReferrer(ctx context.Context, userID int64) bool {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return false
	}
	return SalesCommissionUserEligible(user)
}

// buildRewardNote 构建赠送余额备注，包含被邀请人邮箱(ID)、充值金额、推广人邮箱(ID)
func (s *ReferralService) buildRewardNote(ctx context.Context, prefix string, inviteeID int64, rechargeAmount float64, referrerID int64) string {
	inviteeEmail := ""
	if user, err := s.userRepo.GetByID(ctx, inviteeID); err == nil && user != nil {
		inviteeEmail = user.Email
	}
	if referrerID > 0 {
		referrerEmail := ""
		if user, err := s.userRepo.GetByID(ctx, referrerID); err == nil && user != nil {
			referrerEmail = user.Email
		}
		return fmt.Sprintf("%s（被邀请人: %s(%d), 充值: %.2f, 推广人: %s(%d)）", prefix, inviteeEmail, inviteeID, rechargeAmount, referrerEmail, referrerID)
	}
	return fmt.Sprintf("%s（被邀请人: %s(%d), 充值: %.2f）", prefix, inviteeEmail, inviteeID, rechargeAmount)
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
