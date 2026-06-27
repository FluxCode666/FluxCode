package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type GeneratedImage struct {
	ent.Schema
}

func (GeneratedImage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "generated_images"},
	}
}

func (GeneratedImage) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider").
			MaxLen(32).
			Default("openai"),
		field.Int64("user_id"),
		field.Int64("api_key_id"),
		field.Int64("account_id"),
		field.String("request_id").
			MaxLen(128).
			Optional().
			Nillable(),
		field.String("model").
			MaxLen(100).
			Optional().
			Nillable(),
		field.Text("prompt").
			Optional().
			Nillable(),
		field.Text("revised_prompt").
			Optional().
			Nillable(),
		field.String("response_format").
			MaxLen(20).
			Default("b64_json"),
		field.String("source").
			MaxLen(32).
			Default("b64_json"),
		field.String("content_type").
			MaxLen(100).
			Default("image/png"),
		field.Bytes("image_data"),
		field.Int("size_bytes").
			Default(0),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (GeneratedImage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at"),
		index.Fields("provider", "created_at"),
		index.Fields("user_id", "created_at"),
		index.Fields("api_key_id", "created_at"),
		index.Fields("account_id", "created_at"),
		index.Fields("request_id"),
	}
}
