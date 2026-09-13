-- M6.3: explicit per-user authorization for public Composite model routes.
-- Absence of an enabled row is a deny decision. Permission rows are retained
-- when disabled so administrative replacements remain auditable.
CREATE TABLE IF NOT EXISTS user_model_permissions (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    route_id BIGINT NOT NULL REFERENCES composite_model_routes(id) ON DELETE RESTRICT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, route_id)
);

CREATE INDEX IF NOT EXISTS idx_user_model_permissions_route_enabled
    ON user_model_permissions (route_id, enabled);

CREATE INDEX IF NOT EXISTS idx_user_model_permissions_user_enabled
    ON user_model_permissions (user_id, enabled);

-- Once a route has ever been referenced by a permission, changing its
-- authorization meaning in place is forbidden. Operational fields remain
-- mutable: enabled, priority, notes and updated_at. target_group_id is added
-- by a later forward migration and must be included here at that time.
CREATE OR REPLACE FUNCTION prevent_authorized_composite_route_semantic_change()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF EXISTS (
        SELECT 1
          FROM user_model_permissions
         WHERE route_id = OLD.id
    ) AND (
        NEW.group_id IS DISTINCT FROM OLD.group_id
        OR NEW.public_model IS DISTINCT FROM OLD.public_model
        OR NEW.match_type IS DISTINCT FROM OLD.match_type
        OR NEW.target_platform IS DISTINCT FROM OLD.target_platform
        OR NEW.upstream_model IS DISTINCT FROM OLD.upstream_model
        OR NEW.endpoint IS DISTINCT FROM OLD.endpoint
        OR NEW.deleted_at IS DISTINCT FROM OLD.deleted_at
    ) THEN
        RAISE EXCEPTION 'authorized composite route semantics are immutable; create a new route and replace permissions'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_composite_route_authorization_semantics ON composite_model_routes;
CREATE TRIGGER trg_composite_route_authorization_semantics
BEFORE UPDATE ON composite_model_routes
FOR EACH ROW EXECUTE FUNCTION prevent_authorized_composite_route_semantic_change();
