package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ProviderModelCapability struct{ ent.Schema }

func (ProviderModelCapability) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "provider_model_capabilities"}}
}

func (ProviderModelCapability) Mixin() []ent.Mixin { return []ent.Mixin{mixins.TimeMixin{}} }

func (ProviderModelCapability) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("provider_id").Immutable(),
		field.Int64("logical_model_id").Immutable(),
		field.Int64("endpoint_id").Optional().Nillable(),
		field.String("protocol_family").MaxLen(32).Immutable(),
		field.String("upstream_model").MaxLen(200).NotEmpty(),
		field.String("wire_profile").MaxLen(64).Default("canonical_v1"),
		field.String("feature_profile").MaxLen(64).NotEmpty(),
		field.Bool("enabled").Default(true),
		field.Bool("legacy_compatibility").Default(false),
		field.Int64("version").Default(1).Positive(),
	}
}

func (ProviderModelCapability) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_id", "logical_model_id", "protocol_family").Unique(),
		index.Fields("logical_model_id", "protocol_family", "enabled"),
		index.Fields("provider_id", "enabled"),
	}
}
