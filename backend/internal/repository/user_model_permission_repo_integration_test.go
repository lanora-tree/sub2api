//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserModelPermissionRepositoryAtomicReplacementAndEffectiveAuthorization(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	admin := mustCreateUser(t, integrationEntClient, &service.User{
		Email: fmt.Sprintf("model-permission-admin-%d@example.com", suffix), Role: service.RoleAdmin,
	})
	user := mustCreateUser(t, integrationEntClient, &service.User{
		Email: fmt.Sprintf("model-permission-user-%d@example.com", suffix), Role: service.RoleUser,
	})
	group := mustCreateGroup(t, integrationEntClient, &service.Group{
		Name: fmt.Sprintf("model-permission-composite-%d", suffix), Platform: service.PlatformComposite,
	})
	route1, err := integrationEntClient.CompositeModelRoute.Create().
		SetGroupID(group.ID).
		SetPublicModel("permission-public-a").
		SetMatchType(service.CompositeRouteMatchExact).
		SetTargetPlatform(service.PlatformOpenAI).
		SetUpstreamModel("gpt-5-a").
		SetEndpoint(service.CompositeRouteEndpointAny).
		Save(ctx)
	require.NoError(t, err)
	route2, err := integrationEntClient.CompositeModelRoute.Create().
		SetGroupID(group.ID).
		SetPublicModel("permission-public-b").
		SetMatchType(service.CompositeRouteMatchExact).
		SetTargetPlatform(service.PlatformOpenAI).
		SetUpstreamModel("gpt-5-b").
		SetEndpoint(service.CompositeRouteEndpointAny).
		Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM user_model_permissions WHERE user_id = $1`, user.ID)
		_ = integrationEntClient.CompositeModelRoute.DeleteOneID(route1.ID).Exec(context.Background())
		_ = integrationEntClient.CompositeModelRoute.DeleteOneID(route2.ID).Exec(context.Background())
		_ = integrationEntClient.Group.DeleteOneID(group.ID).Exec(context.Background())
		_ = integrationEntClient.User.DeleteOneID(user.ID).Exec(context.Background())
		_ = integrationEntClient.User.DeleteOneID(admin.ID).Exec(context.Background())
	})

	repo := NewUserModelPermissionRepository(integrationDB)
	empty, err := repo.GetSnapshot(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, empty.Version, 64)
	require.Empty(t, empty.Permissions)

	assigned, err := repo.Replace(ctx, service.ReplaceUserModelPermissionsInput{
		UserID: user.ID, ActorAdminID: admin.ID, ExpectedVersion: empty.Version, RouteIDs: []int64{route1.ID},
	})
	require.NoError(t, err)
	require.NotEqual(t, empty.Version, assigned.Version)
	require.Len(t, assigned.Permissions, 1)
	require.True(t, assigned.Permissions[0].Effective)

	allowed, err := repo.IsAllowed(ctx, user.ID, route1.ID)
	require.NoError(t, err)
	require.True(t, allowed)
	models, err := repo.ListAuthorizedModels(ctx, user.ID, group.ID, service.CompositeRouteEndpointAny)
	require.NoError(t, err)
	require.Equal(t, []string{"permission-public-a"}, models)

	_, err = repo.Replace(ctx, service.ReplaceUserModelPermissionsInput{
		UserID: user.ID, ActorAdminID: admin.ID, ExpectedVersion: empty.Version, RouteIDs: []int64{route2.ID},
	})
	require.ErrorIs(t, err, service.ErrModelPermissionVersionMismatch)

	_, err = integrationDB.ExecContext(ctx, `UPDATE composite_model_routes SET public_model = $1 WHERE id = $2`, "changed-in-place", route1.ID)
	require.Error(t, err, "authorized route semantics must be immutable")
	_, err = integrationDB.ExecContext(ctx, `UPDATE composite_model_routes SET priority = priority + 1, enabled = FALSE WHERE id = $1`, route1.ID)
	require.NoError(t, err, "operational fields remain mutable")
	allowed, err = repo.IsAllowed(ctx, user.ID, route1.ID)
	require.NoError(t, err)
	require.False(t, allowed)
	disabled, err := repo.GetSnapshot(ctx, user.ID)
	require.NoError(t, err)
	require.False(t, disabled.Permissions[0].Effective)
	_, err = integrationDB.ExecContext(ctx, `UPDATE composite_model_routes SET enabled = TRUE WHERE id = $1`, route1.ID)
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, routeIDs := range [][]int64{{route2.ID}, {}} {
		routeIDs := routeIDs
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, replaceErr := repo.Replace(ctx, service.ReplaceUserModelPermissionsInput{
				UserID: user.ID, ActorAdminID: admin.ID, ExpectedVersion: assigned.Version, RouteIDs: routeIDs,
			})
			results <- replaceErr
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for replaceErr := range results {
		switch {
		case replaceErr == nil:
			successes++
		case errors.Is(replaceErr, service.ErrModelPermissionVersionMismatch):
			conflicts++
		default:
			require.NoError(t, replaceErr)
		}
	}
	require.Equal(t, 1, successes, "exactly one concurrent full replacement must commit")
	require.Equal(t, 1, conflicts, "the stale concurrent replacement must be rejected")
}
