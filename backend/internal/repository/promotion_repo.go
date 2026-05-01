package repository

import (
	"context"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/promotion"
	"github.com/Wei-Shaw/sub2api/ent/promotionplanrule"
	"github.com/Wei-Shaw/sub2api/ent/promotionusage"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type promotionRepository struct {
	client *dbent.Client
}

// NewPromotionRepository creates a Postgres-backed PromotionRepository.
func NewPromotionRepository(client *dbent.Client) service.PromotionRepository {
	return &promotionRepository{client: client}
}

// --- helpers ---

func promotionEntityToService(m *dbent.Promotion) *service.Promotion {
	if m == nil {
		return nil
	}
	out := &service.Promotion{
		ID:             m.ID,
		Name:           m.Name,
		Description:    m.Description,
		PromotionType:  m.PromotionType,
		DiscountMode:   m.DiscountMode,
		MaxUsesPerUser: m.MaxUsesPerUser,
		Status:         m.Status,
		Priority:       m.Priority,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
	if m.RechargeRate != nil {
		v := *m.RechargeRate
		out.RechargeRate = &v
	}
	if m.RechargeBonusRate != nil {
		v := *m.RechargeBonusRate
		out.RechargeBonusRate = &v
	}
	if m.StartsAt != nil {
		v := *m.StartsAt
		out.StartsAt = &v
	}
	if m.EndsAt != nil {
		v := *m.EndsAt
		out.EndsAt = &v
	}
	return out
}

func promotionPlanRuleEntityToService(m *dbent.PromotionPlanRule) *service.PromotionPlanRule {
	if m == nil {
		return nil
	}
	out := &service.PromotionPlanRule{
		ID:             m.ID,
		PromotionID:    m.PromotionID,
		PlanID:         m.PlanID,
		DiscountMode:   m.DiscountMode,
		MinPriceFloor:  m.MinPriceFloor,
		MaxUsesPerUser: m.MaxUsesPerUser,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
	if m.DiscountRate != nil {
		v := *m.DiscountRate
		out.DiscountRate = &v
	}
	if m.DiscountAmount != nil {
		v := *m.DiscountAmount
		out.DiscountAmount = &v
	}
	return out
}

func promotionUsageEntityToService(m *dbent.PromotionUsage) *service.PromotionUsage {
	if m == nil {
		return nil
	}
	out := &service.PromotionUsage{
		ID:             m.ID,
		PromotionID:    m.PromotionID,
		UserID:         m.UserID,
		OrderID:        m.OrderID,
		DiscountAmount: m.DiscountAmount,
		BonusAmount:    m.BonusAmount,
		UsedAt:         m.UsedAt,
	}
	if m.PlanID != nil {
		v := *m.PlanID
		out.PlanID = &v
	}
	return out
}

// --- CRUD ---

func (r *promotionRepository) Create(ctx context.Context, p *service.Promotion) error {
	client := clientFromContext(ctx, r.client)

	tx, err := client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	b := tx.Promotion.Create().
		SetName(p.Name).
		SetDescription(p.Description).
		SetPromotionType(p.PromotionType).
		SetDiscountMode(p.DiscountMode).
		SetMaxUsesPerUser(p.MaxUsesPerUser).
		SetStatus(p.Status).
		SetPriority(p.Priority)
	if p.RechargeRate != nil {
		b.SetRechargeRate(*p.RechargeRate)
	}
	if p.RechargeBonusRate != nil {
		b.SetRechargeBonusRate(*p.RechargeBonusRate)
	}
	if p.StartsAt != nil {
		b.SetStartsAt(*p.StartsAt)
	}
	if p.EndsAt != nil {
		b.SetEndsAt(*p.EndsAt)
	}
	created, err := b.Save(ctx)
	if err != nil {
		return err
	}
	p.ID = created.ID
	p.CreatedAt = created.CreatedAt
	p.UpdatedAt = created.UpdatedAt

	if len(p.PlanRules) > 0 {
		if err := createPlanRulesInTx(ctx, tx, created.ID, p.PlanRules); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *promotionRepository) GetByID(ctx context.Context, id int64) (*service.Promotion, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.Promotion.Query().Where(promotion.IDEQ(id)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrPromotionNotFound
		}
		return nil, err
	}
	out := promotionEntityToService(m)
	rules, err := r.ListPlanRulesByPromotionID(ctx, id)
	if err != nil {
		return nil, err
	}
	out.PlanRules = rules
	return out, nil
}

func (r *promotionRepository) GetByIDForUpdate(ctx context.Context, id int64) (*service.Promotion, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.Promotion.Query().Where(promotion.IDEQ(id)).ForUpdate().Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrPromotionNotFound
		}
		return nil, err
	}
	return promotionEntityToService(m), nil
}

func (r *promotionRepository) Update(ctx context.Context, p *service.Promotion) error {
	client := clientFromContext(ctx, r.client)

	tx, err := client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	u := tx.Promotion.UpdateOneID(p.ID).
		SetName(p.Name).
		SetDescription(p.Description).
		SetPromotionType(p.PromotionType).
		SetDiscountMode(p.DiscountMode).
		SetMaxUsesPerUser(p.MaxUsesPerUser).
		SetStatus(p.Status).
		SetPriority(p.Priority)
	if p.RechargeRate != nil {
		u.SetRechargeRate(*p.RechargeRate)
	} else {
		u.ClearRechargeRate()
	}
	if p.RechargeBonusRate != nil {
		u.SetRechargeBonusRate(*p.RechargeBonusRate)
	} else {
		u.ClearRechargeBonusRate()
	}
	if p.StartsAt != nil {
		u.SetStartsAt(*p.StartsAt)
	} else {
		u.ClearStartsAt()
	}
	if p.EndsAt != nil {
		u.SetEndsAt(*p.EndsAt)
	} else {
		u.ClearEndsAt()
	}
	updated, err := u.Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return service.ErrPromotionNotFound
		}
		return err
	}
	p.UpdatedAt = updated.UpdatedAt

	if p.PlanRules != nil {
		if _, err := tx.PromotionPlanRule.Delete().
			Where(promotionplanrule.PromotionIDEQ(p.ID)).
			Exec(ctx); err != nil {
			return err
		}
		if err := createPlanRulesInTx(ctx, tx, p.ID, p.PlanRules); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *promotionRepository) Delete(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	if err := client.Promotion.DeleteOneID(id).Exec(ctx); err != nil {
		if dbent.IsNotFound(err) {
			return service.ErrPromotionNotFound
		}
		return err
	}
	return nil
}

func (r *promotionRepository) List(ctx context.Context, params pagination.PaginationParams, filter service.PromotionListFilter) ([]service.Promotion, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q := client.Promotion.Query()
	if t := strings.TrimSpace(filter.PromotionType); t != "" {
		q = q.Where(promotion.PromotionTypeEQ(t))
	}
	if s := strings.TrimSpace(filter.Status); s != "" {
		q = q.Where(promotion.StatusEQ(s))
	}
	if k := strings.TrimSpace(filter.Search); k != "" {
		q = q.Where(promotion.NameContainsFold(k))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	rows, err := q.
		Order(dbent.Desc(promotion.FieldPriority), dbent.Desc(promotion.FieldID)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	out := make([]service.Promotion, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		s := promotionEntityToService(row)
		out = append(out, *s)
		ids = append(ids, row.ID)
	}
	if len(ids) > 0 {
		ruleMap, err := r.ListPlanRulesByPromotionIDs(ctx, ids)
		if err != nil {
			return nil, nil, err
		}
		for i := range out {
			out[i].PlanRules = ruleMap[out[i].ID]
		}
	}

	return out, paginationResultFromTotal(int64(total), params), nil
}

// --- Active queries (for resolver) ---

func (r *promotionRepository) ListActiveByType(ctx context.Context, promotionType string) ([]service.Promotion, error) {
	client := clientFromContext(ctx, r.client)
	now := time.Now()

	rows, err := client.Promotion.Query().
		Where(
			promotion.PromotionTypeEQ(promotionType),
			promotion.StatusEQ(domain.PromotionStatusActive),
			promotion.Or(promotion.StartsAtIsNil(), promotion.StartsAtLTE(now)),
			promotion.Or(promotion.EndsAtIsNil(), promotion.EndsAtGT(now)),
		).
		Order(dbent.Desc(promotion.FieldPriority), dbent.Asc(promotion.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.hydrateWithRules(ctx, rows)
}

func (r *promotionRepository) ListActiveByPlanID(ctx context.Context, planID int64) ([]service.Promotion, error) {
	client := clientFromContext(ctx, r.client)

	// 先找出包含此 plan 的所有 promotion_id
	var promoIDs []int64
	if err := client.PromotionPlanRule.Query().
		Where(promotionplanrule.PlanIDEQ(planID)).
		Select(promotionplanrule.FieldPromotionID).
		Scan(ctx, &promoIDs); err != nil {
		return nil, err
	}
	if len(promoIDs) == 0 {
		return nil, nil
	}

	now := time.Now()
	rows, err := client.Promotion.Query().
		Where(
			promotion.IDIn(promoIDs...),
			promotion.PromotionTypeEQ(domain.PromotionTypeSubscription),
			promotion.StatusEQ(domain.PromotionStatusActive),
			promotion.Or(promotion.StartsAtIsNil(), promotion.StartsAtLTE(now)),
			promotion.Or(promotion.EndsAtIsNil(), promotion.EndsAtGT(now)),
		).
		Order(dbent.Desc(promotion.FieldPriority), dbent.Asc(promotion.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.hydrateWithRules(ctx, rows)
}

func (r *promotionRepository) hydrateWithRules(ctx context.Context, rows []*dbent.Promotion) ([]service.Promotion, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	ruleMap, err := r.ListPlanRulesByPromotionIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]service.Promotion, 0, len(rows))
	for _, row := range rows {
		s := promotionEntityToService(row)
		s.PlanRules = ruleMap[row.ID]
		out = append(out, *s)
	}
	return out, nil
}

// --- plan rules ---

func createPlanRulesInTx(ctx context.Context, tx *dbent.Tx, promotionID int64, rules []service.PromotionPlanRule) error {
	if len(rules) == 0 {
		return nil
	}
	bulk := make([]*dbent.PromotionPlanRuleCreate, 0, len(rules))
	for _, rule := range rules {
		c := tx.PromotionPlanRule.Create().
			SetPromotionID(promotionID).
			SetPlanID(rule.PlanID).
			SetDiscountMode(rule.DiscountMode).
			SetMinPriceFloor(rule.MinPriceFloor).
			SetMaxUsesPerUser(rule.MaxUsesPerUser)
		if rule.DiscountRate != nil {
			c.SetDiscountRate(*rule.DiscountRate)
		}
		if rule.DiscountAmount != nil {
			c.SetDiscountAmount(*rule.DiscountAmount)
		}
		bulk = append(bulk, c)
	}
	_, err := tx.PromotionPlanRule.CreateBulk(bulk...).Save(ctx)
	return err
}

func (r *promotionRepository) ReplacePlanRules(ctx context.Context, promotionID int64, rules []service.PromotionPlanRule) error {
	client := clientFromContext(ctx, r.client)
	tx, err := client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.PromotionPlanRule.Delete().Where(promotionplanrule.PromotionIDEQ(promotionID)).Exec(ctx); err != nil {
		return err
	}
	if err := createPlanRulesInTx(ctx, tx, promotionID, rules); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *promotionRepository) ListPlanRulesByPromotionID(ctx context.Context, promotionID int64) ([]service.PromotionPlanRule, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.PromotionPlanRule.Query().
		Where(promotionplanrule.PromotionIDEQ(promotionID)).
		Order(dbent.Asc(promotionplanrule.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.PromotionPlanRule, 0, len(rows))
	for _, row := range rows {
		if s := promotionPlanRuleEntityToService(row); s != nil {
			out = append(out, *s)
		}
	}
	return out, nil
}

func (r *promotionRepository) ListPlanRulesByPromotionIDs(ctx context.Context, promotionIDs []int64) (map[int64][]service.PromotionPlanRule, error) {
	if len(promotionIDs) == 0 {
		return map[int64][]service.PromotionPlanRule{}, nil
	}
	client := clientFromContext(ctx, r.client)
	rows, err := client.PromotionPlanRule.Query().
		Where(promotionplanrule.PromotionIDIn(promotionIDs...)).
		Order(dbent.Asc(promotionplanrule.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int64][]service.PromotionPlanRule, len(promotionIDs))
	for _, row := range rows {
		s := promotionPlanRuleEntityToService(row)
		if s == nil {
			continue
		}
		out[row.PromotionID] = append(out[row.PromotionID], *s)
	}
	return out, nil
}

// --- Usage ---

func (r *promotionRepository) CreateUsage(ctx context.Context, usage *service.PromotionUsage) error {
	client := clientFromContext(ctx, r.client)
	b := client.PromotionUsage.Create().
		SetPromotionID(usage.PromotionID).
		SetUserID(usage.UserID).
		SetOrderID(usage.OrderID).
		SetDiscountAmount(usage.DiscountAmount).
		SetBonusAmount(usage.BonusAmount).
		SetUsedAt(usage.UsedAt)
	if usage.PlanID != nil {
		b.SetPlanID(*usage.PlanID)
	}
	created, err := b.Save(ctx)
	if err != nil {
		return err
	}
	usage.ID = created.ID
	return nil
}

func (r *promotionRepository) CountUsageByUser(ctx context.Context, promotionID int64, planID *int64, userID int64) (int, error) {
	client := clientFromContext(ctx, r.client)
	q := client.PromotionUsage.Query().
		Where(
			promotionusage.PromotionIDEQ(promotionID),
			promotionusage.UserIDEQ(userID),
		)
	if planID != nil {
		q = q.Where(promotionusage.PlanIDEQ(*planID))
	}
	return q.Count(ctx)
}

func (r *promotionRepository) ListUsagesByPromotion(ctx context.Context, promotionID int64, params pagination.PaginationParams) ([]service.PromotionUsage, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q := client.PromotionUsage.Query().Where(promotionusage.PromotionIDEQ(promotionID))

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	rows, err := q.
		Order(dbent.Desc(promotionusage.FieldID)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := make([]service.PromotionUsage, 0, len(rows))
	for _, row := range rows {
		if s := promotionUsageEntityToService(row); s != nil {
			out = append(out, *s)
		}
	}
	return out, paginationResultFromTotal(int64(total), params), nil
}

func (r *promotionRepository) DeleteUsagesByOrderID(ctx context.Context, orderID int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.PromotionUsage.Delete().Where(promotionusage.OrderIDEQ(orderID)).Exec(ctx)
	return err
}

// --- referenced order count ---

func (r *promotionRepository) CountActiveOrdersByPromotion(ctx context.Context, promotionID int64) (int, error) {
	client := clientFromContext(ctx, r.client)
	return client.PaymentOrder.Query().
		Where(
			paymentorder.PromotionIDEQ(promotionID),
			paymentorder.StatusIn("PENDING", "PAID", "RECHARGING"),
		).
		Count(ctx)
}
