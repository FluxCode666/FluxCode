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

type SalesCommissionMonthlySnapshot struct {
	ent.Schema
}

func (SalesCommissionMonthlySnapshot) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "sales_commission_monthly_snapshots"}}
}

func (SalesCommissionMonthlySnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("sales_user_id"),
		field.Time("commission_month").
			SchemaType(map[string]string{dialect.Postgres: "date"}),
		field.String("timezone").
			MaxLen(64).
			Default("Asia/Shanghai"),
		field.String("commission_mode").
			MaxLen(16),
		field.Float("fixed_commission_rate").
			SchemaType(map[string]string{dialect.Postgres: "decimal(8,4)"}).
			Default(0),
		field.Float("min_monthly_sales_cny").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Default(0),
		field.JSON("tiers_json", []map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Default([]map[string]any{}),
		field.Time("created_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SalesCommissionMonthlySnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("sales_user_id", "commission_month").
			Unique().
			StorageKey("uq_sales_commission_monthly_snapshots"),
	}
}
