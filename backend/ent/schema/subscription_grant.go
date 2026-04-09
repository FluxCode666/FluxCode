package schema

import (
	"time"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SubscriptionGrant holds the schema definition for the SubscriptionGrant entity.
//
// A SubscriptionGrant represents one entitlement window (starts_at..expires_at)
// attached to a user_subscriptions row. Multiple grants may overlap to provide
// stacked quota limits.
type SubscriptionGrant struct {
	ent.Schema
}

func (SubscriptionGrant) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "subscription_grants"},
	}
}

func (SubscriptionGrant) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (SubscriptionGrant) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("subscription_id"),
		field.Time("starts_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("expires_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Float("daily_usage_usd").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Default(0),
		field.Float("weekly_usage_usd").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Default(0),
		field.Float("monthly_usage_usd").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Default(0),
	}
}

func (SubscriptionGrant) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("subscription", UserSubscription.Type).
			Ref("grants").
			Field("subscription_id").
			Unique().
			Required(),
	}
}

func (SubscriptionGrant) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("subscription_id"),
		index.Fields("subscription_id", "starts_at", "expires_at"),
		index.Fields("expires_at"),
		index.Fields("deleted_at"),
	}
}
