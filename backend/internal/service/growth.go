package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

const (
	GrowthLocationName = "Asia/Shanghai"
	GrowthMaxRangeDays = 180
)

type GrowthGranularity string

const (
	GrowthGranularityDay   GrowthGranularity = "day"
	GrowthGranularityWeek  GrowthGranularity = "week"
	GrowthGranularityMonth GrowthGranularity = "month"
)

var ErrGrowthInvalidQueryRange = errors.New("invalid growth query range")

type GrowthQueryRange struct {
	Start       time.Time
	End         time.Time
	StartDate   string
	EndDate     string
	Granularity GrowthGranularity
}

type GrowthOverview struct {
	TotalUsers            int64   `json:"total_users"`
	DAU                   int64   `json:"dau"`
	MAU                   int64   `json:"mau"`
	TodayNewUsers         int64   `json:"today_new_users"`
	TodayPaidUsers        int64   `json:"today_paid_users"`
	MonthRevenue          float64 `json:"month_revenue"`
	ARPU                  float64 `json:"arpu"`
	PaymentConversionRate float64 `json:"payment_conversion_rate"`
	RepurchaseRate        float64 `json:"repurchase_rate"`
}

type GrowthUserTrendPoint struct {
	Date          string `json:"date"`
	NewRegistered int64  `json:"new_registered"`
	NewActivated  int64  `json:"new_activated"`
	NewPaid       int64  `json:"new_paid"`
}

type GrowthSourceItem struct {
	Source string `json:"source"`
	Users  int64  `json:"users"`
}

type GrowthSourcePaymentRateItem struct {
	Source          string  `json:"source"`
	RegisteredUsers int64   `json:"registered_users"`
	PaidUsers       int64   `json:"paid_users"`
	ConversionRate  float64 `json:"conversion_rate"`
}

type GrowthRetentionCohort struct {
	Date      string             `json:"date"`
	NewUsers  int64              `json:"new_users"`
	Retention map[string]float64 `json:"retention"`
}

type GrowthRetentionMatrix struct {
	Columns []string                `json:"columns"`
	Cohorts []GrowthRetentionCohort `json:"cohorts"`
}

type GrowthRetentionTrendPoint struct {
	Date string  `json:"date"`
	D1   float64 `json:"d1"`
	D7   float64 `json:"d7"`
	D30  float64 `json:"d30"`
}

type GrowthFunnelStep struct {
	Key            string  `json:"key"`
	Label          string  `json:"label"`
	Users          int64   `json:"users"`
	Count          int64   `json:"count"`
	ConversionRate float64 `json:"conversion_rate"`
}

type GrowthPaymentFunnel struct {
	Steps         []GrowthFunnelStep `json:"steps"`
	TrackingReady bool               `json:"tracking_ready"`
}

type GrowthPaymentPlanItem struct {
	PlanID   *int64  `json:"plan_id"`
	PlanName string  `json:"plan_name"`
	Category string  `json:"category"`
	Sales    int64   `json:"sales"`
	Revenue  float64 `json:"revenue"`
}

type GrowthFirstPaymentBucket struct {
	Bucket string  `json:"bucket"`
	Label  string  `json:"label"`
	Users  int64   `json:"users"`
	Ratio  float64 `json:"ratio"`
}

type GrowthFeatureRankingItem struct {
	Feature   string  `json:"feature"`
	Label     string  `json:"label"`
	Uses      int64   `json:"uses"`
	Users     int64   `json:"users"`
	UserRatio float64 `json:"user_ratio"`
}

type GrowthMetricValue struct {
	Available bool    `json:"available"`
	Value     float64 `json:"value"`
}

type GrowthSessionMetrics struct {
	AverageTurns                  GrowthMetricValue `json:"average_turns"`
	AverageSessionDurationSeconds GrowthMetricValue `json:"average_session_duration_seconds"`
	AverageInputTokens            GrowthMetricValue `json:"average_input_tokens"`
	AverageOutputTokens           GrowthMetricValue `json:"average_output_tokens"`
}

type GrowthAudienceItem struct {
	Key       string  `json:"key"`
	Label     string  `json:"label"`
	Users     int64   `json:"users"`
	Requests  int64   `json:"requests"`
	UserRatio float64 `json:"user_ratio"`
}

type GrowthRepository interface {
	GetOverview(ctx context.Context, r GrowthQueryRange, todayStart, todayEnd, monthStart, monthEnd time.Time) (*GrowthOverview, error)
	GetUserTrend(ctx context.Context, r GrowthQueryRange) ([]GrowthUserTrendPoint, error)
	GetUserSources(ctx context.Context, r GrowthQueryRange) ([]GrowthSourceItem, error)
	GetSourcePaymentRates(ctx context.Context, r GrowthQueryRange) ([]GrowthSourcePaymentRateItem, error)
	GetRetentionMatrix(ctx context.Context, r GrowthQueryRange, days []int) (*GrowthRetentionMatrix, error)
	GetRetentionTrend(ctx context.Context, r GrowthQueryRange) ([]GrowthRetentionTrendPoint, error)
	GetPaymentFunnel(ctx context.Context, r GrowthQueryRange) (*GrowthPaymentFunnel, error)
	GetPaymentPlans(ctx context.Context, r GrowthQueryRange) ([]GrowthPaymentPlanItem, error)
	GetFirstPaymentBuckets(ctx context.Context, r GrowthQueryRange) ([]GrowthFirstPaymentBucket, error)
	GetFeatureRanking(ctx context.Context, r GrowthQueryRange) ([]GrowthFeatureRankingItem, error)
	GetSessionMetrics(ctx context.Context, r GrowthQueryRange) (*GrowthSessionMetrics, error)
	GetAudienceDevices(ctx context.Context, r GrowthQueryRange) ([]GrowthAudienceItem, error)
	GetAudienceOS(ctx context.Context, r GrowthQueryRange) ([]GrowthAudienceItem, error)
	GetAudienceBrowsers(ctx context.Context, r GrowthQueryRange) ([]GrowthAudienceItem, error)
	GetAudienceClients(ctx context.Context, r GrowthQueryRange) ([]GrowthAudienceItem, error)
}

type GrowthService struct {
	repo GrowthRepository
	now  func() time.Time
}

func NewGrowthService(repo GrowthRepository) *GrowthService {
	return &GrowthService{repo: repo, now: time.Now}
}

func (s *GrowthService) GetOverview(ctx context.Context, r GrowthQueryRange) (*GrowthOverview, error) {
	if s == nil || s.repo == nil {
		return &GrowthOverview{}, nil
	}

	now := time.Now()
	if s.now != nil {
		now = s.now()
	}
	if now.IsZero() {
		now = time.Now()
	}
	loc := growthLocation()
	localNow := now.In(loc)
	todayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	todayEnd := todayStart.AddDate(0, 0, 1)
	monthStart := time.Date(localNow.Year(), localNow.Month(), 1, 0, 0, 0, 0, loc)
	monthEnd := monthStart.AddDate(0, 1, 0)

	out, err := s.repo.GetOverview(ctx, r, todayStart, todayEnd, monthStart, monthEnd)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return &GrowthOverview{}, nil
	}
	out.MonthRevenue = growthRound2(out.MonthRevenue)
	out.ARPU = growthRound2(out.ARPU)
	out.PaymentConversionRate = growthRound4(out.PaymentConversionRate)
	out.RepurchaseRate = growthRound4(out.RepurchaseRate)
	return out, nil
}

func (s *GrowthService) GetUserTrend(ctx context.Context, r GrowthQueryRange) ([]GrowthUserTrendPoint, error) {
	if s == nil || s.repo == nil {
		return []GrowthUserTrendPoint{}, nil
	}
	out, err := s.repo.GetUserTrend(ctx, r)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return []GrowthUserTrendPoint{}, nil
	}
	return out, nil
}

func (s *GrowthService) GetUserSources(ctx context.Context, r GrowthQueryRange) ([]GrowthSourceItem, error) {
	if s == nil || s.repo == nil {
		return []GrowthSourceItem{}, nil
	}
	out, err := s.repo.GetUserSources(ctx, r)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return []GrowthSourceItem{}, nil
	}
	return out, nil
}

func (s *GrowthService) GetSourcePaymentRates(ctx context.Context, r GrowthQueryRange) ([]GrowthSourcePaymentRateItem, error) {
	if s == nil || s.repo == nil {
		return []GrowthSourcePaymentRateItem{}, nil
	}
	out, err := s.repo.GetSourcePaymentRates(ctx, r)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return []GrowthSourcePaymentRateItem{}, nil
	}
	for i := range out {
		out[i].ConversionRate = growthRound4(out[i].ConversionRate)
	}
	return out, nil
}

func (s *GrowthService) GetRetentionMatrix(ctx context.Context, r GrowthQueryRange) (*GrowthRetentionMatrix, error) {
	if s == nil || s.repo == nil {
		return &GrowthRetentionMatrix{Columns: growthRetentionColumns(), Cohorts: []GrowthRetentionCohort{}}, nil
	}
	out, err := s.repo.GetRetentionMatrix(ctx, r, growthRetentionDays())
	if err != nil {
		return nil, err
	}
	if out == nil {
		return &GrowthRetentionMatrix{Columns: growthRetentionColumns(), Cohorts: []GrowthRetentionCohort{}}, nil
	}
	if len(out.Columns) == 0 {
		out.Columns = growthRetentionColumns()
	}
	if out.Cohorts == nil {
		out.Cohorts = []GrowthRetentionCohort{}
	}
	for i := range out.Cohorts {
		if out.Cohorts[i].Retention == nil {
			out.Cohorts[i].Retention = map[string]float64{}
			continue
		}
		for k, v := range out.Cohorts[i].Retention {
			out.Cohorts[i].Retention[k] = growthRound4(v)
		}
	}
	return out, nil
}

func (s *GrowthService) GetRetentionTrend(ctx context.Context, r GrowthQueryRange) ([]GrowthRetentionTrendPoint, error) {
	if s == nil || s.repo == nil {
		return []GrowthRetentionTrendPoint{}, nil
	}
	out, err := s.repo.GetRetentionTrend(ctx, r)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return []GrowthRetentionTrendPoint{}, nil
	}
	for i := range out {
		out[i].D1 = growthRound4(out[i].D1)
		out[i].D7 = growthRound4(out[i].D7)
		out[i].D30 = growthRound4(out[i].D30)
	}
	return out, nil
}

func (s *GrowthService) GetPaymentFunnel(ctx context.Context, r GrowthQueryRange) (*GrowthPaymentFunnel, error) {
	if s == nil || s.repo == nil {
		return &GrowthPaymentFunnel{Steps: []GrowthFunnelStep{}, TrackingReady: false}, nil
	}
	out, err := s.repo.GetPaymentFunnel(ctx, r)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return &GrowthPaymentFunnel{Steps: []GrowthFunnelStep{}, TrackingReady: false}, nil
	}
	if out.Steps == nil {
		out.Steps = []GrowthFunnelStep{}
	}
	for i := range out.Steps {
		out.Steps[i].ConversionRate = growthRound4(out.Steps[i].ConversionRate)
	}
	return out, nil
}

func (s *GrowthService) GetPaymentPlans(ctx context.Context, r GrowthQueryRange) ([]GrowthPaymentPlanItem, error) {
	if s == nil || s.repo == nil {
		return []GrowthPaymentPlanItem{}, nil
	}
	out, err := s.repo.GetPaymentPlans(ctx, r)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return []GrowthPaymentPlanItem{}, nil
	}
	for i := range out {
		out[i].Revenue = growthRound2(out[i].Revenue)
	}
	return out, nil
}

func (s *GrowthService) GetFirstPaymentBuckets(ctx context.Context, r GrowthQueryRange) ([]GrowthFirstPaymentBucket, error) {
	if s == nil || s.repo == nil {
		return []GrowthFirstPaymentBucket{}, nil
	}
	out, err := s.repo.GetFirstPaymentBuckets(ctx, r)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return []GrowthFirstPaymentBucket{}, nil
	}
	for i := range out {
		out[i].Ratio = growthRound4(out[i].Ratio)
	}
	return out, nil
}

func (s *GrowthService) GetFeatureRanking(ctx context.Context, r GrowthQueryRange) ([]GrowthFeatureRankingItem, error) {
	if s == nil || s.repo == nil {
		return []GrowthFeatureRankingItem{}, nil
	}
	out, err := s.repo.GetFeatureRanking(ctx, r)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return []GrowthFeatureRankingItem{}, nil
	}
	for i := range out {
		out[i].UserRatio = growthRound4(out[i].UserRatio)
	}
	return out, nil
}

func (s *GrowthService) GetSessionMetrics(ctx context.Context, r GrowthQueryRange) (*GrowthSessionMetrics, error) {
	if s == nil || s.repo == nil {
		return &GrowthSessionMetrics{}, nil
	}
	out, err := s.repo.GetSessionMetrics(ctx, r)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return &GrowthSessionMetrics{}, nil
	}
	out.AverageTurns = growthNormalizeMetricValue(out.AverageTurns)
	out.AverageSessionDurationSeconds = growthNormalizeMetricValue(out.AverageSessionDurationSeconds)
	out.AverageInputTokens = growthNormalizeMetricValue(out.AverageInputTokens)
	out.AverageOutputTokens = growthNormalizeMetricValue(out.AverageOutputTokens)
	return out, nil
}

func (s *GrowthService) GetAudienceDevices(ctx context.Context, r GrowthQueryRange) ([]GrowthAudienceItem, error) {
	if s == nil || s.repo == nil {
		return []GrowthAudienceItem{}, nil
	}
	out, err := s.repo.GetAudienceDevices(ctx, r)
	if err != nil {
		return nil, err
	}
	return growthNormalizeAudienceItems(out), nil
}

func (s *GrowthService) GetAudienceOS(ctx context.Context, r GrowthQueryRange) ([]GrowthAudienceItem, error) {
	if s == nil || s.repo == nil {
		return []GrowthAudienceItem{}, nil
	}
	out, err := s.repo.GetAudienceOS(ctx, r)
	if err != nil {
		return nil, err
	}
	return growthNormalizeAudienceItems(out), nil
}

func (s *GrowthService) GetAudienceBrowsers(ctx context.Context, r GrowthQueryRange) ([]GrowthAudienceItem, error) {
	if s == nil || s.repo == nil {
		return []GrowthAudienceItem{}, nil
	}
	out, err := s.repo.GetAudienceBrowsers(ctx, r)
	if err != nil {
		return nil, err
	}
	return growthNormalizeAudienceItems(out), nil
}

func (s *GrowthService) GetAudienceClients(ctx context.Context, r GrowthQueryRange) ([]GrowthAudienceItem, error) {
	if s == nil || s.repo == nil {
		return []GrowthAudienceItem{}, nil
	}
	out, err := s.repo.GetAudienceClients(ctx, r)
	if err != nil {
		return nil, err
	}
	return growthNormalizeAudienceItems(out), nil
}

func ParseGrowthQueryRange(startDate, endDate, granularity string, now time.Time) (GrowthQueryRange, error) {
	loc := growthLocation()
	if now.IsZero() {
		now = time.Now()
	}
	localNow := now.In(loc)
	if startDate == "" {
		startDate = time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -29).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = localNow.Format("2006-01-02")
	}

	g := GrowthGranularity(granularity)
	if g == "" {
		g = GrowthGranularityDay
	}
	switch g {
	case GrowthGranularityDay, GrowthGranularityWeek, GrowthGranularityMonth:
	default:
		return GrowthQueryRange{}, fmt.Errorf("%w: invalid granularity", ErrGrowthInvalidQueryRange)
	}

	start, err := time.ParseInLocation("2006-01-02", startDate, loc)
	if err != nil {
		return GrowthQueryRange{}, fmt.Errorf("%w: invalid start_date", ErrGrowthInvalidQueryRange)
	}
	endDay, err := time.ParseInLocation("2006-01-02", endDate, loc)
	if err != nil {
		return GrowthQueryRange{}, fmt.Errorf("%w: invalid end_date", ErrGrowthInvalidQueryRange)
	}
	end := endDay.AddDate(0, 0, 1)
	if !end.After(start) {
		return GrowthQueryRange{}, fmt.Errorf("%w: start_date must be before or equal to end_date", ErrGrowthInvalidQueryRange)
	}
	if end.Sub(start) > time.Duration(GrowthMaxRangeDays)*24*time.Hour {
		return GrowthQueryRange{}, fmt.Errorf("%w: range exceeds %d days", ErrGrowthInvalidQueryRange, GrowthMaxRangeDays)
	}

	return GrowthQueryRange{
		Start:       start,
		End:         end,
		StartDate:   start.Format("2006-01-02"),
		EndDate:     end.Add(-time.Nanosecond).In(loc).Format("2006-01-02"),
		Granularity: g,
	}, nil
}

func growthLocation() *time.Location {
	loc, err := time.LoadLocation(GrowthLocationName)
	if err != nil {
		return time.FixedZone(GrowthLocationName, 8*60*60)
	}
	return loc
}

func growthSafeRate(numerator, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	return growthRound4(float64(numerator) / float64(denominator))
}

func growthRetentionDays() []int {
	return []int{1, 3, 7, 15, 30}
}

func growthRetentionColumns() []string {
	return []string{"D1", "D3", "D7", "D15", "D30"}
}

func growthNormalizeMetricValue(v GrowthMetricValue) GrowthMetricValue {
	if !v.Available {
		v.Value = 0
		return v
	}
	v.Value = growthRound2(v.Value)
	return v
}

func growthNormalizeAudienceItems(items []GrowthAudienceItem) []GrowthAudienceItem {
	if items == nil {
		return []GrowthAudienceItem{}
	}
	for i := range items {
		items[i].UserRatio = growthRound4(items[i].UserRatio)
	}
	return items
}

func growthRound2(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*100) / 100
}

func growthRound4(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*10000) / 10000
}
