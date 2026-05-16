package schema

import (
	"fmt"
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
		field.Int64("payment_order_id").Optional().Nillable(),
		field.Float("order_pay_amount_cny").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Validate(validatePositiveSalesCommissionAmount("order_pay_amount_cny")),
		field.Float("order_credited_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Validate(validatePositiveSalesCommissionAmount("order_credited_amount")),
		field.Float("commission_rate").
			SchemaType(map[string]string{dialect.Postgres: "decimal(8,4)"}).
			Validate(validateSalesCommissionRate),
		field.Float("commission_total_cny").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Validate(validateNonNegativeSalesCommissionAmount("commission_total_cny")),
		field.Float("credited_used_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Validate(validateNonNegativeSalesCommissionAmount("credited_used_amount")).
			Default(0),
		field.Float("unlocked_cny").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Validate(validateNonNegativeSalesCommissionAmount("unlocked_cny")).
			Default(0),
		field.Float("settled_cny").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Validate(validateNonNegativeSalesCommissionAmount("settled_cny")).
			Default(0),
		field.String("status").
			MaxLen(32).
			Validate(validateSalesCommissionStatus).
			Default("frozen"),
		field.String("note").SchemaType(map[string]string{dialect.Postgres: "text"}).Default(""),
		field.Time("created_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func validateSalesCommissionRate(rate float64) error {
	if rate < 0 || rate > 100 {
		return fmt.Errorf("sales commission rate must be between 0 and 100")
	}
	return nil
}

func validatePositiveSalesCommissionAmount(name string) func(float64) error {
	return func(amount float64) error {
		if amount <= 0 {
			return fmt.Errorf("%s must be greater than 0", name)
		}
		return nil
	}
}

func validateNonNegativeSalesCommissionAmount(name string) func(float64) error {
	return func(amount float64) error {
		if amount < 0 {
			return fmt.Errorf("%s must be greater than or equal to 0", name)
		}
		return nil
	}
}

func validateSalesCommissionStatus(status string) error {
	switch status {
	case "frozen", "partial_unlocked", "unlocked", "settled", "settlement_blocked":
		return nil
	default:
		return fmt.Errorf("invalid sales commission status: %s", status)
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
