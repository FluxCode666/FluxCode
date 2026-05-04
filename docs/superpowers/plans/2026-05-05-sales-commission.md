# Sales Commission Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build sales-user commission accounting where invited-user balance recharges create frozen CNY commission and invited-user ordinary balance usage unlocks commission proportionally.

**Architecture:** Add sales flags to `users`, store commission and settlement rows in dedicated SQL tables, and keep commission accounting independent from existing referral gift balance. Payment fulfillment creates frozen commission records; usage billing unlocks records inside the existing idempotent billing transaction; admin/user handlers expose summaries, records, and manual settlement.

**Tech Stack:** Go, Ent, PostgreSQL migrations, Wire, Gin, `shopspring/decimal`, Vue 3, TypeScript, Vitest.

---

## File Structure

- Modify `backend/ent/schema/user.go`: add `is_sales` and `sales_commission_rate`.
- Create `backend/ent/schema/sales_commission_record.go`: Ent schema for commission records.
- Create `backend/ent/schema/sales_commission_settlement.go`: Ent schema for settlement batches.
- Create `backend/ent/schema/sales_commission_settlement_item.go`: Ent schema for settlement allocation rows.
- Create `backend/migrations/111_sales_commissions.sql`: database migration for sales flags and commission tables.
- Modify generated Ent files under `backend/ent/`: produced by `go generate ./ent`.
- Modify `backend/internal/service/user.go`, `backend/internal/repository/api_key_repo.go`, `backend/internal/repository/user_repo.go`, `backend/internal/service/admin_service.go`, `backend/internal/handler/dto/types.go`, `backend/internal/handler/dto/mappers.go`, `backend/internal/handler/admin/user_handler.go`: carry sales flags through admin user read/write.
- Create `backend/internal/service/sales_commission.go`: service models, constants, and repository interface.
- Create `backend/internal/service/sales_commission_service.go`: business operations for recharge commission creation, summary/listing, and settlement.
- Create `backend/internal/repository/sales_commission_repo.go`: SQL repository for commission listing, creation, summary, and settlement.
- Modify `backend/internal/repository/usage_billing_repo.go`: unlock commission FIFO when ordinary balance is deducted.
- Modify `backend/internal/service/payment_service.go` and `backend/internal/service/payment_fulfillment.go`: inject sales commission service and create frozen commission after balance recharge completion.
- Modify `backend/internal/repository/wire.go`, `backend/internal/service/wire.go`, `backend/internal/handler/wire.go`, `backend/internal/handler/handler.go`, `backend/cmd/server/wire_gen.go`: wire repository, service, and handlers.
- Create `backend/internal/handler/admin/sales_commission_handler.go`: admin summary, records, settlements, create settlement.
- Create `backend/internal/handler/sales_commission_handler.go`: user-facing summary and records.
- Modify `backend/internal/server/routes/admin.go` and `backend/internal/server/routes/user.go`: register route groups.
- Modify `frontend/src/types/index.ts`: add sales user fields and commission types.
- Modify `frontend/src/api/admin/users.ts`: allow sales fields in update payload.
- Create `frontend/src/api/admin/salesCommissions.ts`: admin sales commission API.
- Create `frontend/src/api/salesCommissions.ts`: user sales commission API.
- Modify `frontend/src/api/admin/index.ts`: export admin sales commission API.
- Modify `frontend/src/components/admin/user/UserEditModal.vue`: add sales controls.
- Modify `frontend/src/router/index.ts`: add admin and user sales commission routes.
- Modify `frontend/src/components/layout/AppSidebar.vue`: add admin nav item and conditional user nav item.
- Create `frontend/src/views/admin/SalesCommissionsView.vue`: admin commission management page.
- Create `frontend/src/views/user/SalesCommissionsView.vue`: sales user read-only page.
- Modify `frontend/src/i18n/locales/zh.ts` and `frontend/src/i18n/locales/en.ts`: add nav and page copy.

## Commands

Run backend commands from `backend/`:

```bash
go test ./internal/service ./internal/handler ./internal/server/routes
go test -tags=integration ./internal/repository
go generate ./ent
go generate ./cmd/server
go test ./...
```

Run frontend commands from `frontend/`:

```bash
pnpm test -- --run
pnpm type-check
```

---

### Task 1: Schema, Migration, and User Sales Fields

**Files:**
- Modify: `backend/ent/schema/user.go`
- Create: `backend/ent/schema/sales_commission_record.go`
- Create: `backend/ent/schema/sales_commission_settlement.go`
- Create: `backend/ent/schema/sales_commission_settlement_item.go`
- Create: `backend/migrations/111_sales_commissions.sql`
- Modify: `backend/internal/repository/migrations_schema_integration_test.go`
- Modify: `backend/internal/repository/user_repo_integration_test.go`
- Modify: `backend/internal/service/user.go`
- Modify: `backend/internal/repository/api_key_repo.go`
- Modify: `backend/internal/repository/user_repo.go`
- Test: `backend/internal/repository/migrations_schema_integration_test.go`
- Test: `backend/internal/repository/user_repo_integration_test.go`

- [ ] **Step 1: Write the failing migration/schema test**

Append assertions to `TestMigrationsRunner_IsIdempotent_AndSchemaIsUpToDate` in `backend/internal/repository/migrations_schema_integration_test.go`:

```go
	// sales commissions: user flags and accounting tables
	requireColumn(t, tx, "users", "is_sales", "boolean", 0, false)
	requireColumn(t, tx, "users", "sales_commission_rate", "numeric", 0, false)

	var salesCommissionRecordsRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.sales_commission_records')").Scan(&salesCommissionRecordsRegclass))
	require.True(t, salesCommissionRecordsRegclass.Valid, "expected sales_commission_records table to exist")
	requireColumn(t, tx, "sales_commission_records", "sales_user_id", "bigint", 0, false)
	requireColumn(t, tx, "sales_commission_records", "referee_user_id", "bigint", 0, false)
	requireColumn(t, tx, "sales_commission_records", "payment_order_id", "bigint", 0, false)
	requireColumn(t, tx, "sales_commission_records", "commission_total_cny", "numeric", 0, false)
	requireColumn(t, tx, "sales_commission_records", "credited_used_amount", "numeric", 0, false)
	requireIndex(t, tx, "sales_commission_records", "idx_sales_commission_sales_user")
	requireIndex(t, tx, "sales_commission_records", "idx_sales_commission_referee")
	requireIndex(t, tx, "sales_commission_records", "idx_sales_commission_status")

	var salesCommissionSettlementsRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.sales_commission_settlements')").Scan(&salesCommissionSettlementsRegclass))
	require.True(t, salesCommissionSettlementsRegclass.Valid, "expected sales_commission_settlements table to exist")
	requireColumn(t, tx, "sales_commission_settlements", "amount_cny", "numeric", 0, false)

	var salesCommissionSettlementItemsRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.sales_commission_settlement_items')").Scan(&salesCommissionSettlementItemsRegclass))
	require.True(t, salesCommissionSettlementItemsRegclass.Valid, "expected sales_commission_settlement_items table to exist")
	requireColumn(t, tx, "sales_commission_settlement_items", "commission_record_id", "bigint", 0, false)
```

- [ ] **Step 2: Write the failing user repository test**

Append this test to `backend/internal/repository/user_repo_integration_test.go`:

```go
func (s *UserRepoSuite) TestUserSalesCommissionFieldsPersist() {
	user := s.mustCreateUser(&service.User{
		Email:               "sales-flags@test.com",
		IsSales:             true,
		SalesCommissionRate: 12.5,
	})

	got, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Require().True(got.IsSales)
	s.Require().InDelta(12.5, got.SalesCommissionRate, 0.000001)

	got.IsSales = false
	got.SalesCommissionRate = 0
	s.Require().NoError(s.repo.Update(s.ctx, got))

	updated, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Require().False(updated.IsSales)
	s.Require().InDelta(0, updated.SalesCommissionRate, 0.000001)
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run:

```bash
cd backend
go test -tags=integration ./internal/repository -run 'TestMigrationsRunner_IsIdempotent_AndSchemaIsUpToDate|TestUserRepoSuite/TestUserSalesCommissionFieldsPersist' -count=1
```

Expected: FAIL because `users.is_sales`, `users.sales_commission_rate`, and sales commission tables do not exist, and `service.User` has no sales fields.

- [ ] **Step 4: Add Ent schemas and SQL migration**

In `backend/ent/schema/user.go`, add fields inside `Fields()`:

```go
		field.Bool("is_sales").
			Default(false),
		field.Float("sales_commission_rate").
			SchemaType(map[string]string{dialect.Postgres: "decimal(8,4)"}).
			Default(0),
```

Create `backend/ent/schema/sales_commission_record.go`:

```go
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type SalesCommissionRecord struct {
	ent.Schema
}

func (SalesCommissionRecord) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "sales_commission_records"}}
}

func (SalesCommissionRecord) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("sales_user_id"),
		field.Int64("referee_user_id"),
		field.Int64("referral_id"),
		field.Int64("payment_order_id"),
		field.Float("order_pay_amount_cny").SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}),
		field.Float("order_credited_amount").SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Float("commission_rate").SchemaType(map[string]string{dialect.Postgres: "decimal(8,4)"}),
		field.Float("commission_total_cny").SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}),
		field.Float("credited_used_amount").SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).Default(0),
		field.Float("unlocked_cny").SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).Default(0),
		field.Float("settled_cny").SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).Default(0),
		field.String("status").MaxLen(32).Default("frozen"),
		field.String("note").SchemaType(map[string]string{dialect.Postgres: "text"}).Default(""),
		field.Time("created_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SalesCommissionRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("payment_order_id").Unique(),
		index.Fields("sales_user_id", "created_at"),
		index.Fields("referee_user_id", "id"),
		index.Fields("status"),
	}
}
```

Create `backend/ent/schema/sales_commission_settlement.go`:

```go
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type SalesCommissionSettlement struct {
	ent.Schema
}

func (SalesCommissionSettlement) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "sales_commission_settlements"}}
}

func (SalesCommissionSettlement) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("sales_user_id"),
		field.Float("amount_cny").SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}),
		field.String("note").SchemaType(map[string]string{dialect.Postgres: "text"}).Default(""),
		field.Int64("created_by").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Immutable(),
	}
}

func (SalesCommissionSettlement) Indexes() []ent.Index {
	return []ent.Index{index.Fields("sales_user_id", "created_at")}
}
```

Create `backend/ent/schema/sales_commission_settlement_item.go`:

```go
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type SalesCommissionSettlementItem struct {
	ent.Schema
}

func (SalesCommissionSettlementItem) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "sales_commission_settlement_items"}}
}

func (SalesCommissionSettlementItem) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("settlement_id"),
		field.Int64("commission_record_id"),
		field.Float("amount_cny").SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}),
		field.Time("created_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Immutable(),
	}
}

func (SalesCommissionSettlementItem) Indexes() []ent.Index {
	return []ent.Index{index.Fields("commission_record_id")}
}
```

Create `backend/migrations/111_sales_commissions.sql` with the SQL from the design document. Include `COMMENT ON COLUMN` statements for `users.is_sales`, `users.sales_commission_rate`, `sales_commission_records.commission_total_cny`, and `sales_commission_records.credited_used_amount`.

- [ ] **Step 5: Add sales fields to service and user repository mapping**

In `backend/internal/service/user.go`, add fields to `User`:

```go
	// 销售佣金
	IsSales             bool
	SalesCommissionRate float64
```

In `backend/internal/repository/user_repo.go`, set the fields in `Create()` and `Update()`:

```go
		SetIsSales(userIn.IsSales).
		SetSalesCommissionRate(userIn.SalesCommissionRate).
```

In `backend/internal/repository/api_key_repo.go`, add to `userEntityToService`:

```go
		IsSales:                    u.IsSales,
		SalesCommissionRate:        u.SalesCommissionRate,
```

- [ ] **Step 6: Generate Ent code**

Run:

```bash
cd backend
go generate ./ent
```

Expected: PASS and generated files under `backend/ent/` include user setters and sales commission entity packages.

- [ ] **Step 7: Run tests to verify green**

Run:

```bash
cd backend
go test -tags=integration ./internal/repository -run 'TestMigrationsRunner_IsIdempotent_AndSchemaIsUpToDate|TestUserRepoSuite/TestUserSalesCommissionFieldsPersist' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add backend/ent backend/migrations/111_sales_commissions.sql backend/internal/service/user.go backend/internal/repository/api_key_repo.go backend/internal/repository/user_repo.go backend/internal/repository/migrations_schema_integration_test.go backend/internal/repository/user_repo_integration_test.go
git commit -m "feat: add sales commission schema"
```

---

### Task 2: Sales Commission Domain and Repository

**Files:**
- Create: `backend/internal/service/sales_commission.go`
- Create: `backend/internal/repository/sales_commission_repo.go`
- Create: `backend/internal/repository/sales_commission_repo_integration_test.go`
- Modify: `backend/internal/repository/wire.go`
- Test: `backend/internal/repository/sales_commission_repo_integration_test.go`

- [ ] **Step 1: Write failing repository integration tests**

Create `backend/internal/repository/sales_commission_repo_integration_test.go`:

```go
//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestSalesCommissionRepository_CreateListsAndSettles(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewSalesCommissionRepository(integrationDB)

	sales := mustCreateUser(t, client, &service.User{Email: "sales-" + uuid.NewString() + "@example.com"})
	referee := mustCreateUser(t, client, &service.User{Email: "buyer-" + uuid.NewString() + "@example.com"})

	referralID := int64(0)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		INSERT INTO referrals (referrer_id, referee_id, referral_code, status, created_at, updated_at)
		VALUES ($1, $2, $3, 'completed', NOW(), NOW())
		RETURNING id
	`, sales.ID, referee.ID, "SC"+uuid.NewString()[:8]).Scan(&referralID))

	orderID := int64(0)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		INSERT INTO payment_orders (user_id, order_type, payment_type, amount, pay_amount, status, recharge_code, out_trade_no, created_at, updated_at, completed_at)
		VALUES ($1, $2, 'alipay', 10, 10, $3, $4, $5, NOW(), NOW(), NOW())
		RETURNING id
	`, referee.ID, payment.OrderTypeBalance, payment.OrderStatusCompleted, "RC"+uuid.NewString()[:10], "OT"+uuid.NewString()[:10]).Scan(&orderID))

	err := repo.CreateForOrder(ctx, &service.SalesCommissionCreate{
		SalesUserID:         sales.ID,
		RefereeUserID:       referee.ID,
		ReferralID:          referralID,
		PaymentOrderID:      orderID,
		OrderPayAmountCNY:   10,
		OrderCreditedAmount: 10,
		CommissionRate:      10,
		CommissionTotalCNY:  1,
		Note:                "test commission",
	})
	require.NoError(t, err)

	summary, err := repo.GetSummaryBySalesUser(ctx, sales.ID)
	require.NoError(t, err)
	require.InDelta(t, 1, summary.TotalCommissionCNY, 0.000001)
	require.InDelta(t, 1, summary.FrozenCNY, 0.000001)

	_, err = integrationDB.ExecContext(ctx, `
		UPDATE sales_commission_records
		SET unlocked_cny = 0.40, credited_used_amount = 4, status = 'partial_unlocked'
		WHERE payment_order_id = $1
	`, orderID)
	require.NoError(t, err)

	settlement, err := repo.CreateSettlement(ctx, &service.SalesCommissionSettlementCreate{
		SalesUserID: sales.ID,
		AmountCNY:   0.25,
		Note:        "manual payout",
		CreatedBy:   &sales.ID,
	})
	require.NoError(t, err)
	require.Equal(t, sales.ID, settlement.SalesUserID)
	require.InDelta(t, 0.25, settlement.AmountCNY, 0.000001)

	records, total, err := repo.ListRecords(ctx, service.SalesCommissionRecordListParams{SalesUserID: sales.ID, Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, records, 1)
	require.InDelta(t, 0.25, records[0].SettledCNY, 0.000001)
	require.InDelta(t, 0.15, records[0].SettleableCNY, 0.000001)

	settlements, total, err := repo.ListSettlements(ctx, service.SalesCommissionSettlementListParams{SalesUserID: sales.ID, Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, settlements, 1)
}

func TestSalesCommissionRepository_CreateSettlementRejectsTooMuch(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewSalesCommissionRepository(integrationDB)

	sales := mustCreateUser(t, client, &service.User{Email: "sales-too-much-" + uuid.NewString() + "@example.com"})
	_, err := repo.CreateSettlement(ctx, &service.SalesCommissionSettlementCreate{
		SalesUserID: sales.ID,
		AmountCNY:   1,
		Note:        "too much",
	})
	require.ErrorIs(t, err, service.ErrSalesCommissionSettleAmountExceeded)
	require.WithinDuration(t, time.Now(), time.Now(), time.Second)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd backend
go test -tags=integration ./internal/repository -run 'TestSalesCommissionRepository' -count=1
```

Expected: FAIL because `SalesCommissionRepository`, `SalesCommissionCreate`, list params, and settlement errors do not exist.

- [ ] **Step 3: Add domain models and repository interface**

Create `backend/internal/service/sales_commission.go`:

```go
package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SalesCommissionStatusFrozen           = "frozen"
	SalesCommissionStatusPartialUnlocked  = "partial_unlocked"
	SalesCommissionStatusUnlocked         = "unlocked"
	SalesCommissionStatusSettled          = "settled"
	SalesCommissionStatusSettlementBlocked = "settlement_blocked"
)

var (
	ErrSalesCommissionSettleAmountExceeded = infraerrors.BadRequest("SALES_COMMISSION_SETTLE_AMOUNT_EXCEEDED", "settlement amount exceeds settleable commission")
	ErrSalesCommissionInvalidAmount        = infraerrors.BadRequest("SALES_COMMISSION_INVALID_AMOUNT", "amount must be greater than 0")
)

type SalesCommissionCreate struct {
	SalesUserID         int64
	RefereeUserID       int64
	ReferralID          int64
	PaymentOrderID      int64
	OrderPayAmountCNY   float64
	OrderCreditedAmount float64
	CommissionRate      float64
	CommissionTotalCNY  float64
	Note                string
}

type SalesCommissionRecord struct {
	ID                  int64     `json:"id"`
	SalesUserID         int64     `json:"sales_user_id"`
	SalesEmail          string    `json:"sales_email"`
	SalesUsername       string    `json:"sales_username"`
	RefereeUserID       int64     `json:"referee_user_id"`
	RefereeEmail        string    `json:"referee_email"`
	RefereeUsername     string    `json:"referee_username"`
	ReferralID          int64     `json:"referral_id"`
	PaymentOrderID      int64     `json:"payment_order_id"`
	PaymentOrderStatus  string    `json:"payment_order_status"`
	OrderPayAmountCNY   float64   `json:"order_pay_amount_cny"`
	OrderCreditedAmount float64   `json:"order_credited_amount"`
	CommissionRate      float64   `json:"commission_rate"`
	CommissionTotalCNY  float64   `json:"commission_total_cny"`
	CreditedUsedAmount  float64   `json:"credited_used_amount"`
	FrozenCNY           float64   `json:"frozen_cny"`
	UnlockedCNY         float64   `json:"unlocked_cny"`
	SettledCNY          float64   `json:"settled_cny"`
	SettleableCNY       float64   `json:"settleable_cny"`
	Status              string    `json:"status"`
	Note                string    `json:"note"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type SalesCommissionSummary struct {
	SalesUserID        int64   `json:"sales_user_id"`
	SalesEmail         string  `json:"sales_email"`
	SalesUsername      string  `json:"sales_username"`
	TotalCommissionCNY float64 `json:"total_commission_cny"`
	FrozenCNY          float64 `json:"frozen_cny"`
	UnlockedCNY        float64 `json:"unlocked_cny"`
	SettleableCNY      float64 `json:"settleable_cny"`
	SettledCNY         float64 `json:"settled_cny"`
	RecordsCount       int     `json:"records_count"`
}

type SalesCommissionSettlement struct {
	ID          int64     `json:"id"`
	SalesUserID int64    `json:"sales_user_id"`
	SalesEmail  string   `json:"sales_email"`
	AmountCNY   float64 `json:"amount_cny"`
	Note        string  `json:"note"`
	CreatedBy   *int64  `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type SalesCommissionSettlementCreate struct {
	SalesUserID int64
	AmountCNY   float64
	Note        string
	CreatedBy   *int64
}

type SalesCommissionSummaryListParams struct {
	Search   string
	Page     int
	PageSize int
}

type SalesCommissionRecordListParams struct {
	SalesUserID    int64
	RefereeUserID  int64
	PaymentOrderID int64
	Status         string
	Page           int
	PageSize       int
}

type SalesCommissionSettlementListParams struct {
	SalesUserID int64
	Page        int
	PageSize    int
}

type SalesCommissionRepository interface {
	CreateForOrder(ctx context.Context, input *SalesCommissionCreate) error
	ListSummaries(ctx context.Context, params SalesCommissionSummaryListParams) ([]SalesCommissionSummary, int, error)
	GetSummaryBySalesUser(ctx context.Context, salesUserID int64) (*SalesCommissionSummary, error)
	ListRecords(ctx context.Context, params SalesCommissionRecordListParams) ([]SalesCommissionRecord, int, error)
	CreateSettlement(ctx context.Context, input *SalesCommissionSettlementCreate) (*SalesCommissionSettlement, error)
	ListSettlements(ctx context.Context, params SalesCommissionSettlementListParams) ([]SalesCommissionSettlement, int, error)
}
```

- [ ] **Step 4: Implement SQL repository**

Create `backend/internal/repository/sales_commission_repo.go`. Use `database/sql` and `shopspring/decimal`. Required functions:

```go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type salesCommissionRepository struct {
	db *sql.DB
}

func NewSalesCommissionRepository(sqlDB *sql.DB) service.SalesCommissionRepository {
	return &salesCommissionRepository{db: sqlDB}
}

func salesPage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
```

Implement `CreateForOrder` with:

```sql
INSERT INTO sales_commission_records
    (sales_user_id, referee_user_id, referral_id, payment_order_id, order_pay_amount_cny, order_credited_amount, commission_rate, commission_total_cny, note, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW(),NOW())
ON CONFLICT (payment_order_id) DO NOTHING
```

Implement `CreateSettlement` in a transaction:

1. Lock settleable rows:

```sql
SELECT scr.id, scr.unlocked_cny, scr.settled_cny
FROM sales_commission_records scr
JOIN payment_orders po ON po.id = scr.payment_order_id
WHERE scr.sales_user_id = $1
  AND po.status = $2
  AND scr.unlocked_cny > scr.settled_cny
ORDER BY scr.id ASC
FOR UPDATE
```

2. Sum `unlocked_cny - settled_cny`.
3. Reject with `service.ErrSalesCommissionSettleAmountExceeded` if requested amount is larger than sum.
4. Insert `sales_commission_settlements`.
5. Insert `sales_commission_settlement_items` and increment each record `settled_cny`.
6. Set status to `settled` when `settled_cny >= commission_total_cny`, otherwise keep `partial_unlocked` or `unlocked`.

Use `decimal.NewFromFloat(...).Round(2).InexactFloat64()` for CNY amounts.

- [ ] **Step 5: Register repository in Wire provider set**

In `backend/internal/repository/wire.go`, add `NewSalesCommissionRepository` next to referral repositories:

```go
	NewSalesCommissionRepository,
```

- [ ] **Step 6: Run tests to verify green**

Run:

```bash
cd backend
go test -tags=integration ./internal/repository -run 'TestSalesCommissionRepository' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/service/sales_commission.go backend/internal/repository/sales_commission_repo.go backend/internal/repository/sales_commission_repo_integration_test.go backend/internal/repository/wire.go
git commit -m "feat: add sales commission repository"
```

---

### Task 3: Create Frozen Commission on Balance Recharge

**Files:**
- Create: `backend/internal/service/sales_commission_service.go`
- Create: `backend/internal/service/sales_commission_service_test.go`
- Modify: `backend/internal/service/payment_service.go`
- Modify: `backend/internal/service/payment_fulfillment.go`
- Modify: `backend/internal/service/wire.go`
- Modify: `backend/cmd/server/wire_gen.go`
- Test: `backend/internal/service/sales_commission_service_test.go`

- [ ] **Step 1: Write failing service tests**

Create `backend/internal/service/sales_commission_service_test.go`:

```go
package service

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

type salesCommissionRepoStub struct {
	SalesCommissionRepository
	created *SalesCommissionCreate
}

func (s *salesCommissionRepoStub) CreateForOrder(ctx context.Context, input *SalesCommissionCreate) error {
	cp := *input
	s.created = &cp
	return nil
}

type salesCommissionReferralRepoStub struct {
	ReferralRepository
	ref *Referral
}

func (s *salesCommissionReferralRepoStub) GetByRefereeID(ctx context.Context, refereeID int64) (*Referral, error) {
	return s.ref, nil
}

type salesCommissionUserRepoStub struct {
	UserRepository
	users map[int64]*User
}

func (s *salesCommissionUserRepoStub) GetByID(ctx context.Context, id int64) (*User, error) {
	return s.users[id], nil
}

func TestSalesCommissionServiceHandleBalanceRechargeCompletedCreatesFrozenCommission(t *testing.T) {
	repo := &salesCommissionRepoStub{}
	svc := NewSalesCommissionService(repo, &salesCommissionReferralRepoStub{
		ref: &Referral{ID: 77, ReferrerID: 10, RefereeID: 20},
	}, &salesCommissionUserRepoStub{users: map[int64]*User{
		10: {ID: 10, Email: "sales@example.com", IsSales: true, SalesCommissionRate: 10},
	}})

	err := svc.HandleBalanceRechargeCompleted(context.Background(), &dbent.PaymentOrder{
		ID:        88,
		UserID:    20,
		OrderType: payment.OrderTypeBalance,
		Status:    payment.OrderStatusCompleted,
		PayAmount: 10,
		Amount:    10,
	})
	require.NoError(t, err)
	require.NotNil(t, repo.created)
	require.Equal(t, int64(10), repo.created.SalesUserID)
	require.Equal(t, int64(20), repo.created.RefereeUserID)
	require.Equal(t, int64(77), repo.created.ReferralID)
	require.Equal(t, int64(88), repo.created.PaymentOrderID)
	require.InDelta(t, 1, repo.created.CommissionTotalCNY, 0.000001)
}

func TestSalesCommissionServiceHandleBalanceRechargeCompletedSkipsNonSalesReferrer(t *testing.T) {
	repo := &salesCommissionRepoStub{}
	svc := NewSalesCommissionService(repo, &salesCommissionReferralRepoStub{
		ref: &Referral{ID: 77, ReferrerID: 10, RefereeID: 20},
	}, &salesCommissionUserRepoStub{users: map[int64]*User{
		10: {ID: 10, Email: "user@example.com", IsSales: false, SalesCommissionRate: 10},
	}})

	err := svc.HandleBalanceRechargeCompleted(context.Background(), &dbent.PaymentOrder{
		ID:        88,
		UserID:    20,
		OrderType: payment.OrderTypeBalance,
		Status:    payment.OrderStatusCompleted,
		PayAmount: 10,
		Amount:    10,
	})
	require.NoError(t, err)
	require.Nil(t, repo.created)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd backend
go test ./internal/service -run TestSalesCommissionService -count=1
```

Expected: FAIL because `NewSalesCommissionService` and `HandleBalanceRechargeCompleted` do not exist.

- [ ] **Step 3: Implement sales commission service**

Create `backend/internal/service/sales_commission_service.go`:

```go
package service

import (
	"context"
	"log/slog"

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
	return &SalesCommissionService{repo: repo, referralRepo: referralRepo, userRepo: userRepo}
}

func (s *SalesCommissionService) HandleBalanceRechargeCompleted(ctx context.Context, order *dbent.PaymentOrder) error {
	if s == nil || s.repo == nil || s.referralRepo == nil || s.userRepo == nil || order == nil {
		return nil
	}
	if order.OrderType != payment.OrderTypeBalance || order.Status != payment.OrderStatusCompleted || order.PayAmount <= 0 || order.Amount <= 0 {
		return nil
	}
	ref, err := s.referralRepo.GetByRefereeID(ctx, order.UserID)
	if err != nil || ref == nil {
		return err
	}
	salesUser, err := s.userRepo.GetByID(ctx, ref.ReferrerID)
	if err != nil || salesUser == nil {
		return err
	}
	if !salesUser.IsSales || salesUser.SalesCommissionRate <= 0 {
		return nil
	}
	total := decimal.NewFromFloat(order.PayAmount).
		Mul(decimal.NewFromFloat(salesUser.SalesCommissionRate)).
		Div(decimal.NewFromInt(100)).
		Round(2).
		InexactFloat64()
	if total <= 0 {
		return nil
	}
	err = s.repo.CreateForOrder(ctx, &SalesCommissionCreate{
		SalesUserID:         salesUser.ID,
		RefereeUserID:       order.UserID,
		ReferralID:          ref.ID,
		PaymentOrderID:      order.ID,
		OrderPayAmountCNY:   order.PayAmount,
		OrderCreditedAmount: order.Amount,
		CommissionRate:      salesUser.SalesCommissionRate,
		CommissionTotalCNY:  total,
		Note:                "balance recharge sales commission",
	})
	if err != nil {
		slog.Error("create sales commission", "orderID", order.ID, "salesUserID", salesUser.ID, "error", err)
	}
	return err
}

func (s *SalesCommissionService) ListSummaries(ctx context.Context, params SalesCommissionSummaryListParams) ([]SalesCommissionSummary, int, error) {
	return s.repo.ListSummaries(ctx, params)
}

func (s *SalesCommissionService) GetSummaryBySalesUser(ctx context.Context, salesUserID int64) (*SalesCommissionSummary, error) {
	return s.repo.GetSummaryBySalesUser(ctx, salesUserID)
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
```

- [ ] **Step 4: Inject service into payment fulfillment**

In `backend/internal/service/payment_service.go`, add field and setter:

```go
	salesCommissionService *SalesCommissionService
```

```go
func (s *PaymentService) SetSalesCommissionService(svc *SalesCommissionService) {
	s.salesCommissionService = svc
}
```

In `backend/internal/service/payment_fulfillment.go`, after existing referral reward calls:

```go
	if s.salesCommissionService != nil {
		if err := s.salesCommissionService.HandleBalanceRechargeCompleted(ctx, o); err != nil {
			slog.Error("handle sales commission on balance recharge", "orderID", o.ID, "error", err)
		}
	}
```

Keep this after `markCompleted` so the order status is `completed`.

- [ ] **Step 5: Register service in provider set and generated wire**

In `backend/internal/service/wire.go`, add:

```go
	NewSalesCommissionService,
```

Run:

```bash
cd backend
go generate ./cmd/server
```

Expected: `backend/cmd/server/wire_gen.go` creates `salesCommissionRepository`, `salesCommissionService`, and calls `paymentService.SetSalesCommissionService(salesCommissionService)`. If Wire does not place the setter automatically, add the setter call manually next to `paymentService.SetReferralService(referralService)` and re-run `go test ./cmd/server`.

- [ ] **Step 6: Run tests to verify green**

Run:

```bash
cd backend
go test ./internal/service -run TestSalesCommissionService -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/service/sales_commission_service.go backend/internal/service/sales_commission_service_test.go backend/internal/service/payment_service.go backend/internal/service/payment_fulfillment.go backend/internal/service/wire.go backend/cmd/server/wire_gen.go
git commit -m "feat: create sales commission on recharge"
```

---

### Task 4: Unlock Commission During Usage Billing

**Files:**
- Modify: `backend/internal/repository/usage_billing_repo.go`
- Modify: `backend/internal/repository/usage_billing_repo_integration_test.go`
- Test: `backend/internal/repository/usage_billing_repo_integration_test.go`

- [ ] **Step 1: Write failing integration tests**

Append tests to `backend/internal/repository/usage_billing_repo_integration_test.go`:

```go
func TestUsageBillingRepositoryApply_UnlocksSalesCommissionFromOrdinaryBalanceOnly(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	sales := mustCreateUser(t, client, &service.User{Email: "sales-unlock-" + uuid.NewString() + "@example.com"})
	referee := mustCreateUser(t, client, &service.User{Email: "buyer-unlock-" + uuid.NewString() + "@example.com", Balance: 10})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: referee.ID, Key: "sk-sales-unlock-" + uuid.NewString(), Name: "billing"})

	var referralID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		INSERT INTO referrals (referrer_id, referee_id, referral_code, status, created_at, updated_at)
		VALUES ($1, $2, $3, 'completed', NOW(), NOW()) RETURNING id
	`, sales.ID, referee.ID, "SU"+uuid.NewString()[:8]).Scan(&referralID))
	var orderID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		INSERT INTO payment_orders (user_id, order_type, payment_type, amount, pay_amount, status, recharge_code, out_trade_no, created_at, updated_at, completed_at)
		VALUES ($1, 'balance', 'alipay', 10, 10, 'completed', $2, $3, NOW(), NOW(), NOW()) RETURNING id
	`, referee.ID, "RC"+uuid.NewString()[:8], "OT"+uuid.NewString()[:8]).Scan(&orderID))
	_, err := integrationDB.ExecContext(ctx, `
		INSERT INTO sales_commission_records
			(sales_user_id, referee_user_id, referral_id, payment_order_id, order_pay_amount_cny, order_credited_amount, commission_rate, commission_total_cny)
		VALUES ($1,$2,$3,$4,10,10,10,1)
	`, sales.ID, referee.ID, referralID, orderID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `
		INSERT INTO gift_balance_records (user_id, amount, remaining, source, note, created_at, updated_at)
		VALUES ($1, 1, 1, 'admin_grant', 'gift first', NOW(), NOW())
	`, referee.ID)
	require.NoError(t, err)

	_, err = repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:   uuid.NewString(),
		APIKeyID:    apiKey.ID,
		UserID:      referee.ID,
		BalanceCost: 3,
	})
	require.NoError(t, err)

	var creditedUsed, unlocked float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT credited_used_amount, unlocked_cny FROM sales_commission_records WHERE payment_order_id = $1
	`, orderID).Scan(&creditedUsed, &unlocked))
	require.InDelta(t, 2, creditedUsed, 0.000001, "1 gift dollar should not unlock commission")
	require.InDelta(t, 0.20, unlocked, 0.000001)
}

func TestUsageBillingRepositoryApply_UnlocksSalesCommissionFIFOAndDeduplicates(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	sales := mustCreateUser(t, client, &service.User{Email: "sales-fifo-" + uuid.NewString() + "@example.com"})
	referee := mustCreateUser(t, client, &service.User{Email: "buyer-fifo-" + uuid.NewString() + "@example.com", Balance: 20})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: referee.ID, Key: "sk-sales-fifo-" + uuid.NewString(), Name: "billing"})
	var referralID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		INSERT INTO referrals (referrer_id, referee_id, referral_code, status, created_at, updated_at)
		VALUES ($1, $2, $3, 'completed', NOW(), NOW()) RETURNING id
	`, sales.ID, referee.ID, "SF"+uuid.NewString()[:8]).Scan(&referralID))

	insertOrderAndCommission := func(amount float64) int64 {
		var orderID int64
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
			INSERT INTO payment_orders (user_id, order_type, payment_type, amount, pay_amount, status, recharge_code, out_trade_no, created_at, updated_at, completed_at)
			VALUES ($1, 'balance', 'alipay', $2, $2, 'completed', $3, $4, NOW(), NOW(), NOW()) RETURNING id
		`, referee.ID, amount, "RC"+uuid.NewString()[:8], "OT"+uuid.NewString()[:8]).Scan(&orderID))
		_, err := integrationDB.ExecContext(ctx, `
			INSERT INTO sales_commission_records
				(sales_user_id, referee_user_id, referral_id, payment_order_id, order_pay_amount_cny, order_credited_amount, commission_rate, commission_total_cny)
			VALUES ($1,$2,$3,$4,$5,$5,10,$6)
		`, sales.ID, referee.ID, referralID, orderID, amount, amount*0.1)
		require.NoError(t, err)
		return orderID
	}
	firstOrderID := insertOrderAndCommission(10)
	secondOrderID := insertOrderAndCommission(10)

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{RequestID: requestID, APIKeyID: apiKey.ID, UserID: referee.ID, BalanceCost: 12}
	result, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result.Applied)
	result, err = repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, result.Applied)

	var firstUsed, firstUnlocked, secondUsed, secondUnlocked float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT credited_used_amount, unlocked_cny FROM sales_commission_records WHERE payment_order_id = $1`, firstOrderID).Scan(&firstUsed, &firstUnlocked))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT credited_used_amount, unlocked_cny FROM sales_commission_records WHERE payment_order_id = $1`, secondOrderID).Scan(&secondUsed, &secondUnlocked))
	require.InDelta(t, 10, firstUsed, 0.000001)
	require.InDelta(t, 1, firstUnlocked, 0.000001)
	require.InDelta(t, 2, secondUsed, 0.000001)
	require.InDelta(t, 0.20, secondUnlocked, 0.000001)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd backend
go test -tags=integration ./internal/repository -run 'TestUsageBillingRepositoryApply_UnlocksSalesCommission' -count=1
```

Expected: FAIL because ordinary balance deductions do not update sales commission records.

- [ ] **Step 3: Implement FIFO unlock inside usage billing transaction**

In `deductUsageBillingBalance`, after the ordinary balance update succeeds and before returning `newBalance`, add:

```go
		if err := unlockSalesCommissionFIFO(ctx, tx, userID, remainingCost); err != nil {
			return 0, err
		}
```

Add helper functions in `backend/internal/repository/usage_billing_repo.go`:

```go
func unlockSalesCommissionFIFO(ctx context.Context, tx *sql.Tx, refereeUserID int64, ordinaryUsageAmount float64) error {
	if ordinaryUsageAmount <= 0.0001 {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT scr.id, scr.order_credited_amount, scr.credited_used_amount, scr.commission_total_cny, scr.unlocked_cny
		FROM sales_commission_records scr
		JOIN payment_orders po ON po.id = scr.payment_order_id
		WHERE scr.referee_user_id = $1
			AND po.status = $2
			AND scr.credited_used_amount < scr.order_credited_amount
		ORDER BY scr.id ASC
		FOR UPDATE
	`, refereeUserID, service.OrderStatusCompleted)
	if err != nil {
		return err
	}
	defer rows.Close()

	type commissionRow struct {
		id                  int64
		orderCreditedAmount float64
		creditedUsedAmount  float64
		commissionTotalCNY  float64
		unlockedCNY         float64
	}
	var records []commissionRow
	for rows.Next() {
		var r commissionRow
		if err := rows.Scan(&r.id, &r.orderCreditedAmount, &r.creditedUsedAmount, &r.commissionTotalCNY, &r.unlockedCNY); err != nil {
			return err
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	remaining := decimal.NewFromFloat(ordinaryUsageAmount)
	for _, rec := range records {
		if remaining.LessThanOrEqual(decimal.Zero) {
			break
		}
		orderCredited := decimal.NewFromFloat(rec.orderCreditedAmount)
		alreadyUsed := decimal.NewFromFloat(rec.creditedUsedAmount)
		available := orderCredited.Sub(alreadyUsed)
		if available.LessThanOrEqual(decimal.Zero) || orderCredited.LessThanOrEqual(decimal.Zero) {
			continue
		}
		allocated := decimal.Min(remaining, available)
		newUsed := alreadyUsed.Add(allocated).Round(8)
		totalCommission := decimal.NewFromFloat(rec.commissionTotalCNY)
		unlockDelta := allocated.Div(orderCredited).Mul(totalCommission).Round(2)
		newUnlocked := decimal.NewFromFloat(rec.unlockedCNY).Add(unlockDelta).Round(2)
		if newUsed.GreaterThanOrEqual(orderCredited) {
			newUsed = orderCredited.Round(8)
			newUnlocked = totalCommission.Round(2)
		}
		status := service.SalesCommissionStatusPartialUnlocked
		if newUnlocked.GreaterThanOrEqual(totalCommission) {
			status = service.SalesCommissionStatusUnlocked
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE sales_commission_records
			SET credited_used_amount = $1, unlocked_cny = $2, status = $3, updated_at = NOW()
			WHERE id = $4
		`, newUsed.InexactFloat64(), newUnlocked.InexactFloat64(), status, rec.id); err != nil {
			return err
		}
		remaining = remaining.Sub(allocated)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify green**

Run:

```bash
cd backend
go test -tags=integration ./internal/repository -run 'TestUsageBillingRepositoryApply_UnlocksSalesCommission' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/repository/usage_billing_repo.go backend/internal/repository/usage_billing_repo_integration_test.go
git commit -m "feat: unlock sales commission from usage"
```

---

### Task 5: Admin and User Sales Commission Handlers and Routes

**Files:**
- Create: `backend/internal/handler/admin/sales_commission_handler.go`
- Create: `backend/internal/handler/sales_commission_handler.go`
- Modify: `backend/internal/handler/handler.go`
- Modify: `backend/internal/handler/wire.go`
- Modify: `backend/internal/server/routes/admin.go`
- Modify: `backend/internal/server/routes/user.go`
- Test: `backend/internal/handler/admin/sales_commission_handler_test.go`
- Test: `backend/internal/handler/sales_commission_handler_test.go`

- [ ] **Step 1: Write failing handler tests**

Create `backend/internal/handler/admin/sales_commission_handler_test.go`:

```go
package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestSalesCommissionHandlerCreateSettlementValidatesAmount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSalesCommissionHandler(&service.SalesCommissionService{})
	r := gin.New()
	r.POST("/admin/sales-commissions/settlements", h.CreateSettlement)

	req := httptest.NewRequest(http.MethodPost, "/admin/sales-commissions/settlements", strings.NewReader(`{"sales_user_id":1,"amount_cny":0}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "amount")
}
```

Create `backend/internal/handler/sales_commission_handler_test.go`:

```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestSalesCommissionHandlerRequiresAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSalesCommissionHandler(&service.SalesCommissionService{})
	r := gin.New()
	r.GET("/sales-commissions/summary", h.GetSummary)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/sales-commissions/summary", nil))

	require.Equal(t, http.StatusUnauthorized, w.Code)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd backend
go test ./internal/handler ./internal/handler/admin -run SalesCommission -count=1
```

Expected: FAIL because handlers do not exist.

- [ ] **Step 3: Implement admin handler**

Create `backend/internal/handler/admin/sales_commission_handler.go` with:

```go
package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type SalesCommissionHandler struct {
	service *service.SalesCommissionService
}

func NewSalesCommissionHandler(svc *service.SalesCommissionService) *SalesCommissionHandler {
	return &SalesCommissionHandler{service: svc}
}

func (h *SalesCommissionHandler) ListSummaries(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.service.ListSummaries(c.Request.Context(), service.SalesCommissionSummaryListParams{
		Search: c.Query("search"), Page: page, PageSize: pageSize,
	})
	if err != nil { response.ErrorFrom(c, err); return }
	response.Paginated(c, items, total, page, pageSize)
}

func (h *SalesCommissionHandler) ListRecords(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.service.ListRecords(c.Request.Context(), service.SalesCommissionRecordListParams{
		SalesUserID: parseInt64Query(c, "sales_user_id"),
		RefereeUserID: parseInt64Query(c, "referee_user_id"),
		PaymentOrderID: parseInt64Query(c, "payment_order_id"),
		Status: c.Query("status"),
		Page: page, PageSize: pageSize,
	})
	if err != nil { response.ErrorFrom(c, err); return }
	response.Paginated(c, items, total, page, pageSize)
}

func (h *SalesCommissionHandler) ListSettlements(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.service.ListSettlements(c.Request.Context(), service.SalesCommissionSettlementListParams{
		SalesUserID: parseInt64Query(c, "sales_user_id"), Page: page, PageSize: pageSize,
	})
	if err != nil { response.ErrorFrom(c, err); return }
	response.Paginated(c, items, total, page, pageSize)
}

type createSalesCommissionSettlementRequest struct {
	SalesUserID int64   `json:"sales_user_id" binding:"required,gt=0"`
	AmountCNY   float64 `json:"amount_cny" binding:"required,gt=0"`
	Note        string  `json:"note"`
}

func (h *SalesCommissionHandler) CreateSettlement(c *gin.Context) {
	var req createSalesCommissionSettlementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	createdBy := parseAdminUserID(c)
	result, err := h.service.CreateSettlement(c.Request.Context(), &service.SalesCommissionSettlementCreate{
		SalesUserID: req.SalesUserID, AmountCNY: req.AmountCNY, Note: req.Note, CreatedBy: createdBy,
	})
	if err != nil { response.ErrorFrom(c, err); return }
	response.Success(c, result)
}

func parseInt64Query(c *gin.Context, key string) int64 {
	v, _ := strconv.ParseInt(c.Query(key), 10, 64)
	return v
}

func parseAdminUserID(c *gin.Context) *int64 {
	if v, ok := c.Get("user_id"); ok {
		switch id := v.(type) {
		case int64:
			return &id
		case int:
			x := int64(id); return &x
		}
	}
	return nil
}
```

- [ ] **Step 4: Implement user handler**

Create `backend/internal/handler/sales_commission_handler.go`:

```go
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type SalesCommissionHandler struct {
	service *service.SalesCommissionService
}

func NewSalesCommissionHandler(svc *service.SalesCommissionService) *SalesCommissionHandler {
	return &SalesCommissionHandler{service: svc}
}

func (h *SalesCommissionHandler) GetSummary(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok { response.Unauthorized(c, "Unauthorized"); return }
	summary, err := h.service.GetSummaryBySalesUser(c.Request.Context(), userID)
	if err != nil { response.ErrorFrom(c, err); return }
	response.Success(c, summary)
}

func (h *SalesCommissionHandler) ListRecords(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok { response.Unauthorized(c, "Unauthorized"); return }
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.service.ListRecords(c.Request.Context(), service.SalesCommissionRecordListParams{
		SalesUserID: userID, Status: c.Query("status"), Page: page, PageSize: pageSize,
	})
	if err != nil { response.ErrorFrom(c, err); return }
	response.Paginated(c, items, total, page, pageSize)
}

func currentUserID(c *gin.Context) (int64, bool) {
	if v, ok := c.Get("user_id"); ok {
		switch id := v.(type) {
		case int64:
			return id, true
		case int:
			return int64(id), true
		}
	}
	return 0, false
}
```

- [ ] **Step 5: Wire handlers and routes**

Modify `backend/internal/handler/handler.go`:

```go
	SalesCommission     *admin.SalesCommissionHandler
```

inside `AdminHandlers`, and:

```go
	SalesCommission *SalesCommissionHandler
```

inside `Handlers`.

Modify `backend/internal/handler/wire.go` provider signatures to accept admin and user sales commission handlers and assign them.

Modify `backend/internal/server/routes/admin.go`:

```go
		registerSalesCommissionRoutes(admin, h)
```

Add:

```go
func registerSalesCommissionRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	if h.Admin.SalesCommission == nil {
		return
	}
	group := admin.Group("/sales-commissions")
	{
		group.GET("/summary", h.Admin.SalesCommission.ListSummaries)
		group.GET("/records", h.Admin.SalesCommission.ListRecords)
		group.GET("/settlements", h.Admin.SalesCommission.ListSettlements)
		group.POST("/settlements", h.Admin.SalesCommission.CreateSettlement)
	}
}
```

Modify `backend/internal/server/routes/user.go` under authenticated routes:

```go
		if h.SalesCommission != nil {
			salesCommissions := authenticated.Group("/sales-commissions")
			{
				salesCommissions.GET("/summary", h.SalesCommission.GetSummary)
				salesCommissions.GET("/records", h.SalesCommission.ListRecords)
			}
		}
```

Run:

```bash
cd backend
go generate ./cmd/server
```

- [ ] **Step 6: Run tests to verify green**

Run:

```bash
cd backend
go test ./internal/handler ./internal/handler/admin ./internal/server/routes -run SalesCommission -count=1
go test ./cmd/server -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/handler/admin/sales_commission_handler.go backend/internal/handler/sales_commission_handler.go backend/internal/handler/admin/sales_commission_handler_test.go backend/internal/handler/sales_commission_handler_test.go backend/internal/handler/handler.go backend/internal/handler/wire.go backend/internal/server/routes/admin.go backend/internal/server/routes/user.go backend/cmd/server/wire_gen.go
git commit -m "feat: expose sales commission APIs"
```

---

### Task 6: Admin User Sales Controls

**Files:**
- Modify: `backend/internal/service/admin_service.go`
- Modify: `backend/internal/handler/admin/user_handler.go`
- Modify: `backend/internal/handler/dto/types.go`
- Modify: `backend/internal/handler/dto/mappers.go`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/api/admin/users.ts`
- Modify: `frontend/src/components/admin/user/UserEditModal.vue`
- Create: `frontend/src/components/admin/user/__tests__/UserEditModalSales.spec.ts`

- [ ] **Step 1: Write failing frontend test**

Create `frontend/src/components/admin/user/__tests__/UserEditModalSales.spec.ts`:

```ts
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import UserEditModal from '../UserEditModal.vue'

const update = vi.fn().mockResolvedValue({})
const showError = vi.fn()
const showSuccess = vi.fn()

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { update },
    userAttributes: { updateUserAttributeValues: vi.fn() }
  }
}))

vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('UserEditModal sales fields', () => {
  it('submits sales flag and commission rate', async () => {
    const wrapper = mount(UserEditModal, {
      props: {
        show: true,
        user: {
          id: 1,
          email: 'sales@example.com',
          username: '',
          role: 'user',
          balance: 0,
          concurrency: 5,
          status: 'active',
          allowed_groups: [],
          balance_notify_enabled: true,
          balance_notify_threshold: null,
          balance_notify_extra_emails: [],
          created_at: '2026-05-05T00:00:00Z',
          updated_at: '2026-05-05T00:00:00Z',
          notes: '',
          is_sales: true,
          sales_commission_rate: 10
        }
      },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          UserAttributeForm: true,
          Icon: true
        }
      }
    })

    await wrapper.find('form').trigger('submit.prevent')

    expect(update).toHaveBeenCalledWith(1, expect.objectContaining({
      is_sales: true,
      sales_commission_rate: 10
    }))
  })
})
```

- [ ] **Step 2: Run frontend test to verify it fails**

Run:

```bash
cd frontend
pnpm test -- --run src/components/admin/user/__tests__/UserEditModalSales.spec.ts
```

Expected: FAIL because the modal does not render or submit sales fields.

- [ ] **Step 3: Extend backend DTO and admin update path**

In `backend/internal/handler/dto/types.go`, add to `User`:

```go
	IsSales             bool    `json:"is_sales"`
	SalesCommissionRate float64 `json:"sales_commission_rate"`
```

In `backend/internal/handler/dto/mappers.go`, add to `UserFromServiceShallow`:

```go
		IsSales:                    u.IsSales,
		SalesCommissionRate:        u.SalesCommissionRate,
```

In `backend/internal/service/admin_service.go`, add to `UpdateUserInput`:

```go
	IsSales             *bool
	SalesCommissionRate *float64
```

In `UpdateUser`, add validation and assignment:

```go
	if input.SalesCommissionRate != nil && (*input.SalesCommissionRate < 0 || *input.SalesCommissionRate > 100) {
		return nil, infraerrors.BadRequest("INVALID_SALES_COMMISSION_RATE", "sales commission rate must be between 0 and 100")
	}
	if input.IsSales != nil {
		user.IsSales = *input.IsSales
	}
	if input.SalesCommissionRate != nil {
		user.SalesCommissionRate = *input.SalesCommissionRate
	}
```

Import `infraerrors` if it is not already imported in this file.

In `backend/internal/handler/admin/user_handler.go`, add request fields:

```go
	IsSales             *bool    `json:"is_sales"`
	SalesCommissionRate *float64 `json:"sales_commission_rate"`
```

Pass them into `service.UpdateUserInput`.

- [ ] **Step 4: Extend frontend types and modal**

In `frontend/src/types/index.ts`, add to `User`:

```ts
  is_sales: boolean
  sales_commission_rate: number
```

Ensure `UpdateUserRequest` includes:

```ts
  is_sales?: boolean
  sales_commission_rate?: number
```

In `frontend/src/components/admin/user/UserEditModal.vue`, extend form state:

```ts
const form = reactive({
  email: '',
  password: '',
  username: '',
  notes: '',
  concurrency: 1,
  isSales: false,
  salesCommissionRate: 0,
  customAttributes: {} as UserAttributeValuesMap
})
```

Set watch values:

```ts
isSales: !!u.is_sales,
salesCommissionRate: u.sales_commission_rate || 0,
```

Add controls after concurrency:

```vue
      <div class="rounded-md border border-gray-200 p-3 dark:border-dark-700">
        <label class="flex items-center justify-between gap-3 text-sm font-medium text-gray-700 dark:text-gray-200">
          <span>{{ t('admin.users.sales.isSales') }}</span>
          <input v-model="form.isSales" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
        </label>
        <div v-if="form.isSales" class="mt-3">
          <label class="input-label">{{ t('admin.users.sales.commissionRate') }}</label>
          <input v-model.number="form.salesCommissionRate" type="number" min="0.01" max="100" step="0.01" class="input" />
        </div>
      </div>
```

Add submit validation:

```ts
  if (form.isSales && (form.salesCommissionRate <= 0 || form.salesCommissionRate > 100)) {
    appStore.showError(t('admin.users.sales.invalidRate'))
    return
  }
```

Add payload fields:

```ts
is_sales: form.isSales,
sales_commission_rate: form.isSales ? form.salesCommissionRate : 0
```

- [ ] **Step 5: Add i18n keys**

In `frontend/src/i18n/locales/zh.ts` under `admin.users`, add:

```ts
      sales: {
        isSales: '销售人员',
        commissionRate: '提成比例（%）',
        invalidRate: '销售提成比例必须大于 0 且不超过 100'
      },
```

In `frontend/src/i18n/locales/en.ts` under `admin.users`, add:

```ts
      sales: {
        isSales: 'Sales user',
        commissionRate: 'Commission rate (%)',
        invalidRate: 'Sales commission rate must be greater than 0 and at most 100'
      },
```

- [ ] **Step 6: Run tests to verify green**

Run:

```bash
cd frontend
pnpm test -- --run src/components/admin/user/__tests__/UserEditModalSales.spec.ts
cd ../backend
go test ./internal/service ./internal/handler/admin -run User -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/service/admin_service.go backend/internal/handler/admin/user_handler.go backend/internal/handler/dto/types.go backend/internal/handler/dto/mappers.go frontend/src/types/index.ts frontend/src/api/admin/users.ts frontend/src/components/admin/user/UserEditModal.vue frontend/src/components/admin/user/__tests__/UserEditModalSales.spec.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat: configure sales users"
```

---

### Task 7: Frontend Sales Commission APIs and Views

**Files:**
- Create: `frontend/src/api/admin/salesCommissions.ts`
- Create: `frontend/src/api/salesCommissions.ts`
- Modify: `frontend/src/api/admin/index.ts`
- Create: `frontend/src/views/admin/SalesCommissionsView.vue`
- Create: `frontend/src/views/user/SalesCommissionsView.vue`
- Modify: `frontend/src/router/index.ts`
- Modify: `frontend/src/components/layout/AppSidebar.vue`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Create: `frontend/src/views/admin/__tests__/SalesCommissionsView.spec.ts`
- Create: `frontend/src/views/user/__tests__/SalesCommissionsView.spec.ts`

- [ ] **Step 1: Write failing view tests**

Create `frontend/src/views/admin/__tests__/SalesCommissionsView.spec.ts`:

```ts
import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import SalesCommissionsView from '../SalesCommissionsView.vue'

const listSummaries = vi.fn().mockResolvedValue({ items: [{ sales_user_id: 1, sales_email: 'sales@example.com', total_commission_cny: 1, frozen_cny: 0.8, unlocked_cny: 0.2, settleable_cny: 0.2, settled_cny: 0, records_count: 1 }], total: 1, page: 1, page_size: 20 })
const listRecords = vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
const listSettlements = vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
const createSettlement = vi.fn().mockResolvedValue({})

vi.mock('@/api/admin', () => ({
  adminAPI: {
    salesCommissions: { listSummaries, listRecords, listSettlements, createSettlement }
  }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('Admin SalesCommissionsView', () => {
  it('loads and renders sales commission summary', async () => {
    const wrapper = mount(SalesCommissionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          DataTable: { props: ['data'], template: '<div><span v-for="row in data" :key="row.sales_user_id">{{ row.sales_email }}</span></div>' },
          Pagination: true,
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true
        }
      }
    })
    await flushPromises()
    expect(listSummaries).toHaveBeenCalled()
    expect(wrapper.text()).toContain('sales@example.com')
  })
})
```

Create `frontend/src/views/user/__tests__/SalesCommissionsView.spec.ts`:

```ts
import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import SalesCommissionsView from '../SalesCommissionsView.vue'

const getSummary = vi.fn().mockResolvedValue({ total_commission_cny: 1, frozen_cny: 0.8, unlocked_cny: 0.2, settleable_cny: 0.2, settled_cny: 0 })
const listRecords = vi.fn().mockResolvedValue({ items: [{ id: 1, referee_email: 'buyer@example.com', commission_total_cny: 1, frozen_cny: 0.8, unlocked_cny: 0.2, settled_cny: 0, settleable_cny: 0.2, status: 'partial_unlocked', created_at: '2026-05-05T00:00:00Z' }], total: 1, page: 1, page_size: 20 })

vi.mock('@/api/salesCommissions', () => ({ salesCommissionsAPI: { getSummary, listRecords }, default: { getSummary, listRecords } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('User SalesCommissionsView', () => {
  it('loads current sales commission data', async () => {
    const wrapper = mount(SalesCommissionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          DataTable: { props: ['data'], template: '<div><span v-for="row in data" :key="row.id">{{ row.referee_email }}</span></div>' },
          Pagination: true
        }
      }
    })
    await flushPromises()
    expect(getSummary).toHaveBeenCalled()
    expect(wrapper.text()).toContain('buyer@example.com')
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd frontend
pnpm test -- --run src/views/admin/__tests__/SalesCommissionsView.spec.ts src/views/user/__tests__/SalesCommissionsView.spec.ts
```

Expected: FAIL because views and API modules do not exist.

- [ ] **Step 3: Add frontend API modules**

Create `frontend/src/api/admin/salesCommissions.ts`:

```ts
import { apiClient } from '../client'
import type { BasePaginationResponse, SalesCommissionRecord, SalesCommissionSettlement, SalesCommissionSummary } from '@/types'

export interface SalesCommissionRecordParams {
  page?: number
  page_size?: number
  sales_user_id?: number
  referee_user_id?: number
  payment_order_id?: number
  status?: string
}

export async function listSummaries(params?: { page?: number; page_size?: number; search?: string }): Promise<BasePaginationResponse<SalesCommissionSummary>> {
  const { data } = await apiClient.get<BasePaginationResponse<SalesCommissionSummary>>('/admin/sales-commissions/summary', { params })
  return data
}

export async function listRecords(params?: SalesCommissionRecordParams): Promise<BasePaginationResponse<SalesCommissionRecord>> {
  const { data } = await apiClient.get<BasePaginationResponse<SalesCommissionRecord>>('/admin/sales-commissions/records', { params })
  return data
}

export async function listSettlements(params?: { page?: number; page_size?: number; sales_user_id?: number }): Promise<BasePaginationResponse<SalesCommissionSettlement>> {
  const { data } = await apiClient.get<BasePaginationResponse<SalesCommissionSettlement>>('/admin/sales-commissions/settlements', { params })
  return data
}

export async function createSettlement(payload: { sales_user_id: number; amount_cny: number; note?: string }): Promise<SalesCommissionSettlement> {
  const { data } = await apiClient.post<SalesCommissionSettlement>('/admin/sales-commissions/settlements', payload)
  return data
}

export default { listSummaries, listRecords, listSettlements, createSettlement }
```

Create `frontend/src/api/salesCommissions.ts`:

```ts
import { apiClient } from './client'
import type { BasePaginationResponse, SalesCommissionRecord, SalesCommissionSummary } from '@/types'

export async function getSummary(): Promise<SalesCommissionSummary> {
  const { data } = await apiClient.get<SalesCommissionSummary>('/sales-commissions/summary')
  return data
}

export async function listRecords(params?: { page?: number; page_size?: number; status?: string }): Promise<BasePaginationResponse<SalesCommissionRecord>> {
  const { data } = await apiClient.get<BasePaginationResponse<SalesCommissionRecord>>('/sales-commissions/records', { params })
  return data
}

export const salesCommissionsAPI = { getSummary, listRecords }
export default salesCommissionsAPI
```

In `frontend/src/types/index.ts`, add interfaces matching the service JSON fields.

In `frontend/src/api/admin/index.ts`, import and export `salesCommissionsAPI` as `salesCommissions`.

- [ ] **Step 4: Implement admin and user views**

Create `frontend/src/views/admin/SalesCommissionsView.vue` with:

- `AppLayout`
- four compact summary cards: frozen, unlocked, settleable, settled
- `DataTable` for summary rows
- `DataTable` for records
- settlement dialog with sales user ID, amount, note
- uses `adminAPI.salesCommissions`

Create `frontend/src/views/user/SalesCommissionsView.vue` with:

- `AppLayout`
- four compact summary cards
- read-only `DataTable` for current user's records
- uses `salesCommissionsAPI`

Keep UI compact and operational; do not add marketing copy.

- [ ] **Step 5: Add routes and sidebar entries**

In `frontend/src/router/index.ts`, add user route:

```ts
  {
    path: '/sales-commissions',
    name: 'SalesCommissions',
    component: () => import('@/views/user/SalesCommissionsView.vue'),
    meta: { requiresAuth: true, requiresAdmin: false, titleKey: 'nav.salesCommissions' }
  },
```

Add admin route near referral:

```ts
  {
    path: '/admin/sales-commissions',
    name: 'AdminSalesCommissions',
    component: () => import('@/views/admin/SalesCommissionsView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, titleKey: 'nav.salesCommissions' }
  },
```

In `frontend/src/components/layout/AppSidebar.vue`, add admin item after referral:

```ts
{ path: '/admin/sales-commissions', label: t('nav.salesCommissions'), icon: CurrencyYenIcon, hideInSimpleMode: true },
```

Add user item only if `authStore.user?.is_sales`:

```ts
...(authStore.user?.is_sales ? [{ path: '/sales-commissions', label: t('nav.salesCommissions'), icon: CurrencyYenIcon }] : []),
```

- [ ] **Step 6: Add i18n keys**

In `frontend/src/i18n/locales/zh.ts`, add:

```ts
    salesCommissions: '销售佣金',
```

under `nav`, and a top-level `salesCommissions` object:

```ts
  salesCommissions: {
    title: '销售佣金',
    frozen: '冻结金额',
    unlocked: '已解冻',
    settleable: '可结算',
    settled: '已结算',
    totalCommission: '总佣金',
    records: '佣金明细',
    summaries: '销售汇总',
    settlements: '结算记录',
    createSettlement: '手动结算',
    amount: '金额',
    note: '备注',
    status: '状态',
    salesUser: '销售用户',
    refereeUser: '被邀请用户',
    paymentOrder: '订单',
    createdAt: '创建时间'
  },
```

Mirror keys in `frontend/src/i18n/locales/en.ts`.

- [ ] **Step 7: Run tests to verify green**

Run:

```bash
cd frontend
pnpm test -- --run src/views/admin/__tests__/SalesCommissionsView.spec.ts src/views/user/__tests__/SalesCommissionsView.spec.ts src/components/admin/user/__tests__/UserEditModalSales.spec.ts
pnpm type-check
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/api/admin/salesCommissions.ts frontend/src/api/salesCommissions.ts frontend/src/api/admin/index.ts frontend/src/types/index.ts frontend/src/views/admin/SalesCommissionsView.vue frontend/src/views/user/SalesCommissionsView.vue frontend/src/views/admin/__tests__/SalesCommissionsView.spec.ts frontend/src/views/user/__tests__/SalesCommissionsView.spec.ts frontend/src/router/index.ts frontend/src/components/layout/AppSidebar.vue frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat: add sales commission frontend"
```

---

### Task 8: End-to-End Verification and Cleanup

**Files:**
- Modify only files needed to fix failures found by verification.

- [ ] **Step 1: Run focused backend tests**

Run:

```bash
cd backend
go test ./internal/service ./internal/handler ./internal/handler/admin ./internal/server/routes -count=1
go test -tags=integration ./internal/repository -run 'SalesCommission|UsageBillingRepositoryApply_UnlocksSalesCommission|MigrationsRunner|UserRepoSuite/TestUserSalesCommissionFieldsPersist' -count=1
```

Expected: PASS.

- [ ] **Step 2: Run backend generation and compile checks**

Run:

```bash
cd backend
go generate ./ent
go generate ./cmd/server
go test ./cmd/server ./internal/service ./internal/repository ./internal/handler ./internal/server/routes -count=1
```

Expected: PASS and `git diff` only contains intentional generated changes.

- [ ] **Step 3: Run frontend tests and type checking**

Run:

```bash
cd frontend
pnpm test -- --run
pnpm type-check
```

Expected: PASS.

- [ ] **Step 4: Manual smoke path**

Use local dev environment or API tests to verify:

1. Admin updates a user with `is_sales=true` and `sales_commission_rate=10`.
2. That user has or generates a referral code.
3. Invited user recharges balance.
4. `GET /api/v1/admin/sales-commissions/records` shows frozen commission.
5. Invited user spends ordinary balance through a usage billing path.
6. Admin records show `unlocked_cny` increased.
7. Admin creates settlement for available amount.
8. User endpoint `GET /api/v1/sales-commissions/summary` returns the same totals for the sales user.

- [ ] **Step 5: Commit final verification fixes**

If verification required code changes:

```bash
git add <changed-files>
git commit -m "fix: stabilize sales commission flow"
```

If no code changes were required, do not create an empty commit.

---

## Self-Review Notes

- Spec coverage: sales flags, frozen commission creation, ordinary-balance-only unlock, FIFO attribution, admin settlement, admin/user APIs, frontend visibility, refund blocking, and tests are all covered.
- Type consistency: plan uses `is_sales`, `sales_commission_rate`, `SalesCommissionRecord`, `SalesCommissionSummary`, `SalesCommissionSettlement`, and `SalesCommissionService` consistently across backend and frontend.
- Risk: `go generate ./cmd/server` may not wire setter calls for cyclic-style post-construction injections. The plan explicitly checks generated output and requires the setter next to existing referral setter injection.
