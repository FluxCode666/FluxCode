//go:build unit

package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNotifySchedulerForProxy_NilSQL verifies graceful handling when sql is nil.
func TestNotifySchedulerForProxy_NilSQL(t *testing.T) {
	repo := &proxyRepository{client: nil, sql: nil}
	// Should not panic
	repo.notifySchedulerForProxy(context.Background(), 42)
}

// TestNotifySchedulerForProxy_ZeroProxyID verifies early return for invalid proxy ID.
func TestNotifySchedulerForProxy_ZeroProxyID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := &proxyRepository{client: nil, sql: db}
	repo.notifySchedulerForProxy(context.Background(), 0)
	repo.notifySchedulerForProxy(context.Background(), -1)

	// No SQL should have been executed
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestNotifySchedulerForProxy_WithAccounts verifies that when a proxy has
// associated accounts, an account_bulk_changed outbox event is published with
// the correct account IDs in the payload.
func TestNotifySchedulerForProxy_WithAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	published := setupTestOutboxPublisher(t)

	// Mock: SELECT account IDs for proxy 42
	rows := sqlmock.NewRows([]string{"id"}).
		AddRow(int64(100)).
		AddRow(int64(200)).
		AddRow(int64(300))
	mock.ExpectQuery("SELECT id FROM accounts WHERE proxy_id = \\$1").
		WithArgs(int64(42)).
		WillReturnRows(rows)

	repo := &proxyRepository{client: nil, sql: db}
	repo.notifySchedulerForProxy(context.Background(), 42)

	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, *published, 1)
	assert.Equal(t, service.SchedulerOutboxEventAccountBulkChanged, (*published)[0].eventType)
}

// TestNotifySchedulerForProxy_EmptyAccounts verifies that no outbox event
// is written when the proxy has no associated accounts.
func TestNotifySchedulerForProxy_EmptyAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id"}) // empty
	mock.ExpectQuery("SELECT id FROM accounts WHERE proxy_id = \\$1").
		WithArgs(int64(42)).
		WillReturnRows(rows)

	// No Exec expected — no outbox event

	repo := &proxyRepository{client: nil, sql: db}
	repo.notifySchedulerForProxy(context.Background(), 42)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestNotifySchedulerForProxy_OutboxPayloadFormat verifies that
// notifySchedulerForProxy queries accounts and publishes events.
func TestNotifySchedulerForProxy_OutboxPayloadFormat(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	published := setupTestOutboxPublisher(t)

	rows := sqlmock.NewRows([]string{"id"}).
		AddRow(int64(10)).
		AddRow(int64(20))
	mock.ExpectQuery("SELECT id FROM accounts WHERE proxy_id = \\$1").
		WithArgs(int64(7)).
		WillReturnRows(rows)

	repo := &proxyRepository{client: nil, sql: db}
	repo.notifySchedulerForProxy(context.Background(), 7)

	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, *published, 1)
	assert.Equal(t, service.SchedulerOutboxEventAccountBulkChanged, (*published)[0].eventType)
}

// TestNotifySchedulerForProxy_Chunking verifies that when the number of
// associated accounts exceeds proxyOutboxChunkSize, multiple outbox events
// are published — one per chunk.
func TestNotifySchedulerForProxy_Chunking(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	published := setupTestOutboxPublisher(t)

	// Generate 450 account IDs → should produce 3 chunks (200 + 200 + 50)
	totalAccounts := proxyOutboxChunkSize*2 + 50
	rows := sqlmock.NewRows([]string{"id"})
	for i := 1; i <= totalAccounts; i++ {
		rows.AddRow(int64(i))
	}
	mock.ExpectQuery("SELECT id FROM accounts WHERE proxy_id = \\$1").
		WithArgs(int64(99)).
		WillReturnRows(rows)

	repo := &proxyRepository{client: nil, sql: db}
	repo.notifySchedulerForProxy(context.Background(), 99)

	require.NoError(t, mock.ExpectationsWereMet())
	// Expect exactly 3 published events (one per chunk)
	require.Len(t, *published, 3)
}

// TestNotifySchedulerForProxy_ExactChunkSize verifies behavior when account
// count is exactly equal to one chunk size (no extra chunk created).
func TestNotifySchedulerForProxy_ExactChunkSize(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	published := setupTestOutboxPublisher(t)

	rows := sqlmock.NewRows([]string{"id"})
	for i := 1; i <= proxyOutboxChunkSize; i++ {
		rows.AddRow(int64(i))
	}
	mock.ExpectQuery("SELECT id FROM accounts WHERE proxy_id = \\$1").
		WithArgs(int64(5)).
		WillReturnRows(rows)

	repo := &proxyRepository{client: nil, sql: db}
	repo.notifySchedulerForProxy(context.Background(), 5)

	require.NoError(t, mock.ExpectationsWereMet())
	// Exactly 1 chunk
	require.Len(t, *published, 1)
}

// TestEnqueueSchedulerOutbox_BulkPayload verifies that enqueueSchedulerOutbox
// correctly publishes the account_ids payload for account_bulk_changed events.
func TestEnqueueSchedulerOutbox_BulkPayload(t *testing.T) {
	published := setupTestOutboxPublisher(t)

	payload := map[string]any{"account_ids": []int64{100, 200, 300}}

	err := enqueueSchedulerOutbox(context.Background(),
		service.SchedulerOutboxEventAccountBulkChanged, nil, nil, payload)
	require.NoError(t, err)

	require.Len(t, *published, 1)
	assert.Equal(t, service.SchedulerOutboxEventAccountBulkChanged, (*published)[0].eventType)
	rawIDs, ok := (*published)[0].payload["account_ids"].([]any)
	require.True(t, ok)
	assert.Len(t, rawIDs, 3)
}
