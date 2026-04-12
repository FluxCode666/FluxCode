package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptiongrant"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type subscriptionGrantRepository struct {
	client *dbent.Client
}

func NewSubscriptionGrantRepository(client *dbent.Client) service.SubscriptionGrantRepository {
	return &subscriptionGrantRepository{client: client}
}

func (r *subscriptionGrantRepository) Create(ctx context.Context, grant *service.SubscriptionGrant) error {
	if grant == nil {
		return service.ErrSubscriptionGrantNilInput
	}

	client := clientFromContext(ctx, r.client)
	builder := client.SubscriptionGrant.Create().
		SetSubscriptionID(grant.SubscriptionID).
		SetExpiresAt(grant.ExpiresAt)

	if grant.StartsAt.IsZero() {
		builder.SetStartsAt(time.Now())
	} else {
		builder.SetStartsAt(grant.StartsAt)
	}

	created, err := builder.Save(ctx)
	if err == nil {
		applySubscriptionGrantEntityToService(grant, created)
	}
	return err
}

func (r *subscriptionGrantRepository) CountActiveBySubscriptionID(ctx context.Context, subscriptionID int64, at time.Time) (int, error) {
	client := clientFromContext(ctx, r.client)
	return client.SubscriptionGrant.Query().
		Where(
			subscriptiongrant.SubscriptionIDEQ(subscriptionID),
			subscriptiongrant.DeletedAtIsNil(),
			subscriptiongrant.StartsAtLTE(at),
			subscriptiongrant.ExpiresAtGT(at),
		).
		Count(ctx)
}

func (r *subscriptionGrantRepository) ListActiveBySubscriptionID(ctx context.Context, subscriptionID int64, at time.Time) ([]*service.SubscriptionGrant, error) {
	client := clientFromContext(ctx, r.client)
	models, err := client.SubscriptionGrant.Query().
		Where(
			subscriptiongrant.SubscriptionIDEQ(subscriptionID),
			subscriptiongrant.DeletedAtIsNil(),
			subscriptiongrant.StartsAtLTE(at),
			subscriptiongrant.ExpiresAtGT(at),
		).
		Order(
			dbent.Asc(subscriptiongrant.FieldExpiresAt),
			dbent.Asc(subscriptiongrant.FieldID),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]*service.SubscriptionGrant, 0, len(models))
	for i := range models {
		out = append(out, subscriptionGrantEntityToService(models[i]))
	}
	return out, nil
}

func (r *subscriptionGrantRepository) ListUnexpiredBySubscriptionID(ctx context.Context, subscriptionID int64, at time.Time) ([]*service.SubscriptionGrant, error) {
	client := clientFromContext(ctx, r.client)
	models, err := client.SubscriptionGrant.Query().
		Where(
			subscriptiongrant.SubscriptionIDEQ(subscriptionID),
			subscriptiongrant.DeletedAtIsNil(),
			subscriptiongrant.ExpiresAtGT(at),
		).
		Order(
			dbent.Asc(subscriptiongrant.FieldStartsAt),
			dbent.Asc(subscriptiongrant.FieldExpiresAt),
			dbent.Asc(subscriptiongrant.FieldID),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]*service.SubscriptionGrant, 0, len(models))
	for i := range models {
		out = append(out, subscriptionGrantEntityToService(models[i]))
	}
	return out, nil
}

func (r *subscriptionGrantRepository) SumActiveUsageBySubscriptionID(ctx context.Context, subscriptionID int64, at time.Time) (dailyUSD, weeklyUSD, monthlyUSD float64, err error) {
	const querySQL = `
SELECT
	COALESCE(SUM(daily_usage_usd), 0) AS daily_usage_usd,
	COALESCE(SUM(weekly_usage_usd), 0) AS weekly_usage_usd,
	COALESCE(SUM(monthly_usage_usd), 0) AS monthly_usage_usd
FROM subscription_grants
WHERE subscription_id = $1
	AND deleted_at IS NULL
	AND starts_at <= $2
	AND expires_at > $2
`

	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, querySQL, subscriptionID, at)
	if err != nil {
		return 0, 0, 0, err
	}
	defer rows.Close() //nolint:errcheck

	if !rows.Next() {
		return 0, 0, 0, nil
	}
	if err := rows.Scan(&dailyUSD, &weeklyUSD, &monthlyUSD); err != nil {
		return 0, 0, 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, 0, 0, err
	}
	return dailyUSD, weeklyUSD, monthlyUSD, nil
}

func (r *subscriptionGrantRepository) MinActiveExpiresAtBySubscriptionID(ctx context.Context, subscriptionID int64, at time.Time) (*time.Time, error) {
	const querySQL = `
SELECT MIN(expires_at) AS min_expires_at
FROM subscription_grants
WHERE subscription_id = $1
	AND deleted_at IS NULL
	AND starts_at <= $2
	AND expires_at > $2
`

	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, querySQL, subscriptionID, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	if !rows.Next() {
		return nil, nil
	}

	var minExpires sql.NullTime
	if err := rows.Scan(&minExpires); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !minExpires.Valid {
		return nil, nil
	}

	out := minExpires.Time
	return &out, nil
}

func (r *subscriptionGrantRepository) ListActiveForUpdate(ctx context.Context, subscriptionID int64, at time.Time) ([]*service.SubscriptionGrant, error) {
	const querySQL = `
SELECT
	id,
	subscription_id,
	starts_at,
	expires_at,
	daily_usage_usd,
	weekly_usage_usd,
	monthly_usage_usd,
	created_at,
	updated_at
FROM subscription_grants
WHERE subscription_id = $1
	AND deleted_at IS NULL
	AND starts_at <= $2
	AND expires_at > $2
ORDER BY expires_at ASC, id ASC
FOR UPDATE
`

	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, querySQL, subscriptionID, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	out := make([]*service.SubscriptionGrant, 0)
	for rows.Next() {
		var grant service.SubscriptionGrant
		if err := rows.Scan(
			&grant.ID,
			&grant.SubscriptionID,
			&grant.StartsAt,
			&grant.ExpiresAt,
			&grant.DailyUsageUSD,
			&grant.WeeklyUsageUSD,
			&grant.MonthlyUsageUSD,
			&grant.CreatedAt,
			&grant.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, &grant)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *subscriptionGrantRepository) GetTailGrantBySubscriptionID(ctx context.Context, subscriptionID int64) (*service.SubscriptionGrant, error) {
	client := clientFromContext(ctx, r.client)
	model, err := client.SubscriptionGrant.Query().
		Where(
			subscriptiongrant.SubscriptionIDEQ(subscriptionID),
			subscriptiongrant.DeletedAtIsNil(),
		).
		Order(
			dbent.Desc(subscriptiongrant.FieldExpiresAt),
			dbent.Desc(subscriptiongrant.FieldID),
		).
		First(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || dbent.IsNotFound(err) {
			return nil, service.ErrSubscriptionGrantNotFound
		}
		return nil, err
	}
	return subscriptionGrantEntityToService(model), nil
}

func (r *subscriptionGrantRepository) UpdateExpiresAt(ctx context.Context, grantID int64, newExpiresAt time.Time) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.SubscriptionGrant.UpdateOneID(grantID).
		SetExpiresAt(newExpiresAt).
		Save(ctx)
	if errors.Is(err, sql.ErrNoRows) || dbent.IsNotFound(err) {
		return service.ErrSubscriptionGrantNotFound.WithCause(err)
	}
	return err
}

func (r *subscriptionGrantRepository) ResetDailyUsageBySubscriptionID(ctx context.Context, subscriptionID int64) error {
	const updateSQL = `
UPDATE subscription_grants
SET
	daily_usage_usd = 0,
	updated_at = NOW()
WHERE subscription_id = $1
	AND deleted_at IS NULL
`

	client := clientFromContext(ctx, r.client)
	_, err := client.ExecContext(ctx, updateSQL, subscriptionID)
	return err
}

func (r *subscriptionGrantRepository) ResetWeeklyUsageBySubscriptionID(ctx context.Context, subscriptionID int64) error {
	const updateSQL = `
UPDATE subscription_grants
SET
	weekly_usage_usd = 0,
	updated_at = NOW()
WHERE subscription_id = $1
	AND deleted_at IS NULL
`

	client := clientFromContext(ctx, r.client)
	_, err := client.ExecContext(ctx, updateSQL, subscriptionID)
	return err
}

func (r *subscriptionGrantRepository) ResetMonthlyUsageBySubscriptionID(ctx context.Context, subscriptionID int64) error {
	const updateSQL = `
UPDATE subscription_grants
SET
	monthly_usage_usd = 0,
	updated_at = NOW()
WHERE subscription_id = $1
	AND deleted_at IS NULL
`

	client := clientFromContext(ctx, r.client)
	_, err := client.ExecContext(ctx, updateSQL, subscriptionID)
	return err
}

func (r *subscriptionGrantRepository) AllocateUsageToActiveGrants(ctx context.Context, subscriptionID int64, group *service.Group, at time.Time, costUSD float64) error {
	if costUSD <= 0 {
		return nil
	}

	grants, err := r.ListActiveForUpdate(ctx, subscriptionID, at)
	if err != nil {
		return fmt.Errorf("list active grants for update: %w", err)
	}
	if len(grants) == 0 {
		tail, err := r.GetTailGrantBySubscriptionID(ctx, subscriptionID)
		if err != nil {
			return fmt.Errorf("no active grants and tail grant not found: %w", err)
		}
		client := clientFromContext(ctx, r.client)
		_, err = client.SubscriptionGrant.UpdateOneID(tail.ID).
			AddDailyUsageUsd(costUSD).
			AddWeeklyUsageUsd(costUSD).
			AddMonthlyUsageUsd(costUSD).
			Save(ctx)
		return err
	}

	remaining := costUSD
	client := clientFromContext(ctx, r.client)

	for i, grant := range grants {
		if remaining <= 0 {
			break
		}

		cap := math.Inf(1)
		if group != nil {
			if group.HasDailyLimit() {
				cap = math.Min(cap, *group.DailyLimitUSD-grant.DailyUsageUSD)
			}
			if group.HasWeeklyLimit() {
				cap = math.Min(cap, *group.WeeklyLimitUSD-grant.WeeklyUsageUSD)
			}
			if group.HasMonthlyLimit() {
				cap = math.Min(cap, *group.MonthlyLimitUSD-grant.MonthlyUsageUSD)
			}
		}
		if cap < 0 {
			cap = 0
		}

		allocation := remaining
		if !math.IsInf(cap, 1) {
			allocation = math.Min(remaining, cap)
		}
		if allocation <= 0 && i == len(grants)-1 {
			allocation = remaining
		}
		if allocation <= 0 {
			continue
		}

		if _, err := client.SubscriptionGrant.UpdateOneID(grant.ID).
			AddDailyUsageUsd(allocation).
			AddWeeklyUsageUsd(allocation).
			AddMonthlyUsageUsd(allocation).
			Save(ctx); err != nil {
			return err
		}

		remaining -= allocation
	}

	return nil
}

func subscriptionGrantEntityToService(model *dbent.SubscriptionGrant) *service.SubscriptionGrant {
	if model == nil {
		return nil
	}

	return &service.SubscriptionGrant{
		ID:              model.ID,
		SubscriptionID:  model.SubscriptionID,
		StartsAt:        model.StartsAt,
		ExpiresAt:       model.ExpiresAt,
		DailyUsageUSD:   model.DailyUsageUsd,
		WeeklyUsageUSD:  model.WeeklyUsageUsd,
		MonthlyUsageUSD: model.MonthlyUsageUsd,
		CreatedAt:       model.CreatedAt,
		UpdatedAt:       model.UpdatedAt,
	}
}

func applySubscriptionGrantEntityToService(target *service.SubscriptionGrant, model *dbent.SubscriptionGrant) {
	if target == nil || model == nil {
		return
	}

	target.ID = model.ID
	target.SubscriptionID = model.SubscriptionID
	target.StartsAt = model.StartsAt
	target.ExpiresAt = model.ExpiresAt
	target.DailyUsageUSD = model.DailyUsageUsd
	target.WeeklyUsageUSD = model.WeeklyUsageUsd
	target.MonthlyUsageUSD = model.MonthlyUsageUsd
	target.CreatedAt = model.CreatedAt
	target.UpdatedAt = model.UpdatedAt
}
