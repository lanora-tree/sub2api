package routes

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type compositeRouteRepoStub struct {
	routes []service.CompositeModelRoute
}

type gatewayModelPermissionRepoStub struct {
	allowed      bool
	models       []string
	err          error
	allowedCalls int
	listCalls    int
}

func (s *gatewayModelPermissionRepoStub) GetSnapshot(context.Context, int64) (*service.UserModelPermissionSnapshot, error) {
	return nil, s.err
}

func (s *gatewayModelPermissionRepoStub) Replace(context.Context, service.ReplaceUserModelPermissionsInput) (*service.UserModelPermissionSnapshot, error) {
	return nil, s.err
}

func (s *gatewayModelPermissionRepoStub) IsAllowed(context.Context, int64, int64) (bool, error) {
	s.allowedCalls++
	return s.allowed, s.err
}

func (s *gatewayModelPermissionRepoStub) ListAuthorizedModels(context.Context, int64, int64, string) ([]string, error) {
	s.listCalls++
	return s.models, s.err
}

func (s compositeRouteRepoStub) ListByGroup(ctx context.Context, groupID int64, includeDisabled bool) ([]service.CompositeModelRoute, error) {
	routes := make([]service.CompositeModelRoute, 0, len(s.routes))
	for _, route := range s.routes {
		if route.GroupID != groupID {
			continue
		}
		if !includeDisabled && !route.Enabled {
			continue
		}
		routes = append(routes, route)
	}
	return routes, nil
}

func (s compositeRouteRepoStub) Create(ctx context.Context, route *service.CompositeModelRoute) error {
	return nil
}

func (s compositeRouteRepoStub) Update(ctx context.Context, route *service.CompositeModelRoute) error {
	return nil
}

func (s compositeRouteRepoStub) Delete(ctx context.Context, id int64) error {
	return nil
}

func (s compositeRouteRepoStub) DeleteByGroup(ctx context.Context, groupID int64) error {
	return nil
}

func TestCompositeTargetPlatformMiddlewareResolvesModelAndRestoresBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.HandlerFunc(servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		groupID := int64(1)
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
			GroupID: &groupID,
			Group:   &service.Group{Platform: service.PlatformComposite},
		})
		c.Next()
	})))
	router.Use(compositeTargetPlatformMiddleware(nil))
	router.POST("/", func(c *gin.Context) {
		platform, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, service.PlatformOpenAI, platform)

		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"model":"gpt-5"}`, string(body))
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"gpt-5"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestCompositeTargetPlatformMiddlewareUsesExplicitRouteAndRewritesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	resolver := service.NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []service.CompositeModelRoute{
			{
				ID:             1,
				GroupID:        1,
				PublicModel:    "openrouter/gpt-5",
				MatchType:      service.CompositeRouteMatchExact,
				TargetPlatform: service.PlatformOpenAI,
				UpstreamModel:  "gpt-5",
				Endpoint:       service.CompositeRouteEndpointAny,
				Priority:       100,
				Enabled:        true,
			},
		},
	})
	router.Use(gin.HandlerFunc(servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		groupID := int64(1)
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformComposite},
		})
		c.Next()
	})))
	router.Use(compositeTargetPlatformMiddleware(resolver))
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		platform, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, service.PlatformOpenAI, platform)

		upstreamModel, ok := service.ResolvedUpstreamModelFromContext(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, "gpt-5", upstreamModel)

		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"model":"gpt-5","messages":[]}`, string(body))
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openrouter/gpt-5","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestCompositeTargetPlatformMiddlewareEnforcesExplicitRoutePermission(t *testing.T) {
	tests := []struct {
		name          string
		allowed       bool
		permissionErr error
		wantStatus    int
		wantCode      string
		wantHandler   bool
	}{
		{name: "allowed", allowed: true, wantStatus: http.StatusNoContent, wantHandler: true},
		{name: "denied", wantStatus: http.StatusForbidden, wantCode: "MODEL_NOT_ALLOWED"},
		{name: "unavailable", permissionErr: errors.New("database unavailable"), wantStatus: http.StatusServiceUnavailable, wantCode: "MODEL_PERMISSION_UNAVAILABLE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			permissionRepo := &gatewayModelPermissionRepoStub{allowed: tt.allowed, err: tt.permissionErr}
			resolver := service.NewCompositeRouteResolver(compositeRouteRepoStub{routes: []service.CompositeModelRoute{{
				ID: 41, GroupID: 1, PublicModel: "public-gpt", MatchType: service.CompositeRouteMatchExact,
				TargetPlatform: service.PlatformOpenAI, UpstreamModel: "gpt-5", Endpoint: service.CompositeRouteEndpointAny, Enabled: true,
			}}})
			resolver.SetPermissionService(service.NewUserModelPermissionService(permissionRepo))
			handlerCalled := false
			router := gin.New()
			router.Use(func(c *gin.Context) {
				groupID := int64(1)
				c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformComposite}})
				c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
				c.Next()
			})
			router.Use(compositeTargetPlatformMiddleware(resolver, true))
			router.POST("/v1/chat/completions", func(c *gin.Context) {
				handlerCalled = true
				body, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				require.JSONEq(t, `{"model":"gpt-5"}`, string(body))
				c.Status(http.StatusNoContent)
			})

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"public-gpt"}`))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			require.Equal(t, tt.wantHandler, handlerCalled)
			require.Equal(t, 1, permissionRepo.allowedCalls)
			if tt.wantCode != "" {
				require.Contains(t, rec.Body.String(), `"code":"`+tt.wantCode+`"`)
			}
		})
	}
}

func TestCompositeTargetPlatformMiddlewareRejectsImplicitDetectorRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	permissionRepo := &gatewayModelPermissionRepoStub{allowed: true}
	resolver := service.NewCompositeRouteResolver(nil)
	resolver.SetPermissionService(service.NewUserModelPermissionService(permissionRepo))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(1)
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformComposite}})
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
		c.Next()
	})
	router.Use(compositeTargetPlatformMiddleware(resolver, true))
	router.POST("/v1/chat/completions", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"code":"MODEL_NOT_ALLOWED"`)
	require.Zero(t, permissionRepo.allowedCalls, "implicit routes must never enter the permission lookup")
}

func TestCompositeTargetPlatformMiddlewareAuthorizesDefaultImageModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	permissionRepo := &gatewayModelPermissionRepoStub{allowed: false}
	resolver := service.NewCompositeRouteResolver(compositeRouteRepoStub{routes: []service.CompositeModelRoute{{
		ID: 42, GroupID: 1, PublicModel: "gpt-image-2", MatchType: service.CompositeRouteMatchExact,
		TargetPlatform: service.PlatformOpenAI, UpstreamModel: "gpt-image-2", Endpoint: service.CompositeRouteEndpointImages, Enabled: true,
	}}})
	resolver.SetPermissionService(service.NewUserModelPermissionService(permissionRepo))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(1)
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformComposite}})
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
		c.Next()
	})
	router.Use(compositeTargetPlatformMiddleware(resolver, true))
	router.POST("/v1/images/generations", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"prompt":"draw"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"code":"MODEL_NOT_ALLOWED"`)
	require.Equal(t, 1, permissionRepo.allowedCalls)
}

func TestUserModelPermissionMiddlewareScopesCompositeCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	permissionRepo := &gatewayModelPermissionRepoStub{models: []string{"allowed-a", "allowed-b"}}
	resolver := service.NewCompositeRouteResolver(nil)
	resolver.SetPermissionService(service.NewUserModelPermissionService(permissionRepo))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(1)
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformComposite}})
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
		c.Next()
	})
	router.Use(userModelPermissionMiddleware(resolver))
	router.GET("/v1/models", func(c *gin.Context) {
		models, scoped := service.AuthorizedModelIDsFromContext(c.Request.Context())
		require.True(t, scoped)
		require.Equal(t, []string{"allowed-a", "allowed-b"}, models)
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/models", nil))

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	require.Equal(t, 1, permissionRepo.listCalls)
}

func TestCompositeTargetPlatformMiddlewareRewritesNestedLiveModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	resolver := service.NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []service.CompositeModelRoute{
			{
				ID:             1,
				GroupID:        1,
				PublicModel:    "live-alias",
				MatchType:      service.CompositeRouteMatchExact,
				TargetPlatform: service.PlatformOpenAI,
				UpstreamModel:  "gpt-live",
				Endpoint:       service.CompositeRouteEndpointAny,
				Priority:       100,
				Enabled:        true,
			},
		},
	})
	router.Use(gin.HandlerFunc(servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		groupID := int64(1)
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformComposite},
		})
		c.Next()
	})))
	router.Use(compositeTargetPlatformMiddleware(resolver))
	router.POST("/backend-api/codex/realtime/calls", func(c *gin.Context) {
		platform, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, service.PlatformOpenAI, platform)

		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"session":{"model":"gpt-live"},"sdp":"v=0"}`, string(body))
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(
		http.MethodPost,
		"/backend-api/codex/realtime/calls",
		strings.NewReader(`{"session":{"model":"live-alias"},"sdp":"v=0"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestCompositeRequestModelFromMultipartLiveSession(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("sdp", "v=0"))
	require.NoError(t, writer.WriteField("session", `{"model":"live-alias"}`))
	require.NoError(t, writer.Close())

	require.Equal(t, "live-alias", compositeRequestModelFromBody(writer.FormDataContentType(), body.Bytes()))
}

func TestCompositeCodexControlPathsUseResponsesRoutes(t *testing.T) {
	for _, path := range []string{
		"/v1/alpha/search",
		"/backend-api/codex/alpha/search",
		"/v1/live",
		"/backend-api/codex/realtime/calls",
	} {
		require.Equal(t, service.CompositeRouteEndpointResponses, compositeRouteEndpointForPath(path), "path=%s", path)
	}
}

func TestCompositeTargetPlatformMiddlewareUsesExplicitRouteForMultipartImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	resolver := service.NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []service.CompositeModelRoute{
			{
				ID:             1,
				GroupID:        1,
				PublicModel:    "image-alias",
				MatchType:      service.CompositeRouteMatchExact,
				TargetPlatform: service.PlatformOpenAI,
				UpstreamModel:  "gpt-image-1",
				Endpoint:       service.CompositeRouteEndpointImages,
				Priority:       100,
				Enabled:        true,
			},
		},
	})
	router.Use(gin.HandlerFunc(servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		groupID := int64(1)
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformComposite},
		})
		c.Next()
	})))
	router.Use(compositeTargetPlatformMiddleware(resolver))
	router.POST("/v1/images/edits", func(c *gin.Context) {
		platform, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, service.PlatformOpenAI, platform)

		upstreamModel, ok := service.ResolvedUpstreamModelFromContext(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, "gpt-image-1", upstreamModel)

		publicModel, ok := service.RequestedPublicModelFromContext(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, "image-alias", publicModel)

		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), "image-alias")
		c.Status(http.StatusNoContent)
	})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "image-alias"))
	require.NoError(t, writer.WriteField("prompt", "draw"))
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestCompositeGeminiTargetPlatformMiddlewareUsesPathRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	resolver := service.NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []service.CompositeModelRoute{
			{
				ID:             1,
				GroupID:        1,
				PublicModel:    "openrouter/gemini-pro",
				MatchType:      service.CompositeRouteMatchExact,
				TargetPlatform: service.PlatformGemini,
				UpstreamModel:  "gemini-2.5-pro",
				Endpoint:       service.CompositeRouteEndpointGemini,
				Priority:       100,
				Enabled:        true,
			},
		},
	})
	router.Use(gin.HandlerFunc(servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		groupID := int64(1)
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformComposite},
		})
		c.Next()
	})))
	router.Use(compositeGeminiTargetPlatformMiddleware(resolver))
	router.POST("/v1beta/models/*modelAction", func(c *gin.Context) {
		platform, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, service.PlatformGemini, platform)

		upstreamModel, ok := service.ResolvedUpstreamModelFromContext(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, "gemini-2.5-pro", upstreamModel)
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/openrouter/gemini-pro:generateContent", strings.NewReader(`{"contents":[]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
}

var _ service.UserModelPermissionRepository = (*gatewayModelPermissionRepoStub)(nil)
