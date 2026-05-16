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

// Referral holds the schema definition for the Referral entity.
type Referral struct {
	ent.Schema
}

func (Referral) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "referrals"},
	}
}

func (Referral) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("referrer_id"),
		field.Int64("referee_id"),
		field.String("referral_code").
			MaxLen(20).
			Default(""),
		field.String("status").
			MaxLen(20).
			Default("pending"),
		field.Float("invitee_reward_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("inviter_reward_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Time("invitee_rewarded_at").
			Optional().
			Nillable(),
		field.Time("inviter_rewarded_at").
			Optional().
			Nillable(),
		field.Int("ongoing_reward_count").
			Default(0),
		field.Float("ongoing_reward_total").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
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

func (Referral) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("referee_id").Unique(),
		index.Fields("referrer_id", "created_at"),
		index.Fields("status"),
	}
}
