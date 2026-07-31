package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type LogicalModel struct{ ent.Schema }

func (LogicalModel) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "logical_models"}}
}

func (LogicalModel) Mixin() []ent.Mixin { return []ent.Mixin{mixins.TimeMixin{}} }

func (LogicalModel) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").MaxLen(100).NotEmpty(),
		field.String("display_name").MaxLen(100).Default(""),
		field.Bool("enabled").Default(true),
		field.Int64("version").Default(1).Positive(),
	}
}

func (LogicalModel) Indexes() []ent.Index {
	return []ent.Index{index.Fields("name").Unique(), index.Fields("enabled")}
}
