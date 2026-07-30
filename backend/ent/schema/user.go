package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "users"},
	}
}

func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		// 唯一约束通过部分索引实现（WHERE deleted_at IS NULL），支持软删除后重用
		// 见迁移文件 016_soft_delete_partial_unique_indexes.sql
		field.String("email").
			MaxLen(255).
			NotEmpty(),
		field.String("password_hash").
			MaxLen(255).
			NotEmpty(),
		field.String("role").
			MaxLen(20).
			Default(domain.RoleUser),
		field.Float("balance").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Int("concurrency").
			Default(5),
		field.String("status").
			MaxLen(20).
			Default(domain.StatusActive),

		// Optional profile fields (added later; default '' in DB migration)
		field.String("username").
			MaxLen(100).
			Default(""),
		// wechat field migrated to user_attribute_values (see migration 019)
		field.String("notes").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),

		// TOTP 双因素认证字段
		field.String("totp_secret_encrypted").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Optional().
			Nillable(),
		field.Bool("totp_enabled").
			Default(false),
		field.Time("totp_enabled_at").
			Optional().
			Nillable(),

		// 余额不足通知
		field.Bool("balance_notify_enabled").
			Default(true),
		field.String("balance_notify_threshold_type").
			Default("fixed"), // "fixed" | "percentage"
		field.Float("balance_notify_threshold").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Optional().
			Nillable(),
		field.String("balance_notify_extra_emails").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default("[]"),
		field.Float("total_recharged").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),

		// 用户访问密钥用于开发者接口。密钥原文以 AES-GCM 加密保存，
		// 同时保留 SHA-256 摘要，以便在不解密整张用户表的情况下完成鉴权。
		field.String("user_access_key_hash").
			MaxLen(64).
			Optional().
			Nillable(),
		field.String("user_access_key_encrypted").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Optional().
			Nillable(),
		field.Time("user_access_key_created_at").
			Optional().
			Nillable(),

		// 销售佣金
		field.Bool("is_sales").
			Default(false),
		field.Float("sales_commission_rate").
			SchemaType(map[string]string{dialect.Postgres: "decimal(8,4)"}).
			Validate(validateSalesCommissionRate).
			Default(0),
		field.String("sales_commission_mode").
			MaxLen(16).
			Default("fixed"),
		field.Float("sales_commission_min_monthly_sales").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Default(0),

		// 推广奖励字段
		field.String("referral_code").
			MaxLen(20).
			Default(""),
		field.Int64("referred_by").
			Optional().
			Nillable(),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("api_keys", APIKey.Type),
		edge.To("redeem_codes", RedeemCode.Type),
		edge.To("subscriptions", UserSubscription.Type),
		edge.To("assigned_subscriptions", UserSubscription.Type),
		edge.To("announcement_reads", AnnouncementRead.Type),
		edge.To("allowed_groups", Group.Type).
			Through("user_allowed_groups", UserAllowedGroup.Type),
		edge.To("usage_logs", UsageLog.Type),
		edge.To("attribute_values", UserAttributeValue.Type),
		edge.To("promo_code_usages", PromoCodeUsage.Type),
		edge.To("payment_orders", PaymentOrder.Type),
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{
		// email 字段已在 Fields() 中声明 Unique()，无需重复索引
		index.Fields("status"),
		index.Fields("deleted_at"),
	}
}
