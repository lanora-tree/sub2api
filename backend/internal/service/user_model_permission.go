package service

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrModelNotAllowed                = infraerrors.Forbidden("MODEL_NOT_ALLOWED", "current user is not allowed to use this model")
	ErrModelPermissionVersionRequired = infraerrors.New(http.StatusPreconditionRequired, "MODEL_PERMISSION_VERSION_REQUIRED", "If-Match or version is required")
	ErrModelPermissionVersionMismatch = infraerrors.Conflict("MODEL_PERMISSION_VERSION_MISMATCH", "model permissions changed; reload and retry")
	ErrModelPermissionRouteInvalid    = infraerrors.BadRequest("MODEL_PERMISSION_ROUTE_INVALID", "route must be a non-deleted exact route owned by a Composite group")
	ErrModelPermissionActorInvalid    = infraerrors.Forbidden("MODEL_PERMISSION_ACTOR_INVALID", "an active administrator is required")
	ErrModelPermissionUnavailable     = infraerrors.ServiceUnavailable("MODEL_PERMISSION_UNAVAILABLE", "model permission service is unavailable")
)

type UserModelPermission struct {
	UserID    int64               `json:"user_id"`
	RouteID   int64               `json:"route_id"`
	Enabled   bool                `json:"enabled"`
	Effective bool                `json:"effective"`
	CreatedBy int64               `json:"created_by"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
	Route     CompositeModelRoute `json:"route"`
}

type UserModelPermissionSnapshot struct {
	UserID      int64                 `json:"user_id"`
	Version     string                `json:"version"`
	Permissions []UserModelPermission `json:"permissions"`
}

type ReplaceUserModelPermissionsInput struct {
	UserID          int64
	ActorAdminID    int64
	ExpectedVersion string
	RouteIDs        []int64
}

type UserModelPermissionRepository interface {
	GetSnapshot(ctx context.Context, userID int64) (*UserModelPermissionSnapshot, error)
	Replace(ctx context.Context, input ReplaceUserModelPermissionsInput) (*UserModelPermissionSnapshot, error)
	IsAllowed(ctx context.Context, userID, routeID int64) (bool, error)
	ListAuthorizedModels(ctx context.Context, userID, groupID int64, endpoint string) ([]string, error)
}

type UserModelPermissionService struct {
	repo UserModelPermissionRepository
}

func NewUserModelPermissionService(repo UserModelPermissionRepository) *UserModelPermissionService {
	return &UserModelPermissionService{repo: repo}
}

func (s *UserModelPermissionService) GetSnapshot(ctx context.Context, userID int64) (*UserModelPermissionSnapshot, error) {
	if userID <= 0 {
		return nil, ErrUserNotFound
	}
	if s == nil || s.repo == nil {
		return nil, ErrModelPermissionUnavailable
	}
	return s.repo.GetSnapshot(ctx, userID)
}

func (s *UserModelPermissionService) Replace(ctx context.Context, input ReplaceUserModelPermissionsInput) (*UserModelPermissionSnapshot, error) {
	if input.UserID <= 0 {
		return nil, ErrUserNotFound
	}
	if input.ActorAdminID <= 0 {
		return nil, ErrModelPermissionActorInvalid
	}
	input.ExpectedVersion = normalizePermissionVersion(input.ExpectedVersion)
	if input.ExpectedVersion == "" {
		return nil, ErrModelPermissionVersionRequired
	}
	if len(input.RouteIDs) > 1000 {
		return nil, infraerrors.BadRequest("MODEL_PERMISSION_ROUTE_LIMIT", "at most 1000 routes may be assigned")
	}
	seen := make(map[int64]struct{}, len(input.RouteIDs))
	for _, routeID := range input.RouteIDs {
		if routeID <= 0 {
			return nil, ErrModelPermissionRouteInvalid
		}
		if _, ok := seen[routeID]; ok {
			return nil, infraerrors.BadRequest("MODEL_PERMISSION_ROUTE_DUPLICATE", fmt.Sprintf("duplicate route id: %d", routeID))
		}
		seen[routeID] = struct{}{}
	}
	input.RouteIDs = append([]int64(nil), input.RouteIDs...)
	sort.Slice(input.RouteIDs, func(i, j int) bool { return input.RouteIDs[i] < input.RouteIDs[j] })
	if s == nil || s.repo == nil {
		return nil, ErrModelPermissionUnavailable
	}
	return s.repo.Replace(ctx, input)
}

func (s *UserModelPermissionService) IsAllowed(ctx context.Context, userID, routeID int64) (bool, error) {
	if userID <= 0 || routeID <= 0 {
		return false, nil
	}
	if s == nil || s.repo == nil {
		return false, ErrModelPermissionUnavailable
	}
	return s.repo.IsAllowed(ctx, userID, routeID)
}

func (s *UserModelPermissionService) ListAuthorizedModels(ctx context.Context, userID, groupID int64, endpoint string) ([]string, error) {
	if userID <= 0 || groupID <= 0 {
		return []string{}, nil
	}
	if s == nil || s.repo == nil {
		return nil, ErrModelPermissionUnavailable
	}
	return s.repo.ListAuthorizedModels(ctx, userID, groupID, normalizeCompositeRouteEndpoint(endpoint))
}

func normalizePermissionVersion(version string) string {
	version = strings.TrimSpace(version)
	return strings.Trim(strings.TrimSpace(version), `"`)
}
