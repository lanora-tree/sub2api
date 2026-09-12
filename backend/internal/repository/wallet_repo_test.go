package repository

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestWalletRepositoryApplyCommitsBalanceAndLedgerTogether(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &walletRepository{db: db}
	cmd := walletRepositoryTestMutation()
	prepared, metadataJSON, fingerprint, err := service.PrepareWalletMutation(cmd)
	require.NoError(t, err)
	createdAt := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs(prepared.IdempotencyKey).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM wallet_transactions WHERE idempotency_key = \\$1").
		WithArgs(prepared.IdempotencyKey).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("FROM users WHERE id = \\$1 AND deleted_at IS NULL FOR UPDATE").
		WithArgs(prepared.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow("10.00000000"))
	mock.ExpectExec("UPDATE users SET balance = \\$2::numeric, updated_at = NOW\\(\\) WHERE id = \\$1").
		WithArgs(prepared.UserID, "12.50000000").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO wallet_transactions").
		WithArgs(
			prepared.UserID,
			string(prepared.Type),
			"2.50000000",
			service.WalletCurrencyCNY,
			"10.00000000",
			"12.50000000",
			prepared.ReferenceType,
			prepared.ReferenceID,
			prepared.IdempotencyKey,
			fingerprint,
			nil,
			prepared.Source,
			prepared.Note,
			string(metadataJSON),
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(55), createdAt))
	mock.ExpectCommit()

	result, err := repo.Apply(t.Context(), cmd)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.Equal(t, int64(55), result.Transaction.ID)
	require.True(t, result.Transaction.BalanceBefore.Equal(decimal.NewFromInt(10)))
	require.True(t, result.Transaction.BalanceAfter.Equal(decimal.RequireFromString("12.5")))
	require.Equal(t, fingerprint, result.Transaction.RequestFingerprint)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWalletRepositoryApplyRollsBackWhenLedgerInsertFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &walletRepository{db: db}
	cmd := walletRepositoryTestMutation()

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM wallet_transactions WHERE idempotency_key = \\$1").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("FROM users WHERE id = \\$1 AND deleted_at IS NULL FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow("10.00000000"))
	mock.ExpectExec("UPDATE users").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO wallet_transactions").WillReturnError(errors.New("forced insert failure"))
	mock.ExpectRollback()

	_, err = repo.Apply(t.Context(), cmd)
	require.EqualError(t, err, "forced insert failure")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWalletRepositoryApplyRejectsIdempotencyFingerprintConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &walletRepository{db: db}
	cmd := walletRepositoryTestMutation()

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM wallet_transactions WHERE idempotency_key = \\$1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "type", "amount", "currency", "balance_before", "balance_after",
			"reference_type", "reference_id", "idempotency_key", "request_fingerprint",
			"operator_id", "source", "note", "metadata", "created_at",
		}).AddRow(
			1, 7, "adjustment", "1.00000000", "CNY", "10.00000000", "11.00000000",
			"manual", "ticket-1", cmd.IdempotencyKey, "different-fingerprint",
			nil, "admin", "", `{}`, time.Now(),
		))
	mock.ExpectRollback()

	_, err = repo.Apply(t.Context(), cmd)
	require.ErrorIs(t, err, service.ErrWalletIdempotencyConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWalletRepositoryReconcileUserReturnsExactDifference(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &walletRepository{db: db}
	mock.ExpectQuery("FROM users AS u LEFT JOIN wallet_transactions AS w").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "ledger"}).AddRow("12.50000000", "12.49999999"))

	result, err := repo.ReconcileUser(t.Context(), 7)
	require.NoError(t, err)
	require.Equal(t, "0.00000001", result.Difference.StringFixed(service.WalletAmountScale))
	require.NoError(t, mock.ExpectationsWereMet())
}

func walletRepositoryTestMutation() *service.WalletMutation {
	return &service.WalletMutation{
		UserID:         7,
		Type:           service.WalletTransactionAdjustment,
		Amount:         decimal.RequireFromString("2.5"),
		ReferenceType:  "manual",
		ReferenceID:    "ticket-1",
		IdempotencyKey: "wallet:manual:ticket-1",
		Source:         "admin",
		Metadata:       map[string]any{"ticket": "ticket-1"},
	}
}
