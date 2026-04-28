package service

import (
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestBuildDailySeriesUsesDateRange(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)

	start := time.Date(2026, 4, 1, 0, 0, 0, 0, loc)
	end := time.Date(2026, 4, 4, 0, 0, 0, 0, loc)
	paidAt := time.Date(2026, 4, 2, 15, 30, 0, 0, loc)

	got := buildDailySeries([]*dbent.PaymentOrder{{
		PaidAt:    &paidAt,
		PayAmount: 12.345,
	}}, start, end)

	require.Len(t, got, 3)
	require.Equal(t, DailyStats{Date: "2026-04-01"}, got[0])
	require.Equal(t, DailyStats{Date: "2026-04-02", Amount: 12.35, Count: 1}, got[1])
	require.Equal(t, DailyStats{Date: "2026-04-03"}, got[2])
}
