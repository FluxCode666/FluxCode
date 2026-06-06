package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ChannelMonitorRequestTemplate holds reusable monitor request headers/body overrides.
type ChannelMonitorRequestTemplate struct {
	ent.Schema
}

func (ChannelMonitorRequestTemplate) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "channel_monitor_request_templates"},
	}
}

func (ChannelMonitorRequestTemplate) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ChannelMonitorRequestTemplate) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			MaxLen(100),
		field.Enum("provider").
			Values("openai", "anthropic", "gemini"),
		field.String("api_mode").
			Default("chat_completions").
			MaxLen(32).
			Comment("OpenAI request protocol: chat_completions or responses"),
		field.String("description").
			Optional().
			Default("").
			MaxLen(500),
		field.JSON("extra_headers", map[string]string{}).
			Default(map[string]string{}),
		field.String("body_override_mode").
			Default("off").
			MaxLen(10),
		field.JSON("body_override", map[string]any{}).
			Optional(),
	}
}

func (ChannelMonitorRequestTemplate) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("monitors", ChannelMonitor.Type).
			Ref("request_template"),
	}
}

func (ChannelMonitorRequestTemplate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider", "name").Unique(),
		index.Fields("provider", "api_mode"),
	}
}
