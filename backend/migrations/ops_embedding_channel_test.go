package migrations

import (
	"strings"
	"testing"
)

func TestOpsEmbeddingChannelMigrationAddsSafeCorrelationOnly(t *testing.T) {
	t.Parallel()

	body, err := FS.ReadFile("130_ops_error_logs_add_channel.sql")
	if err != nil {
		t.Fatalf("read ops embedding channel migration: %v", err)
	}
	sql := strings.ToLower(string(body))
	if !strings.Contains(sql, "add column if not exists channel_id bigint") {
		t.Fatal("migration must add the channel correlation column")
	}
	if !strings.Contains(sql, "idx_ops_error_logs_channel_id_created_at") {
		t.Fatal("migration must index channel correlation queries")
	}
	for _, forbidden := range []string{"request_body", "response_body", "embedding_vector", "input_text"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration must not add content storage: %s", forbidden)
		}
	}
}
