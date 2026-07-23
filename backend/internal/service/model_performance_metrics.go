package service

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ModelPerformanceRange is the fixed public observation range accepted by
// the model pricing page.
type ModelPerformanceRange string

const (
	ModelPerformanceRange24Hours ModelPerformanceRange = "24h"
	ModelPerformanceRange7Days   ModelPerformanceRange = "7d"

	modelPerformancePublicSafeDelay = 5 * time.Minute
)

// ModelPerformanceWindow is a closed-hour UTC half-open interval [Start, End).
type ModelPerformanceWindow struct {
	Start time.Time
	End   time.Time
}

// ModelPerformanceMetrics contains only aggregates that are safe to expose
// from the public model catalogue. Nil values represent a missing effective
// sample; callers must not turn them into zeroes.
type ModelPerformanceMetrics struct {
	TPS                *float64 `json:"tps"`
	Availability       *float64 `json:"availability"`
	AverageFirstToken  *float64 `json:"average_first_token_ms"`
	AverageRequestTime *float64 `json:"average_request_time_ms"`
}

// ModelPerformanceHourlyTrendPoint holds the overall model metrics used by
// the public first-token and availability hourly trend charts.
type ModelPerformanceHourlyTrendPoint struct {
	BucketStart       time.Time `json:"bucket_start"`
	Availability      *float64  `json:"availability"`
	AverageFirstToken *float64  `json:"average_first_token_ms"`
}

// ModelPerformanceDetail is the reader result used to attach performance to a
// public model detail. Group metrics are keyed by currently visible group ID.
type ModelPerformanceDetail struct {
	Overall ModelPerformanceMetrics
	Groups  map[int64]ModelPerformanceMetrics
	Trend   []ModelPerformanceHourlyTrendPoint
}

// ModelPerformanceMetricsReader is deliberately narrow so public catalogue
// reads cannot access raw usage or error-log data.
type ModelPerformanceMetricsReader interface {
	ListModelPerformanceSummaries(ctx context.Context, window ModelPerformanceWindow, models []string, groupID *int64) (map[string]ModelPerformanceMetrics, error)
	GetModelPerformanceDetail(ctx context.Context, window ModelPerformanceWindow, model string, groupIDs []int64) (*ModelPerformanceDetail, error)
}

func ParseModelPerformanceRange(value string) (ModelPerformanceRange, error) {
	switch ModelPerformanceRange(strings.TrimSpace(value)) {
	case "", ModelPerformanceRange24Hours:
		return ModelPerformanceRange24Hours, nil
	case ModelPerformanceRange7Days:
		return ModelPerformanceRange7Days, nil
	default:
		return "", fmt.Errorf("invalid performance range %q: must be 24h or 7d", value)
	}
}

// ResolveModelPerformanceWindow uses the same five-minute safety delay as
// model-performance aggregation, then floors to a completed UTC hour.
func ResolveModelPerformanceWindow(now time.Time, performanceRange ModelPerformanceRange) (ModelPerformanceWindow, error) {
	performanceRange, err := ParseModelPerformanceRange(string(performanceRange))
	if err != nil {
		return ModelPerformanceWindow{}, err
	}

	duration := 24 * time.Hour
	if performanceRange == ModelPerformanceRange7Days {
		duration = 7 * 24 * time.Hour
	}
	end := now.UTC().Add(-modelPerformancePublicSafeDelay).Truncate(time.Hour)
	return ModelPerformanceWindow{Start: end.Add(-duration), End: end}, nil
}
