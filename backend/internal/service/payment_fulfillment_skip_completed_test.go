package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestPaymentBalanceFulfillment_SkipCompletedCreatesSalesCommission(t *testing.T) {
	ctx := context.Background()
	entClient, sqlDB := newPaymentFulfillmentEntClient(t)

	user, err := entClient.User.Create().
		SetEmail("buyer@example.com").
		SetPasswordHash("hash").
		SetUsername("buyer").
		Save(ctx)
	require.NoError(t, err)

	order, err := entClient.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(200).
		SetPayAmount(123.45).
		SetRechargeCode("retry-code").
		SetOutTradeNo("out-retry-code").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-retry-code").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRecharging).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)

	commissionRepo := &salesCommissionRepoStub{}
	referralRepo := &salesCommissionReferralRepoStub{
		byReferee: map[int64]*Referral{
			user.ID: {ID: 77, ReferrerID: 10, RefereeID: user.ID},
		},
	}
	commissionUserRepo := &salesCommissionUserRepoStub{
		byID: map[int64]*User{
			10: {ID: 10, IsSales: true, SalesCommissionRate: 10},
		},
	}
	paymentSvc := &PaymentService{
		entClient: entClient,
		redeemService: NewRedeemService(
			&paymentFulfillmentRedeemRepoStub{
				code: &RedeemCode{
					ID:     555,
					Code:   "retry-code",
					Type:   RedeemTypeBalance,
					Value:  200,
					Status: StatusUsed,
				},
			},
			nil,
			nil,
			nil,
			nil,
			entClient,
			nil,
		),
		salesCommissionService: NewSalesCommissionService(commissionRepo, referralRepo, commissionUserRepo),
	}

	err = paymentSvc.doBalance(ctx, order)

	require.NoError(t, err)
	require.False(t, paymentSvc.redeemService.redeemRepo.(*paymentFulfillmentRedeemRepoStub).used, "skip-completed path must not redeem again")
	require.Len(t, commissionRepo.created, 1)
	require.Equal(t, order.ID, commissionRepo.created[0].PaymentOrderID)
	require.Equal(t, user.ID, commissionRepo.created[0].RefereeUserID)

	updated, err := entClient.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, updated.Status)

	var auditCount int
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM payment_audit_logs WHERE action = 'RECHARGE_SUCCESS'").Scan(&auditCount))
	require.Equal(t, 1, auditCount)
}

func TestPaymentBalanceFulfillment_CompletedOrderRetriesSalesCommission(t *testing.T) {
	ctx := context.Background()
	entClient, _ := newPaymentFulfillmentEntClient(t)

	user, err := entClient.User.Create().
		SetEmail("completed-buyer@example.com").
		SetPasswordHash("hash").
		SetUsername("completed-buyer").
		Save(ctx)
	require.NoError(t, err)

	order, err := entClient.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(100).
		SetRechargeCode("completed-retry-code").
		SetOutTradeNo("out-completed-retry").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-completed-retry").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetCompletedAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)

	commissionRepo := &salesCommissionRepoStub{}
	paymentSvc := &PaymentService{
		entClient: entClient,
		salesCommissionService: NewSalesCommissionService(
			commissionRepo,
			&salesCommissionReferralRepoStub{
				byReferee: map[int64]*Referral{
					user.ID: {ID: 78, ReferrerID: 10, RefereeID: user.ID},
				},
			},
			&salesCommissionUserRepoStub{
				byID: map[int64]*User{
					10: {ID: 10, IsSales: true, SalesCommissionRate: 10},
				},
			},
		),
	}

	err = paymentSvc.ExecuteBalanceFulfillment(ctx, order.ID)

	require.NoError(t, err)
	require.Len(t, commissionRepo.created, 1)
	require.Equal(t, order.ID, commissionRepo.created[0].PaymentOrderID)
	require.Equal(t, user.ID, commissionRepo.created[0].RefereeUserID)
}

func newPaymentFulfillmentEntClient(t *testing.T) (*dbent.Client, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:payment_fulfillment_skip_completed?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client, db
}

type paymentFulfillmentRedeemRepoStub struct {
	code *RedeemCode
	used bool
}

func (r *paymentFulfillmentRedeemRepoStub) Create(context.Context, *RedeemCode) error { return nil }

func (r *paymentFulfillmentRedeemRepoStub) CreateBatch(context.Context, []RedeemCode) error {
	return nil
}

func (r *paymentFulfillmentRedeemRepoStub) GetByID(context.Context, int64) (*RedeemCode, error) {
	return r.code, nil
}

func (r *paymentFulfillmentRedeemRepoStub) GetByCode(context.Context, string) (*RedeemCode, error) {
	return r.code, nil
}

func (r *paymentFulfillmentRedeemRepoStub) Update(context.Context, *RedeemCode) error { return nil }

func (r *paymentFulfillmentRedeemRepoStub) Delete(context.Context, int64) error { return nil }

func (r *paymentFulfillmentRedeemRepoStub) Use(context.Context, int64, int64, *string) error {
	r.used = true
	return nil
}

func (r *paymentFulfillmentRedeemRepoStub) List(context.Context, pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *paymentFulfillmentRedeemRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *paymentFulfillmentRedeemRepoStub) ListByUser(context.Context, int64, int) ([]RedeemCode, error) {
	return nil, nil
}

func (r *paymentFulfillmentRedeemRepoStub) ListByUserPaginated(context.Context, int64, pagination.PaginationParams, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *paymentFulfillmentRedeemRepoStub) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	return 0, nil
}
