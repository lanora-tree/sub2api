-- M6.5: encrypt complete upstream account credential documents with an
-- independent, versioned AES-256-GCM runtime key. Application startup performs
-- the authenticated backfill before validating the storage constraint.

ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS credentials_encrypted TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS credentials_key_version SMALLINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS credentials_aad_id UUID,
    ADD COLUMN IF NOT EXISTS credentials_fingerprint VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS credentials_api_key_digest VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS credentials_has_refresh_token BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS credentials_meta JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_accounts_credentials_api_key_digest
    ON accounts (credentials_api_key_digest)
    WHERE credentials_api_key_digest <> '' AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_accounts_credentials_refresh_candidates
    ON accounts (platform, type, id)
    WHERE credentials_has_refresh_token AND deleted_at IS NULL;

ALTER TABLE accounts
    DROP CONSTRAINT IF EXISTS accounts_credentials_encrypted_storage_check;

ALTER TABLE accounts
    ADD CONSTRAINT accounts_credentials_encrypted_storage_check CHECK (
        credentials = '{}'::jsonb
        AND credentials_encrypted <> ''
        AND credentials_key_version > 0
        AND credentials_aad_id IS NOT NULL
        AND credentials_fingerprint ~ '^[0-9a-f]{64}$'
        AND (
            credentials_api_key_digest = ''
            OR credentials_api_key_digest ~ '^[0-9a-f]{64}$'
        )
        AND jsonb_typeof(credentials_meta) = 'object'
    ) NOT VALID;
