package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type SalesCommissionTier struct {
	ent.Schema
}

func (SalesCommissionTier) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "sales_commission_tiers"}}
}

func (SalesCommissionTier) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("sales_user_id"),
		field.Float("month_sales_from_cny").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}),
		field.Float("month_sales_to_cny").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Optional().
			Nillable(),
		field.Float("commission_rate").
			SchemaType(map[string]string{dialect.Postgres: "decimal(8,4)"}).
			Validate(validateSalesCommissionRate),
		field.Int("sort_order"),
		field.Time("created_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SalesCommissionTier) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("sales_user_id", "sort_order", "id").
			StorageKey("idx_sales_commission_tiers_sales_user"),
	}
}
