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

// PromotionUsage holds the schema definition for promotion usage records.
//
// 用户使用活动的记录：履约成功后插入，用于限次校验、报表与对账。
// user_id / order_id / plan_id 不与外键 edge 绑定，便于跨表硬删除时不级联。
type PromotionUsage struct {
	ent.Schema
}

func (PromotionUsage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "promotion_usages"},
	}
}

func (PromotionUsage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("promotion_id").
			Comment("活动 ID"),
		field.Int64("plan_id").
			Optional().
			Nillable().
			Comment("订阅活动记录 plan；充值活动为 NULL"),
		field.Int64("user_id").
			Comment("使用用户 ID"),
		field.Int64("order_id").
			Comment("payment_orders.id"),
		field.Float("discount_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Default(0).
			Comment("减少金额（订阅按减额/折扣后差值；充值 reduce_pay 节省）"),
		field.Float("bonus_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Default(0).
			Comment("充值 bonus_credit 模式赠送金额"),
		field.Time("used_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("使用时间"),
	}
}

func (PromotionUsage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("promotion", Promotion.Type).
			Ref("usages").
			Field("promotion_id").
			Required().
			Unique(),
	}
}

func (PromotionUsage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "promotion_id", "plan_id"),
		index.Fields("order_id"),
	}
}
