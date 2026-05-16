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

// UserReferralConfig holds the schema definition for the UserReferralConfig entity.
type UserReferralConfig struct {
	ent.Schema
}

func (UserReferralConfig) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_referral_configs"},
	}
}

func (UserReferralConfig) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Float("invitee_reward_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Optional().
			Nillable(),
		field.Float("inviter_reward_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Optional().
			Nillable(),
		field.Int("max_invites").
			Optional().
			Nillable(),
		field.Int("reward_expiry_days").
			Optional().
			Nillable(),
		field.Bool("ongoing_reward_enabled").
			Optional().
			Nillable(),
		field.String("ongoing_reward_type").
			SchemaType(map[string]string{dialect.Postgres: "varchar(20)"}).
			Optional().
			Nillable(),
		field.Float("ongoing_reward_value").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Optional().
			Nillable(),
		field.Int("ongoing_reward_max_count").
			Optional().
			Nillable(),
		field.Int("ongoing_reward_duration_days").
			Optional().
			Nillable(),
		field.String("notes").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
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

func (UserReferralConfig) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id").Unique(),
	}
}
