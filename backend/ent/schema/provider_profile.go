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

// ProviderProfile is the provider-domain extension of an existing account.
// Its primary key is the backing account ID.
type ProviderProfile struct{ ent.Schema }

func (ProviderProfile) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "provider_profiles"}}
}

func (ProviderProfile) Mixin() []ent.Mixin { return []ent.Mixin{mixins.TimeMixin{}} }

func (ProviderProfile) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Immutable(),
		field.String("display_name").MaxLen(100).NotEmpty(),
		field.String("status").MaxLen(32).Default("draft"),
		field.Bool("allow_protocol_conversion").Default(false),
		field.String("base_url").MaxLen(500).Optional().Nillable(),
		field.String("auth_type").MaxLen(32).Optional().Nillable(),
		field.JSON("default_headers", map[string]string{}).
			Default(map[string]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Int64("version").Default(1).Positive(),
	}
}

func (ProviderProfile) Indexes() []ent.Index {
	return []ent.Index{index.Fields("status"), index.Fields("version")}
}
