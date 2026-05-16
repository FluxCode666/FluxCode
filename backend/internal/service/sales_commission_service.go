package service

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

type SalesCommissionService struct {
	repo         SalesCommissionRepository
	referralRepo ReferralRepository
	userRepo     UserRepository
}

func NewSalesCommissionService(repo SalesCommissionRepository, referralRepo ReferralRepository, userRepo UserRepository) *SalesCommissionService {
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
	if referrer == nil || !referrer.IsSales || referrer.SalesCommissionRate <= 0 {
		return nil
	}

	commissionTotal, _ := decimal.NewFromFloat(order.PayAmount).
		Mul(decimal.NewFromFloat(referrer.SalesCommissionRate)).
		Div(decimal.NewFromInt(100)).
		Round(2).
		Float64()

	return s.repo.CreateForOrder(ctx, &SalesCommissionCreate{
		SalesUserID:         ref.ReferrerID,
		RefereeUserID:       order.UserID,
		ReferralID:          ref.ID,
		PaymentOrderID:      &order.ID,
		OrderPayAmountCNY:   order.PayAmount,
		OrderCreditedAmount: order.Amount,
		CommissionRate:      referrer.SalesCommissionRate,
		CommissionTotalCNY:  commissionTotal,
		Note:                "Balance recharge commission",
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
	if referrer == nil || !referrer.IsSales || referrer.SalesCommissionRate <= 0 {
		return nil
	}

	commissionTotal, _ := decimal.NewFromFloat(orderPayAmountCNY).
		Mul(decimal.NewFromFloat(referrer.SalesCommissionRate)).
		Div(decimal.NewFromInt(100)).
		Round(2).
		Float64()

	return s.repo.CreateForOrder(ctx, &SalesCommissionCreate{
		SalesUserID:         ref.ReferrerID,
		RefereeUserID:       ref.RefereeID,
		ReferralID:          ref.ID,
		PaymentOrderID:      nil,
		OrderPayAmountCNY:   orderPayAmountCNY,
		OrderCreditedAmount: orderCreditedAmount,
		CommissionRate:      referrer.SalesCommissionRate,
		CommissionTotalCNY:  commissionTotal,
		Note:                "Referral manual completion: " + note,
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
