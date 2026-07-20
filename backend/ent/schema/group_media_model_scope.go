package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// GroupMediaModelScope grants a group access to a canonical media model.
type GroupMediaModelScope struct {
	ent.Schema
}

func (GroupMediaModelScope) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (GroupMediaModelScope) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("group_id"),
		field.Int64("model_definition_id"),
	}
}

func (GroupMediaModelScope) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("group", Group.Type).
			Field("group_id").
			Required().
			Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("model_definition", MediaModelDefinition.Type).
			Field("model_definition_id").
			Required().
			Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (GroupMediaModelScope) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("group_id", "model_definition_id").Unique(),
		index.Fields("model_definition_id"),
	}
}
