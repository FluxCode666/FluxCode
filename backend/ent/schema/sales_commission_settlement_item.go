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

type SalesCommissionSettlementItem struct {
	ent.Schema
}

func (SalesCommissionSettlementItem) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "sales_commission_settlement_items"}}
}

func (SalesCommissionSettlementItem) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("settlement_id"),
		field.Int64("commission_record_id"),
		field.Float("amount_cny").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Validate(validatePositiveSalesCommissionAmount("amount_cny")),
		field.Time("created_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Immutable(),
	}
}

func (SalesCommissionSettlementItem) Indexes() []ent.Index {
	return []ent.Index{index.Fields("commission_record_id")}
}
