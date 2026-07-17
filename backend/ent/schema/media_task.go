package schema

import (
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MediaTask holds the schema definition for the MediaTask entity.
type MediaTask struct {
	ent.Schema
}

func (MediaTask) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (MediaTask) Fields() []ent.Field {
	return []ent.Field{
		field.String("public_id").MaxLen(64).Unique(),
		field.Int64("user_id"),
		field.Int64("api_key_id"),
		field.Int64("group_id"),
		field.Int64("channel_id").Optional().Nillable(),
		field.Int64("account_id").Optional().Nillable(),
		field.String("media_type").MaxLen(16),
		field.String("operation").MaxLen(40),
		field.String("requested_model").MaxLen(128),
		field.String("upstream_model").MaxLen(128).Default(""),
		field.String("adapter").MaxLen(64).Default(""),
		field.String("native_async_mode").MaxLen(16).Default("unsupported"),
		field.Bool("client_async").Default(false),
		field.Bool("sync_fallback").Default(false),
		field.String("status").MaxLen(20).Default("queued"),
		field.String("stage").MaxLen(20).Default("queued"),
		field.Int("progress").SchemaType(map[string]string{dialect.Postgres: "integer"}).Default(0),
		field.JSON("request_spec", json.RawMessage{}),
		field.JSON("candidate_snapshot", json.RawMessage{}),
		field.String("request_fingerprint").MaxLen(64),
		field.String("idempotency_key").MaxLen(255).Default(""),
		field.String("upstream_task_id").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.JSON("poll_metadata", json.RawMessage{}).Optional(),
		field.JSON("billing_snapshot", json.RawMessage{}).Optional(),
		field.JSON("settlement_plan", json.RawMessage{}).Optional(),
		field.JSON("settlement_recovery", json.RawMessage{}).Optional().SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("billing_status").MaxLen(24).Default("pending"),
		field.Float("precharged_amount").SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}).Default(0),
		field.Float("final_amount").SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}).Default(0),
		field.Float("refunded_amount").SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}).Default(0),
		field.Float("additional_charged_amount").SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}).Default(0),
		field.Int("retry_count").SchemaType(map[string]string{dialect.Postgres: "integer"}).Default(0),
		field.String("error_code").MaxLen(64).Default(""),
		field.String("error_message").SchemaType(map[string]string{dialect.Postgres: "text"}).Default(""),
		field.String("worker_id").MaxLen(128).Default(""),
		field.String("claim_token").MaxLen(64).Default(""),
		field.Time("lease_until").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("version").Default(1),
		field.Time("submitted_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("started_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("finished_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("sync_fallback_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (MediaTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at").
			Annotations(entsql.DescColumns("created_at")),
		index.Fields("status", "lease_until"),
		index.Fields("account_id").
			Annotations(entsql.IndexWhere("account_id IS NOT NULL")),
		index.Fields("user_id", "api_key_id", "idempotency_key").
			Unique().
			Annotations(entsql.IndexWhere("idempotency_key <> ''")),
	}
}
