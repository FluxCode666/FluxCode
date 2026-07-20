package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MediaModelAlias maps a requested media model ID to its canonical definition.
type MediaModelAlias struct {
	ent.Schema
}

func (MediaModelAlias) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Annotation{Table: "media_model_aliases"}}
}

func (MediaModelAlias) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (MediaModelAlias) Fields() []ent.Field {
	return []ent.Field{
		field.String("requested_model_id").MaxLen(128).Unique(),
		field.Int64("model_definition_id"),
	}
}

func (MediaModelAlias) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("model_definition", MediaModelDefinition.Type).
			Field("model_definition_id").
			Required().
			Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (MediaModelAlias) Indexes() []ent.Index {
	return []ent.Index{index.Fields("model_definition_id")}
}
