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
)

// UserModelPermission grants one user access to one public Composite route.
type UserModelPermission struct {
	ent.Schema
}

func (UserModelPermission) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_model_permissions"},
		field.ID("user_id", "route_id"),
	}
}

func (UserModelPermission) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("route_id"),
		field.Bool("enabled").Default(true),
		field.Int64("created_by").Immutable(),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UserModelPermission) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("user", User.Type).
			Unique().
			Required().
			Field("user_id").
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("route", CompositeModelRoute.Type).
			Unique().
			Required().
			Field("route_id").
			Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (UserModelPermission) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("route_id", "enabled"),
		index.Fields("user_id", "enabled"),
	}
}
