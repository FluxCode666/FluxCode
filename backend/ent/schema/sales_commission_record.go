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

type SalesCommissionRecord struct {
	ent.Schema
}

func (SalesCommissionRecord) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "sales_commission_records"}}
}

func (SalesCommissionRecord) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("sales_user_id"),
		field.Int64("referee_user_id"),
		field.Int64("referral_id"),
		field.Int64("payment_order_id"),
		field.Float("order_pay_amount_cny").SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}),
		field.Float("order_credited_amount").SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Float("commission_rate").SchemaType(map[string]string{dialect.Postgres: "decimal(8,4)"}),
		field.Float("commission_total_cny").SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}),
		field.Float("credited_used_amount").SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).Default(0),
		field.Float("unlocked_cny").SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).Default(0),
		field.Float("settled_cny").SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).Default(0),
		field.String("status").MaxLen(32).Default("frozen"),
		field.String("note").SchemaType(map[string]string{dialect.Postgres: "text"}).Default(""),
		field.Time("created_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SalesCommissionRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("payment_order_id").Unique(),
		index.Fields("sales_user_id", "created_at"),
		index.Fields("referee_user_id", "id"),
		index.Fields("status"),
	}
}
