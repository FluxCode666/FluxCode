package repository

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type growthRepository struct {
	db *sql.DB
}

var _ service.GrowthRepository = (*growthRepository)(nil)

func NewGrowthRepository(db *sql.DB) service.GrowthRepository {
	return &growthRepository{db: db}
}

func (r *growthRepository) GetOverview(ctx context.Context, qr service.GrowthQueryRange, todayStart, todayEnd, monthStart, monthEnd time.Time) (*service.GrowthOverview, error) {
	if r == nil || r.db == nil {
		return &service.GrowthOverview{}, nil
	}

	query := fmt.Sprintf(`
WITH active_range AS (
  SELECT COUNT(DISTINCT ul.user_id) AS active_users
  FROM usage_logs ul
  JOIN users u ON u.id = ul.user_id AND u.deleted_at IS NULL
  WHERE ul.created_at >= $1 AND ul.created_at < $2
),
paid_range AS (
  SELECT COUNT(DISTINCT po.user_id) AS paid_users
  FROM payment_orders po
  JOIN users u ON u.id = po.user_id AND u.deleted_at IS NULL
  WHERE %[1]s AND po.paid_at >= $1 AND po.paid_at < $2
),
repurchase_range AS (
  SELECT COUNT(*) AS repurchase_users
  FROM (
    SELECT po.user_id
    FROM payment_orders po
    JOIN users u ON u.id = po.user_id AND u.deleted_at IS NULL
    WHERE %[1]s AND po.paid_at >= $1 AND po.paid_at < $2
    GROUP BY po.user_id
    HAVING COUNT(*) >= 2
  ) t
),
range_revenue AS (
  SELECT COALESCE(SUM(po.pay_amount), 0)::float8 AS revenue
  FROM payment_orders po
  JOIN users u ON u.id = po.user_id AND u.deleted_at IS NULL
  WHERE %[1]s AND po.paid_at >= $1 AND po.paid_at < $2
)
SELECT
  (SELECT COUNT(*) FROM users WHERE deleted_at IS NULL)::bigint AS total_users,
  (SELECT COUNT(DISTINCT ul.user_id)
     FROM usage_logs ul
     JOIN users u ON u.id = ul.user_id AND u.deleted_at IS NULL
    WHERE ul.created_at >= $3 AND ul.created_at < $4)::bigint AS dau,
  (SELECT COUNT(DISTINCT ul.user_id)
     FROM usage_logs ul
     JOIN users u ON u.id = ul.user_id AND u.deleted_at IS NULL
    WHERE ul.created_at >= $5 AND ul.created_at < $6)::bigint AS mau,
  (SELECT COUNT(*) FROM users WHERE deleted_at IS NULL AND created_at >= $3 AND created_at < $4)::bigint AS today_new_users,
  (SELECT COUNT(DISTINCT po.user_id)
     FROM payment_orders po
     JOIN users u ON u.id = po.user_id AND u.deleted_at IS NULL
    WHERE %[1]s AND po.paid_at >= $3 AND po.paid_at < $4)::bigint AS today_paid_users,
  (SELECT COALESCE(SUM(po.pay_amount), 0)::float8
     FROM payment_orders po
     JOIN users u ON u.id = po.user_id AND u.deleted_at IS NULL
    WHERE %[1]s AND po.paid_at >= $5 AND po.paid_at < $6) AS month_revenue,
  CASE WHEN (SELECT active_users FROM active_range) > 0
    THEN (SELECT revenue FROM range_revenue) / (SELECT active_users FROM active_range)::float8
    ELSE 0 END AS arpu,
  CASE WHEN (SELECT active_users FROM active_range) > 0
    THEN (SELECT paid_users FROM paid_range)::float8 / (SELECT active_users FROM active_range)::float8
    ELSE 0 END AS payment_conversion_rate,
  CASE WHEN (SELECT paid_users FROM paid_range) > 0
    THEN (SELECT repurchase_users FROM repurchase_range)::float8 / (SELECT paid_users FROM paid_range)::float8
    ELSE 0 END AS repurchase_rate
`, growthPaidStatusPredicate("po"))

	var out service.GrowthOverview
	err := r.db.QueryRowContext(ctx, query, qr.Start, qr.End, todayStart, todayEnd, monthStart, monthEnd).Scan(
		&out.TotalUsers,
		&out.DAU,
		&out.MAU,
		&out.TodayNewUsers,
		&out.TodayPaidUsers,
		&out.MonthRevenue,
		&out.ARPU,
		&out.PaymentConversionRate,
		&out.RepurchaseRate,
	)
	if err != nil {
		return nil, fmt.Errorf("query growth overview: %w", err)
	}
	return &out, nil
}

func (r *growthRepository) GetUserTrend(ctx context.Context, qr service.GrowthQueryRange) ([]service.GrowthUserTrendPoint, error) {
	if r == nil || r.db == nil {
		return []service.GrowthUserTrendPoint{}, nil
	}

	bucketUsers := growthBucketExpression(qr.Granularity, "u.created_at")
	bucketUsage := growthBucketExpression(qr.Granularity, "first_usage.first_used_at")
	bucketPaid := growthBucketExpression(qr.Granularity, "first_paid.first_paid_at")
	query := fmt.Sprintf(`
WITH registered AS (
  SELECT %[1]s AS bucket, COUNT(*)::bigint AS count
  FROM users u
  WHERE u.deleted_at IS NULL AND u.created_at >= $1 AND u.created_at < $2
  GROUP BY bucket
),
first_usage AS (
  SELECT ul.user_id, MIN(ul.created_at) AS first_used_at
  FROM usage_logs ul
  JOIN users u ON u.id = ul.user_id AND u.deleted_at IS NULL
  GROUP BY ul.user_id
),
activated AS (
  SELECT %[2]s AS bucket, COUNT(*)::bigint AS count
  FROM first_usage
  WHERE first_used_at >= $1 AND first_used_at < $2
  GROUP BY bucket
),
first_paid AS (
  SELECT po.user_id, MIN(po.paid_at) AS first_paid_at
  FROM payment_orders po
  JOIN users u ON u.id = po.user_id AND u.deleted_at IS NULL
  WHERE %[4]s AND po.paid_at IS NOT NULL
  GROUP BY po.user_id
),
paid AS (
  SELECT %[3]s AS bucket, COUNT(*)::bigint AS count
  FROM first_paid
  WHERE first_paid_at >= $1 AND first_paid_at < $2
  GROUP BY bucket
),
buckets AS (
  SELECT bucket FROM registered
  UNION SELECT bucket FROM activated
  UNION SELECT bucket FROM paid
)
SELECT b.bucket, COALESCE(r.count, 0), COALESCE(a.count, 0), COALESCE(p.count, 0)
FROM buckets b
LEFT JOIN registered r ON r.bucket = b.bucket
LEFT JOIN activated a ON a.bucket = b.bucket
LEFT JOIN paid p ON p.bucket = b.bucket
ORDER BY b.bucket`, bucketUsers, bucketUsage, bucketPaid, growthPaidStatusPredicate("po"))

	rows, err := r.db.QueryContext(ctx, query, qr.Start, qr.End)
	if err != nil {
		return nil, fmt.Errorf("query growth user trend: %w", err)
	}
	defer rows.Close()

	out := []service.GrowthUserTrendPoint{}
	for rows.Next() {
		var item service.GrowthUserTrendPoint
		if err := rows.Scan(&item.Date, &item.NewRegistered, &item.NewActivated, &item.NewPaid); err != nil {
			return nil, fmt.Errorf("scan growth user trend: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate growth user trend: %w", err)
	}
	return out, nil
}

func (r *growthRepository) GetUserSources(ctx context.Context, qr service.GrowthQueryRange) ([]service.GrowthSourceItem, error) {
	if r == nil || r.db == nil {
		return []service.GrowthSourceItem{}, nil
	}
	var users int64
	err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*)::bigint
FROM users
WHERE deleted_at IS NULL AND created_at >= $1 AND created_at < $2
`, qr.Start, qr.End).Scan(&users)
	if err != nil {
		return nil, fmt.Errorf("query growth user sources: %w", err)
	}
	return []service.GrowthSourceItem{{Source: "Unknown", Users: users}}, nil
}

func (r *growthRepository) GetSourcePaymentRates(ctx context.Context, qr service.GrowthQueryRange) ([]service.GrowthSourcePaymentRateItem, error) {
	if r == nil || r.db == nil {
		return []service.GrowthSourcePaymentRateItem{}, nil
	}

	query := fmt.Sprintf(`
WITH cohort AS (
  SELECT id
  FROM users
  WHERE deleted_at IS NULL AND created_at >= $1 AND created_at < $2
),
paid AS (
  SELECT COUNT(DISTINCT po.user_id)::bigint AS paid_users
  FROM payment_orders po
  JOIN cohort c ON c.id = po.user_id
  WHERE %[1]s AND po.paid_at IS NOT NULL
)
SELECT
  (SELECT COUNT(*) FROM cohort)::bigint AS registered_users,
  (SELECT paid_users FROM paid)::bigint AS paid_users
`, growthPaidStatusPredicate("po"))

	var registeredUsers, paidUsers int64
	if err := r.db.QueryRowContext(ctx, query, qr.Start, qr.End).Scan(&registeredUsers, &paidUsers); err != nil {
		return nil, fmt.Errorf("query growth source payment rates: %w", err)
	}
	return []service.GrowthSourcePaymentRateItem{{
		Source:          "Unknown",
		RegisteredUsers: registeredUsers,
		PaidUsers:       paidUsers,
		ConversionRate:  growthRate(paidUsers, registeredUsers),
	}}, nil
}

func (r *growthRepository) GetRetentionMatrix(ctx context.Context, qr service.GrowthQueryRange, _ []int) (*service.GrowthRetentionMatrix, error) {
	columns := []string{"D1", "D3", "D7", "D15", "D30"}
	if r == nil || r.db == nil {
		return &service.GrowthRetentionMatrix{Columns: columns, Cohorts: []service.GrowthRetentionCohort{}}, nil
	}

	activityEnd := qr.End.AddDate(0, 0, 30)
	rows, err := r.db.QueryContext(ctx, `
WITH cohorts AS (
  SELECT u.id AS user_id, (u.created_at AT TIME ZONE 'Asia/Shanghai')::date AS cohort_date
  FROM users u
  WHERE u.deleted_at IS NULL AND u.created_at >= $1 AND u.created_at < $2
),
activity AS (
  SELECT DISTINCT ul.user_id, (ul.created_at AT TIME ZONE 'Asia/Shanghai')::date AS active_date
  FROM usage_logs ul
  JOIN users u ON u.id = ul.user_id AND u.deleted_at IS NULL
  WHERE ul.created_at >= $1 AND ul.created_at < $3
)
SELECT
  to_char(c.cohort_date, 'YYYY-MM-DD') AS cohort_date,
  COUNT(DISTINCT c.user_id)::bigint AS new_users,
  COUNT(DISTINCT c.user_id) FILTER (WHERE a.active_date = c.cohort_date + 1)::bigint AS d1,
  COUNT(DISTINCT c.user_id) FILTER (WHERE a.active_date = c.cohort_date + 3)::bigint AS d3,
  COUNT(DISTINCT c.user_id) FILTER (WHERE a.active_date = c.cohort_date + 7)::bigint AS d7,
  COUNT(DISTINCT c.user_id) FILTER (WHERE a.active_date = c.cohort_date + 15)::bigint AS d15,
  COUNT(DISTINCT c.user_id) FILTER (WHERE a.active_date = c.cohort_date + 30)::bigint AS d30
FROM cohorts c
LEFT JOIN activity a ON a.user_id = c.user_id
GROUP BY c.cohort_date
ORDER BY c.cohort_date DESC
LIMIT 30
`, qr.Start, qr.End, activityEnd)
	if err != nil {
		return nil, fmt.Errorf("query growth retention matrix: %w", err)
	}
	defer rows.Close()

	cohorts := []service.GrowthRetentionCohort{}
	for rows.Next() {
		var date string
		var newUsers, d1, d3, d7, d15, d30 int64
		if err := rows.Scan(&date, &newUsers, &d1, &d3, &d7, &d15, &d30); err != nil {
			return nil, fmt.Errorf("scan growth retention matrix: %w", err)
		}
		cohorts = append(cohorts, service.GrowthRetentionCohort{
			Date:     date,
			NewUsers: newUsers,
			Retention: map[string]float64{
				"D1":  growthRate(d1, newUsers),
				"D3":  growthRate(d3, newUsers),
				"D7":  growthRate(d7, newUsers),
				"D15": growthRate(d15, newUsers),
				"D30": growthRate(d30, newUsers),
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate growth retention matrix: %w", err)
	}
	return &service.GrowthRetentionMatrix{Columns: columns, Cohorts: cohorts}, nil
}

func (r *growthRepository) GetRetentionTrend(ctx context.Context, qr service.GrowthQueryRange) ([]service.GrowthRetentionTrendPoint, error) {
	if r == nil || r.db == nil {
		return []service.GrowthRetentionTrendPoint{}, nil
	}

	activityEnd := qr.End.AddDate(0, 0, 30)
	rows, err := r.db.QueryContext(ctx, `
WITH cohorts AS (
  SELECT u.id AS user_id, (u.created_at AT TIME ZONE 'Asia/Shanghai')::date AS cohort_date
  FROM users u
  WHERE u.deleted_at IS NULL AND u.created_at >= $1 AND u.created_at < $2
),
activity AS (
  SELECT DISTINCT ul.user_id, (ul.created_at AT TIME ZONE 'Asia/Shanghai')::date AS active_date
  FROM usage_logs ul
  JOIN users u ON u.id = ul.user_id AND u.deleted_at IS NULL
  WHERE ul.created_at >= $1 AND ul.created_at < $3
)
SELECT
  to_char(c.cohort_date, 'YYYY-MM-DD') AS cohort_date,
  COUNT(DISTINCT c.user_id)::bigint AS new_users,
  COUNT(DISTINCT c.user_id) FILTER (WHERE a.active_date = c.cohort_date + 1)::bigint AS d1,
  COUNT(DISTINCT c.user_id) FILTER (WHERE a.active_date = c.cohort_date + 7)::bigint AS d7,
  COUNT(DISTINCT c.user_id) FILTER (WHERE a.active_date = c.cohort_date + 30)::bigint AS d30
FROM cohorts c
LEFT JOIN activity a ON a.user_id = c.user_id
GROUP BY c.cohort_date
ORDER BY c.cohort_date ASC
`, qr.Start, qr.End, activityEnd)
	if err != nil {
		return nil, fmt.Errorf("query growth retention trend: %w", err)
	}
	defer rows.Close()

	out := []service.GrowthRetentionTrendPoint{}
	for rows.Next() {
		var item service.GrowthRetentionTrendPoint
		var newUsers, d1, d7, d30 int64
		if err := rows.Scan(&item.Date, &newUsers, &d1, &d7, &d30); err != nil {
			return nil, fmt.Errorf("scan growth retention trend: %w", err)
		}
		item.D1 = growthRate(d1, newUsers)
		item.D7 = growthRate(d7, newUsers)
		item.D30 = growthRate(d30, newUsers)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate growth retention trend: %w", err)
	}
	return out, nil
}

func (r *growthRepository) GetPaymentFunnel(ctx context.Context, qr service.GrowthQueryRange) (*service.GrowthPaymentFunnel, error) {
	if r == nil || r.db == nil {
		return &service.GrowthPaymentFunnel{Steps: []service.GrowthFunnelStep{}, TrackingReady: false}, nil
	}

	query := fmt.Sprintf(`
WITH created_orders AS (
  SELECT COUNT(*)::bigint AS order_count, COUNT(DISTINCT po.user_id)::bigint AS user_count
  FROM payment_orders po
  JOIN users u ON u.id = po.user_id AND u.deleted_at IS NULL
  WHERE po.created_at >= $1 AND po.created_at < $2
),
successful_orders AS (
  SELECT COUNT(*)::bigint AS order_count, COUNT(DISTINCT po.user_id)::bigint AS user_count
  FROM payment_orders po
  JOIN users u ON u.id = po.user_id AND u.deleted_at IS NULL
  WHERE %[1]s AND po.paid_at >= $1 AND po.paid_at < $2
)
SELECT
  (SELECT user_count FROM created_orders)::bigint AS created_users,
  (SELECT order_count FROM created_orders)::bigint AS created_count,
  (SELECT user_count FROM successful_orders)::bigint AS success_users,
  (SELECT order_count FROM successful_orders)::bigint AS success_count
`, growthPaidStatusPredicate("po"))

	var createdUsers, createdCount, successUsers, successCount int64
	if err := r.db.QueryRowContext(ctx, query, qr.Start, qr.End).Scan(&createdUsers, &createdCount, &successUsers, &successCount); err != nil {
		return nil, fmt.Errorf("query growth payment funnel: %w", err)
	}
	steps := []service.GrowthFunnelStep{
		{
			Key:            "order_created",
			Label:          "创建订单",
			Users:          createdUsers,
			Count:          createdCount,
			ConversionRate: boolRate(createdCount > 0),
		},
		{
			Key:            "payment_success",
			Label:          "支付成功",
			Users:          successUsers,
			Count:          successCount,
			ConversionRate: growthRate(successCount, createdCount),
		},
	}
	return &service.GrowthPaymentFunnel{Steps: steps, TrackingReady: false}, nil
}

func (r *growthRepository) GetPaymentPlans(ctx context.Context, qr service.GrowthQueryRange) ([]service.GrowthPaymentPlanItem, error) {
	if r == nil || r.db == nil {
		return []service.GrowthPaymentPlanItem{}, nil
	}

	query := fmt.Sprintf(`
SELECT
  po.plan_id,
  COALESCE(NULLIF(sp.name, ''), CASE WHEN po.order_type = 'balance' THEN '余额充值' ELSE '其他套餐' END) AS plan_name,
  po.order_type,
  COALESCE(sp.validity_days, 0)::int AS validity_days,
  COALESCE(sp.validity_unit, '') AS validity_unit,
  COUNT(*)::bigint AS sales,
  COALESCE(SUM(po.pay_amount), 0)::float8 AS revenue
FROM payment_orders po
JOIN users u ON u.id = po.user_id AND u.deleted_at IS NULL
LEFT JOIN subscription_plans sp ON sp.id = po.plan_id
WHERE %[1]s AND po.paid_at >= $1 AND po.paid_at < $2
GROUP BY po.plan_id, plan_name, po.order_type, validity_days, validity_unit
ORDER BY sales DESC, revenue DESC
`, growthPaidStatusPredicate("po"))

	rows, err := r.db.QueryContext(ctx, query, qr.Start, qr.End)
	if err != nil {
		return nil, fmt.Errorf("query growth payment plans: %w", err)
	}
	defer rows.Close()

	out := []service.GrowthPaymentPlanItem{}
	for rows.Next() {
		var planID sql.NullInt64
		var planName, orderType, validityUnit string
		var validityDays int
		var item service.GrowthPaymentPlanItem
		if err := rows.Scan(&planID, &planName, &orderType, &validityDays, &validityUnit, &item.Sales, &item.Revenue); err != nil {
			return nil, fmt.Errorf("scan growth payment plans: %w", err)
		}
		if planID.Valid {
			id := planID.Int64
			item.PlanID = &id
		}
		item.PlanName = planName
		item.Category = growthPlanCategory(orderType, planID.Valid, validityDays, validityUnit)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate growth payment plans: %w", err)
	}
	return out, nil
}

func (r *growthRepository) GetFirstPaymentBuckets(ctx context.Context, qr service.GrowthQueryRange) ([]service.GrowthFirstPaymentBucket, error) {
	buckets := []service.GrowthFirstPaymentBucket{
		{Bucket: "within_1_day", Label: "1天内"},
		{Bucket: "within_7_days", Label: "7天内"},
		{Bucket: "within_30_days", Label: "30天内"},
		{Bucket: "over_30_days", Label: "30天以上"},
	}
	if r == nil || r.db == nil {
		return buckets, nil
	}

	query := fmt.Sprintf(`
WITH first_paid AS (
  SELECT po.user_id, MIN(po.paid_at) AS first_paid_at
  FROM payment_orders po
  WHERE %[1]s AND po.paid_at IS NOT NULL
  GROUP BY po.user_id
)
SELECT u.created_at, fp.first_paid_at
FROM users u
JOIN first_paid fp ON fp.user_id = u.id
WHERE u.deleted_at IS NULL AND u.created_at >= $1 AND u.created_at < $2
`, growthPaidStatusPredicate("po"))

	rows, err := r.db.QueryContext(ctx, query, qr.Start, qr.End)
	if err != nil {
		return nil, fmt.Errorf("query growth first payment buckets: %w", err)
	}
	defer rows.Close()

	counts := make([]int64, len(buckets))
	var total int64
	for rows.Next() {
		var createdAt, firstPaidAt time.Time
		if err := rows.Scan(&createdAt, &firstPaidAt); err != nil {
			return nil, fmt.Errorf("scan growth first payment buckets: %w", err)
		}
		diffDays := growthShanghaiDateDiffDays(createdAt, firstPaidAt)
		switch {
		case diffDays <= 1:
			counts[0]++
		case diffDays <= 7:
			counts[1]++
		case diffDays <= 30:
			counts[2]++
		default:
			counts[3]++
		}
		total++
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate growth first payment buckets: %w", err)
	}
	for i := range buckets {
		buckets[i].Users = counts[i]
		buckets[i].Ratio = growthRate(counts[i], total)
	}
	return buckets, nil
}

func (r *growthRepository) GetFeatureRanking(ctx context.Context, qr service.GrowthQueryRange) ([]service.GrowthFeatureRankingItem, error) {
	if r == nil || r.db == nil {
		return []service.GrowthFeatureRankingItem{}, nil
	}

	query := fmt.Sprintf(`
WITH active_users AS (
  SELECT COUNT(DISTINCT ul.user_id)::bigint AS total
  FROM usage_logs ul
  JOIN users u ON u.id = ul.user_id AND u.deleted_at IS NULL
  WHERE ul.created_at >= $1 AND ul.created_at < $2
),
categorized AS (
  SELECT
    %s AS feature,
    ul.user_id
  FROM usage_logs ul
  JOIN users u ON u.id = ul.user_id AND u.deleted_at IS NULL
  WHERE ul.created_at >= $1 AND ul.created_at < $2
)
SELECT
  c.feature,
  COUNT(*)::bigint AS uses,
  COUNT(DISTINCT c.user_id)::bigint AS users,
  (SELECT total FROM active_users)::bigint AS active_users
FROM categorized c
GROUP BY c.feature
ORDER BY uses DESC, users DESC, c.feature ASC
`, growthFeatureCaseExpression())

	rows, err := r.db.QueryContext(ctx, query, qr.Start, qr.End)
	if err != nil {
		return nil, fmt.Errorf("query growth feature ranking: %w", err)
	}
	defer rows.Close()

	out := []service.GrowthFeatureRankingItem{}
	for rows.Next() {
		var item service.GrowthFeatureRankingItem
		var activeUsers int64
		if err := rows.Scan(&item.Feature, &item.Uses, &item.Users, &activeUsers); err != nil {
			return nil, fmt.Errorf("scan growth feature ranking: %w", err)
		}
		item.Label = growthFeatureLabel(item.Feature)
		item.UserRatio = growthRate(item.Users, activeUsers)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate growth feature ranking: %w", err)
	}
	return out, nil
}

func (r *growthRepository) GetSessionMetrics(ctx context.Context, qr service.GrowthQueryRange) (*service.GrowthSessionMetrics, error) {
	if r == nil || r.db == nil {
		return &service.GrowthSessionMetrics{}, nil
	}

	var avgInput, avgOutput float64
	var usageCount int64
	err := r.db.QueryRowContext(ctx, `
SELECT COALESCE(AVG(input_tokens), 0)::float8, COALESCE(AVG(output_tokens), 0)::float8, COUNT(*)::bigint
FROM usage_logs ul
JOIN users u ON u.id = ul.user_id AND u.deleted_at IS NULL
WHERE ul.created_at >= $1 AND ul.created_at < $2
`, qr.Start, qr.End).Scan(&avgInput, &avgOutput, &usageCount)
	if err != nil {
		return nil, fmt.Errorf("query growth session metrics: %w", err)
	}

	tokenMetricsAvailable := usageCount > 0
	return &service.GrowthSessionMetrics{
		AverageTurns:                  service.GrowthMetricValue{Available: false, Value: 0},
		AverageSessionDurationSeconds: service.GrowthMetricValue{Available: false, Value: 0},
		AverageInputTokens:            service.GrowthMetricValue{Available: tokenMetricsAvailable, Value: avgInput},
		AverageOutputTokens:           service.GrowthMetricValue{Available: tokenMetricsAvailable, Value: avgOutput},
	}, nil
}

func (r *growthRepository) GetAudienceDevices(ctx context.Context, qr service.GrowthQueryRange) ([]service.GrowthAudienceItem, error) {
	return r.queryGrowthAudience(ctx, qr, growthAudienceDeviceCaseExpression(), growthAudienceDeviceLabel, "devices")
}

func (r *growthRepository) GetAudienceOS(ctx context.Context, qr service.GrowthQueryRange) ([]service.GrowthAudienceItem, error) {
	return r.queryGrowthAudience(ctx, qr, growthAudienceOSCaseExpression(), growthAudienceOSLabel, "os")
}

func (r *growthRepository) GetAudienceBrowsers(ctx context.Context, qr service.GrowthQueryRange) ([]service.GrowthAudienceItem, error) {
	return r.queryGrowthAudience(ctx, qr, growthAudienceBrowserCaseExpression(), growthAudienceBrowserLabel, "browsers")
}

func (r *growthRepository) GetAudienceClients(ctx context.Context, qr service.GrowthQueryRange) ([]service.GrowthAudienceItem, error) {
	return r.queryGrowthAudience(ctx, qr, growthAudienceClientCaseExpression(), growthAudienceClientLabel, "clients")
}

func (r *growthRepository) queryGrowthAudience(ctx context.Context, qr service.GrowthQueryRange, keyExpression string, label func(string) string, metricName string) ([]service.GrowthAudienceItem, error) {
	if r == nil || r.db == nil {
		return []service.GrowthAudienceItem{}, nil
	}

	query := fmt.Sprintf(`
WITH active_users AS (
  SELECT COUNT(DISTINCT ul.user_id)::bigint AS total
  FROM usage_logs ul
  JOIN users u ON u.id = ul.user_id AND u.deleted_at IS NULL
  WHERE ul.created_at >= $1 AND ul.created_at < $2
),
categorized AS (
  SELECT
    %s AS audience_key,
    ul.user_id
  FROM usage_logs ul
  JOIN users u ON u.id = ul.user_id AND u.deleted_at IS NULL
  WHERE ul.created_at >= $1 AND ul.created_at < $2
)
SELECT
  c.audience_key,
  COUNT(DISTINCT c.user_id)::bigint AS users,
  COUNT(*)::bigint AS requests,
  (SELECT total FROM active_users)::bigint AS active_users
FROM categorized c
GROUP BY c.audience_key
ORDER BY users DESC, requests DESC, c.audience_key ASC
LIMIT 20`, keyExpression)

	rows, err := r.db.QueryContext(ctx, query, qr.Start, qr.End)
	if err != nil {
		return nil, fmt.Errorf("query growth audience %s: %w", metricName, err)
	}
	defer rows.Close()

	out := []service.GrowthAudienceItem{}
	for rows.Next() {
		var item service.GrowthAudienceItem
		var activeUsers int64
		if err := rows.Scan(&item.Key, &item.Users, &item.Requests, &activeUsers); err != nil {
			return nil, fmt.Errorf("scan growth audience %s: %w", metricName, err)
		}
		item.Label = label(item.Key)
		item.UserRatio = growthRate(item.Users, activeUsers)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate growth audience %s: %w", metricName, err)
	}
	return out, nil
}

func growthBucketExpression(granularity service.GrowthGranularity, column string) string {
	switch granularity {
	case service.GrowthGranularityWeek:
		return fmt.Sprintf("to_char(date_trunc('week', %s AT TIME ZONE 'Asia/Shanghai'), 'IYYY-IW')", column)
	case service.GrowthGranularityMonth:
		return fmt.Sprintf("to_char(date_trunc('month', %s AT TIME ZONE 'Asia/Shanghai'), 'YYYY-MM')", column)
	default:
		return fmt.Sprintf("to_char(%s AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD')", column)
	}
}

func growthPaidStatusPredicate(alias string) string {
	prefix := ""
	if trimmed := strings.TrimSpace(alias); trimmed != "" {
		prefix = trimmed + "."
	}
	return fmt.Sprintf(
		"%sstatus IN ('%s','%s','%s')",
		prefix,
		payment.OrderStatusPaid,
		payment.OrderStatusRecharging,
		payment.OrderStatusCompleted,
	)
}

func growthFeatureCaseExpression() string {
	endpointExpr := "LOWER(COALESCE(ul.inbound_endpoint, '') || ' ' || COALESCE(ul.upstream_endpoint, ''))"
	modelExpr := "LOWER(COALESCE(ul.model, '') || ' ' || COALESCE(ul.requested_model, '') || ' ' || COALESCE(ul.upstream_model, ''))"
	billingModeExpr := "LOWER(COALESCE(ul.billing_mode, ''))"
	return fmt.Sprintf(`CASE
  WHEN COALESCE(ul.image_count, 0) > 0 OR %s = 'image' OR %s LIKE '%%image%%' THEN 'image'
  WHEN %s LIKE '%%pdf%%' OR %s LIKE '%%file%%' OR %s LIKE '%%document%%' OR %s LIKE '%%ocr%%' OR %s LIKE '%%extract%%' THEN 'file'
  WHEN %s LIKE '%%translate%%' OR %s LIKE '%%translate%%' THEN 'translate'
  WHEN %s LIKE '%%search%%' OR %s LIKE '%%search%%' THEN 'search'
  WHEN %s LIKE '%%chat%%' OR %s LIKE '%%responses%%' OR %s LIKE '%%messages%%' THEN 'chat'
  ELSE 'other'
END`, billingModeExpr, endpointExpr, endpointExpr, endpointExpr, endpointExpr, endpointExpr, endpointExpr, endpointExpr, modelExpr, endpointExpr, modelExpr, endpointExpr, endpointExpr, endpointExpr)
}

func growthFeatureLabel(feature string) string {
	switch feature {
	case "chat":
		return "聊天"
	case "image":
		return "绘图"
	case "file":
		return "文件分析"
	case "translate":
		return "翻译"
	case "search":
		return "联网搜索"
	default:
		return "其他"
	}
}

func growthAudienceUAExpr() string {
	return "LOWER(COALESCE(ul.user_agent, ''))"
}

func growthAudienceEmptyUAExpr() string {
	return "BTRIM(COALESCE(ul.user_agent, '')) = ''"
}

func growthAudienceDeviceCaseExpression() string {
	ua := growthAudienceUAExpr()
	return fmt.Sprintf(`CASE
  WHEN %s THEN 'unknown'
  WHEN %[2]s LIKE '%%codex%%' OR %[2]s LIKE '%%claude-code%%' OR %[2]s LIKE '%%claude code%%' OR %[2]s LIKE '%%claude-cli%%' OR %[2]s LIKE '%%geminicli%%' OR %[2]s LIKE '%%gemini-cli%%' OR %[2]s LIKE '%%antigravity%%' THEN 'cli'
  WHEN %[2]s LIKE '%%curl%%' OR %[2]s LIKE '%%wget%%' OR %[2]s LIKE '%%python-requests%%' OR %[2]s LIKE '%%go-http-client%%' OR %[2]s LIKE '%%httpie%%' OR %[2]s LIKE '%%postman%%' OR %[2]s LIKE '%%insomnia%%' OR %[2]s LIKE '%%okhttp%%' OR %[2]s LIKE '%%node-fetch%%' OR %[2]s LIKE '%%axios%%' OR %[2]s LIKE '%%openai%%' THEN 'api'
  WHEN %[2]s LIKE '%%ipad%%' OR %[2]s LIKE '%%tablet%%' THEN 'tablet'
  WHEN %[2]s LIKE '%%mobile%%' OR %[2]s LIKE '%%iphone%%' OR %[2]s LIKE '%%android%%' THEN 'mobile'
  WHEN %[2]s LIKE '%%windows%%' OR %[2]s LIKE '%%macintosh%%' OR %[2]s LIKE '%%mac os x%%' OR %[2]s LIKE '%%x11%%' OR %[2]s LIKE '%%linux%%' THEN 'desktop'
  ELSE 'unknown'
END`, growthAudienceEmptyUAExpr(), ua)
}

func growthAudienceOSCaseExpression() string {
	ua := growthAudienceUAExpr()
	return fmt.Sprintf(`CASE
  WHEN %s THEN 'unknown'
  WHEN %[2]s LIKE '%%iphone%%' OR %[2]s LIKE '%%ipad%%' OR %[2]s LIKE '%%ipod%%' THEN 'ios'
  WHEN %[2]s LIKE '%%android%%' THEN 'android'
  WHEN %[2]s LIKE '%%windows%%' THEN 'windows'
  WHEN %[2]s LIKE '%%macintosh%%' OR %[2]s LIKE '%%mac os x%%' OR %[2]s LIKE '%%darwin%%' THEN 'macos'
  WHEN %[2]s LIKE '%%linux%%' OR %[2]s LIKE '%%x11%%' THEN 'linux'
  ELSE 'unknown'
END`, growthAudienceEmptyUAExpr(), ua)
}

func growthAudienceBrowserCaseExpression() string {
	ua := growthAudienceUAExpr()
	return fmt.Sprintf(`CASE
  WHEN %s THEN 'unknown'
  WHEN %[2]s LIKE '%%edg/%%' OR %[2]s LIKE '%%edge/%%' THEN 'edge'
  WHEN %[2]s LIKE '%%firefox/%%' OR %[2]s LIKE '%%fxios/%%' THEN 'firefox'
  WHEN %[2]s LIKE '%%chrome/%%' OR %[2]s LIKE '%%chromium/%%' OR %[2]s LIKE '%%crios/%%' THEN 'chrome'
  WHEN %[2]s LIKE '%%safari/%%' THEN 'safari'
  ELSE 'unknown'
END`, growthAudienceEmptyUAExpr(), ua)
}

func growthAudienceClientCaseExpression() string {
	ua := growthAudienceUAExpr()
	return fmt.Sprintf(`CASE
  WHEN %s THEN 'unknown'
  WHEN %[2]s LIKE '%%codex%%' THEN 'codex_cli'
  WHEN %[2]s LIKE '%%claude-code%%' OR %[2]s LIKE '%%claude code%%' OR %[2]s LIKE '%%claude-cli%%' THEN 'claude_code'
  WHEN %[2]s LIKE '%%geminicli%%' OR %[2]s LIKE '%%gemini-cli%%' OR %[2]s LIKE '%%antigravity%%' THEN 'gemini_cli'
  WHEN %[2]s LIKE '%%curl%%' THEN 'curl'
  WHEN %[2]s LIKE '%%mozilla/%%' OR %[2]s LIKE '%%chrome/%%' OR %[2]s LIKE '%%safari/%%' OR %[2]s LIKE '%%firefox/%%' OR %[2]s LIKE '%%edg/%%' THEN 'browser'
  WHEN %[2]s LIKE '%%openai%%' OR %[2]s LIKE '%%anthropic%%' OR %[2]s LIKE '%%python-requests%%' OR %[2]s LIKE '%%go-http-client%%' OR %[2]s LIKE '%%node-fetch%%' OR %[2]s LIKE '%%axios%%' OR %[2]s LIKE '%%okhttp%%' OR %[2]s LIKE '%%postman%%' OR %[2]s LIKE '%%insomnia%%' OR %[2]s LIKE '%%httpie%%' OR %[2]s LIKE '%%wget%%' THEN 'sdk'
  ELSE 'unknown'
END`, growthAudienceEmptyUAExpr(), ua)
}

func growthAudienceDeviceLabel(key string) string {
	switch key {
	case "desktop":
		return "Desktop"
	case "mobile":
		return "Mobile"
	case "tablet":
		return "Tablet"
	case "cli":
		return "CLI"
	case "api":
		return "API"
	default:
		return "Unknown"
	}
}

func growthAudienceOSLabel(key string) string {
	switch key {
	case "windows":
		return "Windows"
	case "macos":
		return "macOS"
	case "linux":
		return "Linux"
	case "ios":
		return "iOS"
	case "android":
		return "Android"
	default:
		return "Unknown"
	}
}

func growthAudienceBrowserLabel(key string) string {
	switch key {
	case "chrome":
		return "Chrome"
	case "safari":
		return "Safari"
	case "edge":
		return "Edge"
	case "firefox":
		return "Firefox"
	default:
		return "Unknown"
	}
}

func growthAudienceClientLabel(key string) string {
	switch key {
	case "browser":
		return "Browser"
	case "codex_cli":
		return "Codex CLI"
	case "claude_code":
		return "Claude Code"
	case "gemini_cli":
		return "Gemini CLI"
	case "sdk":
		return "SDK"
	case "curl":
		return "curl"
	default:
		return "Unknown"
	}
}

func growthPlanCategory(orderType string, hasPlanID bool, validityDays int, validityUnit string) string {
	if !hasPlanID || orderType == payment.OrderTypeBalance {
		return "credits"
	}
	unit := strings.ToLower(strings.TrimSpace(validityUnit))
	switch {
	case unit == "year" || validityDays >= 365:
		return "yearly"
	case unit == "month" && validityDays >= 3 || validityDays >= 80 && validityDays <= 100:
		return "quarterly"
	case unit == "month" || validityDays >= 28 && validityDays <= 31:
		return "monthly"
	default:
		return "other"
	}
}

func growthShanghaiDateDiffDays(start, end time.Time) int {
	loc := growthLocation()
	startLocal := start.In(loc)
	endLocal := end.In(loc)
	startDay := time.Date(startLocal.Year(), startLocal.Month(), startLocal.Day(), 0, 0, 0, 0, loc)
	endDay := time.Date(endLocal.Year(), endLocal.Month(), endLocal.Day(), 0, 0, 0, 0, loc)
	days := int(endDay.Sub(startDay).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

func growthLocation() *time.Location {
	loc, err := time.LoadLocation(service.GrowthLocationName)
	if err != nil {
		return time.FixedZone(service.GrowthLocationName, 8*60*60)
	}
	return loc
}

func growthRate(numerator, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	v := float64(numerator) / float64(denominator)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*10000) / 10000
}

func boolRate(ok bool) float64 {
	if ok {
		return 1
	}
	return 0
}
