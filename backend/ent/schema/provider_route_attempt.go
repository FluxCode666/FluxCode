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

// ProviderRouteAttempt is an append-only, payload-free diagnostic record for
// one concrete upstream route attempt. It intentionally stores RouteIdentity
// as both a stable string and low-cardinality dimensions so operators can
// diagnose failover without persisting request bodies or credentials.
type ProviderRouteAttempt struct{ ent.Schema }

func (ProviderRouteAttempt) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "provider_route_attempts"}}
}

func (ProviderRouteAttempt) Fields() []ent.Field {
	return []ent.Field{
		field.String("trace_id").MaxLen(64).NotEmpty(),
		field.Int64("group_id"),
		field.Int64("provider_id"),
		field.Int64("capability_id"),
		field.Int64("endpoint_id").Default(0),
		field.String("route_identity").MaxLen(512).NotEmpty(),
		field.String("logical_model").MaxLen(100).NotEmpty(),
		field.String("upstream_model").MaxLen(200).Default(""),
		field.String("ingress_protocol").MaxLen(32).NotEmpty(),
		field.String("upstream_protocol").MaxLen(32).NotEmpty(),
		field.String("wire_profile").MaxLen(64).Default(""),
		field.String("route_tier").MaxLen(16).NotEmpty(),
		field.Bool("conversion_used").Default(false),
		field.String("outcome").MaxLen(16).NotEmpty(),
		field.Int("status_code").Default(0),
		field.String("failure_category").MaxLen(64).Default(""),
		field.String("upstream_request_id").MaxLen(255).Default(""),
		field.Int64("duration_ms").Default(0),
		field.Int64("bytes_committed").Default(0),
		field.String("final_reason").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (ProviderRouteAttempt) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at"),
		index.Fields("trace_id", "created_at"),
		index.Fields("group_id", "created_at"),
		index.Fields("provider_id", "created_at"),
		index.Fields("ingress_protocol", "upstream_protocol", "created_at"),
		index.Fields("outcome", "failure_category", "created_at"),
	}
}
