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

// Promotion holds the schema definition for the Promotion entity.
//
// 充值/订阅促销活动主表，与 PromotionPlanRule、PromotionUsage 配套使用。
//
// 删除策略：硬删除
// 活动通过 status 字段管理启停；删除前需检查 PromotionUsage 是否仍有引用。
type Promotion struct {
	ent.Schema
}

func (Promotion) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "promotions"},
	}
}

func (Promotion) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(100).
			NotEmpty().
			Comment("活动名称"),
		field.String("description").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default("").
			Comment("活动描述"),
		field.String("promotion_type").
			MaxLen(20).
			Comment("活动类型: recharge | subscription"),
		field.String("discount_mode").
			MaxLen(20).
			Default("").
			Comment("充值活动模式: reduce_pay | bonus_credit；订阅活动留空（由子规则定义）"),
		field.Float("recharge_rate").
			SchemaType(map[string]string{dialect.Postgres: "decimal(10,4)"}).
			Optional().
			Nillable().
			Comment("充值 reduce_pay 模式的实付倍率，0<rate<=1"),
		field.Float("recharge_bonus_rate").
			SchemaType(map[string]string{dialect.Postgres: "decimal(10,4)"}).
			Optional().
			Nillable().
			Comment("充值 bonus_credit 模式的到账倍率，>=1"),
		field.Int("max_uses_per_user").
			Default(0).
			Comment("每用户限次，0 = 不限"),
		field.Time("starts_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("活动开始时间，NULL 表示不限"),
		field.Time("ends_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("活动结束时间，NULL 表示不限"),
		field.String("status").
			MaxLen(20).
			Default("active").
			Comment("状态: active | disabled"),
		field.Int("priority").
			Default(0).
			Comment("辅助排序，多活动并存时仍按对用户最优挑选"),
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

func (Promotion) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("plan_rules", PromotionPlanRule.Type),
		edge.To("usages", PromotionUsage.Type),
	}
}

func (Promotion) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("promotion_type", "status"),
		index.Fields("starts_at", "ends_at"),
	}
}
