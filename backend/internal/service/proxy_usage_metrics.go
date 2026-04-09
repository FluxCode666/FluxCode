package service

import (
	"context"
	"time"
)

const (
	ProxyUsageMetricsPlatformOpenAI = PlatformOpenAI
)

// ProxyUsageSummaryItem represents aggregated proxy usage counts in a time window.
type ProxyUsageSummaryItem struct {
	ProxyID      int64
	ProxyName    string
	TotalCount   int64
	SuccessCount int64
	FailureCount int64
}

// ProxyUsageHourlyBucket represents one persisted hourly usage row for a proxy.
type ProxyUsageHourlyBucket struct {
	BucketStart  time.Time
	ProxyID      int64
	ProxyName    string
	ProxyAddr    string
	ProxyStatus  string
	TotalCount   int64
	SuccessCount int64
	FailureCount int64
}

// ProxyUsageMetricsRepository persists and queries aggregated proxy usage metrics.
type ProxyUsageMetricsRepository interface {
	Increment(ctx context.Context, platform string, proxyID int64, occurredAt time.Time, success bool) error
	ListSummary(ctx context.Context, platform string, startTime, endTime time.Time) ([]ProxyUsageSummaryItem, error)
	ListHourlyBuckets(ctx context.Context, platform string, startTime, endTime time.Time) ([]ProxyUsageHourlyBucket, error)
}
