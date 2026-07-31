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

type ProviderMigrationReview struct{ ent.Schema }

func (ProviderMigrationReview) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "provider_migration_reviews"}}
}

func (ProviderMigrationReview) Mixin() []ent.Mixin { return []ent.Mixin{mixins.TimeMixin{}} }

func (ProviderMigrationReview) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("provider_id").Immutable(),
		field.Int64("group_id").Optional().Nillable(),
		field.String("status").MaxLen(32).Default("pending"),
		field.String("reason").SchemaType(map[string]string{dialect.Postgres: "text"}).Default(""),
		field.JSON("evidence", map[string]any{}).
			Default(map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Int64("snapshot_version").Default(1).Positive(),
		field.Int64("reviewed_by").Optional().Nillable(),
		field.Time("reviewed_at").Optional().Nillable(),
	}
}

func (ProviderMigrationReview) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_id", "group_id", "snapshot_version").Unique(),
		index.Fields("status"),
	}
}
