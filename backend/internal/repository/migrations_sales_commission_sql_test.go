package repository

import (
	"strings"
	"testing"

	"entgo.io/ent/dialect/sql/schema"
	entmigrate "github.com/Wei-Shaw/sub2api/ent/migrate"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestSalesCommissionMigrationSQLDefinesLedgerConstraints(t *testing.T) {
	content, err := migrations.FS.ReadFile("112_sales_commission_constraints.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))

	expectedFragments := []string{
		"add constraint chk_users_sales_commission_rate",
		"add constraint fk_sales_commission_records_sales_user",
		"references users(id)",
		"add constraint fk_sales_commission_records_referee_user",
		"add constraint fk_sales_commission_records_referral",
		"references referrals(id)",
		"add constraint fk_sales_commission_records_payment_order",
		"references payment_orders(id)",
		"add constraint chk_sales_commission_records_amounts",
		"add constraint chk_sales_commission_records_status",
		"add constraint fk_sales_commission_settlements_sales_user",
		"add constraint fk_sales_commission_settlements_created_by",
		"add constraint chk_sales_commission_settlements_amount",
		"idx_sales_commission_settlement_items_settlement",
		"add constraint fk_sales_commission_settlement_items_settlement",
		"references sales_commission_settlements(id)",
		"add constraint fk_sales_commission_settlement_items_record",
		"references sales_commission_records(id)",
		"add constraint chk_sales_commission_settlement_items_amount",
	}
	for _, fragment := range expectedFragments {
		require.Contains(t, sql, fragment)
	}
}

func TestMigrationsSalesCommissionSQL(t *testing.T) {
	content, err := migrations.FS.ReadFile("115_sales_commission_tiered_rules.sql")
	require.NoError(t, err)

	sql := strings.ToLower(string(content))

	expectedFragments := []string{
		"sales_commission_mode",
		"sales_commission_min_monthly_sales",
		"sales_commission_tiers",
		"sales_commission_monthly_snapshots",
		"uq_sales_commission_monthly_snapshots",
		"commission_event_at",
		"commission_month",
		"idx_sales_commission_tiers_sales_user",
		"idx_sales_commission_records_month",
		"alter column commission_month set not null",
		"timezone('asia/shanghai'",
	}
	for _, fragment := range expectedFragments {
		require.Contains(t, sql, fragment)
	}
}

func TestSalesCommissionEntSchemaDefinesTieredIndexes(t *testing.T) {
	requireSchemaIndex(t, entmigrate.SalesCommissionTiersTable.Indexes, "idx_sales_commission_tiers_sales_user", false, []string{"sales_user_id", "sort_order", "id"})
	requireSchemaIndex(t, entmigrate.SalesCommissionMonthlySnapshotsTable.Indexes, "uq_sales_commission_monthly_snapshots", true, []string{"sales_user_id", "commission_month"})
	requireSchemaIndex(t, entmigrate.SalesCommissionRecordsTable.Indexes, "idx_sales_commission_records_month", false, []string{"sales_user_id", "commission_month", "commission_event_at", "id"})
}

func requireSchemaIndex(t *testing.T, indexes []*schema.Index, name string, unique bool, columns []string) {
	t.Helper()

	for _, idx := range indexes {
		if idx.Name != name {
			continue
		}
		require.Equal(t, unique, idx.Unique)
		require.Len(t, idx.Columns, len(columns))
		for i, column := range columns {
			require.Equal(t, column, idx.Columns[i].Name)
		}
		return
	}

	t.Fatalf("expected schema index %s", name)
}
