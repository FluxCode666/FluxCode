package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ProviderProtocolEndpoint struct{ ent.Schema }

func (ProviderProtocolEndpoint) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "provider_protocol_endpoints"}}
}

func (ProviderProtocolEndpoint) Mixin() []ent.Mixin { return []ent.Mixin{mixins.TimeMixin{}} }

func (ProviderProtocolEndpoint) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("provider_id").Immutable(),
		field.String("protocol_family").MaxLen(32).Immutable(),
		field.String("wire_profile").MaxLen(64).Default("canonical_v1"),
		field.String("base_url").MaxLen(500).Optional().Nillable(),
		field.String("path").MaxLen(255).NotEmpty(),
		field.String("auth_type").MaxLen(32).Optional().Nillable(),
		field.JSON("headers", map[string]string{}).
			Default(map[string]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Bool("enabled").Default(true),
		field.Int64("version").Default(1).Positive(),
	}
}

func (ProviderProtocolEndpoint) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_id", "protocol_family").Unique(),
		index.Fields("protocol_family", "enabled"),
	}
}
