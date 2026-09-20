//go:build m6integration

package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apikeyhmac"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyCredentialMigrationOnPostgreSQL(t *testing.T) {
	dsn := os.Getenv("SUB2API_M6_API_KEY_TEST_DSN")
	if dsn == "" {
		t.Skip("SUB2API_M6_API_KEY_TEST_DSN is not set")
	}

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	require.NoError(t, db.PingContext(ctx))

	_, err = db.ExecContext(ctx, `
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE users (
    id BIGINT PRIMARY KEY,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    role VARCHAR(20) NOT NULL DEFAULT 'user',
    deleted_at TIMESTAMPTZ
);
CREATE TABLE groups (
    id BIGINT PRIMARY KEY,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    is_exclusive BOOLEAN NOT NULL DEFAULT FALSE,
    allow_image_generation BOOLEAN NOT NULL DEFAULT FALSE,
    platform VARCHAR(50) NOT NULL DEFAULT 'gemini',
    subscription_type VARCHAR(20) NOT NULL DEFAULT 'standard',
    rate_multiplier NUMERIC NOT NULL DEFAULT 1,
    peak_rate_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    peak_start VARCHAR(10) NOT NULL DEFAULT '',
    peak_end VARCHAR(10) NOT NULL DEFAULT '',
    peak_rate_multiplier NUMERIC NOT NULL DEFAULT 1,
    profit_control_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    profit_min_margin NUMERIC NOT NULL DEFAULT 0,
    profit_safety_buffer NUMERIC NOT NULL DEFAULT 0,
    deleted_at TIMESTAMPTZ
);
CREATE TABLE user_allowed_groups (
    user_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL
);
CREATE TABLE api_keys (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    key VARCHAR(128) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    group_id BIGINT,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    ip_whitelist JSONB,
    ip_blacklist JSONB,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_api_keys_key ON api_keys(key);
CREATE INDEX idx_api_keys_key_trgm ON api_keys(key);
CREATE TABLE deleted_api_key_audits (
    id BIGSERIAL PRIMARY KEY,
    key VARCHAR(128) NOT NULL,
    api_key_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    key_name VARCHAR(100) NOT NULL DEFAULT '',
    deleted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX deletedapikeyaudit_key ON deleted_api_key_audits(key);
INSERT INTO users (id) VALUES (10), (11), (12);
INSERT INTO groups (id, is_exclusive) VALUES (20, TRUE);
`)
	require.NoError(t, err)
	for _, migrationName := range []string{
		"184_auth_cache_invalidation_outbox.sql",
		"193_group_profit_control_auth_cache_invalidation.sql",
	} {
		legacyMigration, readErr := dbmigrations.FS.ReadFile(migrationName)
		require.NoError(t, readErr)
		_, execErr := db.ExecContext(ctx, string(legacyMigration))
		require.NoError(t, execErr)
	}

	const activeRaw = "sk-active-legacy-api-key-000001"
	const deletedRaw = "sk-deleted-legacy-api-key-0001"
	var activeID, deletedID int64
	require.NoError(t, db.QueryRowContext(ctx,
		`INSERT INTO api_keys (user_id, key, name, group_id) VALUES (10, $1, 'active', 20) RETURNING id`, activeRaw,
	).Scan(&activeID))
	require.NoError(t, db.QueryRowContext(ctx,
		`INSERT INTO api_keys (user_id, key, name, deleted_at) VALUES (11, $1, 'deleted', NOW()) RETURNING id`, deletedRaw,
	).Scan(&deletedID))
	_, err = db.ExecContext(ctx,
		`INSERT INTO deleted_api_key_audits (key, api_key_id, user_id, key_name) VALUES ($1, $2, 11, 'deleted')`,
		deletedRaw, deletedID,
	)
	require.NoError(t, err)

	migrationSQL, err := dbmigrations.FS.ReadFile("243_api_key_hmac.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	// NOT VALID exempts only pre-existing rows; it must immediately block any
	// plaintext write made by an older application image.
	_, err = db.ExecContext(ctx,
		`INSERT INTO api_keys (user_id, key, name) VALUES (12, 'sk-old-binary-write', 'must-fail')`,
	)
	require.Error(t, err)

	const pepper = "0123456789abcdef0123456789abcdef"
	cfg := &config.Config{APIKeyHMAC: config.APIKeyHMACConfig{
		ActiveVersion: 4,
		ActivePepper:  pepper,
	}}
	require.NoError(t, migrateAPIKeyCredentials(ctx, db, cfg, time.Now()))
	// The migration is restart-safe once every plaintext value has been removed.
	require.NoError(t, migrateAPIKeyCredentials(ctx, db, cfg, time.Now()))

	keyring, err := apikeyhmac.NewKeyring(4, pepper, 0, "", "")
	require.NoError(t, err)
	want := keyring.Active(activeRaw)
	wantPrefix, wantLastFour := apikeyhmac.DisplayMetadata(activeRaw)
	var raw, digest, prefix, lastFour string
	var version int
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT key, key_hash, key_prefix, key_last_four, key_version
FROM api_keys WHERE id = $1`, activeID).Scan(&raw, &digest, &prefix, &lastFour, &version))
	require.Empty(t, raw)
	require.Equal(t, want.Digest, digest)
	require.Equal(t, wantPrefix, prefix)
	require.Equal(t, wantLastFour, lastFour)
	require.Equal(t, 4, version)

	require.NoError(t, db.QueryRowContext(ctx, `
SELECT key, key_hash, key_version FROM api_keys WHERE id = $1`, deletedID,
	).Scan(&raw, &digest, &version))
	require.Empty(t, raw)
	require.Empty(t, digest)
	require.Zero(t, version)

	var auditRaw string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT key FROM deleted_api_key_audits WHERE api_key_id = $1`, deletedID,
	).Scan(&auditRaw))
	require.Empty(t, auditRaw)

	var validated bool
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT convalidated FROM pg_constraint
WHERE conrelid = 'api_keys'::regclass AND conname = 'api_keys_hmac_storage_check'
`).Scan(&validated))
	require.True(t, validated)

	_, err = db.ExecContext(ctx, `DELETE FROM auth_cache_invalidation_outbox`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE api_keys SET status = 'disabled' WHERE id = $1`, activeID)
	require.NoError(t, err)
	var invalidatedDigest string
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT cache_key FROM auth_cache_invalidation_outbox ORDER BY id DESC LIMIT 1
`).Scan(&invalidatedDigest))
	require.Equal(t, want.Digest, invalidatedDigest)
	require.NotEqual(t, activeRaw, invalidatedDigest)

	var plaintextUniqueConstraints int
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM pg_constraint con
JOIN pg_class rel ON rel.oid = con.conrelid
WHERE rel.relname = 'api_keys' AND con.contype = 'u'
  AND pg_get_constraintdef(con.oid) LIKE '%(key)%'
`).Scan(&plaintextUniqueConstraints))
	require.Zero(t, plaintextUniqueConstraints)
}
