package repository

import (
	"strings"
	"testing"

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
