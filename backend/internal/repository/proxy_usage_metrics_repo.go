package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type proxyUsageMetricsRepository struct {
	sql sqlExecutor
}

func NewProxyUsageMetricsRepository(sqlDB *sql.DB) service.ProxyUsageMetricsRepository {
	return newProxyUsageMetricsRepositoryWithSQL(sqlDB)
}

func newProxyUsageMetricsRepositoryWithSQL(sqlq sqlExecutor) *proxyUsageMetricsRepository {
	return &proxyUsageMetricsRepository{sql: sqlq}
}

func (r *proxyUsageMetricsRepository) Increment(ctx context.Context, platform string, proxyID int64, occurredAt time.Time, success bool) error {
	if r == nil || r.sql == nil || proxyID <= 0 {
		return nil
	}
	platform = normalizeProxyUsagePlatform(platform)
	if platform == "" {
		return nil
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	bucketStart := occurredAt.UTC().Truncate(time.Hour)

	successDelta := int64(0)
	failureDelta := int64(1)
	if success {
		successDelta = 1
		failureDelta = 0
	}

	const query = `
INSERT INTO proxy_usage_metrics_hourly (
	bucket_start,
	platform,
	proxy_id,
	total_count,
	success_count,
	failure_count
) VALUES ($1, $2, $3, 1, $4, $5)
ON CONFLICT (bucket_start, platform, proxy_id)
DO UPDATE SET
	total_count = proxy_usage_metrics_hourly.total_count + 1,
	success_count = proxy_usage_metrics_hourly.success_count + EXCLUDED.success_count,
	failure_count = proxy_usage_metrics_hourly.failure_count + EXCLUDED.failure_count,
	updated_at = NOW()
`
	_, err := r.sql.ExecContext(ctx, query, bucketStart, platform, proxyID, successDelta, failureDelta)
	return err
}

func (r *proxyUsageMetricsRepository) ListSummary(ctx context.Context, platform string, startTime, endTime time.Time) (result []service.ProxyUsageSummaryItem, err error) {
	if r == nil || r.sql == nil {
		return []service.ProxyUsageSummaryItem{}, nil
	}
	platform = normalizeProxyUsagePlatform(platform)
	if platform == "" || !endTime.After(startTime) {
		return []service.ProxyUsageSummaryItem{}, nil
	}

	const query = `
SELECT
	m.proxy_id,
	COALESCE(p.name, '') AS proxy_name,
	COALESCE(SUM(m.total_count), 0) AS total_count,
	COALESCE(SUM(m.success_count), 0) AS success_count,
	COALESCE(SUM(m.failure_count), 0) AS failure_count
FROM proxy_usage_metrics_hourly m
LEFT JOIN proxies p ON p.id = m.proxy_id
WHERE m.platform = $1
  AND m.bucket_start >= $2
  AND m.bucket_start < $3
GROUP BY m.proxy_id, p.name
HAVING COALESCE(SUM(m.total_count), 0) > 0
ORDER BY total_count DESC, m.proxy_id ASC
`
	rows, err := r.sql.QueryContext(ctx, query, platform, startTime.UTC(), endTime.UTC())
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			result = nil
		}
	}()

	result = make([]service.ProxyUsageSummaryItem, 0)
	for rows.Next() {
		var item service.ProxyUsageSummaryItem
		if err = rows.Scan(
			&item.ProxyID,
			&item.ProxyName,
			&item.TotalCount,
			&item.SuccessCount,
			&item.FailureCount,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *proxyUsageMetricsRepository) ListHourlyBuckets(ctx context.Context, platform string, startTime, endTime time.Time) (result []service.ProxyUsageHourlyBucket, err error) {
	if r == nil || r.sql == nil {
		return []service.ProxyUsageHourlyBucket{}, nil
	}
	platform = normalizeProxyUsagePlatform(platform)
	if platform == "" || !endTime.After(startTime) {
		return []service.ProxyUsageHourlyBucket{}, nil
	}

	const query = `
SELECT
	m.bucket_start,
	m.proxy_id,
	COALESCE(p.name, '') AS proxy_name,
	CASE
		WHEN COALESCE(p.host, '') = '' THEN ''
		WHEN p.port > 0 THEN p.host || ':' || p.port::text
		ELSE p.host
	END AS proxy_addr,
	COALESCE(p.status, '') AS proxy_status,
	COALESCE(m.total_count, 0) AS total_count,
	COALESCE(m.success_count, 0) AS success_count,
	COALESCE(m.failure_count, 0) AS failure_count
FROM proxy_usage_metrics_hourly m
LEFT JOIN proxies p ON p.id = m.proxy_id
WHERE m.platform = $1
  AND m.bucket_start >= $2
  AND m.bucket_start < $3
  AND COALESCE(m.total_count, 0) > 0
ORDER BY m.bucket_start ASC, m.proxy_id ASC
`
	rows, err := r.sql.QueryContext(ctx, query, platform, startTime.UTC(), endTime.UTC())
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			result = nil
		}
	}()

	result = make([]service.ProxyUsageHourlyBucket, 0)
	for rows.Next() {
		var item service.ProxyUsageHourlyBucket
		if err = rows.Scan(
			&item.BucketStart,
			&item.ProxyID,
			&item.ProxyName,
			&item.ProxyAddr,
			&item.ProxyStatus,
			&item.TotalCount,
			&item.SuccessCount,
			&item.FailureCount,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func normalizeProxyUsagePlatform(platform string) string {
	return strings.ToLower(strings.TrimSpace(platform))
}
