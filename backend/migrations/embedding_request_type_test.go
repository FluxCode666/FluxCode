package migrations

import (
	"strings"
	"testing"
)

func TestEmbeddingRequestTypeMigrationReplacesCheckConstraint(t *testing.T) {
	t.Parallel()

	body, err := FS.ReadFile("129_add_embedding_request_type.sql")
	if err != nil {
		t.Fatalf("read embedding request type migration: %v", err)
	}
	sql := strings.ToLower(string(body))
	if !strings.Contains(sql, "drop constraint if exists usage_logs_request_type_check") {
		t.Fatal("migration must replace the existing request_type check constraint")
	}
	if !strings.Contains(sql, "check (request_type in (0, 1, 2, 3, 4))") {
		t.Fatal("migration must permit request_type 4 and keep 0..3")
	}
}
