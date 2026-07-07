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

// ChannelMonitor holds the schema definition for the ChannelMonitor entity.
type ChannelMonitor struct {
	ent.Schema
}

func (ChannelMonitor) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "channel_monitors"},
	}
}

func (ChannelMonitor) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ChannelMonitor) Fields() []ent.Field {
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
		field.String("endpoint").
			NotEmpty().
			MaxLen(500),
		field.String("api_key_encrypted").
			NotEmpty().
			Sensitive(),
		field.String("primary_model").
			NotEmpty().
			MaxLen(200),
		field.JSON("extra_models", []string{}).
			Default([]string{}),
		field.String("group_name").
			Optional().
			Default("").
			MaxLen(100),
		field.Bool("enabled").
			Default(true),
		field.Int("interval_seconds").
			Range(15, 3600),
		field.Int("jitter_seconds").
			Default(0).
			NonNegative().
			Comment("Per-run scheduling jitter in seconds; actual delay is interval_seconds +/- jitter_seconds, and service validation keeps the effective interval >= 15 seconds"),
		field.Time("last_checked_at").
			Optional().
			Nillable(),
		field.Int64("created_by").
			Default(0),
		field.Int64("template_id").
			Optional().
			Nillable(),
		field.JSON("extra_headers", map[string]string{}).
			Default(map[string]string{}),
		field.String("body_override_mode").
			Default("off").
			MaxLen(10),
		field.JSON("body_override", map[string]any{}).
			Optional(),
	}
}

func (ChannelMonitor) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("history", ChannelMonitorHistory.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("daily_rollups", ChannelMonitorDailyRollup.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("request_template", ChannelMonitorRequestTemplate.Type).
			Field("template_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
	}
}

func (ChannelMonitor) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled", "last_checked_at"),
		index.Fields("provider"),
		index.Fields("provider", "api_mode"),
		index.Fields("group_name"),
		index.Fields("template_id"),
	}
}
