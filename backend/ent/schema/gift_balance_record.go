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

// GiftBalanceRecord holds the schema definition for the GiftBalanceRecord entity.
type GiftBalanceRecord struct {
	ent.Schema
}

func (GiftBalanceRecord) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "gift_balance_records"},
	}
}

func (GiftBalanceRecord) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Float("amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("remaining").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.String("source").
			MaxLen(50).
			Default(""),
		field.Int64("source_ref_id").
			Optional().
			Nillable(),
		field.String("note").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.Time("expires_at").
			Optional().
			Nillable(),
		field.Time("created_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (GiftBalanceRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at"),
		index.Fields("expires_at"),
		index.Fields("source", "source_ref_id"),
	}
}
