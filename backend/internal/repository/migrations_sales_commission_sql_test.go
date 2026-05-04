package repository

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestSalesCommissionMigrationSQLDefinesLedgerConstraints(t *testing.T) {
	content, err := migrations.FS.ReadFile("111_sales_commissions.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))

	expectedFragments := []string{
		"constraint chk_users_sales_commission_rate",
		"constraint fk_sales_commission_records_sales_user",
		"references users(id)",
		"constraint fk_sales_commission_records_referee_user",
		"constraint fk_sales_commission_records_referral",
		"references referrals(id)",
		"constraint fk_sales_commission_records_payment_order",
		"references payment_orders(id)",
		"constraint chk_sales_commission_records_amounts",
		"constraint chk_sales_commission_records_status",
		"constraint fk_sales_commission_settlements_sales_user",
		"constraint fk_sales_commission_settlements_created_by",
		"constraint chk_sales_commission_settlements_amount",
		"idx_sales_commission_settlement_items_settlement",
		"constraint fk_sales_commission_settlement_items_settlement",
		"references sales_commission_settlements(id)",
		"constraint fk_sales_commission_settlement_items_record",
		"references sales_commission_records(id)",
		"constraint chk_sales_commission_settlement_items_amount",
	}
	for _, fragment := range expectedFragments {
		require.Contains(t, sql, fragment)
	}
}
