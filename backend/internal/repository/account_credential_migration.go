package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
)

const accountCredentialMigrationAdvisoryLock int64 = 739146205842117430

// migrateAccountCredentials encrypts every legacy or previous-version account
// credential document under one transaction-scoped advisory lock. Constraint
// validation happens only after every row decrypts and re-encrypts successfully.
func migrateAccountCredentials(ctx context.Context, db *sql.DB, cfg *config.Config, now time.Time) error {
	if db == nil {
		return fmt.Errorf("migrate account credentials: database is required")
	}
	if cfg == nil {
		return fmt.Errorf("migrate account credentials: config is required")
	}
	keyring, err := cfg.AccountCredentials.Keyring()
	if err != nil {
		return fmt.Errorf("migrate account credentials: %w", err)
	}
	store, err := newAccountCredentialStore(keyring)
	if err != nil {
		return fmt.Errorf("migrate account credentials: %w", err)
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("migrate account credentials: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, accountCredentialMigrationAdvisoryLock); err != nil {
		return fmt.Errorf("migrate account credentials: acquire advisory lock: %w", err)
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id, platform, credentials, credentials_encrypted,
		       credentials_key_version, credentials_aad_id::text,
		       credentials_fingerprint, credentials_api_key_digest,
		       credentials_has_refresh_token, credentials_meta
		FROM accounts
		ORDER BY id
		FOR UPDATE`)
	if err != nil {
		return fmt.Errorf("migrate account credentials: query accounts: %w", err)
	}
	type pendingUpdate struct {
		id      int64
		columns accountCredentialColumns
	}
	updates := make([]pendingUpdate, 0)
	for rows.Next() {
		var (
			id              int64
			platform        string
			legacyJSON      []byte
			ciphertext      string
			keyVersion      int
			aadText         sql.NullString
			fingerprint     string
			apiKeyDigest    string
			hasRefreshToken bool
			metadataJSON    []byte
		)
		if err := rows.Scan(
			&id, &platform, &legacyJSON, &ciphertext, &keyVersion, &aadText,
			&fingerprint, &apiKeyDigest, &hasRefreshToken, &metadataJSON,
		); err != nil {
			_ = rows.Close()
			return fmt.Errorf("migrate account credentials: scan account: %w", err)
		}

		legacyCredentials, err := decodeCredentialJSON(legacyJSON)
		if err != nil {
			_ = rows.Close()
			return fmt.Errorf("migrate account credentials: account %d legacy JSON: %w", id, err)
		}
		aadID := uuid.Nil
		if aadText.Valid && aadText.String != "" {
			aadID, err = uuid.Parse(aadText.String)
			if err != nil {
				_ = rows.Close()
				return fmt.Errorf("migrate account credentials: account %d aad_id: %w", id, err)
			}
		}
		if aadID == uuid.Nil {
			aadID = uuid.New()
		}
		currentMetadata, err := decodeCredentialJSON(metadataJSON)
		if err != nil {
			_ = rows.Close()
			return fmt.Errorf("migrate account credentials: account %d metadata JSON: %w", id, err)
		}

		credentials := legacyCredentials
		if ciphertext != "" {
			if len(legacyCredentials) != 0 {
				_ = rows.Close()
				return fmt.Errorf("migrate account credentials: account %d has both plaintext and encrypted credentials", id)
			}
			credentials, _, err = store.open(accountCredentialColumns{
				Ciphertext:      ciphertext,
				KeyVersion:      keyVersion,
				AADID:           aadID,
				Fingerprint:     fingerprint,
				APIKeyDigest:    apiKeyDigest,
				HasRefreshToken: hasRefreshToken,
			}, platform, now)
			if err != nil {
				_ = rows.Close()
				return fmt.Errorf("migrate account credentials: account %d decrypt: %w", id, err)
			}
		}
		columns, err := store.seal(credentials, aadID, platform)
		if err != nil {
			_ = rows.Close()
			return fmt.Errorf("migrate account credentials: account %d encrypt: %w", id, err)
		}
		if ciphertext != "" && len(legacyCredentials) == 0 &&
			keyVersion == columns.KeyVersion &&
			fingerprint == columns.Fingerprint &&
			apiKeyDigest == columns.APIKeyDigest &&
			hasRefreshToken == columns.HasRefreshToken &&
			reflect.DeepEqual(currentMetadata, columns.Metadata) {
			continue
		}
		updates = append(updates, pendingUpdate{id: id, columns: columns})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("migrate account credentials: iterate accounts: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("migrate account credentials: close account rows: %w", err)
	}

	for _, update := range updates {
		metadata, err := json.Marshal(update.columns.Metadata)
		if err != nil {
			return fmt.Errorf("migrate account credentials: account %d metadata: %w", update.id, err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE accounts
			SET credentials = '{}'::jsonb,
			    credentials_encrypted = $2,
			    credentials_key_version = $3,
			    credentials_aad_id = $4::uuid,
			    credentials_fingerprint = $5,
			    credentials_api_key_digest = $6,
			    credentials_has_refresh_token = $7,
			    credentials_meta = $8::jsonb
			WHERE id = $1`,
			update.id,
			update.columns.Ciphertext,
			update.columns.KeyVersion,
			update.columns.AADID.String(),
			update.columns.Fingerprint,
			update.columns.APIKeyDigest,
			update.columns.HasRefreshToken,
			string(metadata),
		); err != nil {
			return fmt.Errorf("migrate account credentials: update account %d: %w", update.id, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		ALTER TABLE accounts
		    ALTER COLUMN credentials_aad_id SET NOT NULL;
		ALTER TABLE accounts
		    VALIDATE CONSTRAINT accounts_credentials_encrypted_storage_check`); err != nil {
		return fmt.Errorf("migrate account credentials: validate storage constraint: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migrate account credentials: commit: %w", err)
	}
	return nil
}

func decodeCredentialJSON(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return map[string]any{}, nil
	}
	return value, nil
}
