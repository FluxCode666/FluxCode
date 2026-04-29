package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type dashboardAggregationRepository struct {
	sql sqlExecutor
}

const usageLogsCleanupBatchSize = 10000
const usageBillingDedupCleanupBatchSize = 10000

// NewDashboardAggregationRepository 创建仪表盘预聚合仓储。
func NewDashboardAggregationRepository(sqlDB *sql.DB) service.DashboardAggregationRepository {
	if sqlDB == nil {
		return nil
	}
	if !isPostgresDriver(sqlDB) {
		log.Printf("[DashboardAggregation] 检测到非 PostgreSQL 驱动，已自动禁用预聚合")
		return nil
	}
	return newDashboardAggregationRepositoryWithSQL(sqlDB)
}

func newDashboardAggregationRepositoryWithSQL(sqlq sqlExecutor) *dashboardAggregationRepository {
	return &dashboardAggregationRepository{sql: sqlq}
}

func isPostgresDriver(db *sql.DB) bool {
	if db == nil {
		return false
	}
	_, ok := db.Driver().(*pq.Driver)
	return ok
}

func (r *dashboardAggregationRepository) AggregateRange(ctx context.Context, start, end time.Time) error {
	if r == nil || r.sql == nil {
		return nil
	}
	loc := timezone.Location()
	startLocal := start.In(loc)
	endLocal := end.In(loc)
	if !endLocal.After(startLocal) {
		return nil
	}

	hourStart := startLocal.Truncate(time.Hour)
	hourEnd := endLocal.Truncate(time.Hour)
	if endLocal.After(hourEnd) {
		hourEnd = hourEnd.Add(time.Hour)
	}

	dayStart := truncateToDay(startLocal)
	dayEnd := truncateToDay(endLocal)
	if endLocal.After(dayEnd) {
		dayEnd = dayEnd.Add(24 * time.Hour)
	}

	if db, ok := r.sql.(*sql.DB); ok {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		txRepo := newDashboardAggregationRepositoryWithSQL(tx)
		if err := txRepo.aggregateRangeInTx(ctx, hourStart, hourEnd, dayStart, dayEnd); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	return r.aggregateRangeInTx(ctx, hourStart, hourEnd, dayStart, dayEnd)
}

func (r *dashboardAggregationRepository) aggregateRangeInTx(ctx context.Context, hourStart, hourEnd, dayStart, dayEnd time.Time) error {
	// 以桶边界聚合，允许覆盖 end 所在桶的剩余区间。
	if err := r.insertHourlyActiveUsers(ctx, hourStart, hourEnd); err != nil {
		return err
	}
	if err := r.insertDailyActiveUsers(ctx, hourStart, hourEnd); err != nil {
		return err
	}
	if err := r.upsertHourlyAggregates(ctx, hourStart, hourEnd); err != nil {
		return err
	}
	if err := r.upsertDailyAggregates(ctx, dayStart, dayEnd); err != nil {
		return err
	}
	if err := r.upsertSubscriptionExhaustionDaily(ctx, dayStart, dayEnd); err != nil {
		return err
	}
	return nil
}

func (r *dashboardAggregationRepository) RecomputeRange(ctx context.Context, start, end time.Time) error {
	if r == nil || r.sql == nil {
		return nil
	}
	loc := timezone.Location()
	startLocal := start.In(loc)
	endLocal := end.In(loc)
	if !endLocal.After(startLocal) {
		return nil
	}

	hourStart := startLocal.Truncate(time.Hour)
	hourEnd := endLocal.Truncate(time.Hour)
	if endLocal.After(hourEnd) {
		hourEnd = hourEnd.Add(time.Hour)
	}

	dayStart := truncateToDay(startLocal)
	dayEnd := truncateToDay(endLocal)
	if endLocal.After(dayEnd) {
		dayEnd = dayEnd.Add(24 * time.Hour)
	}

	// 尽量使用事务保证范围内的一致性（允许在非 *sql.DB 的情况下退化为非事务执行）。
	if db, ok := r.sql.(*sql.DB); ok {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		txRepo := newDashboardAggregationRepositoryWithSQL(tx)
		if err := txRepo.recomputeRangeInTx(ctx, hourStart, hourEnd, dayStart, dayEnd); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	return r.recomputeRangeInTx(ctx, hourStart, hourEnd, dayStart, dayEnd)
}

func (r *dashboardAggregationRepository) recomputeRangeInTx(ctx context.Context, hourStart, hourEnd, dayStart, dayEnd time.Time) error {
	// 先清空范围内桶，再重建（避免仅增量插入导致活跃用户等指标无法回退）。
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_hourly WHERE bucket_start >= $1 AND bucket_start < $2", hourStart, hourEnd); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_hourly_users WHERE bucket_start >= $1 AND bucket_start < $2", hourStart, hourEnd); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_daily WHERE bucket_date >= $1::date AND bucket_date < $2::date", dayStart, dayEnd); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_daily_users WHERE bucket_date >= $1::date AND bucket_date < $2::date", dayStart, dayEnd); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM subscription_exhaustion_daily_stats WHERE bucket_date >= $1::date AND bucket_date < $2::date", dayStart, dayEnd); err != nil {
		return err
	}

	if err := r.insertHourlyActiveUsers(ctx, hourStart, hourEnd); err != nil {
		return err
	}
	if err := r.insertDailyActiveUsers(ctx, hourStart, hourEnd); err != nil {
		return err
	}
	if err := r.upsertHourlyAggregates(ctx, hourStart, hourEnd); err != nil {
		return err
	}
	if err := r.upsertDailyAggregates(ctx, dayStart, dayEnd); err != nil {
		return err
	}
	if err := r.upsertSubscriptionExhaustionDaily(ctx, dayStart, dayEnd); err != nil {
		return err
	}
	return nil
}

func (r *dashboardAggregationRepository) GetAggregationWatermark(ctx context.Context) (time.Time, error) {
	var ts time.Time
	query := "SELECT last_aggregated_at FROM usage_dashboard_aggregation_watermark WHERE id = 1"
	if err := scanSingleRow(ctx, r.sql, query, nil, &ts); err != nil {
		if err == sql.ErrNoRows {
			return time.Unix(0, 0).UTC(), nil
		}
		return time.Time{}, err
	}
	return ts.UTC(), nil
}

func (r *dashboardAggregationRepository) UpdateAggregationWatermark(ctx context.Context, aggregatedAt time.Time) error {
	query := `
		INSERT INTO usage_dashboard_aggregation_watermark (id, last_aggregated_at, updated_at)
		VALUES (1, $1, NOW())
		ON CONFLICT (id)
		DO UPDATE SET last_aggregated_at = EXCLUDED.last_aggregated_at, updated_at = EXCLUDED.updated_at
	`
	_, err := r.sql.ExecContext(ctx, query, aggregatedAt.UTC())
	return err
}

func (r *dashboardAggregationRepository) CleanupAggregates(ctx context.Context, hourlyCutoff, dailyCutoff time.Time) error {
	hourlyCutoffUTC := hourlyCutoff.UTC()
	dailyCutoffUTC := dailyCutoff.UTC()
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_hourly WHERE bucket_start < $1", hourlyCutoffUTC); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_hourly_users WHERE bucket_start < $1", hourlyCutoffUTC); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_daily WHERE bucket_date < $1::date", dailyCutoffUTC); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM usage_dashboard_daily_users WHERE bucket_date < $1::date", dailyCutoffUTC); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, "DELETE FROM subscription_exhaustion_daily_stats WHERE bucket_date < $1::date", dailyCutoffUTC); err != nil {
		return err
	}
	return nil
}

func (r *dashboardAggregationRepository) CleanupUsageLogs(ctx context.Context, cutoff time.Time) error {
	isPartitioned, err := r.isUsageLogsPartitioned(ctx)
	if err != nil {
		return err
	}
	if isPartitioned {
		return r.dropUsageLogsPartitions(ctx, cutoff)
	}
	for {
		res, err := r.sql.ExecContext(ctx, `
			WITH victims AS (
				SELECT ctid
				FROM usage_logs
				WHERE created_at < $1
				LIMIT $2
			)
			DELETE FROM usage_logs
			WHERE ctid IN (SELECT ctid FROM victims)
		`, cutoff.UTC(), usageLogsCleanupBatchSize)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected < usageLogsCleanupBatchSize {
			return nil
		}
	}
}

func (r *dashboardAggregationRepository) CleanupUsageBillingDedup(ctx context.Context, cutoff time.Time) error {
	for {
		res, err := r.sql.ExecContext(ctx, `
			WITH victims AS (
				SELECT ctid, request_id, api_key_id, request_fingerprint, created_at
				FROM usage_billing_dedup
				WHERE created_at < $1
				LIMIT $2
			), archived AS (
				INSERT INTO usage_billing_dedup_archive (request_id, api_key_id, request_fingerprint, created_at)
				SELECT request_id, api_key_id, request_fingerprint, created_at
				FROM victims
				ON CONFLICT (request_id, api_key_id) DO NOTHING
			)
			DELETE FROM usage_billing_dedup
			WHERE ctid IN (SELECT ctid FROM victims)
		`, cutoff.UTC(), usageBillingDedupCleanupBatchSize)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected < usageBillingDedupCleanupBatchSize {
			return nil
		}
	}
}

func (r *dashboardAggregationRepository) EnsureUsageLogsPartitions(ctx context.Context, now time.Time) error {
	isPartitioned, err := r.isUsageLogsPartitioned(ctx)
	if err != nil || !isPartitioned {
		return err
	}
	monthStart := truncateToMonthUTC(now)
	prevMonth := monthStart.AddDate(0, -1, 0)
	nextMonth := monthStart.AddDate(0, 1, 0)

	for _, m := range []time.Time{prevMonth, monthStart, nextMonth} {
		if err := r.createUsageLogsPartition(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (r *dashboardAggregationRepository) insertHourlyActiveUsers(ctx context.Context, start, end time.Time) error {
	tzName := timezone.Name()
	query := `
		INSERT INTO usage_dashboard_hourly_users (bucket_start, user_id)
		SELECT DISTINCT
			date_trunc('hour', created_at AT TIME ZONE $3) AT TIME ZONE $3 AS bucket_start,
			user_id
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
		ON CONFLICT DO NOTHING
	`
	_, err := r.sql.ExecContext(ctx, query, start, end, tzName)
	return err
}

func (r *dashboardAggregationRepository) insertDailyActiveUsers(ctx context.Context, start, end time.Time) error {
	tzName := timezone.Name()
	query := `
		INSERT INTO usage_dashboard_daily_users (bucket_date, user_id)
		SELECT DISTINCT
			(bucket_start AT TIME ZONE $3)::date AS bucket_date,
			user_id
		FROM usage_dashboard_hourly_users
		WHERE bucket_start >= $1 AND bucket_start < $2
		ON CONFLICT DO NOTHING
	`
	_, err := r.sql.ExecContext(ctx, query, start, end, tzName)
	return err
}

func (r *dashboardAggregationRepository) upsertHourlyAggregates(ctx context.Context, start, end time.Time) error {
	tzName := timezone.Name()
	query := `
		WITH hourly AS (
			SELECT
				date_trunc('hour', created_at AT TIME ZONE $3) AT TIME ZONE $3 AS bucket_start,
				COUNT(*) AS total_requests,
				COALESCE(SUM(input_tokens), 0) AS input_tokens,
				COALESCE(SUM(output_tokens), 0) AS output_tokens,
				COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
				COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
				COALESCE(SUM(total_cost), 0) AS total_cost,
				COALESCE(SUM(actual_cost), 0) AS actual_cost,
				COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) AS account_cost,
				COALESCE(SUM(COALESCE(duration_ms, 0)), 0) AS total_duration_ms
			FROM usage_logs
			WHERE created_at >= $1 AND created_at < $2
			GROUP BY 1
		),
		user_counts AS (
			SELECT bucket_start, COUNT(*) AS active_users
			FROM usage_dashboard_hourly_users
			WHERE bucket_start >= $1 AND bucket_start < $2
			GROUP BY bucket_start
		)
		INSERT INTO usage_dashboard_hourly (
			bucket_start,
			total_requests,
			input_tokens,
			output_tokens,
			cache_creation_tokens,
			cache_read_tokens,
			total_cost,
			actual_cost,
			account_cost,
			total_duration_ms,
			active_users,
			computed_at
		)
		SELECT
			hourly.bucket_start,
			hourly.total_requests,
			hourly.input_tokens,
			hourly.output_tokens,
			hourly.cache_creation_tokens,
			hourly.cache_read_tokens,
			hourly.total_cost,
			hourly.actual_cost,
			hourly.account_cost,
			hourly.total_duration_ms,
			COALESCE(user_counts.active_users, 0) AS active_users,
			NOW()
		FROM hourly
		LEFT JOIN user_counts ON user_counts.bucket_start = hourly.bucket_start
		ON CONFLICT (bucket_start)
		DO UPDATE SET
			total_requests = EXCLUDED.total_requests,
			input_tokens = EXCLUDED.input_tokens,
			output_tokens = EXCLUDED.output_tokens,
			cache_creation_tokens = EXCLUDED.cache_creation_tokens,
			cache_read_tokens = EXCLUDED.cache_read_tokens,
			total_cost = EXCLUDED.total_cost,
			actual_cost = EXCLUDED.actual_cost,
			account_cost = EXCLUDED.account_cost,
			total_duration_ms = EXCLUDED.total_duration_ms,
			active_users = EXCLUDED.active_users,
			computed_at = EXCLUDED.computed_at
	`
	_, err := r.sql.ExecContext(ctx, query, start, end, tzName)
	return err
}

func (r *dashboardAggregationRepository) upsertDailyAggregates(ctx context.Context, start, end time.Time) error {
	tzName := timezone.Name()
	query := `
		WITH daily AS (
			SELECT
				(bucket_start AT TIME ZONE $5)::date AS bucket_date,
				COALESCE(SUM(total_requests), 0) AS total_requests,
				COALESCE(SUM(input_tokens), 0) AS input_tokens,
				COALESCE(SUM(output_tokens), 0) AS output_tokens,
				COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
				COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
				COALESCE(SUM(total_cost), 0) AS total_cost,
				COALESCE(SUM(actual_cost), 0) AS actual_cost,
				COALESCE(SUM(account_cost), 0) AS account_cost,
				COALESCE(SUM(total_duration_ms), 0) AS total_duration_ms
			FROM usage_dashboard_hourly
			WHERE bucket_start >= $1 AND bucket_start < $2
			GROUP BY (bucket_start AT TIME ZONE $5)::date
		),
		user_counts AS (
			SELECT bucket_date, COUNT(*) AS active_users
			FROM usage_dashboard_daily_users
			WHERE bucket_date >= $3::date AND bucket_date < $4::date
			GROUP BY bucket_date
		)
		INSERT INTO usage_dashboard_daily (
			bucket_date,
			total_requests,
			input_tokens,
			output_tokens,
			cache_creation_tokens,
			cache_read_tokens,
			total_cost,
			actual_cost,
			account_cost,
			total_duration_ms,
			active_users,
			computed_at
		)
		SELECT
			daily.bucket_date,
			daily.total_requests,
			daily.input_tokens,
			daily.output_tokens,
			daily.cache_creation_tokens,
			daily.cache_read_tokens,
			daily.total_cost,
			daily.actual_cost,
			daily.account_cost,
			daily.total_duration_ms,
			COALESCE(user_counts.active_users, 0) AS active_users,
			NOW()
		FROM daily
		LEFT JOIN user_counts ON user_counts.bucket_date = daily.bucket_date
		ON CONFLICT (bucket_date)
		DO UPDATE SET
			total_requests = EXCLUDED.total_requests,
			input_tokens = EXCLUDED.input_tokens,
			output_tokens = EXCLUDED.output_tokens,
			cache_creation_tokens = EXCLUDED.cache_creation_tokens,
			cache_read_tokens = EXCLUDED.cache_read_tokens,
			total_cost = EXCLUDED.total_cost,
			actual_cost = EXCLUDED.actual_cost,
			account_cost = EXCLUDED.account_cost,
			total_duration_ms = EXCLUDED.total_duration_ms,
			active_users = EXCLUDED.active_users,
			computed_at = EXCLUDED.computed_at
	`
	_, err := r.sql.ExecContext(ctx, query, start, end, start, end, tzName)
	return err
}

func (r *dashboardAggregationRepository) upsertSubscriptionExhaustionDaily(ctx context.Context, start, end time.Time) error {
	if !end.After(start) {
		return nil
	}

	tzName := timezone.Name()
	startDate := start.In(timezone.Location()).Format("2006-01-02")
	endDate := end.In(timezone.Location()).Format("2006-01-02")
	query := `
		WITH days AS (
			SELECT
				gs::date AS bucket_date,
				gs::timestamp AT TIME ZONE $3 AS day_start,
				(gs + INTERVAL '1 day')::timestamp AT TIME ZONE $3 AS day_end
			FROM generate_series($1::date, ($2::date - INTERVAL '1 day')::date, INTERVAL '1 day') AS gs
		),
		grant_limits AS (
			SELECT
				d.bucket_date,
				d.day_start,
				d.day_end,
				sg.id AS grant_id,
				sg.subscription_id,
				sg.expires_at,
				CASE WHEN g.daily_limit_usd IS NOT NULL AND g.daily_limit_usd > 0 THEN g.daily_limit_usd ELSE NULL END AS daily_limit_usd,
				CASE WHEN g.weekly_limit_usd IS NOT NULL AND g.weekly_limit_usd > 0 THEN g.weekly_limit_usd ELSE NULL END AS weekly_limit_usd,
				CASE WHEN g.monthly_limit_usd IS NOT NULL AND g.monthly_limit_usd > 0 THEN g.monthly_limit_usd ELSE NULL END AS monthly_limit_usd
			FROM days d
			JOIN subscription_grants sg
				ON sg.deleted_at IS NULL
			   AND sg.starts_at < d.day_end
			   AND sg.expires_at > d.day_start
			JOIN user_subscriptions us
				ON us.id = sg.subscription_id
			   AND us.deleted_at IS NULL
			   AND us.starts_at < d.day_end
			   AND us.expires_at > d.day_start
			   AND (
					us.status = 'active'
					OR (us.status = 'expired' AND us.expires_at <= NOW())
			   )
			JOIN groups g
				ON g.id = us.group_id
			   AND g.deleted_at IS NULL
			   AND g.status = 'active'
		),
		grant_order AS (
			SELECT
				gl.*,
				COALESCE(SUM(COALESCE(gl.daily_limit_usd, 0)) OVER (
					PARTITION BY gl.bucket_date, gl.subscription_id
					ORDER BY gl.expires_at ASC, gl.grant_id ASC
					ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING
				), 0) AS daily_capacity_before,
				COALESCE(SUM(COALESCE(gl.weekly_limit_usd, 0)) OVER (
					PARTITION BY gl.bucket_date, gl.subscription_id
					ORDER BY gl.expires_at ASC, gl.grant_id ASC
					ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING
				), 0) AS weekly_capacity_before,
				COALESCE(SUM(COALESCE(gl.monthly_limit_usd, 0)) OVER (
					PARTITION BY gl.bucket_date, gl.subscription_id
					ORDER BY gl.expires_at ASC, gl.grant_id ASC
					ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING
				), 0) AS monthly_capacity_before
			FROM grant_limits gl
		),
		subscription_days AS (
			SELECT DISTINCT bucket_date, subscription_id, day_start, day_end
			FROM grant_limits
		),
		subscription_usage AS (
			SELECT
				sd.bucket_date,
				sd.subscription_id,
				COALESCE(SUM(CASE
					WHEN ul.created_at >= sd.day_start AND ul.created_at < sd.day_end
					THEN COALESCE(NULLIF(ul.total_cost, 0), ul.actual_cost, 0)
					ELSE 0
				END), 0) AS daily_usage_usd,
				COALESCE(SUM(CASE
					WHEN ul.created_at >= sd.day_end - INTERVAL '7 days' AND ul.created_at < sd.day_end
					THEN COALESCE(NULLIF(ul.total_cost, 0), ul.actual_cost, 0)
					ELSE 0
				END), 0) AS weekly_usage_usd,
				COALESCE(SUM(CASE
					WHEN ul.created_at >= sd.day_end - INTERVAL '30 days' AND ul.created_at < sd.day_end
					THEN COALESCE(NULLIF(ul.total_cost, 0), ul.actual_cost, 0)
					ELSE 0
				END), 0) AS monthly_usage_usd
			FROM subscription_days sd
			LEFT JOIN usage_logs ul
				ON ul.subscription_id = sd.subscription_id
			   AND ul.created_at >= sd.day_end - INTERVAL '30 days'
			   AND ul.created_at < sd.day_end
			GROUP BY sd.bucket_date, sd.subscription_id
		),
		replay_counts AS (
			SELECT
				go.bucket_date,
				COUNT(go.grant_id) AS total_subscriptions,
				COUNT(go.grant_id) FILTER (
					WHERE (
						go.daily_limit_usd IS NOT NULL
						AND su.daily_usage_usd >= go.daily_capacity_before + go.daily_limit_usd
					) OR (
						go.weekly_limit_usd IS NOT NULL
						AND su.weekly_usage_usd >= go.weekly_capacity_before + go.weekly_limit_usd
					) OR (
						go.monthly_limit_usd IS NOT NULL
						AND su.monthly_usage_usd >= go.monthly_capacity_before + go.monthly_limit_usd
					)
				) AS exhausted_subscriptions
			FROM grant_order go
			JOIN subscription_usage su
				ON su.bucket_date = go.bucket_date
			   AND su.subscription_id = go.subscription_id
			GROUP BY go.bucket_date
		),
		live_current_counts AS (
			SELECT
				d.bucket_date,
				COUNT(sg.id) FILTER (
					WHERE us.id IS NOT NULL AND g.id IS NOT NULL
				) AS total_subscriptions,
				COUNT(sg.id) FILTER (
					WHERE us.id IS NOT NULL
					  AND g.id IS NOT NULL
					  AND (
						(g.daily_limit_usd IS NOT NULL AND g.daily_limit_usd > 0 AND sg.daily_usage_usd >= g.daily_limit_usd)
						OR (g.weekly_limit_usd IS NOT NULL AND g.weekly_limit_usd > 0 AND sg.weekly_usage_usd >= g.weekly_limit_usd)
						OR (g.monthly_limit_usd IS NOT NULL AND g.monthly_limit_usd > 0 AND sg.monthly_usage_usd >= g.monthly_limit_usd)
					  )
				) AS exhausted_subscriptions
			FROM days d
			LEFT JOIN subscription_grants sg
				ON sg.deleted_at IS NULL
			   AND sg.starts_at < d.day_end
			   AND sg.expires_at > d.day_start
			LEFT JOIN user_subscriptions us
				ON us.id = sg.subscription_id
			   AND us.deleted_at IS NULL
			   AND us.starts_at < d.day_end
			   AND us.expires_at > d.day_start
			   AND us.status = 'active'
			LEFT JOIN groups g
				ON g.id = us.group_id
			   AND g.deleted_at IS NULL
			   AND g.status = 'active'
			WHERE d.bucket_date = (NOW() AT TIME ZONE $3)::date
			GROUP BY d.bucket_date
		),
		stats AS (
			SELECT
				d.bucket_date,
				CASE
					WHEN d.bucket_date = (NOW() AT TIME ZONE $3)::date THEN COALESCE(live.total_subscriptions, 0)
					ELSE COALESCE(replay.total_subscriptions, 0)
				END AS total_subscriptions,
				CASE
					WHEN d.bucket_date = (NOW() AT TIME ZONE $3)::date THEN COALESCE(live.exhausted_subscriptions, 0)
					ELSE COALESCE(replay.exhausted_subscriptions, 0)
				END AS exhausted_subscriptions
			FROM days d
			LEFT JOIN replay_counts replay ON replay.bucket_date = d.bucket_date
			LEFT JOIN live_current_counts live ON live.bucket_date = d.bucket_date
		)
		INSERT INTO subscription_exhaustion_daily_stats (
			bucket_date,
			total_subscriptions,
			exhausted_subscriptions,
			exhaustion_rate,
			computed_at
		)
		SELECT
			bucket_date,
			total_subscriptions,
			exhausted_subscriptions,
			CASE
				WHEN total_subscriptions > 0 THEN exhausted_subscriptions::float8 / total_subscriptions::float8 * 100
				ELSE 0
			END AS exhaustion_rate,
			NOW()
		FROM stats
		ON CONFLICT (bucket_date)
		DO UPDATE SET
			total_subscriptions = EXCLUDED.total_subscriptions,
			exhausted_subscriptions = EXCLUDED.exhausted_subscriptions,
			exhaustion_rate = EXCLUDED.exhaustion_rate,
			computed_at = EXCLUDED.computed_at
		WHERE subscription_exhaustion_daily_stats.bucket_date = (NOW() AT TIME ZONE $3)::date
	`
	_, err := r.sql.ExecContext(ctx, query, startDate, endDate, tzName)
	return err
}

func (r *dashboardAggregationRepository) isUsageLogsPartitioned(ctx context.Context) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1
			FROM pg_partitioned_table pt
			JOIN pg_class c ON c.oid = pt.partrelid
			WHERE c.relname = 'usage_logs'
		)
	`
	var partitioned bool
	if err := scanSingleRow(ctx, r.sql, query, nil, &partitioned); err != nil {
		return false, err
	}
	return partitioned, nil
}

func (r *dashboardAggregationRepository) dropUsageLogsPartitions(ctx context.Context, cutoff time.Time) error {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT c.relname
		FROM pg_inherits
		JOIN pg_class c ON c.oid = pg_inherits.inhrelid
		JOIN pg_class p ON p.oid = pg_inherits.inhparent
		WHERE p.relname = 'usage_logs'
	`)
	if err != nil {
		return err
	}
	defer func() {
		_ = rows.Close()
	}()

	cutoffMonth := truncateToMonthUTC(cutoff)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		if !strings.HasPrefix(name, "usage_logs_") {
			continue
		}
		suffix := strings.TrimPrefix(name, "usage_logs_")
		month, err := time.Parse("200601", suffix)
		if err != nil {
			continue
		}
		month = month.UTC()
		if month.Before(cutoffMonth) {
			if _, err := r.sql.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", pq.QuoteIdentifier(name))); err != nil {
				return err
			}
		}
	}
	return rows.Err()
}

func (r *dashboardAggregationRepository) createUsageLogsPartition(ctx context.Context, month time.Time) error {
	monthStart := truncateToMonthUTC(month)
	nextMonth := monthStart.AddDate(0, 1, 0)
	name := fmt.Sprintf("usage_logs_%s", monthStart.Format("200601"))
	query := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s PARTITION OF usage_logs FOR VALUES FROM (%s) TO (%s)",
		pq.QuoteIdentifier(name),
		pq.QuoteLiteral(monthStart.Format("2006-01-02")),
		pq.QuoteLiteral(nextMonth.Format("2006-01-02")),
	)
	_, err := r.sql.ExecContext(ctx, query)
	return err
}

func truncateToDay(t time.Time) time.Time {
	return timezone.StartOfDay(t)
}

func truncateToMonthUTC(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}
