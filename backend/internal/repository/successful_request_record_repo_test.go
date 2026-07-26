package repository

import (
	"context"
	"database/sql/driver"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func successfulRequestRecordForRepositoryTest() *service.SuccessfulRequestRecord {
	requestBody := `{"model":"test-model"}`
	responseBody := `{"id":"response-1"}`
	return &service.SuccessfulRequestRecord{
		EventID:             "5ac529da-09e9-46ad-9490-6af9e21517bb",
		UserID:              10,
		APIKeyID:            20,
		TraceID:             "trace-1",
		RequestID:           "request-1",
		ClientRequestID:     "client-request-1",
		Method:              "POST",
		Endpoint:            "/v1/messages",
		RoutePattern:        "/v1/messages",
		Model:               "test-model",
		StatusCode:          200,
		RequestContentType:  "application/json",
		ResponseContentType: "application/json",
		RequestBody:         &requestBody,
		ResponseBody:        &responseBody,
		RequestBodyBytes:    int64(len(requestBody)),
		ResponseBodyBytes:   int64(len(responseBody)),
		CreatedAt:           time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC),
	}
}

func successfulRequestRecordSQLArgs() []driver.Value {
	args := make([]driver.Value, 25)
	for index := range args {
		args[index] = sqlmock.AnyArg()
	}
	return args
}

func TestSuccessfulRequestRecordRepositoryCreateIsIdempotent(t *testing.T) {
	tests := []struct {
		name         string
		affectedRows int64
		inserted     bool
	}{
		{name: "inserted", affectedRows: 1, inserted: true},
		{name: "duplicate_event_id", affectedRows: 0, inserted: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })

			mock.ExpectExec(`INSERT INTO usage_log_payloads`).
				WithArgs(successfulRequestRecordSQLArgs()...).
				WillReturnResult(sqlmock.NewResult(0, test.affectedRows))

			repository := NewSuccessfulRequestRecordRepository(db)
			inserted, err := repository.Create(context.Background(), successfulRequestRecordForRepositoryTest())

			require.NoError(t, err)
			require.Equal(t, test.inserted, inserted)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestSuccessfulRequestRecordRepositoryCreateSQLLinksUsageLogByClientRequestIDAndAPIKey(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec(`(?s)COALESCE\(.*\$2.*SELECT id.*FROM usage_logs.*request_id = 'client:' \|\| \$8.*api_key_id = \$4`).
		WithArgs(successfulRequestRecordSQLArgs()...).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repository := NewSuccessfulRequestRecordRepository(db)
	inserted, err := repository.Create(context.Background(), successfulRequestRecordForRepositoryTest())

	require.NoError(t, err)
	require.True(t, inserted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSuccessfulRequestRecordRepositoryReconcileUnlinked(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec(`(?s)payload.created_at >= NOW\(\) - INTERVAL '24 hours'.*UPDATE usage_log_payloads AS payload.*SET usage_log_id = matches.usage_log_id`).
		WithArgs(500).
		WillReturnResult(sqlmock.NewResult(0, 2))

	repository := NewSuccessfulRequestRecordRepository(db)
	updated, err := repository.ReconcileUnlinked(context.Background(), 500)

	require.NoError(t, err)
	require.Equal(t, int64(2), updated)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogPayloadMigrationDeletionContract(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "133_usage_log_payloads.sql")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	sql := strings.ToUpper(string(raw))

	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS USAGE_LOG_PAYLOADS")
	require.Contains(t, sql, "USAGE_LOG_ID            BIGINT REFERENCES USAGE_LOGS(ID) ON DELETE SET NULL")
	require.NotContains(t, sql, "ON DELETE CASCADE")
	require.Contains(t, sql, "UNIQUE (EVENT_ID)")
	require.Contains(t, sql, "WHERE USAGE_LOG_ID IS NOT NULL")
	require.NotContains(t, sql, "DELETE FROM USAGE_LOG_PAYLOADS")
}
