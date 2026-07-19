//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMigrationsRunner_IsIdempotent_AndSchemaIsUpToDate(t *testing.T) {
	tx := testTx(t)

	// Re-apply migrations to verify idempotency (no errors, no duplicate rows).
	require.NoError(t, ApplyMigrations(context.Background(), integrationDB))

	// schema_migrations should have at least the current migration set.
	var applied int
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM schema_migrations").Scan(&applied))
	require.GreaterOrEqual(t, applied, 7, "expected schema_migrations to contain applied migrations")

	// users: columns required by repository queries
	requireColumn(t, tx, "users", "username", "character varying", 100, false)
	requireColumn(t, tx, "users", "notes", "text", 0, false)
	requireColumn(t, tx, "users", "is_sales", "boolean", 0, false)
	requireColumn(t, tx, "users", "sales_commission_rate", "numeric", 0, false)
	requireColumn(t, tx, "users", "sales_commission_mode", "character varying", 16, false)
	requireColumn(t, tx, "users", "sales_commission_min_monthly_sales", "numeric", 0, false)
	requireCheckConstraint(t, tx, "users", "chk_users_sales_commission_rate")

	// accounts: schedulable and rate-limit fields
	requireColumn(t, tx, "accounts", "notes", "text", 0, true)
	requireColumn(t, tx, "accounts", "schedulable", "boolean", 0, false)
	requireColumn(t, tx, "accounts", "rate_limited_at", "timestamp with time zone", 0, true)
	requireColumn(t, tx, "accounts", "rate_limit_reset_at", "timestamp with time zone", 0, true)
	requireColumn(t, tx, "accounts", "overload_until", "timestamp with time zone", 0, true)
	requireColumn(t, tx, "accounts", "session_window_status", "character varying", 20, true)

	// api_keys: key length should be 128
	requireColumn(t, tx, "api_keys", "key", "character varying", 128, false)
	requireColumn(t, tx, "api_keys", "system_prompt", "text", 0, false)
	requireColumn(t, tx, "api_keys", "system_prompt_mode", "character varying", 20, false)
	requireColumn(t, tx, "groups", "system_prompt", "text", 0, false)
	requireColumn(t, tx, "groups", "system_prompt_mode", "character varying", 20, false)

	// redeem_codes: subscription fields
	requireColumn(t, tx, "redeem_codes", "group_id", "bigint", 0, true)
	requireColumn(t, tx, "redeem_codes", "validity_days", "integer", 0, false)
	requireColumn(t, tx, "redeem_codes", "subscription_mode", "character varying", 16, true)

	// usage_logs: billing_type used by filters/stats
	requireColumn(t, tx, "usage_logs", "billing_type", "smallint", 0, false)
	requireColumn(t, tx, "usage_logs", "request_type", "smallint", 0, false)
	requireColumn(t, tx, "usage_logs", "openai_ws_mode", "boolean", 0, false)

	// usage_billing_dedup: billing idempotency narrow table
	var usageBillingDedupRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.usage_billing_dedup')").Scan(&usageBillingDedupRegclass))
	require.True(t, usageBillingDedupRegclass.Valid, "expected usage_billing_dedup table to exist")
	requireColumn(t, tx, "usage_billing_dedup", "request_fingerprint", "character varying", 64, false)
	requireIndex(t, tx, "usage_billing_dedup", "idx_usage_billing_dedup_request_api_key")
	requireIndex(t, tx, "usage_billing_dedup", "idx_usage_billing_dedup_created_at_brin")

	var usageBillingDedupArchiveRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.usage_billing_dedup_archive')").Scan(&usageBillingDedupArchiveRegclass))
	require.True(t, usageBillingDedupArchiveRegclass.Valid, "expected usage_billing_dedup_archive table to exist")
	requireColumn(t, tx, "usage_billing_dedup_archive", "request_fingerprint", "character varying", 64, false)
	requireIndex(t, tx, "usage_billing_dedup_archive", "usage_billing_dedup_archive_pkey")

	// settings table should exist
	var settingsRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.settings')").Scan(&settingsRegclass))
	require.True(t, settingsRegclass.Valid, "expected settings table to exist")

	// security_secrets table should exist
	var securitySecretsRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.security_secrets')").Scan(&securitySecretsRegclass))
	require.True(t, securitySecretsRegclass.Valid, "expected security_secrets table to exist")

	// user_allowed_groups table should exist
	var uagRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.user_allowed_groups')").Scan(&uagRegclass))
	require.True(t, uagRegclass.Valid, "expected user_allowed_groups table to exist")

	// user_subscriptions: deleted_at for soft delete support (migration 012)
	requireColumn(t, tx, "user_subscriptions", "deleted_at", "timestamp with time zone", 0, true)

	// orphan_allowed_groups_audit table should exist (migration 013)
	var orphanAuditRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.orphan_allowed_groups_audit')").Scan(&orphanAuditRegclass))
	require.True(t, orphanAuditRegclass.Valid, "expected orphan_allowed_groups_audit table to exist")

	// account_groups: created_at should be timestamptz
	requireColumn(t, tx, "account_groups", "created_at", "timestamp with time zone", 0, false)

	// user_allowed_groups: created_at should be timestamptz
	requireColumn(t, tx, "user_allowed_groups", "created_at", "timestamp with time zone", 0, false)

	// pricing_plan_groups / pricing_plans: consolidated pricing migration (079)
	var pricingPlanGroupsRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.pricing_plan_groups')").Scan(&pricingPlanGroupsRegclass))
	require.True(t, pricingPlanGroupsRegclass.Valid, "expected pricing_plan_groups table to exist")
	requireColumn(t, tx, "pricing_plan_groups", "name", "character varying", 100, false)
	requireColumn(t, tx, "pricing_plan_groups", "status", "character varying", 20, false)
	requireIndex(t, tx, "pricing_plan_groups", "idx_pricing_plan_groups_status")
	requireIndex(t, tx, "pricing_plan_groups", "idx_pricing_plan_groups_sort_order")

	var pricingPlansRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.pricing_plans')").Scan(&pricingPlansRegclass))
	require.True(t, pricingPlansRegclass.Valid, "expected pricing_plans table to exist")
	requireColumn(t, tx, "pricing_plans", "price_currency", "character varying", 10, false)
	requireColumn(t, tx, "pricing_plans", "contact_methods", "jsonb", 0, false)
	requireColumn(t, tx, "pricing_plans", "icon_url", "text", 0, true)
	requireColumn(t, tx, "pricing_plans", "badge_text", "text", 0, true)
	requireColumn(t, tx, "pricing_plans", "tagline", "text", 0, true)
	requireColumnDefaultContains(t, tx, "pricing_plans", "price_currency", "CNY")
	requireIndex(t, tx, "pricing_plans", "idx_pricing_plans_group_id")
	requireIndex(t, tx, "pricing_plans", "idx_pricing_plans_status")
	requireIndex(t, tx, "pricing_plans", "idx_pricing_plans_sort_order")
	requireIndex(t, tx, "pricing_plans", "idx_pricing_plans_is_featured")

	// account_pool_alert_configs / proxy_usage_metrics_hourly: consolidated pool monitor migration (080)
	var poolAlertConfigRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.account_pool_alert_configs')").Scan(&poolAlertConfigRegclass))
	require.True(t, poolAlertConfigRegclass.Valid, "expected account_pool_alert_configs table to exist")
	requireColumn(t, tx, "account_pool_alert_configs", "proxy_active_probe_enabled", "boolean", 0, false)
	requireColumn(t, tx, "account_pool_alert_configs", "proxy_probe_interval_minutes", "integer", 0, false)
	requireColumn(t, tx, "account_pool_alert_configs", "disabled_proxy_schedule_mode", "character varying", 32, false)
	requireColumn(t, tx, "account_pool_alert_configs", "alert_emails", "jsonb", 0, false)
	requireColumnDefaultContains(t, tx, "account_pool_alert_configs", "disabled_proxy_schedule_mode", "direct_without_proxy")

	var proxyUsageMetricsRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.proxy_usage_metrics_hourly')").Scan(&proxyUsageMetricsRegclass))
	require.True(t, proxyUsageMetricsRegclass.Valid, "expected proxy_usage_metrics_hourly table to exist")
	requireColumn(t, tx, "proxy_usage_metrics_hourly", "bucket_start", "timestamp with time zone", 0, false)
	requireColumn(t, tx, "proxy_usage_metrics_hourly", "platform", "character varying", 20, false)
	requireColumn(t, tx, "proxy_usage_metrics_hourly", "proxy_id", "bigint", 0, false)
	requireIndex(t, tx, "proxy_usage_metrics_hourly", "idx_proxy_usage_metrics_hourly_platform_bucket")
	requireIndex(t, tx, "proxy_usage_metrics_hourly", "idx_proxy_usage_metrics_hourly_platform_proxy_bucket")

	// model_performance_metrics_hourly: public model performance rollups and independent progress.
	var modelPerformanceRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.model_performance_metrics_hourly')").Scan(&modelPerformanceRegclass))
	require.True(t, modelPerformanceRegclass.Valid, "expected model_performance_metrics_hourly table to exist")
	requireColumn(t, tx, "model_performance_metrics_hourly", "bucket_start", "timestamp with time zone", 0, false)
	requireColumn(t, tx, "model_performance_metrics_hourly", "model", "character varying", 100, false)
	requireColumn(t, tx, "model_performance_metrics_hourly", "group_id", "bigint", 0, true)
	requireColumn(t, tx, "model_performance_metrics_hourly", "success_count", "bigint", 0, false)
	requireColumn(t, tx, "model_performance_metrics_hourly", "valid_failure_count", "bigint", 0, false)
	requireColumn(t, tx, "model_performance_metrics_hourly", "output_tokens", "bigint", 0, false)
	requireColumn(t, tx, "model_performance_metrics_hourly", "total_duration_ms", "bigint", 0, false)
	requireColumn(t, tx, "model_performance_metrics_hourly", "total_first_token_ms", "bigint", 0, false)
	requireColumn(t, tx, "model_performance_metrics_hourly", "first_token_count", "bigint", 0, false)
	requireIndex(t, tx, "model_performance_metrics_hourly", "idx_model_performance_metrics_hourly_unique_dim")
	requireIndex(t, tx, "model_performance_metrics_hourly", "idx_model_performance_metrics_hourly_model_bucket")
	requireIndex(t, tx, "model_performance_metrics_hourly", "idx_model_performance_metrics_hourly_model_group_bucket")
	requireIndex(t, tx, "model_performance_metrics_hourly", "idx_model_performance_metrics_hourly_bucket")

	bucketStart := time.Date(2026, time.July, 19, 12, 0, 0, 0, time.UTC)
	_, err := tx.ExecContext(context.Background(), `
INSERT INTO model_performance_metrics_hourly (bucket_start, model, group_id)
VALUES ($1, 'model-performance-schema-test', NULL), ($1, 'model-performance-schema-test', 101), ($1, 'model-performance-schema-test', 102)
	`, bucketStart)
	require.NoError(t, err, "overall and per-group rows for one model-hour should coexist")
	_, err = tx.ExecContext(context.Background(), "SAVEPOINT model_performance_uniqueness")
	require.NoError(t, err)
	_, err = tx.ExecContext(context.Background(), `
INSERT INTO model_performance_metrics_hourly (bucket_start, model, group_id)
VALUES ($1, 'model-performance-schema-test', NULL)
`, bucketStart)
	require.Error(t, err, "a duplicate all-groups row must be rejected")
	_, err = tx.ExecContext(context.Background(), "ROLLBACK TO SAVEPOINT model_performance_uniqueness")
	require.NoError(t, err)

	var modelPerformanceWatermarkRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.model_performance_metrics_aggregation_watermark')").Scan(&modelPerformanceWatermarkRegclass))
	require.True(t, modelPerformanceWatermarkRegclass.Valid, "expected model_performance_metrics_aggregation_watermark table to exist")
	requireColumn(t, tx, "model_performance_metrics_aggregation_watermark", "last_aggregated_at", "timestamp with time zone", 0, true)
	requireCheckConstraint(t, tx, "model_performance_metrics_aggregation_watermark", "model_performance_metrics_aggregation_watermark_singleton")

	// subscription_grants: stacked subscription foundation (081)
	var subscriptionGrantsRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.subscription_grants')").Scan(&subscriptionGrantsRegclass))
	require.True(t, subscriptionGrantsRegclass.Valid, "expected subscription_grants table to exist")
	requireColumn(t, tx, "subscription_grants", "subscription_id", "bigint", 0, false)
	requireColumn(t, tx, "subscription_grants", "starts_at", "timestamp with time zone", 0, false)
	requireColumn(t, tx, "subscription_grants", "expires_at", "timestamp with time zone", 0, false)
	requireColumn(t, tx, "subscription_grants", "daily_usage_usd", "numeric", 0, false)
	requireColumn(t, tx, "subscription_grants", "weekly_usage_usd", "numeric", 0, false)
	requireColumn(t, tx, "subscription_grants", "monthly_usage_usd", "numeric", 0, false)
	requireColumn(t, tx, "subscription_grants", "deleted_at", "timestamp with time zone", 0, true)
	requireIndex(t, tx, "subscription_grants", "idx_subscription_grants_subscription_id_active")
	requireIndex(t, tx, "subscription_grants", "idx_subscription_grants_active_lookup")
	requireIndex(t, tx, "subscription_grants", "idx_subscription_grants_expires_at_active")

	// subscription_exhaustion_daily_stats: dashboard daily exhaustion snapshots (108)
	var subscriptionExhaustionStatsRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.subscription_exhaustion_daily_stats')").Scan(&subscriptionExhaustionStatsRegclass))
	require.True(t, subscriptionExhaustionStatsRegclass.Valid, "expected subscription_exhaustion_daily_stats table to exist")
	requireColumn(t, tx, "subscription_exhaustion_daily_stats", "bucket_date", "date", 0, false)
	requireColumn(t, tx, "subscription_exhaustion_daily_stats", "total_subscriptions", "bigint", 0, false)
	requireColumn(t, tx, "subscription_exhaustion_daily_stats", "exhausted_subscriptions", "bigint", 0, false)
	requireColumn(t, tx, "subscription_exhaustion_daily_stats", "exhaustion_rate", "double precision", 0, false)
	requireIndex(t, tx, "subscription_exhaustion_daily_stats", "idx_subscription_exhaustion_daily_stats_bucket_date")

	// sales commissions: accounting tables for frozen, unlocked, and settled commission
	var salesCommissionRecordsRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.sales_commission_records')").Scan(&salesCommissionRecordsRegclass))
	require.True(t, salesCommissionRecordsRegclass.Valid, "expected sales_commission_records table to exist")
	requireColumn(t, tx, "sales_commission_records", "sales_user_id", "bigint", 0, false)
	requireColumn(t, tx, "sales_commission_records", "referee_user_id", "bigint", 0, false)
	requireColumn(t, tx, "sales_commission_records", "referral_id", "bigint", 0, false)
	requireColumn(t, tx, "sales_commission_records", "payment_order_id", "bigint", 0, true)
	requireColumn(t, tx, "sales_commission_records", "order_pay_amount_cny", "numeric", 0, false)
	requireColumn(t, tx, "sales_commission_records", "order_credited_amount", "numeric", 0, false)
	requireColumn(t, tx, "sales_commission_records", "commission_rate", "numeric", 0, false)
	requireColumn(t, tx, "sales_commission_records", "commission_total_cny", "numeric", 0, false)
	requireColumn(t, tx, "sales_commission_records", "credited_used_amount", "numeric", 0, false)
	requireColumn(t, tx, "sales_commission_records", "unlocked_cny", "numeric", 0, false)
	requireColumn(t, tx, "sales_commission_records", "settled_cny", "numeric", 0, false)
	requireColumn(t, tx, "sales_commission_records", "status", "character varying", 32, false)
	requireColumn(t, tx, "sales_commission_records", "note", "text", 0, false)
	requireColumn(t, tx, "sales_commission_records", "commission_month", "date", 0, false)
	requireColumn(t, tx, "sales_commission_records", "snapshot_id", "bigint", 0, true)
	requireColumn(t, tx, "sales_commission_records", "commission_mode", "character varying", 16, false)
	requireColumn(t, tx, "sales_commission_records", "commission_event_at", "timestamp with time zone", 0, true)
	requireColumn(t, tx, "sales_commission_records", "monthly_sales_before_cny", "numeric", 0, false)
	requireColumn(t, tx, "sales_commission_records", "monthly_sales_after_cny", "numeric", 0, false)
	requireColumn(t, tx, "sales_commission_records", "created_at", "timestamp with time zone", 0, false)
	requireColumn(t, tx, "sales_commission_records", "updated_at", "timestamp with time zone", 0, false)
	requireIndex(t, tx, "sales_commission_records", "sales_commission_records_payment_order_id_key")
	requireIndex(t, tx, "sales_commission_records", "idx_sales_commission_sales_user")
	requireIndex(t, tx, "sales_commission_records", "idx_sales_commission_referee")
	requireIndex(t, tx, "sales_commission_records", "idx_sales_commission_status")
	requireIndex(t, tx, "sales_commission_records", "idx_sales_commission_records_month")
	requireForeignKey(t, tx, "sales_commission_records", "fk_sales_commission_records_sales_user", "users")
	requireForeignKey(t, tx, "sales_commission_records", "fk_sales_commission_records_referee_user", "users")
	requireForeignKey(t, tx, "sales_commission_records", "fk_sales_commission_records_referral", "referrals")
	requireForeignKey(t, tx, "sales_commission_records", "fk_sales_commission_records_payment_order", "payment_orders")
	requireCheckConstraint(t, tx, "sales_commission_records", "chk_sales_commission_records_amounts")
	requireCheckConstraint(t, tx, "sales_commission_records", "chk_sales_commission_records_status")

	var tiersRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.sales_commission_tiers')").Scan(&tiersRegclass))
	require.True(t, tiersRegclass.Valid, "expected sales_commission_tiers table to exist")
	requireColumn(t, tx, "sales_commission_tiers", "month_sales_from_cny", "numeric", 0, false)
	requireColumn(t, tx, "sales_commission_tiers", "month_sales_to_cny", "numeric", 0, true)
	requireIndex(t, tx, "sales_commission_tiers", "idx_sales_commission_tiers_sales_user")

	var snapshotsRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.sales_commission_monthly_snapshots')").Scan(&snapshotsRegclass))
	require.True(t, snapshotsRegclass.Valid, "expected sales_commission_monthly_snapshots table to exist")
	requireColumn(t, tx, "sales_commission_monthly_snapshots", "commission_month", "date", 0, false)
	requireColumn(t, tx, "sales_commission_monthly_snapshots", "tiers_json", "jsonb", 0, false)
	requireIndex(t, tx, "sales_commission_monthly_snapshots", "uq_sales_commission_monthly_snapshots")

	var salesCommissionSettlementsRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.sales_commission_settlements')").Scan(&salesCommissionSettlementsRegclass))
	require.True(t, salesCommissionSettlementsRegclass.Valid, "expected sales_commission_settlements table to exist")
	requireColumn(t, tx, "sales_commission_settlements", "sales_user_id", "bigint", 0, false)
	requireColumn(t, tx, "sales_commission_settlements", "amount_cny", "numeric", 0, false)
	requireColumn(t, tx, "sales_commission_settlements", "note", "text", 0, false)
	requireColumn(t, tx, "sales_commission_settlements", "created_by", "bigint", 0, true)
	requireColumn(t, tx, "sales_commission_settlements", "created_at", "timestamp with time zone", 0, false)
	requireIndex(t, tx, "sales_commission_settlements", "idx_sales_commission_settlements_sales_user")
	requireForeignKey(t, tx, "sales_commission_settlements", "fk_sales_commission_settlements_sales_user", "users")
	requireForeignKey(t, tx, "sales_commission_settlements", "fk_sales_commission_settlements_created_by", "users")
	requireCheckConstraint(t, tx, "sales_commission_settlements", "chk_sales_commission_settlements_amount")

	var salesCommissionSettlementItemsRegclass sql.NullString
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT to_regclass('public.sales_commission_settlement_items')").Scan(&salesCommissionSettlementItemsRegclass))
	require.True(t, salesCommissionSettlementItemsRegclass.Valid, "expected sales_commission_settlement_items table to exist")
	requireColumn(t, tx, "sales_commission_settlement_items", "settlement_id", "bigint", 0, false)
	requireColumn(t, tx, "sales_commission_settlement_items", "commission_record_id", "bigint", 0, false)
	requireColumn(t, tx, "sales_commission_settlement_items", "amount_cny", "numeric", 0, false)
	requireColumn(t, tx, "sales_commission_settlement_items", "created_at", "timestamp with time zone", 0, false)
	requireIndex(t, tx, "sales_commission_settlement_items", "idx_sales_commission_settlement_items_record")
	requireIndex(t, tx, "sales_commission_settlement_items", "idx_sales_commission_settlement_items_settlement")
	requireForeignKey(t, tx, "sales_commission_settlement_items", "fk_sales_commission_settlement_items_settlement", "sales_commission_settlements")
	requireForeignKey(t, tx, "sales_commission_settlement_items", "fk_sales_commission_settlement_items_record", "sales_commission_records")
	requireCheckConstraint(t, tx, "sales_commission_settlement_items", "chk_sales_commission_settlement_items_amount")
}

func requireIndex(t *testing.T, tx *sql.Tx, table, index string) {
	t.Helper()

	var exists bool
	err := tx.QueryRowContext(context.Background(), `
SELECT EXISTS (
	SELECT 1
	FROM pg_indexes
	WHERE schemaname = 'public'
	  AND tablename = $1
	  AND indexname = $2
)
`, table, index).Scan(&exists)
	require.NoError(t, err, "query pg_indexes for %s.%s", table, index)
	require.True(t, exists, "expected index %s on %s", index, table)
}

func requireColumn(t *testing.T, tx *sql.Tx, table, column, dataType string, maxLen int, nullable bool) {
	t.Helper()

	var row struct {
		DataType string
		MaxLen   sql.NullInt64
		Nullable string
	}

	err := tx.QueryRowContext(context.Background(), `
SELECT
  data_type,
  character_maximum_length,
  is_nullable
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name = $1
  AND column_name = $2
`, table, column).Scan(&row.DataType, &row.MaxLen, &row.Nullable)
	require.NoError(t, err, "query information_schema.columns for %s.%s", table, column)
	require.Equal(t, dataType, row.DataType, "data_type mismatch for %s.%s", table, column)

	if maxLen > 0 {
		require.True(t, row.MaxLen.Valid, "expected maxLen for %s.%s", table, column)
		require.Equal(t, int64(maxLen), row.MaxLen.Int64, "maxLen mismatch for %s.%s", table, column)
	}

	if nullable {
		require.Equal(t, "YES", row.Nullable, "nullable mismatch for %s.%s", table, column)
	} else {
		require.Equal(t, "NO", row.Nullable, "nullable mismatch for %s.%s", table, column)
	}
}

func requireColumnDefaultContains(t *testing.T, tx *sql.Tx, table, column, expected string) {
	t.Helper()

	var columnDefault sql.NullString
	err := tx.QueryRowContext(context.Background(), `
SELECT column_default
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name = $1
  AND column_name = $2
`, table, column).Scan(&columnDefault)
	require.NoError(t, err, "query column default for %s.%s", table, column)
	require.True(t, columnDefault.Valid, "expected default for %s.%s", table, column)
	require.Contains(t, columnDefault.String, expected, "default mismatch for %s.%s", table, column)
}

func requireForeignKey(t *testing.T, tx *sql.Tx, table, constraint, referencedTable string) {
	t.Helper()

	var actualReferencedTable string
	err := tx.QueryRowContext(context.Background(), `
SELECT ccu.table_name
FROM information_schema.table_constraints tc
JOIN information_schema.constraint_column_usage ccu
  ON ccu.constraint_schema = tc.constraint_schema
 AND ccu.constraint_name = tc.constraint_name
WHERE tc.table_schema = 'public'
  AND tc.table_name = $1
  AND tc.constraint_name = $2
  AND tc.constraint_type = 'FOREIGN KEY'
`, table, constraint).Scan(&actualReferencedTable)
	require.NoError(t, err, "query foreign key %s on %s", constraint, table)
	require.Equal(t, referencedTable, actualReferencedTable, "referenced table mismatch for %s.%s", table, constraint)
}

func requireCheckConstraint(t *testing.T, tx *sql.Tx, table, constraint string) {
	t.Helper()

	var exists bool
	err := tx.QueryRowContext(context.Background(), `
SELECT EXISTS (
	SELECT 1
	FROM information_schema.table_constraints
	WHERE table_schema = 'public'
	  AND table_name = $1
	  AND constraint_name = $2
	  AND constraint_type = 'CHECK'
)
`, table, constraint).Scan(&exists)
	require.NoError(t, err, "query check constraint %s on %s", constraint, table)
	require.True(t, exists, "expected check constraint %s on %s", constraint, table)
}
