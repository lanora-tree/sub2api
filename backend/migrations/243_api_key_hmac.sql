-- M6.4: replace recoverable user API key credentials with versioned HMAC
-- digests. Existing plaintext rows are converted by the application startup
-- backfill after the active Pepper has passed runtime validation.

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS key_hash VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS key_prefix VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS key_last_four VARCHAR(4) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS key_version INTEGER NOT NULL DEFAULT 0;

-- Drop the historical plaintext uniqueness regardless of the constraint name
-- selected by an older schema generator.
DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    FOR constraint_name IN
        SELECT con.conname
          FROM pg_constraint con
          JOIN pg_class rel ON rel.oid = con.conrelid
          JOIN pg_namespace ns ON ns.oid = rel.relnamespace
         WHERE ns.nspname = current_schema()
           AND rel.relname = 'api_keys'
           AND con.contype = 'u'
           AND ARRAY(
                SELECT att.attname
                  FROM unnest(con.conkey) WITH ORDINALITY AS cols(attnum, ord)
                  JOIN pg_attribute att
                    ON att.attrelid = con.conrelid
                   AND att.attnum = cols.attnum
                 ORDER BY cols.ord
           ) = ARRAY['key']::name[]
    LOOP
        EXECUTE format('ALTER TABLE api_keys DROP CONSTRAINT %I', constraint_name);
    END LOOP;
END;
$$;

DROP INDEX IF EXISTS idx_api_keys_key;
DROP INDEX IF EXISTS api_keys_key;
DROP INDEX IF EXISTS idx_api_keys_key_trgm;

CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_key_hash_active
    ON api_keys (key_hash)
    WHERE key_hash <> '';

-- Auth-cache invalidations now use the active/previous HMAC digest directly.
-- Rebuild every trigger body that historically derived SHA-256 from plaintext.
CREATE OR REPLACE FUNCTION enqueue_auth_cache_invalidation_digest(cache_key_digest TEXT)
RETURNS VOID
LANGUAGE plpgsql
AS $$
BEGIN
    IF cache_key_digest IS NULL OR cache_key_digest = '' THEN
        RETURN;
    END IF;
    IF cache_key_digest !~ '^[0-9a-f]{64}$' THEN
        RAISE EXCEPTION 'invalid API key HMAC digest for cache invalidation';
    END IF;
    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    VALUES (cache_key_digest);
END;
$$;

CREATE OR REPLACE FUNCTION enqueue_api_key_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM enqueue_auth_cache_invalidation_digest(OLD.key_hash);
        RETURN OLD;
    END IF;

    IF OLD.key_hash IS DISTINCT FROM NEW.key_hash
       OR OLD.status IS DISTINCT FROM NEW.status
       OR OLD.deleted_at IS DISTINCT FROM NEW.deleted_at
       OR OLD.user_id IS DISTINCT FROM NEW.user_id
       OR OLD.group_id IS DISTINCT FROM NEW.group_id
       OR OLD.ip_whitelist IS DISTINCT FROM NEW.ip_whitelist
       OR OLD.ip_blacklist IS DISTINCT FROM NEW.ip_blacklist
       OR OLD.expires_at IS DISTINCT FROM NEW.expires_at THEN
        PERFORM enqueue_auth_cache_invalidation_digest(OLD.key_hash);
        IF NEW.deleted_at IS NULL AND NEW.key_hash IS DISTINCT FROM OLD.key_hash THEN
            PERFORM enqueue_auth_cache_invalidation_digest(NEW.key_hash);
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION enqueue_user_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    target_user_id BIGINT;
BEGIN
    target_user_id := OLD.id;
    IF TG_OP = 'UPDATE'
       AND OLD.status IS NOT DISTINCT FROM NEW.status
       AND OLD.role IS NOT DISTINCT FROM NEW.role
       AND OLD.deleted_at IS NOT DISTINCT FROM NEW.deleted_at THEN
        RETURN NEW;
    END IF;

    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT k.key_hash
    FROM api_keys AS k
    WHERE k.user_id = target_user_id
      AND k.deleted_at IS NULL
      AND k.key_hash <> '';
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION enqueue_group_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    target_group_id BIGINT;
BEGIN
    target_group_id := OLD.id;
    IF TG_OP = 'UPDATE'
       AND OLD.status IS NOT DISTINCT FROM NEW.status
       AND OLD.is_exclusive IS NOT DISTINCT FROM NEW.is_exclusive
       AND OLD.allow_image_generation IS NOT DISTINCT FROM NEW.allow_image_generation
       AND OLD.platform IS NOT DISTINCT FROM NEW.platform
       AND OLD.subscription_type IS NOT DISTINCT FROM NEW.subscription_type
       AND OLD.rate_multiplier IS NOT DISTINCT FROM NEW.rate_multiplier
       AND OLD.peak_rate_enabled IS NOT DISTINCT FROM NEW.peak_rate_enabled
       AND OLD.peak_start IS NOT DISTINCT FROM NEW.peak_start
       AND OLD.peak_end IS NOT DISTINCT FROM NEW.peak_end
       AND OLD.peak_rate_multiplier IS NOT DISTINCT FROM NEW.peak_rate_multiplier
       AND OLD.profit_control_enabled IS NOT DISTINCT FROM NEW.profit_control_enabled
       AND OLD.profit_min_margin IS NOT DISTINCT FROM NEW.profit_min_margin
       AND OLD.profit_safety_buffer IS NOT DISTINCT FROM NEW.profit_safety_buffer
       AND OLD.deleted_at IS NOT DISTINCT FROM NEW.deleted_at THEN
        RETURN NEW;
    END IF;

    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT k.key_hash
    FROM api_keys AS k
    WHERE k.group_id = target_group_id
      AND k.deleted_at IS NULL
      AND k.key_hash <> '';
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION enqueue_allowed_group_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    target_user_id BIGINT;
    target_group_id BIGINT;
BEGIN
    IF TG_OP = 'UPDATE'
       AND (OLD.user_id IS DISTINCT FROM NEW.user_id
            OR OLD.group_id IS DISTINCT FROM NEW.group_id) THEN
        IF EXISTS (
            SELECT 1 FROM groups g
            WHERE g.id = OLD.group_id AND g.is_exclusive = TRUE
        ) THEN
            INSERT INTO auth_cache_invalidation_outbox (cache_key)
            SELECT k.key_hash
            FROM api_keys AS k
            WHERE k.user_id = OLD.user_id
              AND k.group_id = OLD.group_id
              AND k.deleted_at IS NULL
              AND k.key_hash <> '';
        END IF;
        target_user_id := NEW.user_id;
        target_group_id := NEW.group_id;
    ELSIF TG_OP = 'UPDATE' THEN
        RETURN NEW;
    ELSIF TG_OP = 'INSERT' THEN
        target_user_id := NEW.user_id;
        target_group_id := NEW.group_id;
    ELSE
        target_user_id := OLD.user_id;
        target_group_id := OLD.group_id;
    END IF;

    IF EXISTS (
        SELECT 1 FROM groups g
        WHERE g.id = target_group_id AND g.is_exclusive = TRUE
    ) THEN
        INSERT INTO auth_cache_invalidation_outbox (cache_key)
        SELECT k.key_hash
        FROM api_keys AS k
        WHERE k.user_id = target_user_id
          AND k.group_id = target_group_id
          AND k.deleted_at IS NULL
          AND k.key_hash <> '';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

DROP FUNCTION IF EXISTS enqueue_auth_cache_invalidation(TEXT);

COMMENT ON TABLE auth_cache_invalidation_outbox IS
    'Durable cross-instance auth cache invalidations; cache_key is a versioned API-key HMAC digest';

-- NOT VALID preserves legacy rows until the startup backfill updates them,
-- while immediately rejecting any new plaintext write from an old binary.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
          FROM pg_constraint
         WHERE conrelid = 'api_keys'::regclass
           AND conname = 'api_keys_hmac_storage_check'
    ) THEN
        ALTER TABLE api_keys
            ADD CONSTRAINT api_keys_hmac_storage_check CHECK (
                key = ''
                AND (
                    deleted_at IS NOT NULL
                    OR (
                        key_hash ~ '^[0-9a-f]{64}$'
                        AND key_prefix <> ''
                        AND length(key_last_four) = 4
                        AND key_version > 0
                    )
                )
            ) NOT VALID;
    END IF;
END;
$$;

-- This legacy table is no longer written. Eradicate any credential material
-- that older releases may have retained without introducing a new copy.
UPDATE deleted_api_key_audits SET key = '' WHERE key <> '';
DROP INDEX IF EXISTS deletedapikeyaudit_key;

COMMENT ON COLUMN api_keys.key IS 'Deprecated plaintext credential column; must remain empty';
COMMENT ON COLUMN api_keys.key_hash IS 'Lowercase HMAC-SHA256 digest of the credential';
COMMENT ON COLUMN api_keys.key_prefix IS 'Non-secret display prefix';
COMMENT ON COLUMN api_keys.key_last_four IS 'Non-secret final four characters';
COMMENT ON COLUMN api_keys.key_version IS 'HMAC Pepper version used for key_hash';
