package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const userModelPermissionVersionDomain = "sub2api:user-model-permissions:v1\n"

type userModelPermissionRepository struct {
	db *sql.DB
}

func NewUserModelPermissionRepository(db *sql.DB) service.UserModelPermissionRepository {
	return &userModelPermissionRepository{db: db}
}

type userModelPermissionQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r *userModelPermissionRepository) GetSnapshot(ctx context.Context, userID int64) (*service.UserModelPermissionSnapshot, error) {
	if r == nil || r.db == nil {
		return nil, service.ErrModelPermissionUnavailable
	}
	return getUserModelPermissionSnapshot(ctx, r.db, userID, true)
}

func (r *userModelPermissionRepository) Replace(ctx context.Context, input service.ReplaceUserModelPermissionsInput) (*service.UserModelPermissionSnapshot, error) {
	if r == nil || r.db == nil {
		return nil, service.ErrModelPermissionUnavailable
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin user model permission replacement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('user-model-permissions:' || $1::text, 0))`, input.UserID); err != nil {
		return nil, fmt.Errorf("lock user model permissions: %w", err)
	}

	if err = requirePermissionTargetUser(ctx, tx, input.UserID); err != nil {
		return nil, err
	}
	if err = requirePermissionAdminActor(ctx, tx, input.ActorAdminID); err != nil {
		return nil, err
	}

	currentIDs, err := listEnabledPermissionRouteIDs(ctx, tx, input.UserID)
	if err != nil {
		return nil, err
	}
	if permissionVersion(currentIDs) != input.ExpectedVersion {
		return nil, service.ErrModelPermissionVersionMismatch
	}
	if err = validatePermissionRoutes(ctx, tx, input.RouteIDs); err != nil {
		return nil, err
	}

	if _, err = tx.ExecContext(ctx, `
		UPDATE user_model_permissions
		   SET enabled = FALSE,
		       updated_at = NOW()
		 WHERE user_id = $1
		   AND enabled = TRUE
		   AND NOT (route_id = ANY($2::bigint[]))
	`, input.UserID, pq.Array(input.RouteIDs)); err != nil {
		return nil, fmt.Errorf("disable removed user model permissions: %w", err)
	}
	if len(input.RouteIDs) > 0 {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO user_model_permissions (
				user_id, route_id, enabled, created_by, created_at, updated_at
			)
			SELECT $1, route_id, TRUE, $3, NOW(), NOW()
			  FROM unnest($2::bigint[]) AS route_id
			ON CONFLICT (user_id, route_id) DO UPDATE
			   SET enabled = TRUE,
			       updated_at = NOW()
		`, input.UserID, pq.Array(input.RouteIDs), input.ActorAdminID); err != nil {
			return nil, fmt.Errorf("upsert user model permissions: %w", err)
		}
	}

	snapshot, err := getUserModelPermissionSnapshot(ctx, tx, input.UserID, false)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit user model permission replacement: %w", err)
	}
	return snapshot, nil
}

func (r *userModelPermissionRepository) IsAllowed(ctx context.Context, userID, routeID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, service.ErrModelPermissionUnavailable
	}
	var allowed bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM user_model_permissions AS permission
			  JOIN composite_model_routes AS route ON route.id = permission.route_id
			  JOIN groups AS access_group ON access_group.id = route.group_id
			 WHERE permission.user_id = $1
			   AND permission.route_id = $2
			   AND permission.enabled = TRUE
			   AND route.enabled = TRUE
			   AND route.deleted_at IS NULL
			   AND access_group.deleted_at IS NULL
			   AND access_group.status = 'active'
			   AND access_group.platform = 'composite'
		)
	`, userID, routeID).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("check user model permission: %w", err)
	}
	return allowed, nil
}

func (r *userModelPermissionRepository) ListAuthorizedModels(ctx context.Context, userID, groupID int64, endpoint string) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, service.ErrModelPermissionUnavailable
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT route.public_model
		  FROM user_model_permissions AS permission
		  JOIN composite_model_routes AS route ON route.id = permission.route_id
		  JOIN groups AS access_group ON access_group.id = route.group_id
		 WHERE permission.user_id = $1
		   AND route.group_id = $2
		   AND permission.enabled = TRUE
		   AND route.enabled = TRUE
		   AND route.deleted_at IS NULL
		   AND route.match_type = 'exact'
		   AND access_group.deleted_at IS NULL
		   AND access_group.status = 'active'
		   AND access_group.platform = 'composite'
		   AND ($3 = 'any' OR route.endpoint IN ('any', $3))
		 ORDER BY route.public_model
	`, userID, groupID, endpoint)
	if err != nil {
		return nil, fmt.Errorf("list authorized models: %w", err)
	}
	defer rows.Close()
	models := make([]string, 0)
	for rows.Next() {
		var model string
		if err = rows.Scan(&model); err != nil {
			return nil, fmt.Errorf("scan authorized model: %w", err)
		}
		models = append(models, model)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate authorized models: %w", err)
	}
	return models, nil
}

func requirePermissionTargetUser(ctx context.Context, q userModelPermissionQuerier, userID int64) error {
	var exists bool
	if err := q.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND deleted_at IS NULL)`, userID).Scan(&exists); err != nil {
		return fmt.Errorf("check permission target user: %w", err)
	}
	if !exists {
		return service.ErrUserNotFound
	}
	return nil
}

func requirePermissionAdminActor(ctx context.Context, q userModelPermissionQuerier, actorID int64) error {
	var exists bool
	if err := q.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM users
			 WHERE id = $1
			   AND role = 'admin'
			   AND status = 'active'
			   AND deleted_at IS NULL
		)
	`, actorID).Scan(&exists); err != nil {
		return fmt.Errorf("check permission administrator: %w", err)
	}
	if !exists {
		return service.ErrModelPermissionActorInvalid
	}
	return nil
}

func validatePermissionRoutes(ctx context.Context, q userModelPermissionQuerier, routeIDs []int64) error {
	if len(routeIDs) == 0 {
		return nil
	}
	var count int
	if err := q.QueryRowContext(ctx, `
		SELECT COUNT(*)
		  FROM composite_model_routes AS route
		  JOIN groups AS access_group ON access_group.id = route.group_id
		 WHERE route.id = ANY($1::bigint[])
		   AND route.deleted_at IS NULL
		   AND route.match_type = 'exact'
		   AND access_group.deleted_at IS NULL
		   AND access_group.platform = 'composite'
	`, pq.Array(routeIDs)).Scan(&count); err != nil {
		return fmt.Errorf("validate permission routes: %w", err)
	}
	if count != len(routeIDs) {
		return service.ErrModelPermissionRouteInvalid
	}
	return nil
}

func getUserModelPermissionSnapshot(ctx context.Context, q userModelPermissionQuerier, userID int64, checkUser bool) (*service.UserModelPermissionSnapshot, error) {
	if checkUser {
		if err := requirePermissionTargetUser(ctx, q, userID); err != nil {
			return nil, err
		}
	}
	rows, err := q.QueryContext(ctx, `
		SELECT permission.user_id,
		       permission.route_id,
		       permission.enabled,
		       permission.enabled
		           AND route.enabled
		           AND route.deleted_at IS NULL
		           AND route.match_type = 'exact'
		           AND access_group.deleted_at IS NULL
		           AND access_group.status = 'active'
		           AND access_group.platform = 'composite' AS effective,
		       permission.created_by,
		       permission.created_at,
		       permission.updated_at,
		       route.group_id,
		       route.public_model,
		       route.match_type,
		       route.target_platform,
		       route.upstream_model,
		       route.endpoint,
		       route.priority,
		       route.enabled,
		       route.notes,
		       route.created_at,
		       route.updated_at
		  FROM user_model_permissions AS permission
		  JOIN composite_model_routes AS route ON route.id = permission.route_id
		  JOIN groups AS access_group ON access_group.id = route.group_id
		 WHERE permission.user_id = $1
		 ORDER BY route.public_model, route.endpoint, permission.route_id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user model permissions: %w", err)
	}
	defer rows.Close()
	permissions := make([]service.UserModelPermission, 0)
	enabledIDs := make([]int64, 0)
	for rows.Next() {
		var permission service.UserModelPermission
		var notes sql.NullString
		if err = rows.Scan(
			&permission.UserID,
			&permission.RouteID,
			&permission.Enabled,
			&permission.Effective,
			&permission.CreatedBy,
			&permission.CreatedAt,
			&permission.UpdatedAt,
			&permission.Route.GroupID,
			&permission.Route.PublicModel,
			&permission.Route.MatchType,
			&permission.Route.TargetPlatform,
			&permission.Route.UpstreamModel,
			&permission.Route.Endpoint,
			&permission.Route.Priority,
			&permission.Route.Enabled,
			&notes,
			&permission.Route.CreatedAt,
			&permission.Route.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan user model permission: %w", err)
		}
		permission.Route.ID = permission.RouteID
		if notes.Valid {
			permission.Route.Notes = notes.String
		}
		if permission.Enabled {
			enabledIDs = append(enabledIDs, permission.RouteID)
		}
		permissions = append(permissions, permission)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user model permissions: %w", err)
	}
	return &service.UserModelPermissionSnapshot{
		UserID:      userID,
		Version:     permissionVersion(enabledIDs),
		Permissions: permissions,
	}, nil
}

func listEnabledPermissionRouteIDs(ctx context.Context, q userModelPermissionQuerier, userID int64) ([]int64, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT route_id
		  FROM user_model_permissions
		 WHERE user_id = $1 AND enabled = TRUE
		 ORDER BY route_id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list enabled permission routes: %w", err)
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan enabled permission route: %w", err)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enabled permission routes: %w", err)
	}
	return ids, nil
}

func permissionVersion(routeIDs []int64) string {
	sorted := append([]int64(nil), routeIDs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	var payload strings.Builder
	payload.WriteString(userModelPermissionVersionDomain)
	for _, routeID := range sorted {
		payload.WriteString(strconv.FormatInt(routeID, 10))
		payload.WriteByte('\n')
	}
	digest := sha256.Sum256([]byte(payload.String()))
	return hex.EncodeToString(digest[:])
}

var _ service.UserModelPermissionRepository = (*userModelPermissionRepository)(nil)
