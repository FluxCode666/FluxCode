package service

import (
	"context"
	"errors"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestSalesCommissionService_HandleBalanceRechargeCompleted_CreatesFrozenCommission(t *testing.T) {
	ctx := context.Background()
	repo := &salesCommissionRepoStub{}
	refRepo := &salesCommissionReferralRepoStub{
		byReferee: map[int64]*Referral{
			20: {ID: 7, ReferrerID: 10, RefereeID: 20},
		},
	}
	userRepo := &salesCommissionUserRepoStub{
		byID: map[int64]*User{
			10: {ID: 10, IsSales: true, SalesCommissionRate: 12.5},
		},
	}
	svc := NewSalesCommissionService(repo, refRepo, userRepo)

	err := svc.HandleBalanceRechargeCompleted(ctx, &dbent.PaymentOrder{
		ID:        99,
		UserID:    20,
		OrderType: payment.OrderTypeBalance,
		Status:    payment.OrderStatusCompleted,
		PayAmount: 123.456,
		Amount:    200.12345678,
	})

	require.NoError(t, err)
	require.Len(t, repo.created, 1)
	require.NotNil(t, repo.created[0].PaymentOrderID)
	require.Equal(t, int64(10), repo.created[0].SalesUserID)
	require.Equal(t, int64(20), repo.created[0].RefereeUserID)
	require.Equal(t, int64(7), repo.created[0].ReferralID)
	require.Equal(t, 123.456, repo.created[0].OrderPayAmountCNY)
	require.Equal(t, 200.12345678, repo.created[0].OrderCreditedAmount)
	require.Equal(t, 12.5, repo.created[0].CommissionRate)
	require.Equal(t, 15.43, repo.created[0].CommissionTotalCNY)
	require.Equal(t, "Balance recharge commission", repo.created[0].Note)
	require.Equal(t, int64(99), *repo.created[0].PaymentOrderID)
}

func TestSalesCommissionService_HandleReferralManualCompletion_CreatesCommissionRecord(t *testing.T) {
	ctx := context.Background()
	repo := &salesCommissionRepoStub{}
	refRepo := &salesCommissionReferralRepoStub{}
	userRepo := &salesCommissionUserRepoStub{
		byID: map[int64]*User{
			10: {ID: 10, IsSales: true, SalesCommissionRate: 12.5},
		},
	}
	svc := NewSalesCommissionService(repo, refRepo, userRepo)

	err := svc.HandleReferralManualCompletion(ctx, &Referral{
		ID:         7,
		ReferrerID: 10,
		RefereeID:  20,
		Status:     ReferralStatusPending,
	}, 123.456, 200.12345678, "manual completion")

	require.NoError(t, err)
	require.Len(t, repo.created, 1)
	require.Equal(t, int64(10), repo.created[0].SalesUserID)
	require.Equal(t, int64(20), repo.created[0].RefereeUserID)
	require.Equal(t, int64(7), repo.created[0].ReferralID)
	require.Nil(t, repo.created[0].PaymentOrderID)
	require.Equal(t, 123.456, repo.created[0].OrderPayAmountCNY)
	require.Equal(t, 200.12345678, repo.created[0].OrderCreditedAmount)
	require.Equal(t, 15.43, repo.created[0].CommissionTotalCNY)
	require.Contains(t, repo.created[0].Note, "manual completion")
}

func TestSalesCommissionService_HandleBalanceRechargeCompleted_IgnoresIneligibleOrders(t *testing.T) {
	tests := []struct {
		name  string
		order *dbent.PaymentOrder
	}{
		{
			name: "nil order",
		},
		{
			name: "subscription order",
			order: &dbent.PaymentOrder{
				OrderType: payment.OrderTypeSubscription,
				Status:    payment.OrderStatusCompleted,
				PayAmount: 100,
				Amount:    100,
			},
		},
		{
			name: "not completed",
			order: &dbent.PaymentOrder{
				OrderType: payment.OrderTypeBalance,
				Status:    payment.OrderStatusPaid,
				PayAmount: 100,
				Amount:    100,
			},
		},
		{
			name: "zero pay amount",
			order: &dbent.PaymentOrder{
				OrderType: payment.OrderTypeBalance,
				Status:    payment.OrderStatusCompleted,
				PayAmount: 0,
				Amount:    100,
			},
		},
		{
			name: "zero credited amount",
			order: &dbent.PaymentOrder{
				OrderType: payment.OrderTypeBalance,
				Status:    payment.OrderStatusCompleted,
				PayAmount: 100,
				Amount:    0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &salesCommissionRepoStub{}
			refRepo := &salesCommissionReferralRepoStub{}
			userRepo := &salesCommissionUserRepoStub{}
			svc := NewSalesCommissionService(repo, refRepo, userRepo)

			err := svc.HandleBalanceRechargeCompleted(context.Background(), tt.order)

			require.NoError(t, err)
			require.Empty(t, repo.created)
			require.Zero(t, refRepo.getByRefereeCalls)
		})
	}
}

func TestSalesCommissionService_HandleBalanceRechargeCompleted_IgnoresMissingOrNonSalesReferrer(t *testing.T) {
	tests := []struct {
		name      string
		ref       *Referral
		refErr    error
		referrer  *User
		userErr   error
		wantError error
	}{
		{name: "no referral"},
		{name: "referral lookup error", refErr: errors.New("referral store down"), wantError: errors.New("referral store down")},
		{name: "referrer lookup error", ref: &Referral{ID: 1, ReferrerID: 10, RefereeID: 20}, userErr: errors.New("user store down"), wantError: errors.New("user store down")},
		{name: "not sales", ref: &Referral{ID: 1, ReferrerID: 10, RefereeID: 20}, referrer: &User{ID: 10, IsSales: false, SalesCommissionRate: 10}},
		{name: "zero rate", ref: &Referral{ID: 1, ReferrerID: 10, RefereeID: 20}, referrer: &User{ID: 10, IsSales: true, SalesCommissionRate: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &salesCommissionRepoStub{}
			refRepo := &salesCommissionReferralRepoStub{
				byReferee: map[int64]*Referral{20: tt.ref},
				err:       tt.refErr,
			}
			userRepo := &salesCommissionUserRepoStub{
				byID: map[int64]*User{10: tt.referrer},
				err:  tt.userErr,
			}
			svc := NewSalesCommissionService(repo, refRepo, userRepo)

			err := svc.HandleBalanceRechargeCompleted(context.Background(), completedBalanceOrder())

			if tt.wantError != nil {
				require.ErrorContains(t, err, tt.wantError.Error())
			} else {
				require.NoError(t, err)
			}
			require.Empty(t, repo.created)
		})
	}
}

func TestSalesCommissionService_CreateSettlement_RejectsInvalidAmount(t *testing.T) {
	svc := NewSalesCommissionService(&salesCommissionRepoStub{}, &salesCommissionReferralRepoStub{}, &salesCommissionUserRepoStub{})

	for _, input := range []*SalesCommissionSettlementCreate{
		nil,
		{SalesUserID: 10, AmountCNY: 0},
		{SalesUserID: 10, AmountCNY: -1},
	} {
		got, err := svc.CreateSettlement(context.Background(), input)

		require.Nil(t, got)
		require.ErrorIs(t, err, ErrSalesCommissionInvalidAmount)
	}
}

func TestSalesCommissionService_ForwardsReadOperationsToRepository(t *testing.T) {
	ctx := context.Background()
	repo := &salesCommissionRepoStub{
		summaries:   []SalesCommissionSummary{{SalesUserID: 10}},
		records:     []SalesCommissionRecord{{ID: 1}},
		settlements: []SalesCommissionSettlement{{ID: 2}},
		summary:     &SalesCommissionSummary{SalesUserID: 10},
		settlement:  &SalesCommissionSettlement{ID: 3},
	}
	svc := NewSalesCommissionService(repo, &salesCommissionReferralRepoStub{}, &salesCommissionUserRepoStub{})

	summaries, total, err := svc.ListSummaries(ctx, SalesCommissionSummaryListParams{Page: 2, PageSize: 3})
	require.NoError(t, err)
	require.Equal(t, repo.summaries, summaries)
	require.Equal(t, 1, total)
	require.Equal(t, SalesCommissionSummaryListParams{Page: 2, PageSize: 3}, repo.lastSummaryParams)

	summary, err := svc.GetSummary(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, repo.summary, summary)

	records, total, err := svc.ListRecords(ctx, SalesCommissionRecordListParams{SalesUserID: 10, Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, repo.records, records)
	require.Equal(t, 1, total)
	require.Equal(t, SalesCommissionRecordListParams{SalesUserID: 10, Page: 1, PageSize: 20}, repo.lastRecordParams)

	settlement, err := svc.CreateSettlement(ctx, &SalesCommissionSettlementCreate{SalesUserID: 10, AmountCNY: 5})
	require.NoError(t, err)
	require.Equal(t, repo.settlement, settlement)

	settlements, total, err := svc.ListSettlements(ctx, SalesCommissionSettlementListParams{SalesUserID: 10, Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, repo.settlements, settlements)
	require.Equal(t, 1, total)
	require.Equal(t, SalesCommissionSettlementListParams{SalesUserID: 10, Page: 1, PageSize: 20}, repo.lastSettlementParams)
}

func completedBalanceOrder() *dbent.PaymentOrder {
	return &dbent.PaymentOrder{
		ID:        99,
		UserID:    20,
		OrderType: payment.OrderTypeBalance,
		Status:    payment.OrderStatusCompleted,
		PayAmount: 100,
		Amount:    100,
	}
}

type salesCommissionRepoStub struct {
	created               []*SalesCommissionCreate
	summaries             []SalesCommissionSummary
	summary               *SalesCommissionSummary
	records               []SalesCommissionRecord
	settlement            *SalesCommissionSettlement
	settlements           []SalesCommissionSettlement
	lastSummaryParams     SalesCommissionSummaryListParams
	lastRecordParams      SalesCommissionRecordListParams
	lastSettlementParams  SalesCommissionSettlementListParams
	createSettlementInput *SalesCommissionSettlementCreate
}

func (r *salesCommissionRepoStub) CreateForOrder(_ context.Context, input *SalesCommissionCreate) error {
	r.created = append(r.created, input)
	return nil
}

func (r *salesCommissionRepoStub) ListSummaries(_ context.Context, params SalesCommissionSummaryListParams) ([]SalesCommissionSummary, int, error) {
	r.lastSummaryParams = params
	return r.summaries, len(r.summaries), nil
}

func (r *salesCommissionRepoStub) GetSummaryBySalesUser(_ context.Context, salesUserID int64) (*SalesCommissionSummary, error) {
	if r.summary != nil {
		return r.summary, nil
	}
	return &SalesCommissionSummary{SalesUserID: salesUserID}, nil
}

func (r *salesCommissionRepoStub) ListRecords(_ context.Context, params SalesCommissionRecordListParams) ([]SalesCommissionRecord, int, error) {
	r.lastRecordParams = params
	return r.records, len(r.records), nil
}

func (r *salesCommissionRepoStub) CreateSettlement(_ context.Context, input *SalesCommissionSettlementCreate) (*SalesCommissionSettlement, error) {
	r.createSettlementInput = input
	return r.settlement, nil
}

func (r *salesCommissionRepoStub) ListSettlements(_ context.Context, params SalesCommissionSettlementListParams) ([]SalesCommissionSettlement, int, error) {
	r.lastSettlementParams = params
	return r.settlements, len(r.settlements), nil
}

type salesCommissionReferralRepoStub struct {
	byReferee         map[int64]*Referral
	err               error
	getByRefereeCalls int
}

func (r *salesCommissionReferralRepoStub) Create(context.Context, *Referral) error { return nil }

func (r *salesCommissionReferralRepoStub) GetByID(_ context.Context, _ int64) (*Referral, error) {
	return nil, r.err
}

func (r *salesCommissionReferralRepoStub) GetByRefereeID(_ context.Context, refereeID int64) (*Referral, error) {
	r.getByRefereeCalls++
	if r.err != nil {
		return nil, r.err
	}
	return r.byReferee[refereeID], nil
}

func (r *salesCommissionReferralRepoStub) GetByReferrerID(context.Context, int64, int, int) ([]Referral, int, error) {
	return nil, 0, nil
}

func (r *salesCommissionReferralRepoStub) CountByReferrerID(context.Context, int64) (int, error) {
	return 0, nil
}

func (r *salesCommissionReferralRepoStub) UpdateStatus(context.Context, int64, string) error {
	return nil
}

func (r *salesCommissionReferralRepoStub) MarkCompleted(context.Context, int64, float64, string) error {
	return nil
}

func (r *salesCommissionReferralRepoStub) SetInviteeRewarded(context.Context, int64) error {
	return nil
}

func (r *salesCommissionReferralRepoStub) SetInviterRewarded(context.Context, int64, float64) error {
	return nil
}

func (r *salesCommissionReferralRepoStub) IncrementOngoingReward(context.Context, int64, float64) error {
	return nil
}

func (r *salesCommissionReferralRepoStub) GetStatsByReferrerID(context.Context, int64) (*ReferralStats, error) {
	return nil, nil
}

func (r *salesCommissionReferralRepoStub) ListAll(context.Context, string, int, int) ([]Referral, int, error) {
	return nil, 0, nil
}

func (r *salesCommissionReferralRepoStub) GetLeaderboard(context.Context, string, int) ([]ReferralLeaderboardEntry, error) {
	return nil, nil
}

func (r *salesCommissionReferralRepoStub) GetTrendByReferrerID(context.Context, int64, int) ([]ReferralTrendPoint, error) {
	return nil, nil
}

func (r *salesCommissionReferralRepoStub) GetGlobalTrend(context.Context, int) ([]ReferralTrendPoint, error) {
	return nil, nil
}

func (r *salesCommissionReferralRepoStub) CountFirstRecharges(context.Context) (int, error) {
	return 0, nil
}

func (r *salesCommissionReferralRepoStub) CountAll(context.Context) (int, error) { return 0, nil }

type salesCommissionUserRepoStub struct {
	byID map[int64]*User
	err  error
}

func (r *salesCommissionUserRepoStub) Create(context.Context, *User) error { return nil }

func (r *salesCommissionUserRepoStub) GetByID(_ context.Context, id int64) (*User, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.byID[id], nil
}

func (r *salesCommissionUserRepoStub) GetByEmail(context.Context, string) (*User, error) {
	return nil, nil
}

func (r *salesCommissionUserRepoStub) GetFirstAdmin(context.Context) (*User, error) { return nil, nil }

func (r *salesCommissionUserRepoStub) Update(context.Context, *User) error { return nil }

func (r *salesCommissionUserRepoStub) Delete(context.Context, int64) error { return nil }

func (r *salesCommissionUserRepoStub) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *salesCommissionUserRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *salesCommissionUserRepoStub) UpdateBalance(context.Context, int64, float64) error {
	return nil
}

func (r *salesCommissionUserRepoStub) DeductBalance(context.Context, int64, float64) error {
	return nil
}

func (r *salesCommissionUserRepoStub) UpdateConcurrency(context.Context, int64, int) error {
	return nil
}

func (r *salesCommissionUserRepoStub) ExistsByEmail(context.Context, string) (bool, error) {
	return false, nil
}

func (r *salesCommissionUserRepoStub) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}

func (r *salesCommissionUserRepoStub) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	return nil
}

func (r *salesCommissionUserRepoStub) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	return nil
}

func (r *salesCommissionUserRepoStub) UpdateTotpSecret(context.Context, int64, *string) error {
	return nil
}

func (r *salesCommissionUserRepoStub) EnableTotp(context.Context, int64) error { return nil }

func (r *salesCommissionUserRepoStub) DisableTotp(context.Context, int64) error { return nil }

func (r *salesCommissionUserRepoStub) GetByReferralCode(context.Context, string) (*User, error) {
	return nil, nil
}

func (r *salesCommissionUserRepoStub) UpdateReferralCode(context.Context, int64, string) error {
	return nil
}

func (r *salesCommissionUserRepoStub) UpdateReferredBy(context.Context, int64, int64) error {
	return nil
}

func (r *salesCommissionUserRepoStub) IsFirstRecharge(context.Context, int64) (bool, error) {
	return false, nil
}

func (r *salesCommissionUserRepoStub) ListActiveUserIDs(context.Context) ([]int64, error) {
	return nil, nil
}
