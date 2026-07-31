package migrations

import (
	"strings"
	"testing"
)

func TestProviderProtocolPoolBuildsUsageIndexConcurrently(t *testing.T) {
	t.Parallel()

	body, err := FS.ReadFile("136_provider_protocol_pool.sql")
	if err != nil {
		t.Fatalf("read provider protocol pool migration: %v", err)
	}
	if strings.Contains(strings.ToLower(string(body)), "idx_usage_logs_provider_route") {
		t.Fatal("transactional provider migration must not build an index on hot usage_logs")
	}

	indexBody, err := FS.ReadFile("137_provider_usage_route_index_notx.sql")
	if err != nil {
		t.Fatalf("read provider usage route index migration: %v", err)
	}
	indexSQL := strings.ToLower(string(indexBody))
	if !strings.Contains(indexSQL, "create index concurrently if not exists idx_usage_logs_provider_route") {
		t.Fatal("provider usage route index must be created concurrently in a notx migration")
	}
}
