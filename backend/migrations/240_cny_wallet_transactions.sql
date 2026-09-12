-- M6.1: immutable CNY wallet ledger foundation.
-- users.balance remains the authoritative current balance. New wallet-aware
-- writers must update users.balance and insert one ledger row in one transaction.
CREATE TABLE IF NOT EXISTS wallet_transactions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    type VARCHAR(24) NOT NULL,
    amount NUMERIC(20,8) NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'CNY',
    balance_before NUMERIC(20,8) NOT NULL,
    balance_after NUMERIC(20,8) NOT NULL,
    reference_type VARCHAR(32) NOT NULL,
    reference_id VARCHAR(128) NOT NULL,
    idempotency_key VARCHAR(160) NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    operator_id BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
    source VARCHAR(64) NOT NULL,
    note VARCHAR(500) NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT wallet_transactions_type_check
        CHECK (type IN ('recharge', 'usage', 'refund', 'adjustment')),
    CONSTRAINT wallet_transactions_amount_sign_check
        CHECK (
            (type IN ('recharge', 'refund') AND amount > 0)
            OR (type = 'usage' AND amount <= 0)
            OR type = 'adjustment'
        ),
    CONSTRAINT wallet_transactions_currency_check
        CHECK (currency = 'CNY'),
    CONSTRAINT wallet_transactions_balance_equation_check
        CHECK (balance_after = balance_before + amount),
    CONSTRAINT wallet_transactions_required_text_check
        CHECK (
            btrim(reference_type) <> ''
            AND btrim(reference_id) <> ''
            AND btrim(idempotency_key) <> ''
            AND btrim(source) <> ''
        ),
    CONSTRAINT wallet_transactions_fingerprint_check
        CHECK (request_fingerprint ~ '^[0-9a-f]{64}$'),
    CONSTRAINT wallet_transactions_idempotency_key_key
        UNIQUE (idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_wallet_transactions_user_created
    ON wallet_transactions (user_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_wallet_transactions_reference
    ON wallet_transactions (reference_type, reference_id);

-- Ledger rows are corrections-by-compensation only. Even the application table
-- owner must not rewrite or delete history accidentally.
CREATE OR REPLACE FUNCTION reject_wallet_transaction_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'wallet_transactions are immutable; write a compensating transaction'
        USING ERRCODE = '55000';
END;
$$;

DROP TRIGGER IF EXISTS trg_wallet_transactions_immutable ON wallet_transactions;
CREATE TRIGGER trg_wallet_transactions_immutable
BEFORE UPDATE OR DELETE ON wallet_transactions
FOR EACH ROW EXECUTE FUNCTION reject_wallet_transaction_mutation();

-- This repository currently contains only non-production M0 test balances.
-- The product owner confirmed CNY as the sole ledger currency and approved a
-- numeric 1:1 opening entry. Real USD holdings must never use this shortcut.
INSERT INTO wallet_transactions (
    user_id,
    type,
    amount,
    currency,
    balance_before,
    balance_after,
    reference_type,
    reference_id,
    idempotency_key,
    request_fingerprint,
    source,
    note,
    metadata
)
SELECT
    u.id,
    'adjustment',
    u.balance,
    'CNY',
    0,
    u.balance,
    'opening_balance',
    u.id::text,
    'm6-opening:user:' || u.id::text,
    encode(sha256(convert_to(
        'm6-opening|user=' || u.id::text || '|amount=' || u.balance::numeric(20,8)::text || '|currency=CNY',
        'UTF8'
    )), 'hex'),
    'migration',
    'M6 opening balance; numeric 1:1 CNY decision',
    jsonb_build_object('migration', '240_cny_wallet_transactions.sql')
FROM users AS u
ON CONFLICT (idempotency_key) DO NOTHING;
