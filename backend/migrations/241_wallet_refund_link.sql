-- M6.2: bind every refund to exactly one original usage transaction.
-- HTTP idempotency protects retries of one request; this database constraint
-- independently prevents a second refund with a different request key.
ALTER TABLE wallet_transactions
    ADD COLUMN IF NOT EXISTS reverses_transaction_id BIGINT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'wallet_transactions'::regclass
          AND conname = 'wallet_transactions_reverses_transaction_id_fkey'
    ) THEN
        ALTER TABLE wallet_transactions
            ADD CONSTRAINT wallet_transactions_reverses_transaction_id_fkey
            FOREIGN KEY (reverses_transaction_id)
            REFERENCES wallet_transactions(id)
            ON DELETE RESTRICT;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'wallet_transactions'::regclass
          AND conname = 'wallet_transactions_refund_link_check'
    ) THEN
        ALTER TABLE wallet_transactions
            ADD CONSTRAINT wallet_transactions_refund_link_check
            CHECK (
                (type = 'refund' AND reverses_transaction_id IS NOT NULL)
                OR (type <> 'refund' AND reverses_transaction_id IS NULL)
            ) NOT VALID;
    END IF;
END
$$;

ALTER TABLE wallet_transactions
    VALIDATE CONSTRAINT wallet_transactions_refund_link_check;

CREATE UNIQUE INDEX IF NOT EXISTS idx_wallet_transactions_one_refund_per_usage
    ON wallet_transactions (reverses_transaction_id)
    WHERE type = 'refund';

CREATE OR REPLACE FUNCTION validate_wallet_refund_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    original_user_id BIGINT;
    original_type VARCHAR(24);
    original_amount NUMERIC(20,8);
    original_currency CHAR(3);
BEGIN
    IF NEW.type <> 'refund' THEN
        RETURN NEW;
    END IF;

    SELECT user_id, type, amount, currency
      INTO original_user_id, original_type, original_amount, original_currency
      FROM wallet_transactions
     WHERE id = NEW.reverses_transaction_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'refund target wallet transaction does not exist'
            USING ERRCODE = '23503';
    END IF;
    IF original_user_id <> NEW.user_id
       OR original_type <> 'usage'
       OR original_amount >= 0
       OR original_currency <> 'CNY'
       OR NEW.amount <> -original_amount THEN
        RAISE EXCEPTION 'refund must fully reverse a negative CNY usage transaction for the same user'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_wallet_refund_validate_insert ON wallet_transactions;
CREATE TRIGGER trg_wallet_refund_validate_insert
BEFORE INSERT ON wallet_transactions
FOR EACH ROW EXECUTE FUNCTION validate_wallet_refund_insert();
