package service

import (
	"context"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type userModelPermissionRepoStub struct {
	replaceInput ReplaceUserModelPermissionsInput
	snapshot     *UserModelPermissionSnapshot
	allowed      bool
	models       []string
	err          error
}

func (s *userModelPermissionRepoStub) GetSnapshot(context.Context, int64) (*UserModelPermissionSnapshot, error) {
	return s.snapshot, s.err
}

func (s *userModelPermissionRepoStub) Replace(_ context.Context, input ReplaceUserModelPermissionsInput) (*UserModelPermissionSnapshot, error) {
	s.replaceInput = input
	return s.snapshot, s.err
}

func (s *userModelPermissionRepoStub) IsAllowed(context.Context, int64, int64) (bool, error) {
	return s.allowed, s.err
}

func (s *userModelPermissionRepoStub) ListAuthorizedModels(context.Context, int64, int64, string) ([]string, error) {
	return s.models, s.err
}

func TestUserModelPermissionReplaceRequiresVersionAndUniquePositiveRoutes(t *testing.T) {
	service := NewUserModelPermissionService(&userModelPermissionRepoStub{})
	base := ReplaceUserModelPermissionsInput{UserID: 7, ActorAdminID: 3, ExpectedVersion: "v1"}

	missingVersion := base
	missingVersion.ExpectedVersion = ""
	_, err := service.Replace(context.Background(), missingVersion)
	require.ErrorIs(t, err, ErrModelPermissionVersionRequired)

	duplicate := base
	duplicate.RouteIDs = []int64{2, 2}
	_, err = service.Replace(context.Background(), duplicate)
	require.Equal(t, "MODEL_PERMISSION_ROUTE_DUPLICATE", infraerrors.Reason(err))

	invalid := base
	invalid.RouteIDs = []int64{0}
	_, err = service.Replace(context.Background(), invalid)
	require.ErrorIs(t, err, ErrModelPermissionRouteInvalid)
}

func TestUserModelPermissionReplaceNormalizesVersionAndSortsCopy(t *testing.T) {
	repo := &userModelPermissionRepoStub{snapshot: &UserModelPermissionSnapshot{UserID: 7, Version: "next"}}
	service := NewUserModelPermissionService(repo)
	routeIDs := []int64{9, 2, 5}

	snapshot, err := service.Replace(context.Background(), ReplaceUserModelPermissionsInput{
		UserID: 7, ActorAdminID: 3, ExpectedVersion: ` "current" `, RouteIDs: routeIDs,
	})

	require.NoError(t, err)
	require.Equal(t, "next", snapshot.Version)
	require.Equal(t, "current", repo.replaceInput.ExpectedVersion)
	require.Equal(t, []int64{2, 5, 9}, repo.replaceInput.RouteIDs)
	require.Equal(t, []int64{9, 2, 5}, routeIDs, "service must not mutate the caller's slice")
}

func TestUserModelPermissionServiceFailsClosedWhenUnavailable(t *testing.T) {
	service := NewUserModelPermissionService(nil)

	allowed, err := service.IsAllowed(context.Background(), 7, 2)
	require.False(t, allowed)
	require.ErrorIs(t, err, ErrModelPermissionUnavailable)

	models, err := service.ListAuthorizedModels(context.Background(), 7, 2, CompositeRouteEndpointAny)
	require.Nil(t, models)
	require.ErrorIs(t, err, ErrModelPermissionUnavailable)
}

var _ UserModelPermissionRepository = (*userModelPermissionRepoStub)(nil)
