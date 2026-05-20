package service

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

var salesCommissionMonthLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type salesCommissionUserRepository interface {
	GetByID(ctx context.Context, id int64) (*User, error)
}

type SalesCommissionService struct {
	repo         SalesCommissionRepository
	referralRepo ReferralRepository
	userRepo     salesCommissionUserRepository
}

func NewSalesCommissionService(repo SalesCommissionRepository, referralRepo ReferralRepository, userRepo salesCommissionUserRepository) *SalesCommissionService {
	return &SalesCommissionService{
		repo:         repo,
		referralRepo: referralRepo,
		userRepo:     userRepo,
	}
}

func (s *SalesCommissionService) HandleBalanceRechargeCompleted(ctx context.Context, order *dbent.PaymentOrder) error {
	if s == nil || s.repo == nil || s.referralRepo == nil || s.userRepo == nil || order == nil {
		return nil
	}
	if order.OrderType != payment.OrderTypeBalance ||
		order.Status != payment.OrderStatusCompleted ||
		order.PayAmount <= 0 ||
		order.Amount <= 0 {
		return nil
	}

	ref, err := s.referralRepo.GetByRefereeID(ctx, order.UserID)
	if err != nil {
		return err
	}
	if ref == nil {
		return nil
	}

	referrer, err := s.userRepo.GetByID(ctx, ref.ReferrerID)
	if err != nil {
		return err
	}
	if !SalesCommissionUserEligible(referrer) {
		return nil
	}

	eventAt := salesCommissionEventTimeFromOrder(order)

	return s.repo.CreateForOrder(ctx, &SalesCommissionCreate{
		SalesUserID:               ref.ReferrerID,
		RefereeUserID:             order.UserID,
		ReferralID:                ref.ID,
		PaymentOrderID:            &order.ID,
		OrderPayAmountCNY:         order.PayAmount,
		OrderCreditedAmount:       order.Amount,
		CommissionMode:            NormalizeSalesCommissionMode(referrer.SalesCommissionMode),
		CommissionRate:            referrer.SalesCommissionRate,
		CommissionMinMonthlySales: referrer.SalesCommissionMinMonthlySales,
		CommissionTiers:           CloneSalesCommissionTiers(referrer.SalesCommissionTiers),
		CommissionEventAt:         eventAt,
		CommissionMonth:           salesCommissionMonthStart(eventAt),
		Note:                      "Balance recharge commission",
	})
}

func (s *SalesCommissionService) HandleReferralManualCompletion(ctx context.Context, ref *Referral, orderPayAmountCNY float64, orderCreditedAmount float64, note string) error {
	if s == nil || s.repo == nil || s.userRepo == nil || ref == nil {
		return nil
	}
	if orderPayAmountCNY <= 0 || orderCreditedAmount <= 0 {
		return nil
	}

	referrer, err := s.userRepo.GetByID(ctx, ref.ReferrerID)
	if err != nil {
		return err
	}
	if !SalesCommissionUserEligible(referrer) {
		return nil
	}

	eventAt := time.Now().UTC()

	return s.repo.CreateForOrder(ctx, &SalesCommissionCreate{
		SalesUserID:               ref.ReferrerID,
		RefereeUserID:             ref.RefereeID,
		ReferralID:                ref.ID,
		PaymentOrderID:            nil,
		OrderPayAmountCNY:         orderPayAmountCNY,
		OrderCreditedAmount:       orderCreditedAmount,
		CommissionMode:            NormalizeSalesCommissionMode(referrer.SalesCommissionMode),
		CommissionRate:            referrer.SalesCommissionRate,
		CommissionMinMonthlySales: referrer.SalesCommissionMinMonthlySales,
		CommissionTiers:           CloneSalesCommissionTiers(referrer.SalesCommissionTiers),
		CommissionEventAt:         eventAt,
		CommissionMonth:           salesCommissionMonthStart(eventAt),
		Note:                      "Referral manual completion: " + note,
	})
}

func (s *SalesCommissionService) ListSummaries(ctx context.Context, params SalesCommissionSummaryListParams) ([]SalesCommissionSummary, int, error) {
	return s.repo.ListSummaries(ctx, params)
}

func (s *SalesCommissionService) GetSummaryBySalesUser(ctx context.Context, salesUserID int64) (*SalesCommissionSummary, error) {
	return s.repo.GetSummaryBySalesUser(ctx, salesUserID)
}

func (s *SalesCommissionService) GetSummary(ctx context.Context, salesUserID int64) (*SalesCommissionSummary, error) {
	return s.GetSummaryBySalesUser(ctx, salesUserID)
}

func (s *SalesCommissionService) IsSalesUser(ctx context.Context, userID int64) (bool, error) {
	if s == nil || s.userRepo == nil {
		return false, nil
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}
	return user != nil && user.IsSales, nil
}

func (s *SalesCommissionService) ListRecords(ctx context.Context, params SalesCommissionRecordListParams) ([]SalesCommissionRecord, int, error) {
	return s.repo.ListRecords(ctx, params)
}

func (s *SalesCommissionService) CreateSettlement(ctx context.Context, input *SalesCommissionSettlementCreate) (*SalesCommissionSettlement, error) {
	if input == nil || input.AmountCNY <= 0 {
		return nil, ErrSalesCommissionInvalidAmount
	}
	return s.repo.CreateSettlement(ctx, input)
}

func (s *SalesCommissionService) ListSettlements(ctx context.Context, params SalesCommissionSettlementListParams) ([]SalesCommissionSettlement, int, error) {
	return s.repo.ListSettlements(ctx, params)
}

// GetMonthlyProgress 返回销售用户当月梯度进度（spec §9）。
//
//   - 当月已经产生过返佣事件 → 使用 sales_commission_monthly_snapshots 已冻结的规则展示。
//   - 当月还没有事件 → 用 user 当前规则做 "下笔订单将使用的预期规则" 展示，
//     并在响应里通过 snapshot_frozen=false 让前端区分两种语义（spec §8.4）。
//
// 当用户既没有销售身份也没有任何梯度配置时返回 nil，由调用方决定是否当作未授权处理。
func (s *SalesCommissionService) GetMonthlyProgress(ctx context.Context, salesUserID int64) (*SalesCommissionMonthlyProgress, error) {
	if s == nil || s.repo == nil || s.userRepo == nil {
		return nil, nil
	}
	user, err := s.userRepo.GetByID(ctx, salesUserID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsSales {
		return nil, nil
	}

	now := time.Now().UTC()
	commissionMonth := salesCommissionMonthStart(now)

	data, err := s.repo.GetMonthlyProgress(ctx, salesUserID, commissionMonth)
	if err != nil {
		return nil, err
	}

	progress := &SalesCommissionMonthlyProgress{
		SalesUserID:      salesUserID,
		CommissionMonth:  commissionMonth,
		CurrentTierIndex: -1,
		NextTierIndex:    -1,
		Tiers:            []SalesCommissionTier{},
	}

	var snapshot SalesCommissionSnapshot
	if data != nil && data.Snapshot != nil {
		snapshot = *data.Snapshot
		progress.SnapshotFrozen = true
		progress.MonthlySalesCNY = data.MonthlySalesCNY
		progress.MonthlyCommissionCNY = data.MonthlyCommissionCNY
	} else {
		// 当月无 snapshot，按 user 现行规则做 "预期" 展示。
		snapshot = SalesCommissionSnapshot{
			CommissionMode:      NormalizeSalesCommissionMode(user.SalesCommissionMode),
			FixedCommissionRate: user.SalesCommissionRate,
			MinMonthlySalesCNY:  user.SalesCommissionMinMonthlySales,
			Tiers:               CloneSalesCommissionTiers(user.SalesCommissionTiers),
		}
		progress.SnapshotFrozen = false
	}
	progress.CommissionMode = NormalizeSalesCommissionMode(snapshot.CommissionMode)
	if progress.CommissionMode == SalesCommissionModeTiered {
		// tiered 模式下 fixed_commission_rate 不参与计算，统一返回 0 给前端避免误读。
		progress.FixedCommissionRate = 0
	} else {
		progress.FixedCommissionRate = snapshot.FixedCommissionRate
	}
	progress.MinMonthlySalesCNY = snapshot.MinMonthlySalesCNY
	progress.Tiers = CloneSalesCommissionTiers(snapshot.Tiers)

	progress.ThresholdMet = progress.MonthlySalesCNY >= snapshot.MinMonthlySalesCNY
	if !progress.ThresholdMet {
		progress.ToThresholdCNY = roundSalesCommissionMoney(snapshot.MinMonthlySalesCNY - progress.MonthlySalesCNY)
		if progress.ToThresholdCNY < 0 {
			progress.ToThresholdCNY = 0
		}
	}

	if progress.CommissionMode == SalesCommissionModeTiered && len(progress.Tiers) > 0 {
		idx, next := locateMonthlyTierProgress(progress.MonthlySalesCNY, progress.Tiers)
		progress.CurrentTierIndex = idx
		progress.NextTierIndex = next
		if next >= 0 && next < len(progress.Tiers) {
			progress.NextTierRate = progress.Tiers[next].CommissionRate
			diff := progress.Tiers[next].MonthSalesFromCNY - progress.MonthlySalesCNY
			if diff < 0 {
				diff = 0
			}
			progress.ToNextTierCNY = roundSalesCommissionMoney(diff)
		}
	}

	return progress, nil
}

// locateMonthlyTierProgress 给定当月销售额与已规范排序的梯度档，定位 (current, next) 索引。
//   - current = 销售额命中的档位索引；销售额为 0 或不足以进入第 1 档时为 -1。
//   - next = 当前所在档之后的下一档索引；已位于最高（开口）档时为 -1。
func locateMonthlyTierProgress(monthlySales float64, tiers []SalesCommissionTier) (int, int) {
	current := -1
	for i, tier := range tiers {
		if monthlySales >= tier.MonthSalesFromCNY {
			current = i
		} else {
			break
		}
	}
	if current < 0 {
		// 未达任何档位入口（通常代表第 1 档下界 > 0），下一档就是第 1 档。
		if len(tiers) > 0 {
			return -1, 0
		}
		return -1, -1
	}
	next := current + 1
	if next >= len(tiers) {
		next = -1
	}
	// 命中开口档（无 to 上限）时也视为已在最高档。
	if current >= 0 && tiers[current].MonthSalesToCNY == nil {
		next = -1
	}
	return current, next
}

func salesCommissionEventTimeFromOrder(order *dbent.PaymentOrder) time.Time {
	if order == nil {
		return time.Now().UTC()
	}
	if order.PaidAt != nil && !order.PaidAt.IsZero() {
		return order.PaidAt.UTC()
	}
	if order.CompletedAt != nil && !order.CompletedAt.IsZero() {
		return order.CompletedAt.UTC()
	}
	if !order.CreatedAt.IsZero() {
		return order.CreatedAt.UTC()
	}
	return time.Now().UTC()
}

func salesCommissionMonthStart(eventAt time.Time) time.Time {
	local := eventAt.In(salesCommissionMonthLocation)
	return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, time.UTC)
}
