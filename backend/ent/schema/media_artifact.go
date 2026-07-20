package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MediaArtifact holds the schema definition for the MediaArtifact entity.
type MediaArtifact struct {
	ent.Schema
}

func (MediaArtifact) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (MediaArtifact) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("task_id"),
		field.String("direction").MaxLen(16),
		field.Int("position").SchemaType(map[string]string{dialect.Postgres: "integer"}).Default(0),
		field.String("media_type").MaxLen(16),
		field.String("content_type").MaxLen(128),
		field.Int64("size_bytes").Default(0),
		field.String("checksum_sha256").MaxLen(64).Default(""),
		field.Int("width").SchemaType(map[string]string{dialect.Postgres: "integer"}).Optional().Nillable(),
		field.Int("height").SchemaType(map[string]string{dialect.Postgres: "integer"}).Optional().Nillable(),
		field.Float("duration_seconds").Optional().Nillable(),
		field.String("resolution").MaxLen(32).Default(""),
		field.Float("fps").Optional().Nillable(),
		field.String("storage_status").MaxLen(24).Default("pending"),
		field.String("storage_provider").MaxLen(32).Default("legacy"),
		field.String("object_key").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.String("public_url").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.Text("upstream_reference").Optional().Nillable().Sensitive(),
		field.Time("expires_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (MediaArtifact) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id", "direction", "position").Unique(),
	}
}
