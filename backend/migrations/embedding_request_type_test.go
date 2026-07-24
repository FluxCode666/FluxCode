package migrations

import (
	"strings"
	"testing"
)

func TestEmbeddingRequestTypeMigrationUsesOnlineConstraintValidation(t *testing.T) {
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
	if !strings.Contains(sql, "not valid") || !strings.Contains(sql, "lock_timeout") {
		t.Fatal("replacement constraint must avoid a validated hot-table scan")
	}

	validateBody, err := FS.ReadFile("131_validate_embedding_request_type.sql")
	if err != nil {
		t.Fatalf("read embedding constraint validation migration: %v", err)
	}
	validateSQL := strings.ToLower(string(validateBody))
	if !strings.Contains(validateSQL, "validate constraint usage_logs_request_type_check_embedding") {
		t.Fatal("a later migration must validate the new constraint")
	}
}
