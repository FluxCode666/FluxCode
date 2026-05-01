package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PromotionPlanRule holds the schema definition for per-plan subscription discount rules.
//
// 订阅活动按 plan 维度独立配置的折扣规则。一个 Promotion (subscription 类型) 可挂多个 plan，
// 每个 plan 可独立选择折扣模式（rate 按比例 / amount 按减额）、参数与最低价保护。
type PromotionPlanRule struct {
	ent.Schema
}

func (PromotionPlanRule) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "promotion_plan_rules"},
	}
}

func (PromotionPlanRule) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("promotion_id").
			Comment("所属活动 ID"),
		field.Int64("plan_id").
			Comment("订阅计划 ID（不加 ent edge 以兼容 plan 硬删除策略）"),
		field.String("discount_mode").
			MaxLen(20).
			Comment("折扣模式: rate | amount"),
		field.Float("discount_rate").
			SchemaType(map[string]string{dialect.Postgres: "decimal(10,4)"}).
			Optional().
			Nillable().
			Comment("rate 模式：折扣倍率，0<rate<=1（如 0.8 = 八折）"),
		field.Float("discount_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Optional().
			Nillable().
			Comment("amount 模式：固定减额，>=0"),
		field.Float("min_price_floor").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Default(0.01).
			Comment("优惠后价格最低保护"),
		field.Int("max_uses_per_user").
			Default(0).
			Comment("该 plan 每用户限次，0 = 跟随活动级"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (PromotionPlanRule) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("promotion", Promotion.Type).
			Ref("plan_rules").
			Field("promotion_id").
			Required().
			Unique(),
	}
}

func (PromotionPlanRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("plan_id"),
		// 同一活动下每个 plan 唯一
		index.Fields("promotion_id", "plan_id").Unique(),
	}
}
