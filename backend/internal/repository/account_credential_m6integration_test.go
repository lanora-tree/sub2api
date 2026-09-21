//go:build m6integration

package repository

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/accountcredential"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestAccountCredentialMigrationOnPostgreSQL(t *testing.T) {
	dsn := os.Getenv("SUB2API_M6_ACCOUNT_CREDENTIAL_TEST_DSN")
	if dsn == "" {
		t.Skip("SUB2API_M6_ACCOUNT_CREDENTIAL_TEST_DSN is not set")
	}

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	require.NoError(t, db.PingContext(ctx))

	_, err = db.ExecContext(ctx, `
CREATE TABLE accounts (
    id BIGSERIAL PRIMARY KEY,
    platform VARCHAR(50) NOT NULL,
    type VARCHAR(20) NOT NULL,
    credentials JSONB NOT NULL DEFAULT '{}'::jsonb,
    deleted_at TIMESTAMPTZ
);
INSERT INTO accounts (platform, type, credentials) VALUES
    ('openai', 'apikey', '{"api_key":"sk-provider","base_url":"https://ollama.com","unknown_token":"secret"}'),
    ('openai', 'oauth', '{"access_token":"access-secret","refresh_token":"refresh-secret"}'),
    ('anthropic', 'cookie', '{"cookie":"session-secret"}');
`)
	require.NoError(t, err)

	migrationSQL, err := dbmigrations.FS.ReadFile("244_account_credentials_encryption.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	// NOT VALID keeps legacy rows available for the startup backfill but blocks
	// new plaintext writes from an old application image immediately.
	_, err = db.ExecContext(ctx, `
INSERT INTO accounts (platform, type, credentials)
VALUES ('deepseek', 'apikey', '{"api_key":"must-fail"}')`)
	require.Error(t, err)

	oldKey := strings.Repeat("11", 32)
	oldConfig := &config.Config{AccountCredentials: config.AccountCredentialsConfig{
		ActiveVersion: 4,
		ActiveKey:     oldKey,
	}}
	now := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, migrateAccountCredentials(ctx, db, oldConfig, now))
	require.NoError(t, migrateAccountCredentials(ctx, db, oldConfig, now))

	oldKeyring, err := accountcredential.NewKeyring(4, oldKey, 0, "", "")
	require.NoError(t, err)
	var (
		legacyJSON      []byte
		ciphertext      string
		keyVersion      int
		aadID           string
		fingerprint     string
		apiKeyDigest    string
		hasRefreshToken bool
		metadataJSON    []byte
	)
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT credentials, credentials_encrypted, credentials_key_version,
       credentials_aad_id::text, credentials_fingerprint,
       credentials_api_key_digest, credentials_has_refresh_token,
       credentials_meta
FROM accounts WHERE id = 1`).Scan(
		&legacyJSON, &ciphertext, &keyVersion, &aadID, &fingerprint,
		&apiKeyDigest, &hasRefreshToken, &metadataJSON,
	))
	require.JSONEq(t, `{}`, string(legacyJSON))
	require.Equal(t, 4, keyVersion)
	require.NotEmpty(t, aadID)
	require.True(t, accountcredential.IsFingerprint(fingerprint))
	require.False(t, hasRefreshToken)
	require.JSONEq(t, `{"base_url":"https://ollama.com"}`, string(metadataJSON))
	lookup, err := oldKeyring.LookupCandidates("api_key", "sk-provider", now)
	require.NoError(t, err)
	require.Equal(t, lookup[0].Fingerprint, apiKeyDigest)
	plaintext, err := oldKeyring.Open(ciphertext, keyVersion, aadID, "openai", now)
	require.NoError(t, err)
	require.JSONEq(t, `{"api_key":"sk-provider","base_url":"https://ollama.com","unknown_token":"secret"}`, string(plaintext))

	var oauthHasRefresh bool
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT credentials_has_refresh_token FROM accounts WHERE id = 2`,
	).Scan(&oauthHasRefresh))
	require.True(t, oauthHasRefresh)

	newKey := strings.Repeat("22", 32)
	rotatingConfig := &config.Config{AccountCredentials: config.AccountCredentialsConfig{
		ActiveVersion:       5,
		ActiveKey:           newKey,
		PreviousVersion:     4,
		PreviousKey:         oldKey,
		PreviousAcceptUntil: now.Add(time.Hour).Format(time.RFC3339),
	}}
	require.NoError(t, migrateAccountCredentials(ctx, db, rotatingConfig, now))
	var rotatedCiphertext string
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT credentials_encrypted, credentials_key_version
FROM accounts WHERE id = 1`).Scan(&rotatedCiphertext, &keyVersion))
	require.Equal(t, 5, keyVersion)
	require.NotEqual(t, ciphertext, rotatedCiphertext)

	newKeyring, err := rotatingConfig.AccountCredentials.Keyring()
	require.NoError(t, err)
	plaintext, err = newKeyring.Open(rotatedCiphertext, keyVersion, aadID, "openai", now)
	require.NoError(t, err)
	require.Contains(t, string(plaintext), "sk-provider")

	var validated bool
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT convalidated FROM pg_constraint
WHERE conrelid = 'accounts'::regclass
  AND conname = 'accounts_credentials_encrypted_storage_check'
`).Scan(&validated))
	require.True(t, validated)
}
