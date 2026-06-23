package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func mustGrowthRange(t *testing.T) service.GrowthQueryRange {
	t.Helper()
	r, err := service.ParseGrowthQueryRange(
		"2026-05-01",
		"2026-05-30",
		"day",
		time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)
	return r
}

func TestGrowthRepositoryGetOverviewScansRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewGrowthRepository(db)
	r := mustGrowthRange(t)
	loc := growthLocation()
	todayStart := time.Date(2026, 5, 30, 0, 0, 0, 0, loc)
	todayEnd := todayStart.AddDate(0, 0, 1)
	monthStart := time.Date(2026, 5, 1, 0, 0, 0, 0, loc)
	monthEnd := monthStart.AddDate(0, 1, 0)

	mock.ExpectQuery("WITH active_range AS").
		WithArgs(r.Start, r.End, todayStart, todayEnd, monthStart, monthEnd).
		WillReturnRows(sqlmock.NewRows([]string{
			"total_users",
			"dau",
			"mau",
			"today_new_users",
			"today_paid_users",
			"month_revenue",
			"arpu",
			"payment_conversion_rate",
			"repurchase_rate",
		}).AddRow(int64(1000), int64(120), int64(600), int64(24), int64(10), 123.456, 1.23456, 0.333333, 0.666666))

	got, err := repo.GetOverview(context.Background(), r, todayStart, todayEnd, monthStart, monthEnd)

	require.NoError(t, err)
	require.Equal(t, int64(1000), got.TotalUsers)
	require.Equal(t, int64(120), got.DAU)
	require.Equal(t, int64(600), got.MAU)
	require.Equal(t, int64(24), got.TodayNewUsers)
	require.Equal(t, int64(10), got.TodayPaidUsers)
	require.Equal(t, 123.456, got.MonthRevenue)
	require.Equal(t, 1.23456, got.ARPU)
	require.Equal(t, 0.333333, got.PaymentConversionRate)
	require.Equal(t, 0.666666, got.RepurchaseRate)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrowthRepositoryGetUserTrendScansRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewGrowthRepository(db)
	r := mustGrowthRange(t)

	mock.ExpectQuery("WITH registered AS").
		WithArgs(r.Start, r.End).
		WillReturnRows(sqlmock.NewRows([]string{"date", "new_registered", "new_activated", "new_paid"}).
			AddRow("2026-05-01", int64(12), int64(8), int64(3)))

	got, err := repo.GetUserTrend(context.Background(), r)

	require.NoError(t, err)
	require.Equal(t, []service.GrowthUserTrendPoint{{
		Date:          "2026-05-01",
		NewRegistered: 12,
		NewActivated:  8,
		NewPaid:       3,
	}}, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrowthRepositoryGetSessionMetricsMarksSessionFieldsUnavailable(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewGrowthRepository(db)
	r := mustGrowthRange(t)

	mock.ExpectQuery("SELECT COALESCE\\(AVG\\(input_tokens\\), 0\\)").
		WithArgs(r.Start, r.End).
		WillReturnRows(sqlmock.NewRows([]string{"avg_input", "avg_output", "usage_count"}).
			AddRow(float64(100.5), float64(220.25), int64(10)))

	got, err := repo.GetSessionMetrics(context.Background(), r)

	require.NoError(t, err)
	require.False(t, got.AverageTurns.Available)
	require.False(t, got.AverageSessionDurationSeconds.Available)
	require.True(t, got.AverageInputTokens.Available)
	require.Equal(t, 100.5, got.AverageInputTokens.Value)
	require.True(t, got.AverageOutputTokens.Available)
	require.Equal(t, 220.25, got.AverageOutputTokens.Value)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrowthRepositoryGetAudienceDevicesScansRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewGrowthRepository(db)
	r := mustGrowthRange(t)

	mock.ExpectQuery("WITH active_users AS").
		WithArgs(r.Start, r.End).
		WillReturnRows(sqlmock.NewRows([]string{"key", "users", "requests", "active_users"}).
			AddRow("desktop", int64(3), int64(9), int64(10)).
			AddRow("mobile", int64(2), int64(5), int64(10)))

	got, err := repo.GetAudienceDevices(context.Background(), r)

	require.NoError(t, err)
	require.Equal(t, []service.GrowthAudienceItem{
		{Key: "desktop", Label: "Desktop", Users: 3, Requests: 9, UserRatio: 0.3},
		{Key: "mobile", Label: "Mobile", Users: 2, Requests: 5, UserRatio: 0.2},
	}, got)
	require.NoError(t, mock.ExpectationsWereMet())
}
