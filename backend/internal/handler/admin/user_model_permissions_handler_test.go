package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type adminModelPermissionRepoStub struct {
	snapshot     *service.UserModelPermissionSnapshot
	replaceInput service.ReplaceUserModelPermissionsInput
	err          error
}

func (s *adminModelPermissionRepoStub) GetSnapshot(context.Context, int64) (*service.UserModelPermissionSnapshot, error) {
	return s.snapshot, s.err
}

func (s *adminModelPermissionRepoStub) Replace(_ context.Context, input service.ReplaceUserModelPermissionsInput) (*service.UserModelPermissionSnapshot, error) {
	s.replaceInput = input
	return s.snapshot, s.err
}

func (s *adminModelPermissionRepoStub) IsAllowed(context.Context, int64, int64) (bool, error) {
	return false, s.err
}

func (s *adminModelPermissionRepoStub) ListAuthorizedModels(context.Context, int64, int64, string) ([]string, error) {
	return nil, s.err
}

func setupAdminModelPermissionRouter(t *testing.T, repo *adminModelPermissionRepoStub) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewUserHandler(newStubAdminService(), nil, nil, nil, nil, nil, nil)
	h.SetModelPermissionService(service.NewUserModelPermissionService(repo))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 3})
		c.Next()
	})
	router.GET("/api/v1/admin/users/:id/model-permissions", h.GetModelPermissions)
	router.PUT("/api/v1/admin/users/:id/model-permissions", h.ReplaceModelPermissions)
	return router
}

func TestAdminModelPermissionsGetReturnsStrongETagAndStringIDs(t *testing.T) {
	now := time.Date(2026, 9, 13, 4, 0, 0, 0, time.UTC)
	repo := &adminModelPermissionRepoStub{snapshot: &service.UserModelPermissionSnapshot{
		UserID:  9007199254740993,
		Version: "version-1",
		Permissions: []service.UserModelPermission{{
			UserID: 9007199254740993, RouteID: 9007199254740995, Enabled: true, Effective: true,
			CreatedBy: 3, CreatedAt: now, UpdatedAt: now,
			Route: service.CompositeModelRoute{ID: 9007199254740995, GroupID: 9007199254740997, PublicModel: "public-gpt", MatchType: service.CompositeRouteMatchExact, TargetPlatform: service.PlatformOpenAI, UpstreamModel: "gpt-5", Endpoint: service.CompositeRouteEndpointAny, Enabled: true},
		}},
	}}
	router := setupAdminModelPermissionRouter(t, repo)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/9007199254740993/model-permissions", nil))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, `"version-1"`, rec.Header().Get("ETag"))
	require.Contains(t, rec.Body.String(), `"user_id":"9007199254740993"`)
	require.Contains(t, rec.Body.String(), `"route_id":"9007199254740995"`)
	require.Contains(t, rec.Body.String(), `"access_group_id":"9007199254740997"`)
}

func TestAdminModelPermissionsPutRequiresStrongVersionAndCapturesActor(t *testing.T) {
	repo := &adminModelPermissionRepoStub{snapshot: &service.UserModelPermissionSnapshot{UserID: 7, Version: "version-2"}}
	router := setupAdminModelPermissionRouter(t, repo)

	for _, tc := range []struct {
		name   string
		header string
		status int
	}{
		{name: "missing", status: http.StatusPreconditionRequired},
		{name: "weak", header: `W/"version-1"`, status: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/7/model-permissions", strings.NewReader(`{"route_ids":["9","2"]}`))
			req.Header.Set("Content-Type", "application/json")
			if tc.header != "" {
				req.Header.Set("If-Match", tc.header)
			}
			router.ServeHTTP(rec, req)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
		})
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/7/model-permissions", strings.NewReader(`{"route_ids":["9",2]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", `"version-1"`)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, int64(7), repo.replaceInput.UserID)
	require.Equal(t, int64(3), repo.replaceInput.ActorAdminID)
	require.Equal(t, "version-1", repo.replaceInput.ExpectedVersion)
	require.Equal(t, []int64{2, 9}, repo.replaceInput.RouteIDs)
}

var _ service.UserModelPermissionRepository = (*adminModelPermissionRepoStub)(nil)
