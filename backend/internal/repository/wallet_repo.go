package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

type walletRepository struct {
	db *sql.DB
}

func NewWalletRepository(db *sql.DB) service.WalletRepository {
	return &walletRepository{db: db}
}

func (r *walletRepository) Apply(ctx context.Context, cmd *service.WalletMutation) (_ *service.WalletApplyResult, err error) {
	if r == nil || r.db == nil {
		return nil, errors.New("wallet repository db is nil")
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	result, err := r.applyInTx(ctx, tx, cmd)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return result, nil
}

func (r *walletRepository) applyInTx(ctx context.Context, tx *sql.Tx, cmd *service.WalletMutation) (*service.WalletApplyResult, error) {
	prepared, metadataJSON, fingerprint, err := service.PrepareWalletMutation(cmd)
	if err != nil {
		return nil, err
	}

	// A transaction-scoped advisory lock makes the check-then-insert path safe
	// for concurrent retries without retaining a lock after commit/rollback.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, prepared.IdempotencyKey); err != nil {
		return nil, err
	}

	existing, err := findWalletTransactionByIdempotencyKey(ctx, tx, prepared.IdempotencyKey)
	if err == nil {
		if existing.RequestFingerprint != fingerprint {
			return nil, service.ErrWalletIdempotencyConflict
		}
		return &service.WalletApplyResult{Applied: false, Transaction: existing}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	if prepared.Type == service.WalletTransactionRefund {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("wallet-refund:%d", *prepared.ReversesTransactionID)); err != nil {
			return nil, err
		}
		if err := validateWalletRefundTarget(ctx, tx, prepared); err != nil {
			return nil, err
		}
	}

	var beforeRaw string
	if err := tx.QueryRowContext(ctx, `
		SELECT balance::text
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`, prepared.UserID).Scan(&beforeRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrWalletUserNotFound
		}
		return nil, err
	}
	before, err := decimal.NewFromString(beforeRaw)
	if err != nil {
		return nil, fmt.Errorf("parse current wallet balance: %w", err)
	}
	after := before.Add(prepared.Amount)
	if !service.WalletDecimalFits(after) {
		return nil, fmt.Errorf("%w: resulting balance exceeds NUMERIC(20,8)", service.ErrWalletInvalidMutation)
	}
	if after.IsNegative() && !prepared.AllowNegativeBalance {
		return nil, service.ErrWalletInsufficientBalance
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		SET balance = $2::numeric, updated_at = NOW()
		WHERE id = $1
	`, prepared.UserID, after.StringFixed(service.WalletAmountScale)); err != nil {
		return nil, err
	}

	entry := &service.WalletTransaction{
		UserID:                prepared.UserID,
		Type:                  prepared.Type,
		Amount:                prepared.Amount,
		Currency:              prepared.Currency,
		BalanceBefore:         before,
		BalanceAfter:          after,
		ReferenceType:         prepared.ReferenceType,
		ReferenceID:           prepared.ReferenceID,
		IdempotencyKey:        prepared.IdempotencyKey,
		RequestFingerprint:    fingerprint,
		OperatorID:            prepared.OperatorID,
		ReversesTransactionID: prepared.ReversesTransactionID,
		Source:                prepared.Source,
		Note:                  prepared.Note,
		Metadata:              prepared.Metadata,
	}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO wallet_transactions (
			user_id, type, amount, currency, balance_before, balance_after,
			reference_type, reference_id, idempotency_key, request_fingerprint,
			operator_id, reverses_transaction_id, source, note, metadata
		) VALUES (
			$1, $2, $3::numeric, $4, $5::numeric, $6::numeric,
			$7, $8, $9, $10, $11, $12, $13, $14, $15::jsonb
		)
		RETURNING id, created_at
	`,
		entry.UserID,
		string(entry.Type),
		entry.Amount.StringFixed(service.WalletAmountScale),
		entry.Currency,
		entry.BalanceBefore.StringFixed(service.WalletAmountScale),
		entry.BalanceAfter.StringFixed(service.WalletAmountScale),
		entry.ReferenceType,
		entry.ReferenceID,
		entry.IdempotencyKey,
		entry.RequestFingerprint,
		entry.OperatorID,
		entry.ReversesTransactionID,
		entry.Source,
		entry.Note,
		string(metadataJSON),
	).Scan(&entry.ID, &entry.CreatedAt); err != nil {
		return nil, err
	}

	return &service.WalletApplyResult{Applied: true, Transaction: entry}, nil
}

func validateWalletRefundTarget(ctx context.Context, tx *sql.Tx, cmd *service.WalletMutation) error {
	var existingRefundID int64
	err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM wallet_transactions
		WHERE type = 'refund' AND reverses_transaction_id = $1
	`, *cmd.ReversesTransactionID).Scan(&existingRefundID)
	if err == nil {
		return service.ErrWalletAlreadyRefunded
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	var (
		originalUserID   int64
		originalType     string
		originalAmount   string
		originalCurrency string
	)
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, type, amount::text, currency
		FROM wallet_transactions
		WHERE id = $1
	`, *cmd.ReversesTransactionID).Scan(&originalUserID, &originalType, &originalAmount, &originalCurrency)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrWalletTransactionNotFound
	}
	if err != nil {
		return err
	}
	amount, err := decimal.NewFromString(originalAmount)
	if err != nil {
		return fmt.Errorf("parse refund target amount: %w", err)
	}
	if originalUserID != cmd.UserID || originalType != string(service.WalletTransactionUsage) || originalCurrency != service.WalletCurrencyCNY || !amount.IsNegative() || !cmd.Amount.Equal(amount.Neg()) {
		return service.ErrWalletRefundTargetInvalid
	}
	return nil
}

func (r *walletRepository) GetTransaction(ctx context.Context, transactionID int64) (*service.WalletTransaction, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("wallet repository db is nil")
	}
	if transactionID <= 0 {
		return nil, fmt.Errorf("%w: transaction_id must be positive", service.ErrWalletInvalidMutation)
	}
	entry, err := scanWalletTransaction(r.db.QueryRowContext(ctx, `
		SELECT
			id, user_id, type, amount::text, currency,
			balance_before::text, balance_after::text,
			reference_type, reference_id, idempotency_key, request_fingerprint,
			operator_id, reverses_transaction_id, source, note, metadata::text, created_at
		FROM wallet_transactions
		WHERE id = $1
	`, transactionID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrWalletTransactionNotFound
	}
	return entry, err
}

func (r *walletRepository) ReconcileUser(ctx context.Context, userID int64) (*service.WalletReconciliation, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("wallet repository db is nil")
	}
	return reconcileWalletUser(ctx, r.db, userID)
}

type walletQueryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func reconcileWalletUser(ctx context.Context, queryer walletQueryRower, userID int64) (*service.WalletReconciliation, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("%w: user_id must be positive", service.ErrWalletInvalidMutation)
	}

	var currentRaw, ledgerRaw string
	err := queryer.QueryRowContext(ctx, `
		SELECT
			u.balance::text,
			COALESCE(SUM(w.amount), 0)::numeric(20,8)::text
		FROM users AS u
		LEFT JOIN wallet_transactions AS w ON w.user_id = u.id
		WHERE u.id = $1 AND u.deleted_at IS NULL
		GROUP BY u.id, u.balance
	`, userID).Scan(&currentRaw, &ledgerRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrWalletUserNotFound
	}
	if err != nil {
		return nil, err
	}

	current, err := decimal.NewFromString(currentRaw)
	if err != nil {
		return nil, fmt.Errorf("parse current wallet balance: %w", err)
	}
	ledger, err := decimal.NewFromString(ledgerRaw)
	if err != nil {
		return nil, fmt.Errorf("parse wallet ledger balance: %w", err)
	}
	return &service.WalletReconciliation{
		UserID:         userID,
		CurrentBalance: current,
		LedgerBalance:  ledger,
		Difference:     current.Sub(ledger),
	}, nil
}

type walletRowScanner interface {
	Scan(dest ...any) error
}

func findWalletTransactionByIdempotencyKey(ctx context.Context, tx *sql.Tx, key string) (*service.WalletTransaction, error) {
	return scanWalletTransaction(tx.QueryRowContext(ctx, `
		SELECT
			id, user_id, type, amount::text, currency,
			balance_before::text, balance_after::text,
			reference_type, reference_id, idempotency_key, request_fingerprint,
			operator_id, reverses_transaction_id, source, note, metadata::text, created_at
		FROM wallet_transactions
		WHERE idempotency_key = $1
	`, key))
}

func scanWalletTransaction(row walletRowScanner) (*service.WalletTransaction, error) {
	entry := &service.WalletTransaction{}
	var transactionType, amountRaw, beforeRaw, afterRaw, metadataRaw string
	if err := row.Scan(
		&entry.ID,
		&entry.UserID,
		&transactionType,
		&amountRaw,
		&entry.Currency,
		&beforeRaw,
		&afterRaw,
		&entry.ReferenceType,
		&entry.ReferenceID,
		&entry.IdempotencyKey,
		&entry.RequestFingerprint,
		&entry.OperatorID,
		&entry.ReversesTransactionID,
		&entry.Source,
		&entry.Note,
		&metadataRaw,
		&entry.CreatedAt,
	); err != nil {
		return nil, err
	}
	entry.Type = service.WalletTransactionType(transactionType)
	var err error
	if entry.Amount, err = decimal.NewFromString(amountRaw); err != nil {
		return nil, fmt.Errorf("parse wallet transaction amount: %w", err)
	}
	if entry.BalanceBefore, err = decimal.NewFromString(beforeRaw); err != nil {
		return nil, fmt.Errorf("parse wallet balance_before: %w", err)
	}
	if entry.BalanceAfter, err = decimal.NewFromString(afterRaw); err != nil {
		return nil, fmt.Errorf("parse wallet balance_after: %w", err)
	}
	if err := json.Unmarshal([]byte(metadataRaw), &entry.Metadata); err != nil {
		return nil, fmt.Errorf("parse wallet transaction metadata: %w", err)
	}
	return entry, nil
}

var _ service.WalletRepository = (*walletRepository)(nil)
