package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageBillingRepositoryApplyInsertsEmbeddingUsageInSameTransaction(t *testing.T) {
	t.Parallel()

	db, mock := newSQLMock(t)
	repo := &usageBillingRepository{db: db}
	createdAt := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	usage := &service.UsageLog{
		UserID:         1,
		APIKeyID:       2,
		AccountID:      3,
		RequestID:      "embedding-request",
		Model:          "text-embedding-3-small",
		RequestedModel: "text-embedding-3-small",
		InputTokens:    17,
		InputCost:      0.00001,
		TotalCost:      0.00001,
		ActualCost:     0.00001,
		RateMultiplier: 1,
		RequestType:    service.RequestTypeEmbedding,
		CreatedAt:      createdAt,
	}
	cmd := &service.UsageBillingCommand{
		RequestID: "embedding-request",
		APIKeyID:  2,
		UserID:    1,
		AccountID: 3,
		UsageLog:  usage,
	}

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO usage_billing_dedup").
		WithArgs(cmd.RequestID, cmd.APIKeyID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(10)))
	mock.ExpectQuery("SELECT request_fingerprint[[:space:]]+FROM usage_billing_dedup_archive").
		WithArgs(cmd.RequestID, cmd.APIKeyID).
		WillReturnRows(sqlmock.NewRows([]string{"request_fingerprint"}))
	mock.ExpectExec("INSERT INTO usage_logs").
		WithArgs(anySliceToDriverValues(prepareUsageLogInsert(usage).args)...).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	result, err := repo.Apply(context.Background(), cmd)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageBillingRepositoryApplyRollsBackWhenEmbeddingUsageInsertFails(t *testing.T) {
	t.Parallel()

	db, mock := newSQLMock(t)
	repo := &usageBillingRepository{db: db}
	usage := &service.UsageLog{RequestID: "embedding-request", RequestType: service.RequestTypeEmbedding}
	cmd := &service.UsageBillingCommand{RequestID: "embedding-request", APIKeyID: 2, UsageLog: usage}

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO usage_billing_dedup").
		WithArgs(cmd.RequestID, cmd.APIKeyID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(10)))
	mock.ExpectQuery("SELECT request_fingerprint[[:space:]]+FROM usage_billing_dedup_archive").
		WithArgs(cmd.RequestID, cmd.APIKeyID).
		WillReturnRows(sqlmock.NewRows([]string{"request_fingerprint"}))
	mock.ExpectExec("INSERT INTO usage_logs").WillReturnError(context.DeadlineExceeded)
	mock.ExpectRollback()

	_, err := repo.Apply(context.Background(), cmd)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageBillingRepositoryApplyReturnsFailureWhenCommitFailsAfterEmbeddingEffects(t *testing.T) {
	t.Parallel()

	db, mock := newSQLMock(t)
	repo := &usageBillingRepository{db: db}
	usage := &service.UsageLog{RequestID: "embedding-commit", APIKeyID: 2, RequestType: service.RequestTypeEmbedding, CreatedAt: time.Now()}
	cmd := &service.UsageBillingCommand{
		RequestID: "embedding-commit", APIKeyID: 2, APIKeyQuotaCost: 1.25, UsageLog: usage,
	}
	commitErr := errors.New("commit failed")

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO usage_billing_dedup").
		WithArgs(cmd.RequestID, cmd.APIKeyID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(10)))
	mock.ExpectQuery("SELECT request_fingerprint[[:space:]]+FROM usage_billing_dedup_archive").
		WithArgs(cmd.RequestID, cmd.APIKeyID).
		WillReturnRows(sqlmock.NewRows([]string{"request_fingerprint"}))
	mock.ExpectQuery("UPDATE api_keys[[:space:]]+SET quota_used").
		WithArgs(cmd.APIKeyQuotaCost, cmd.APIKeyID, service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted).
		WillReturnRows(sqlmock.NewRows([]string{"exhausted"}).AddRow(false))
	mock.ExpectExec("INSERT INTO usage_logs").
		WithArgs(anySliceToDriverValues(prepareUsageLogInsert(usage).args)...).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(commitErr)

	result, err := repo.Apply(context.Background(), cmd)
	require.Nil(t, result)
	require.ErrorIs(t, err, commitErr)
	require.NoError(t, mock.ExpectationsWereMet())
}
