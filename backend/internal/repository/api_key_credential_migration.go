package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apikeyhmac"
)

const apiKeyCredentialMigrationAdvisoryLock int64 = 0x535542324150494B // "SUB2APIK"

type legacyAPIKeyCredential struct {
	id        int64
	raw       string
	deletedAt sql.NullTime
}

// migrateAPIKeyCredentials performs the irreversible plaintext-to-HMAC
// backfill under one PostgreSQL transaction and advisory lock. It runs after
// migration 243 has installed a NOT VALID constraint that blocks new plaintext
// writes, and validates that constraint only after every legacy row is clean.
func migrateAPIKeyCredentials(ctx context.Context, db *sql.DB, cfg *config.Config, now time.Time) (err error) {
	if db == nil {
		return fmt.Errorf("migrate api key credentials: database is required")
	}
	keyring, err := cfg.APIKeyHMAC.Keyring()
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin api key credential migration: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, apiKeyCredentialMigrationAdvisoryLock); err != nil {
		return fmt.Errorf("lock api key credential migration: %w", err)
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id, key, deleted_at
		FROM api_keys
		WHERE key <> ''
		ORDER BY id
		FOR UPDATE`)
	if err != nil {
		return fmt.Errorf("select legacy api key credentials: %w", err)
	}
	credentials := make([]legacyAPIKeyCredential, 0)
	for rows.Next() {
		var credential legacyAPIKeyCredential
		if scanErr := rows.Scan(&credential.id, &credential.raw, &credential.deletedAt); scanErr != nil {
			_ = rows.Close()
			return fmt.Errorf("scan legacy api key credential: %w", scanErr)
		}
		credentials = append(credentials, credential)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate legacy api key credentials: %w", err)
	}
	if err = rows.Close(); err != nil {
		return fmt.Errorf("close legacy api key credentials: %w", err)
	}

	for _, credential := range credentials {
		if credential.deletedAt.Valid {
			if _, err = tx.ExecContext(ctx, `
				UPDATE api_keys
				SET key = '', key_hash = '', key_version = 0, updated_at = NOW()
				WHERE id = $1`, credential.id); err != nil {
				return fmt.Errorf("clear deleted api key credential: %w", err)
			}
			continue
		}

		active := keyring.Active(credential.raw)
		prefix, lastFour := apikeyhmac.DisplayMetadata(credential.raw)
		if _, err = tx.ExecContext(ctx, `
			UPDATE api_keys
			SET key = '', key_hash = $1, key_prefix = $2, key_last_four = $3,
				key_version = $4, updated_at = NOW()
			WHERE id = $5`, active.Digest, prefix, lastFour, active.Version, credential.id); err != nil {
			return fmt.Errorf("backfill api key credential id %d: %w", credential.id, err)
		}
	}

	// Deleted credentials never need to remain addressable, so release their
	// digest even if they were already migrated by a previous release.
	if _, err = tx.ExecContext(ctx, `
		UPDATE api_keys
		SET key = '', key_hash = '', key_version = 0, updated_at = NOW()
		WHERE deleted_at IS NOT NULL AND (key <> '' OR key_hash <> '' OR key_version <> 0)`); err != nil {
		return fmt.Errorf("clear deleted api key digests: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE deleted_api_key_audits SET key = '' WHERE key <> ''`); err != nil {
		return fmt.Errorf("clear legacy deleted api key audit credentials: %w", err)
	}

	if keyring.PreviousAccepted(now) {
		if _, err = tx.ExecContext(ctx, `
			UPDATE api_keys
			SET status = 'disabled', updated_at = NOW()
			WHERE deleted_at IS NULL
			  AND key_version NOT IN ($1, $2)
			  AND status <> 'disabled'`, keyring.ActiveVersion(), keyring.PreviousVersion()); err != nil {
			return fmt.Errorf("disable unknown api key HMAC versions: %w", err)
		}
	} else {
		if _, err = tx.ExecContext(ctx, `
			UPDATE api_keys
			SET status = 'disabled', updated_at = NOW()
			WHERE deleted_at IS NULL
			  AND key_version <> $1
			  AND status <> 'disabled'`, keyring.ActiveVersion()); err != nil {
			return fmt.Errorf("disable expired api key HMAC versions: %w", err)
		}
	}

	if _, err = tx.ExecContext(ctx, `ALTER TABLE api_keys VALIDATE CONSTRAINT api_keys_hmac_storage_check`); err != nil {
		return fmt.Errorf("validate api key HMAC storage constraint: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit api key credential migration: %w", err)
	}
	return nil
}
