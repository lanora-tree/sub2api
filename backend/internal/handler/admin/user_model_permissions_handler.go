package admin

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type replaceUserModelPermissionsRequest struct {
	Version  json.RawMessage   `json:"version"`
	RouteIDs []json.RawMessage `json:"route_ids"`
}

type userModelPermissionResponse struct {
	RouteID        string `json:"route_id"`
	Enabled        bool   `json:"enabled"`
	Effective      bool   `json:"effective"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	AccessGroupID  string `json:"access_group_id"`
	PublicModel    string `json:"public_model"`
	MatchType      string `json:"match_type"`
	TargetPlatform string `json:"target_platform"`
	UpstreamModel  string `json:"upstream_model"`
	Endpoint       string `json:"endpoint"`
	RouteEnabled   bool   `json:"route_enabled"`
}

type userModelPermissionSnapshotResponse struct {
	UserID      string                        `json:"user_id"`
	Version     string                        `json:"version"`
	RouteIDs    []string                      `json:"route_ids"`
	Permissions []userModelPermissionResponse `json:"permissions"`
}

// GetModelPermissions returns the complete permission state and a strong ETag.
func (h *UserHandler) GetModelPermissions(c *gin.Context) {
	userID, err := parsePositiveInt64(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	if h.modelPermissionService == nil {
		response.ErrorFrom(c, service.ErrModelPermissionUnavailable)
		return
	}
	snapshot, err := h.modelPermissionService.GetSnapshot(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	writeUserModelPermissionSnapshot(c, snapshot)
}

// ReplaceModelPermissions atomically replaces the enabled route set. The
// caller must present the ETag from GET (or the same opaque version in JSON).
func (h *UserHandler) ReplaceModelPermissions(c *gin.Context) {
	userID, err := parsePositiveInt64(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	var req replaceUserModelPermissionsRequest
	if err = c.ShouldBindJSON(&req); err != nil || req.RouteIDs == nil {
		response.BadRequest(c, "Invalid request: route_ids must be an array")
		return
	}
	routeIDs := make([]int64, 0, len(req.RouteIDs))
	for _, raw := range req.RouteIDs {
		routeID, parseErr := parseJSONInt64(raw)
		if parseErr != nil || routeID <= 0 {
			response.BadRequest(c, "Invalid route ID")
			return
		}
		routeIDs = append(routeIDs, routeID)
	}
	expectedVersion := strings.TrimSpace(c.GetHeader("If-Match"))
	if strings.HasPrefix(expectedVersion, "W/") {
		response.BadRequest(c, "Invalid version: weak ETags are not supported")
		return
	}
	if expectedVersion == "" && len(bytes.TrimSpace(req.Version)) > 0 {
		expectedVersion, err = parseJSONVersion(req.Version)
		if err != nil {
			response.BadRequest(c, "Invalid version")
			return
		}
	}
	if h.modelPermissionService == nil {
		response.ErrorFrom(c, service.ErrModelPermissionUnavailable)
		return
	}
	snapshot, err := h.modelPermissionService.Replace(c.Request.Context(), service.ReplaceUserModelPermissionsInput{
		UserID:          userID,
		ActorAdminID:    getAdminIDFromContext(c),
		ExpectedVersion: expectedVersion,
		RouteIDs:        routeIDs,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	middleware.SetAuditExtra(c, map[string]any{
		"target_user_id": userID,
		"route_count":    len(routeIDs),
		"version":        snapshot.Version,
	})
	writeUserModelPermissionSnapshot(c, snapshot)
}

func writeUserModelPermissionSnapshot(c *gin.Context, snapshot *service.UserModelPermissionSnapshot) {
	if snapshot == nil {
		response.ErrorFrom(c, service.ErrModelPermissionUnavailable)
		return
	}
	c.Header("ETag", `"`+snapshot.Version+`"`)
	out := userModelPermissionSnapshotResponse{
		UserID:      strconv.FormatInt(snapshot.UserID, 10),
		Version:     snapshot.Version,
		RouteIDs:    make([]string, 0),
		Permissions: make([]userModelPermissionResponse, 0, len(snapshot.Permissions)),
	}
	for _, permission := range snapshot.Permissions {
		if permission.Enabled {
			out.RouteIDs = append(out.RouteIDs, strconv.FormatInt(permission.RouteID, 10))
		}
		out.Permissions = append(out.Permissions, userModelPermissionResponse{
			RouteID:        strconv.FormatInt(permission.RouteID, 10),
			Enabled:        permission.Enabled,
			Effective:      permission.Effective,
			CreatedBy:      strconv.FormatInt(permission.CreatedBy, 10),
			CreatedAt:      permission.CreatedAt.UTC().Format(time.RFC3339Nano),
			UpdatedAt:      permission.UpdatedAt.UTC().Format(time.RFC3339Nano),
			AccessGroupID:  strconv.FormatInt(permission.Route.GroupID, 10),
			PublicModel:    permission.Route.PublicModel,
			MatchType:      permission.Route.MatchType,
			TargetPlatform: permission.Route.TargetPlatform,
			UpstreamModel:  permission.Route.UpstreamModel,
			Endpoint:       permission.Route.Endpoint,
			RouteEnabled:   permission.Route.Enabled,
		})
	}
	response.Success(c, out)
}

func parsePositiveInt64(value string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, strconv.ErrSyntax
	}
	return id, nil
}

func parseJSONInt64(raw json.RawMessage) (int64, error) {
	value := strings.TrimSpace(string(raw))
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return 0, err
		}
		value = decoded
	}
	return strconv.ParseInt(value, 10, 64)
}

func parseJSONVersion(raw json.RawMessage) (string, error) {
	var version string
	if err := json.Unmarshal(raw, &version); err == nil {
		return strings.TrimSpace(version), nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return "", err
	}
	return number.String(), nil
}
