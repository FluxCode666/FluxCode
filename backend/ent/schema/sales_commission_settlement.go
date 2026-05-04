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

type SalesCommissionSettlement struct {
	ent.Schema
}

func (SalesCommissionSettlement) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "sales_commission_settlements"}}
}

func (SalesCommissionSettlement) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("sales_user_id"),
		field.Float("amount_cny").SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}),
		field.String("note").SchemaType(map[string]string{dialect.Postgres: "text"}).Default(""),
		field.Int64("created_by").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Immutable(),
	}
}

func (SalesCommissionSettlement) Indexes() []ent.Index {
	return []ent.Index{index.Fields("sales_user_id", "created_at")}
}
