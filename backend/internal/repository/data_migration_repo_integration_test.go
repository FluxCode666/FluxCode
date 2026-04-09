//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDataMigrationRepository_ApplyPlanCopiesSubscriptionGrantData(t *testing.T) {
	ctx := context.Background()
	repo := NewDataMigrationRepository()
	sourceSchema := createMigrationTestSchema(t)
	targetSchema := createMigrationTestSchema(t)

	createMigrationCoreTables(t, sourceSchema)
	createMigrationCoreTables(t, targetSchema)
	createMigrationSubscriptionTables(t, sourceSchema)
	createMigrationSubscriptionTables(t, targetSchema)

	mustExecMigrationSQL(t, fmt.Sprintf(`
		INSERT INTO %s.groups (id, name) VALUES (1, 'Pro');
		INSERT INTO %s.users (id, email) VALUES (1, 'stack@example.com');
		INSERT INTO %s.user_subscriptions (id, user_id, group_id, status, quota_multiplier, expires_at)
		VALUES (10, 1, 1, 'active', 2, '2026-04-30T00:00:00Z');
		INSERT INTO %s.subscription_grants (id, subscription_id, starts_at, expires_at, quota_multiplier)
		VALUES (100, 10, '2026-03-01T00:00:00Z', '2026-04-30T00:00:00Z', 2);
		INSERT INTO %s.redeem_codes (id, code, type, status, group_id, validity_days, subscription_mode)
		VALUES (1000, 'STACK-CODE', 'subscription', 'used', 1, 30, 'stack');
	`, quoteMigrationSchema(sourceSchema), quoteMigrationSchema(sourceSchema), quoteMigrationSchema(sourceSchema), quoteMigrationSchema(sourceSchema), quoteMigrationSchema(sourceSchema)))

	_, err := repo.ApplyPlan(ctx, integrationDB, integrationDB, sourceSchema, targetSchema, "core", true)
	require.NoError(t, err)

	phases, err := repo.ApplyPlan(ctx, integrationDB, integrationDB, sourceSchema, targetSchema, "subscriptions", false)
	require.NoError(t, err)
	require.Len(t, phases, 1)

	var quotaMultiplier int
	err = integrationDB.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT quota_multiplier FROM %s.user_subscriptions WHERE id = 10",
		quoteMigrationSchema(targetSchema),
	)).Scan(&quotaMultiplier)
	require.NoError(t, err)
	require.Equal(t, 2, quotaMultiplier)

	var grantCount int
	err = integrationDB.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT COUNT(*) FROM %s.subscription_grants WHERE subscription_id = 10",
		quoteMigrationSchema(targetSchema),
	)).Scan(&grantCount)
	require.NoError(t, err)
	require.Equal(t, 1, grantCount)

	var subscriptionMode string
	err = integrationDB.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT subscription_mode FROM %s.redeem_codes WHERE id = 1000",
		quoteMigrationSchema(targetSchema),
	)).Scan(&subscriptionMode)
	require.NoError(t, err)
	require.Equal(t, "stack", subscriptionMode)

	require.Equal(t, int64(0), phases[0].Tables[0].Difference)
}

func TestDataMigrationRepository_ApplyPlanDoesNotTruncateWhenResetTargetFalse(t *testing.T) {
	ctx := context.Background()
	repo := NewDataMigrationRepository()
	sourceSchema := createMigrationTestSchema(t)
	targetSchema := createMigrationTestSchema(t)

	createMigrationCoreTables(t, sourceSchema)
	createMigrationCoreTables(t, targetSchema)

	mustExecMigrationSQL(t, fmt.Sprintf(`
		INSERT INTO %s.groups (id, name) VALUES (1, 'Source');
		INSERT INTO %s.users (id, email) VALUES (1, 'source@example.com');
	`, quoteMigrationSchema(sourceSchema), quoteMigrationSchema(sourceSchema)))

	// Target has extra rows that should NOT be lost when resetTarget=false
	mustExecMigrationSQL(t, fmt.Sprintf(`
		INSERT INTO %s.groups (id, name) VALUES (2, 'Extra');
		INSERT INTO %s.users (id, email) VALUES (2, 'extra@example.com');
	`, quoteMigrationSchema(targetSchema), quoteMigrationSchema(targetSchema)))

	_, err := repo.ApplyPlan(ctx, integrationDB, integrationDB, sourceSchema, targetSchema, "core", false)
	require.NoError(t, err)

	var groupCount int
	err = integrationDB.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT COUNT(*) FROM %s.groups",
		quoteMigrationSchema(targetSchema),
	)).Scan(&groupCount)
	require.NoError(t, err)
	require.Equal(t, 2, groupCount)

	var userCount int
	err = integrationDB.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT COUNT(*) FROM %s.users",
		quoteMigrationSchema(targetSchema),
	)).Scan(&userCount)
	require.NoError(t, err)
	require.Equal(t, 2, userCount)
}

func TestDataMigrationRepository_ApplyPlanResetsSerialSequenceForIDColumns(t *testing.T) {
	ctx := context.Background()
	repo := NewDataMigrationRepository()
	sourceSchema := createMigrationTestSchema(t)
	targetSchema := createMigrationTestSchema(t)

	createMigrationCoreTablesWithSerialID(t, sourceSchema)
	createMigrationCoreTablesWithSerialID(t, targetSchema)

	// Insert explicit IDs so we can verify the sequence is reset to MAX(id).
	mustExecMigrationSQL(t, fmt.Sprintf(`
		INSERT INTO %s.groups (id, name) VALUES (100, 'Pro');
	`, quoteMigrationSchema(sourceSchema)))

	_, err := repo.ApplyPlan(ctx, integrationDB, integrationDB, sourceSchema, targetSchema, "core", true)
	require.NoError(t, err)

	var nextID int64
	err = integrationDB.QueryRowContext(ctx, fmt.Sprintf(
		"INSERT INTO %s.groups (name) VALUES ('Next') RETURNING id",
		quoteMigrationSchema(targetSchema),
	)).Scan(&nextID)
	require.NoError(t, err)
	require.Greater(t, nextID, int64(100))
}

func TestDataMigrationRepository_BuildPlanReportsPricingCounts(t *testing.T) {
	ctx := context.Background()
	repo := NewDataMigrationRepository()
	sourceSchema := createMigrationTestSchema(t)
	targetSchema := createMigrationTestSchema(t)

	createMigrationPricingTables(t, sourceSchema)
	createMigrationPricingTables(t, targetSchema)

	mustExecMigrationSQL(t, fmt.Sprintf(`
		INSERT INTO %s.pricing_plan_groups (id, name) VALUES (1, 'Starter');
		INSERT INTO %s.pricing_plans (id, group_id, name) VALUES (10, 1, 'Monthly');
	`, quoteMigrationSchema(sourceSchema), quoteMigrationSchema(sourceSchema)))

	phases, err := repo.BuildPlan(ctx, integrationDB, integrationDB, sourceSchema, targetSchema, "pricing")
	require.NoError(t, err)
	require.Len(t, phases, 1)
	require.Equal(t, "pricing", phases[0].Phase)
	require.Len(t, phases[0].Tables, 2)
	require.Equal(t, int64(1), phases[0].Tables[0].SourceRows)
	require.Equal(t, int64(0), phases[0].Tables[0].TargetRows)
	require.Equal(t, int64(1), phases[0].Tables[0].Difference)
	require.Equal(t, int64(1), phases[0].Tables[1].Difference)
}

func createMigrationTestSchema(t *testing.T) string {
	t.Helper()

	name := fmt.Sprintf(
		"dm_%d_%d",
		time.Now().UnixNano(),
		len(t.Name()),
	)
	mustExecMigrationSQL(t, fmt.Sprintf("CREATE SCHEMA %s", quoteMigrationSchema(name)))
	t.Cleanup(func() {
		mustExecMigrationSQL(t, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", quoteMigrationSchema(name)))
	})
	return name
}

func createMigrationCoreTables(t *testing.T, schema string) {
	t.Helper()
	mustExecMigrationSQL(t, fmt.Sprintf(`
		CREATE TABLE %s.groups (
			id BIGINT PRIMARY KEY,
			name TEXT NOT NULL
		);
		CREATE TABLE %s.users (
			id BIGINT PRIMARY KEY,
			email TEXT NOT NULL
		);
		CREATE TABLE %s.settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`, quoteMigrationSchema(schema), quoteMigrationSchema(schema), quoteMigrationSchema(schema)))
}

func createMigrationCoreTablesWithSerialID(t *testing.T, schema string) {
	t.Helper()
	mustExecMigrationSQL(t, fmt.Sprintf(`
		CREATE TABLE %s.groups (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL
		);
		CREATE TABLE %s.users (
			id BIGSERIAL PRIMARY KEY,
			email TEXT NOT NULL
		);
		CREATE TABLE %s.settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`, quoteMigrationSchema(schema), quoteMigrationSchema(schema), quoteMigrationSchema(schema)))
}

func createMigrationSubscriptionTables(t *testing.T, schema string) {
	t.Helper()
	mustExecMigrationSQL(t, fmt.Sprintf(`
		CREATE TABLE %s.user_subscriptions (
			id BIGINT PRIMARY KEY,
			user_id BIGINT NOT NULL REFERENCES %s.users(id),
			group_id BIGINT NOT NULL REFERENCES %s.groups(id),
			status TEXT NOT NULL,
			quota_multiplier INTEGER NOT NULL DEFAULT 1,
			expires_at TIMESTAMPTZ,
			daily_usage_usd NUMERIC NOT NULL DEFAULT 0,
			weekly_usage_usd NUMERIC NOT NULL DEFAULT 0,
			monthly_usage_usd NUMERIC NOT NULL DEFAULT 0
		);
		CREATE TABLE %s.subscription_grants (
			id BIGINT PRIMARY KEY,
			subscription_id BIGINT NOT NULL REFERENCES %s.user_subscriptions(id),
			starts_at TIMESTAMPTZ NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			quota_multiplier INTEGER NOT NULL DEFAULT 1
		);
		CREATE TABLE %s.redeem_codes (
			id BIGINT PRIMARY KEY,
			code TEXT NOT NULL,
			type TEXT NOT NULL,
			status TEXT NOT NULL,
			group_id BIGINT REFERENCES %s.groups(id),
			validity_days INTEGER,
			subscription_mode TEXT
		);
	`, quoteMigrationSchema(schema), quoteMigrationSchema(schema), quoteMigrationSchema(schema), quoteMigrationSchema(schema), quoteMigrationSchema(schema), quoteMigrationSchema(schema), quoteMigrationSchema(schema)))
}

func createMigrationPricingTables(t *testing.T, schema string) {
	t.Helper()
	mustExecMigrationSQL(t, fmt.Sprintf(`
		CREATE TABLE %s.pricing_plan_groups (
			id BIGINT PRIMARY KEY,
			name TEXT NOT NULL
		);
		CREATE TABLE %s.pricing_plans (
			id BIGINT PRIMARY KEY,
			group_id BIGINT NOT NULL REFERENCES %s.pricing_plan_groups(id),
			name TEXT NOT NULL
		);
	`, quoteMigrationSchema(schema), quoteMigrationSchema(schema), quoteMigrationSchema(schema)))
}

func mustExecMigrationSQL(t *testing.T, query string) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(), query)
	require.NoError(t, err)
}

func quoteMigrationSchema(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
