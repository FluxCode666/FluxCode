package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type pricingPlanRepository struct {
	db *sql.DB
}

func NewPricingPlanRepository(db *sql.DB) service.PricingPlanRepository {
	return &pricingPlanRepository{db: db}
}

func (r *pricingPlanRepository) ListGroupsWithPlans(ctx context.Context, onlyActive bool) ([]service.PricingPlanGroup, error) {
	query := `
SELECT
  id,
  name,
  description,
  sort_order,
  status,
  created_at,
  updated_at
FROM pricing_plan_groups
`

	var args []any
	if onlyActive {
		query += "WHERE status = $1\n"
		args = append(args, service.StatusActive)
	}
	query += "ORDER BY sort_order ASC, id ASC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	groups := make([]service.PricingPlanGroup, 0)
	groupIDs := make([]int64, 0)

	for rows.Next() {
		var g service.PricingPlanGroup
		var desc sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &desc, &g.SortOrder, &g.Status, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		if desc.Valid {
			d := desc.String
			g.Description = &d
		}
		groups = append(groups, g)
		groupIDs = append(groupIDs, g.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return groups, nil
	}

	plansByGroupID, err := r.loadPlansByGroupIDs(ctx, groupIDs, onlyActive)
	if err != nil {
		return nil, err
	}

	for i := range groups {
		groups[i].Plans = plansByGroupID[groups[i].ID]
	}

	return groups, nil
}

func (r *pricingPlanRepository) loadPlansByGroupIDs(ctx context.Context, groupIDs []int64, onlyActive bool) (map[int64][]service.PricingPlan, error) {
	query := `
SELECT
  id,
  group_id,
  name,
  description,
  icon_url,
  badge_text,
  tagline,
  price_amount,
  price_currency,
  price_period,
  price_text,
  features,
  contact_methods,
  is_featured,
  sort_order,
  status,
  created_at,
  updated_at
FROM pricing_plans
WHERE group_id = ANY($1)
`
	args := []any{pq.Array(groupIDs)}
	if onlyActive {
		query += "  AND status = $2\n"
		args = append(args, service.StatusActive)
	}
	query += "ORDER BY sort_order ASC, id ASC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int64][]service.PricingPlan, len(groupIDs))

	for rows.Next() {
		var p service.PricingPlan
		var desc sql.NullString
		var iconURL sql.NullString
		var badgeText sql.NullString
		var tagline sql.NullString
		var priceAmount sql.NullString
		var priceText sql.NullString
		var featuresRaw []byte
		var contactMethodsRaw []byte

		if err := rows.Scan(
			&p.ID,
			&p.GroupID,
			&p.Name,
			&desc,
			&iconURL,
			&badgeText,
			&tagline,
			&priceAmount,
			&p.PriceCurrency,
			&p.PricePeriod,
			&priceText,
			&featuresRaw,
			&contactMethodsRaw,
			&p.IsFeatured,
			&p.SortOrder,
			&p.Status,
			&p.CreatedAt,
			&p.UpdatedAt,
		); err != nil {
			return nil, err
		}

		if desc.Valid {
			d := desc.String
			p.Description = &d
		}
		if iconURL.Valid {
			v := iconURL.String
			p.IconURL = &v
		}
		if badgeText.Valid {
			v := badgeText.String
			p.BadgeText = &v
		}
		if tagline.Valid {
			v := tagline.String
			p.Tagline = &v
		}

		if priceAmount.Valid {
			if v, err := strconv.ParseFloat(priceAmount.String, 64); err == nil {
				p.PriceAmount = &v
			}
		}

		if priceText.Valid {
			t := priceText.String
			p.PriceText = &t
		}

		if len(featuresRaw) > 0 {
			var features []string
			if err := json.Unmarshal(featuresRaw, &features); err == nil {
				p.Features = features
			}
		}

		if len(contactMethodsRaw) > 0 {
			var methods []service.PricingPlanContactMethod
			if err := json.Unmarshal(contactMethodsRaw, &methods); err == nil {
				p.ContactMethods = methods
			}
		}

		out[p.GroupID] = append(out[p.GroupID], p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *pricingPlanRepository) GetGroupByID(ctx context.Context, id int64) (*service.PricingPlanGroup, error) {
	var g service.PricingPlanGroup
	var desc sql.NullString

	err := r.db.QueryRowContext(ctx, `
SELECT
  id,
  name,
  description,
  sort_order,
  status,
  created_at,
  updated_at
FROM pricing_plan_groups
WHERE id = $1
`, id).Scan(&g.ID, &g.Name, &desc, &g.SortOrder, &g.Status, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrPricingPlanGroupNotFound, nil)
	}
	if desc.Valid {
		d := desc.String
		g.Description = &d
	}
	return &g, nil
}

func (r *pricingPlanRepository) CreateGroup(ctx context.Context, group *service.PricingPlanGroup) error {
	var desc any
	if group.Description != nil {
		desc = *group.Description
	}

	var createdAt time.Time
	var updatedAt time.Time
	err := r.db.QueryRowContext(ctx, `
INSERT INTO pricing_plan_groups (name, description, sort_order, status)
VALUES ($1, $2, $3, $4)
RETURNING id, created_at, updated_at
`, group.Name, desc, group.SortOrder, group.Status).Scan(&group.ID, &createdAt, &updatedAt)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrPricingPlanGroupExists)
	}
	group.CreatedAt = createdAt
	group.UpdatedAt = updatedAt
	return nil
}

func (r *pricingPlanRepository) UpdateGroup(ctx context.Context, group *service.PricingPlanGroup) error {
	var desc any
	if group.Description != nil {
		desc = *group.Description
	}

	var updatedAt time.Time
	err := r.db.QueryRowContext(ctx, `
UPDATE pricing_plan_groups
SET
  name = $2,
  description = $3,
  sort_order = $4,
  status = $5,
  updated_at = NOW()
WHERE id = $1
RETURNING updated_at
`, group.ID, group.Name, desc, group.SortOrder, group.Status).Scan(&updatedAt)
	if err != nil {
		return translatePersistenceError(err, service.ErrPricingPlanGroupNotFound, service.ErrPricingPlanGroupExists)
	}
	group.UpdatedAt = updatedAt
	return nil
}

func (r *pricingPlanRepository) DeleteGroup(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM pricing_plan_groups WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return service.ErrPricingPlanGroupNotFound
	}
	return nil
}

func (r *pricingPlanRepository) GetPlanByID(ctx context.Context, id int64) (*service.PricingPlan, error) {
	var p service.PricingPlan
	var desc sql.NullString
	var iconURL sql.NullString
	var badgeText sql.NullString
	var tagline sql.NullString
	var priceAmount sql.NullString
	var priceText sql.NullString
	var featuresRaw []byte
	var contactMethodsRaw []byte

	err := r.db.QueryRowContext(ctx, `
SELECT
  id,
  group_id,
  name,
  description,
  icon_url,
  badge_text,
  tagline,
  price_amount,
  price_currency,
  price_period,
  price_text,
  features,
  contact_methods,
  is_featured,
  sort_order,
  status,
  created_at,
  updated_at
FROM pricing_plans
WHERE id = $1
`, id).Scan(
		&p.ID,
		&p.GroupID,
		&p.Name,
		&desc,
		&iconURL,
		&badgeText,
		&tagline,
		&priceAmount,
		&p.PriceCurrency,
		&p.PricePeriod,
		&priceText,
		&featuresRaw,
		&contactMethodsRaw,
		&p.IsFeatured,
		&p.SortOrder,
		&p.Status,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrPricingPlanNotFound, nil)
	}

	if desc.Valid {
		d := desc.String
		p.Description = &d
	}
	if iconURL.Valid {
		v := iconURL.String
		p.IconURL = &v
	}
	if badgeText.Valid {
		v := badgeText.String
		p.BadgeText = &v
	}
	if tagline.Valid {
		v := tagline.String
		p.Tagline = &v
	}
	if priceAmount.Valid {
		if v, err := strconv.ParseFloat(priceAmount.String, 64); err == nil {
			p.PriceAmount = &v
		}
	}
	if priceText.Valid {
		t := priceText.String
		p.PriceText = &t
	}
	if len(featuresRaw) > 0 {
		var features []string
		if err := json.Unmarshal(featuresRaw, &features); err == nil {
			p.Features = features
		}
	}
	if len(contactMethodsRaw) > 0 {
		var methods []service.PricingPlanContactMethod
		if err := json.Unmarshal(contactMethodsRaw, &methods); err == nil {
			p.ContactMethods = methods
		}
	}

	return &p, nil
}

func (r *pricingPlanRepository) CreatePlan(ctx context.Context, plan *service.PricingPlan) error {
	var desc any
	if plan.Description != nil {
		desc = *plan.Description
	}
	var iconURL any
	if plan.IconURL != nil {
		iconURL = *plan.IconURL
	}
	var badgeText any
	if plan.BadgeText != nil {
		badgeText = *plan.BadgeText
	}
	var tagline any
	if plan.Tagline != nil {
		tagline = *plan.Tagline
	}
	var priceAmount any
	if plan.PriceAmount != nil {
		priceAmount = *plan.PriceAmount
	}
	var priceText any
	if plan.PriceText != nil {
		priceText = *plan.PriceText
	}

	featuresJSON, err := json.Marshal(plan.Features)
	if err != nil {
		return err
	}
	if string(featuresJSON) == "null" {
		featuresJSON = []byte("[]")
	}

	contactMethodsJSON, err := json.Marshal(plan.ContactMethods)
	if err != nil {
		return err
	}
	if string(contactMethodsJSON) == "null" {
		contactMethodsJSON = []byte("[]")
	}

	var createdAt time.Time
	var updatedAt time.Time
	err = r.db.QueryRowContext(ctx, `
INSERT INTO pricing_plans (
  group_id,
  name,
  description,
  icon_url,
  badge_text,
  tagline,
  price_amount,
  price_currency,
  price_period,
  price_text,
  features,
  contact_methods,
  is_featured,
  sort_order,
  status
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
RETURNING id, created_at, updated_at
`, plan.GroupID, plan.Name, desc, iconURL, badgeText, tagline, priceAmount, plan.PriceCurrency, plan.PricePeriod, priceText, featuresJSON, contactMethodsJSON, plan.IsFeatured, plan.SortOrder, plan.Status).
		Scan(&plan.ID, &createdAt, &updatedAt)
	if err != nil {
		return err
	}
	plan.CreatedAt = createdAt
	plan.UpdatedAt = updatedAt
	return nil
}

func (r *pricingPlanRepository) UpdatePlan(ctx context.Context, plan *service.PricingPlan) error {
	var desc any
	if plan.Description != nil {
		desc = *plan.Description
	}
	var iconURL any
	if plan.IconURL != nil {
		iconURL = *plan.IconURL
	}
	var badgeText any
	if plan.BadgeText != nil {
		badgeText = *plan.BadgeText
	}
	var tagline any
	if plan.Tagline != nil {
		tagline = *plan.Tagline
	}
	var priceAmount any
	if plan.PriceAmount != nil {
		priceAmount = *plan.PriceAmount
	}
	var priceText any
	if plan.PriceText != nil {
		priceText = *plan.PriceText
	}

	featuresJSON, err := json.Marshal(plan.Features)
	if err != nil {
		return err
	}
	if string(featuresJSON) == "null" {
		featuresJSON = []byte("[]")
	}

	contactMethodsJSON, err := json.Marshal(plan.ContactMethods)
	if err != nil {
		return err
	}
	if string(contactMethodsJSON) == "null" {
		contactMethodsJSON = []byte("[]")
	}

	var updatedAt time.Time
	err = r.db.QueryRowContext(ctx, `
UPDATE pricing_plans
SET
  group_id = $2,
  name = $3,
  description = $4,
  icon_url = $5,
  badge_text = $6,
  tagline = $7,
  price_amount = $8,
  price_currency = $9,
  price_period = $10,
  price_text = $11,
  features = $12,
  contact_methods = $13,
  is_featured = $14,
  sort_order = $15,
  status = $16,
  updated_at = NOW()
WHERE id = $1
RETURNING updated_at
`, plan.ID, plan.GroupID, plan.Name, desc, iconURL, badgeText, tagline, priceAmount, plan.PriceCurrency, plan.PricePeriod, priceText, featuresJSON, contactMethodsJSON, plan.IsFeatured, plan.SortOrder, plan.Status).
		Scan(&updatedAt)
	if err != nil {
		return translatePersistenceError(err, service.ErrPricingPlanNotFound, nil)
	}
	plan.UpdatedAt = updatedAt
	return nil
}

func (r *pricingPlanRepository) DeletePlan(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM pricing_plans WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return service.ErrPricingPlanNotFound
	}
	return nil
}
