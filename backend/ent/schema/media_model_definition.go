package schema

import (
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MediaModelDefinition holds the schema definition for the MediaModelDefinition entity.
type MediaModelDefinition struct {
	ent.Schema
}

func (MediaModelDefinition) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (MediaModelDefinition) Fields() []ent.Field {
	return []ent.Field{
		field.String("model_id").MaxLen(128).Unique(),
		field.String("media_type").MaxLen(16),
		field.JSON("operations", []string{}),
		field.JSON("constraints", json.RawMessage{}),
		field.String("billing_unit").MaxLen(32),
		field.Bool("enabled").Default(true),
	}
}

func (MediaModelDefinition) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled", "media_type"),
	}
}
