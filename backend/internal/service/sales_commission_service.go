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
