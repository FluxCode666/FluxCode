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

type GroupRouteSnapshot struct{ ent.Schema }

func (GroupRouteSnapshot) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "group_route_snapshots"}}
}

func (GroupRouteSnapshot) Mixin() []ent.Mixin { return []ent.Mixin{mixins.TimeMixin{}} }

func (GroupRouteSnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("group_id").Immutable(),
		field.Int64("version").Positive().Immutable(),
		field.String("status").MaxLen(32).Default("draft"),
		field.JSON("manifest", map[string]any{}).
			Default(map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.JSON("shadow_diff", map[string]any{}).
			Default(map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Int64("approved_by").Optional().Nillable(),
		field.Time("approved_at").Optional().Nillable(),
	}
}

func (GroupRouteSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("group_id", "version").Unique(),
		index.Fields("group_id", "status"),
	}
}
