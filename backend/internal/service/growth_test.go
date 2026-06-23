package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseGrowthQueryRange_DefaultsToLast30DaysShanghai(t *testing.T) {
	now := time.Date(2026, 5, 30, 15, 10, 0, 0, time.UTC)

	got, err := ParseGrowthQueryRange("", "", "", now)

	require.NoError(t, err)
	require.Equal(t, GrowthGranularityDay, got.Granularity)
	require.Equal(t, "2026-05-01", got.StartDate)
	require.Equal(t, "2026-05-30", got.EndDate)
	require.Equal(t, "Asia/Shanghai", got.Start.Location().String())
	require.Equal(t, time.Date(2026, 5, 1, 0, 0, 0, 0, got.Start.Location()), got.Start)
	require.Equal(t, time.Date(2026, 5, 31, 0, 0, 0, 0, got.End.Location()), got.End)
}

func TestParseGrowthQueryRange_RejectsBadInputs(t *testing.T) {
	now := time.Date(2026, 5, 30, 15, 10, 0, 0, time.UTC)

	cases := []struct {
		name        string
		start       string
		end         string
		granularity string
	}{
		{name: "bad start date", start: "2026-99-01", end: "2026-05-30", granularity: "day"},
		{name: "bad end date", start: "2026-05-01", end: "bad", granularity: "day"},
		{name: "start after end", start: "2026-05-30", end: "2026-05-01", granularity: "day"},
		{name: "bad granularity", start: "2026-05-01", end: "2026-05-30", granularity: "hour"},
		{name: "range too wide", start: "2025-01-01", end: "2026-05-30", granularity: "day"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseGrowthQueryRange(tc.start, tc.end, tc.granularity, now)
			require.Error(t, err)
		})
	}
}

func TestGrowthSafeRate(t *testing.T) {
	require.Equal(t, 0.0, growthSafeRate(10, 0))
	require.Equal(t, 0.25, growthSafeRate(25, 100))
}

type growthRepoOverviewStub struct {
	overview            *GrowthOverview
	userTrend           []GrowthUserTrendPoint
	sources             []GrowthSourceItem
	sourceRates         []GrowthSourcePaymentRateItem
	retentionMatrix     *GrowthRetentionMatrix
	retentionTrend      []GrowthRetentionTrendPoint
	paymentFunnel       *GrowthPaymentFunnel
	paymentPlans        []GrowthPaymentPlanItem
	firstPaymentBuckets []GrowthFirstPaymentBucket
	featureRanking      []GrowthFeatureRankingItem
	sessionMetrics      *GrowthSessionMetrics
	audienceDevices     []GrowthAudienceItem
	audienceOS          []GrowthAudienceItem
	audienceBrowsers    []GrowthAudienceItem
	audienceClients     []GrowthAudienceItem
	gotRange            GrowthQueryRange
	gotTodayStart       time.Time
	gotTodayEnd         time.Time
	gotMonthStart       time.Time
	gotMonthEnd         time.Time
}

func (s *growthRepoOverviewStub) GetOverview(ctx context.Context, r GrowthQueryRange, todayStart, todayEnd, monthStart, monthEnd time.Time) (*GrowthOverview, error) {
	s.gotRange = r
	s.gotTodayStart = todayStart
	s.gotTodayEnd = todayEnd
	s.gotMonthStart = monthStart
	s.gotMonthEnd = monthEnd
	if s.overview == nil {
		return &GrowthOverview{}, nil
	}
	out := *s.overview
	return &out, nil
}

func (s *growthRepoOverviewStub) GetUserTrend(context.Context, GrowthQueryRange) ([]GrowthUserTrendPoint, error) {
	return s.userTrend, nil
}

func (s *growthRepoOverviewStub) GetUserSources(context.Context, GrowthQueryRange) ([]GrowthSourceItem, error) {
	return s.sources, nil
}

func (s *growthRepoOverviewStub) GetSourcePaymentRates(context.Context, GrowthQueryRange) ([]GrowthSourcePaymentRateItem, error) {
	return s.sourceRates, nil
}

func (s *growthRepoOverviewStub) GetRetentionMatrix(context.Context, GrowthQueryRange, []int) (*GrowthRetentionMatrix, error) {
	return s.retentionMatrix, nil
}

func (s *growthRepoOverviewStub) GetRetentionTrend(context.Context, GrowthQueryRange) ([]GrowthRetentionTrendPoint, error) {
	return s.retentionTrend, nil
}

func (s *growthRepoOverviewStub) GetPaymentFunnel(context.Context, GrowthQueryRange) (*GrowthPaymentFunnel, error) {
	return s.paymentFunnel, nil
}

func (s *growthRepoOverviewStub) GetPaymentPlans(context.Context, GrowthQueryRange) ([]GrowthPaymentPlanItem, error) {
	return s.paymentPlans, nil
}

func (s *growthRepoOverviewStub) GetFirstPaymentBuckets(context.Context, GrowthQueryRange) ([]GrowthFirstPaymentBucket, error) {
	return s.firstPaymentBuckets, nil
}

func (s *growthRepoOverviewStub) GetFeatureRanking(context.Context, GrowthQueryRange) ([]GrowthFeatureRankingItem, error) {
	return s.featureRanking, nil
}

func (s *growthRepoOverviewStub) GetSessionMetrics(context.Context, GrowthQueryRange) (*GrowthSessionMetrics, error) {
	return s.sessionMetrics, nil
}

func (s *growthRepoOverviewStub) GetAudienceDevices(context.Context, GrowthQueryRange) ([]GrowthAudienceItem, error) {
	return s.audienceDevices, nil
}

func (s *growthRepoOverviewStub) GetAudienceOS(context.Context, GrowthQueryRange) ([]GrowthAudienceItem, error) {
	return s.audienceOS, nil
}

func (s *growthRepoOverviewStub) GetAudienceBrowsers(context.Context, GrowthQueryRange) ([]GrowthAudienceItem, error) {
	return s.audienceBrowsers, nil
}

func (s *growthRepoOverviewStub) GetAudienceClients(context.Context, GrowthQueryRange) ([]GrowthAudienceItem, error) {
	return s.audienceClients, nil
}

func TestGrowthServiceGetOverviewRoundsUnsafeValues(t *testing.T) {
	repo := &growthRepoOverviewStub{
		overview: &GrowthOverview{
			TotalUsers:            100,
			DAU:                   10,
			MAU:                   70,
			TodayNewUsers:         5,
			TodayPaidUsers:        2,
			MonthRevenue:          123.456,
			ARPU:                  1.23456,
			PaymentConversionRate: 0.333333,
			RepurchaseRate:        0.666666,
		},
	}
	svc := NewGrowthService(repo)
	svc.now = func() time.Time {
		return time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)
	}
	r, err := ParseGrowthQueryRange("2026-05-01", "2026-05-30", "day", svc.now())
	require.NoError(t, err)

	got, err := svc.GetOverview(context.Background(), r)

	require.NoError(t, err)
	require.Equal(t, 123.46, got.MonthRevenue)
	require.Equal(t, 1.23, got.ARPU)
	require.Equal(t, 0.3333, got.PaymentConversionRate)
	require.Equal(t, 0.6667, got.RepurchaseRate)
	require.Equal(t, "Asia/Shanghai", repo.gotTodayStart.Location().String())
	require.Equal(t, time.Date(2026, 5, 30, 0, 0, 0, 0, repo.gotTodayStart.Location()), repo.gotTodayStart)
	require.Equal(t, time.Date(2026, 5, 31, 0, 0, 0, 0, repo.gotTodayEnd.Location()), repo.gotTodayEnd)
	require.Equal(t, time.Date(2026, 5, 1, 0, 0, 0, 0, repo.gotMonthStart.Location()), repo.gotMonthStart)
	require.Equal(t, time.Date(2026, 6, 1, 0, 0, 0, 0, repo.gotMonthEnd.Location()), repo.gotMonthEnd)
}

func TestGrowthServiceReadEndpointsPassThroughRepositoryData(t *testing.T) {
	planID := int64(7)
	repo := &growthRepoOverviewStub{
		userTrend: []GrowthUserTrendPoint{{Date: "2026-05-01", NewRegistered: 5}},
		sources:   []GrowthSourceItem{{Source: "Unknown", Users: 5}},
		sourceRates: []GrowthSourcePaymentRateItem{{
			Source:          "Unknown",
			RegisteredUsers: 5,
			PaidUsers:       2,
			ConversionRate:  0.333333,
		}},
		retentionMatrix: &GrowthRetentionMatrix{
			Columns: []string{"D1"},
			Cohorts: []GrowthRetentionCohort{{
				Date:      "2026-05-01",
				NewUsers:  5,
				Retention: map[string]float64{"D1": 0.333333},
			}},
		},
		retentionTrend: []GrowthRetentionTrendPoint{{Date: "2026-05-01", D1: 0.333333, D7: 0.111111, D30: 0}},
		paymentFunnel: &GrowthPaymentFunnel{Steps: []GrowthFunnelStep{{
			Key:            "payment_success",
			Label:          "支付成功",
			Users:          2,
			Count:          3,
			ConversionRate: 0.333333,
		}}},
		paymentPlans: []GrowthPaymentPlanItem{{
			PlanID:   &planID,
			PlanName: "月卡",
			Category: "monthly",
			Sales:    3,
			Revenue:  12.345,
		}},
		firstPaymentBuckets: []GrowthFirstPaymentBucket{{Bucket: "within_1_day", Users: 2, Ratio: 0.333333}},
		featureRanking:      []GrowthFeatureRankingItem{{Feature: "chat", Label: "聊天", Uses: 10, Users: 3, UserRatio: 0.333333}},
		sessionMetrics: &GrowthSessionMetrics{
			AverageInputTokens:  GrowthMetricValue{Available: true, Value: 12.345},
			AverageOutputTokens: GrowthMetricValue{Available: true, Value: 67.891},
		},
		audienceDevices:  []GrowthAudienceItem{{Key: "desktop", Label: "Desktop", Users: 3, Requests: 9, UserRatio: 0.333333}},
		audienceOS:       []GrowthAudienceItem{{Key: "macos", Label: "macOS", Users: 2, Requests: 6, UserRatio: 0.222222}},
		audienceBrowsers: []GrowthAudienceItem{{Key: "chrome", Label: "Chrome", Users: 4, Requests: 12, UserRatio: 0.444444}},
		audienceClients:  []GrowthAudienceItem{{Key: "codex_cli", Label: "Codex CLI", Users: 1, Requests: 5, UserRatio: 0.111111}},
	}
	svc := NewGrowthService(repo)
	r, err := ParseGrowthQueryRange("2026-05-01", "2026-05-30", "day", time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	userTrend, err := svc.GetUserTrend(context.Background(), r)
	require.NoError(t, err)
	require.Equal(t, repo.userTrend, userTrend)

	sources, err := svc.GetUserSources(context.Background(), r)
	require.NoError(t, err)
	require.Equal(t, repo.sources, sources)

	sourceRates, err := svc.GetSourcePaymentRates(context.Background(), r)
	require.NoError(t, err)
	require.Equal(t, 0.3333, sourceRates[0].ConversionRate)

	matrix, err := svc.GetRetentionMatrix(context.Background(), r)
	require.NoError(t, err)
	require.Equal(t, 0.3333, matrix.Cohorts[0].Retention["D1"])

	retentionTrend, err := svc.GetRetentionTrend(context.Background(), r)
	require.NoError(t, err)
	require.Equal(t, 0.3333, retentionTrend[0].D1)
	require.Equal(t, 0.1111, retentionTrend[0].D7)

	funnel, err := svc.GetPaymentFunnel(context.Background(), r)
	require.NoError(t, err)
	require.Equal(t, 0.3333, funnel.Steps[0].ConversionRate)

	plans, err := svc.GetPaymentPlans(context.Background(), r)
	require.NoError(t, err)
	require.Equal(t, 12.35, plans[0].Revenue)

	firstPayment, err := svc.GetFirstPaymentBuckets(context.Background(), r)
	require.NoError(t, err)
	require.Equal(t, 0.3333, firstPayment[0].Ratio)

	features, err := svc.GetFeatureRanking(context.Background(), r)
	require.NoError(t, err)
	require.Equal(t, 0.3333, features[0].UserRatio)

	sessionMetrics, err := svc.GetSessionMetrics(context.Background(), r)
	require.NoError(t, err)
	require.Equal(t, 12.35, sessionMetrics.AverageInputTokens.Value)
	require.Equal(t, 67.89, sessionMetrics.AverageOutputTokens.Value)

	devices, err := svc.GetAudienceDevices(context.Background(), r)
	require.NoError(t, err)
	require.Equal(t, 0.3333, devices[0].UserRatio)

	osItems, err := svc.GetAudienceOS(context.Background(), r)
	require.NoError(t, err)
	require.Equal(t, 0.2222, osItems[0].UserRatio)

	browsers, err := svc.GetAudienceBrowsers(context.Background(), r)
	require.NoError(t, err)
	require.Equal(t, 0.4444, browsers[0].UserRatio)

	clients, err := svc.GetAudienceClients(context.Background(), r)
	require.NoError(t, err)
	require.Equal(t, 0.1111, clients[0].UserRatio)
}
