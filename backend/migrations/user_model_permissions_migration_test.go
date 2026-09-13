package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserModelPermissionsMigrationKeepsAuthorizationIdentityStable(t *testing.T) {
	content, err := FS.ReadFile("242_user_model_permissions.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")

	require.Contains(t, sql, "PRIMARY KEY (user_id, route_id)")
	require.Contains(t, sql, "created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT")
	require.Contains(t, sql, "route_id BIGINT NOT NULL REFERENCES composite_model_routes(id) ON DELETE RESTRICT")
	require.Contains(t, sql, "CREATE TRIGGER trg_composite_route_authorization_semantics")
	for _, field := range []string{"group_id", "public_model", "match_type", "target_platform", "upstream_model", "endpoint", "deleted_at"} {
		require.Contains(t, sql, "NEW."+field+" IS DISTINCT FROM OLD."+field)
	}
}
