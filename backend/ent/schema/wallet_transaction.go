package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/shopspring/decimal"
)

// WalletTransaction is the immutable CNY ledger entry for a user's balance.
// users.balance remains the authoritative current balance; every M6 wallet
// mutation must update it in the same PostgreSQL transaction as this record.
type WalletTransaction struct {
	ent.Schema
}

func (WalletTransaction) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "wallet_transactions"},
	}
}

func (WalletTransaction) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id").
			Immutable(),
		field.Enum("type").
			Values("recharge", "usage", "refund", "adjustment").
			Immutable(),
		field.Float("amount").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}).
			Immutable(),
		field.String("currency").
			MaxLen(3).
			Default("CNY").
			Immutable(),
		field.Float("balance_before").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}).
			Immutable(),
		field.Float("balance_after").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}).
			Immutable(),
		field.String("reference_type").
			MaxLen(32).
			Immutable(),
		field.String("reference_id").
			MaxLen(128).
			Immutable(),
		field.String("idempotency_key").
			MaxLen(160).
			Unique().
			Immutable(),
		field.String("request_fingerprint").
			MaxLen(64).
			Immutable(),
		field.Int64("operator_id").
			Optional().
			Nillable().
			Immutable(),
		field.String("source").
			MaxLen(64).
			Immutable(),
		field.String("note").
			MaxLen(500).
			Default("").
			Immutable(),
		field.JSON("metadata", map[string]any{}).
			Default(func() map[string]any { return map[string]any{} }).
			Immutable(),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (WalletTransaction) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("wallet_transactions").
			Field("user_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Unique().
			Required().
			Immutable(),
		edge.From("operator", User.Type).
			Ref("operated_wallet_transactions").
			Field("operator_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Unique().
			Immutable(),
	}
}

func (WalletTransaction) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at"),
		index.Fields("reference_type", "reference_id"),
	}
}
