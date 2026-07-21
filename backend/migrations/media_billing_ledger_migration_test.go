package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMediaBillingLedgerMigrationContract(t *testing.T) {
	body, err := FS.ReadFile("132_media_billing_ledger.sql")
	require.NoError(t, err)
	sql := string(body)

	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS media_billing_operations",
		"task_id BIGINT NOT NULL REFERENCES media_tasks(id) ON DELETE RESTRICT",
		"idempotency_key VARCHAR(96) NOT NULL UNIQUE",
		"operation VARCHAR(16) NOT NULL CHECK (operation IN ('precharge', 'success', 'failure'))",
		"request_fingerprint VARCHAR(64) NOT NULL",
		"precharged_amount NUMERIC(20,8) NOT NULL DEFAULT 0",
		"final_amount NUMERIC(20,8) NOT NULL DEFAULT 0",
		"refunded_amount NUMERIC(20,8) NOT NULL DEFAULT 0",
		"additional_charged_amount NUMERIC(20,8) NOT NULL DEFAULT 0",
		"allocation JSONB NOT NULL DEFAULT '{}'::jsonb",
		"UNIQUE (task_id, operation)",
		"idx_media_billing_operations_terminal",
		"WHERE operation IN ('success', 'failure')",
		"idx_media_billing_operations_public_id",
		"idx_media_billing_operations_user_created",
	} {
		require.Contains(t, sql, fragment)
	}
}
