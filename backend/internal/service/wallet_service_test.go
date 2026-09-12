package service

import (
	"context"
	"errors"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type walletServiceRepoStub struct {
	applyInput        *WalletMutation
	applyResult       *WalletApplyResult
	applyErr          error
	transaction       *WalletTransaction
	transactionErr    error
	reconciliation    *WalletReconciliation
	reconciliationErr error
}

func (s *walletServiceRepoStub) Apply(_ context.Context, cmd *WalletMutation) (*WalletApplyResult, error) {
	clone := *cmd
	s.applyInput = &clone
	if s.applyResult != nil || s.applyErr != nil {
		return s.applyResult, s.applyErr
	}
	return &WalletApplyResult{Applied: true, Transaction: &WalletTransaction{
		ID:                    90,
		UserID:                cmd.UserID,
		Type:                  cmd.Type,
		Amount:                cmd.Amount,
		Currency:              WalletCurrencyCNY,
		BalanceBefore:         decimal.NewFromInt(10),
		BalanceAfter:          decimal.NewFromInt(10).Add(cmd.Amount),
		ReversesTransactionID: cmd.ReversesTransactionID,
		CreatedAt:             time.Date(2026, 9, 13, 3, 0, 0, 0, time.UTC),
	}}, nil
}

func (s *walletServiceRepoStub) GetTransaction(_ context.Context, _ int64) (*WalletTransaction, error) {
	return s.transaction, s.transactionErr
}

func (s *walletServiceRepoStub) ReconcileUser(_ context.Context, _ int64) (*WalletReconciliation, error) {
	return s.reconciliation, s.reconciliationErr
}

func TestWalletServiceRechargeUsesExactDecimalAndAdminAuditFields(t *testing.T) {
	repo := &walletServiceRepoStub{}
	svc := NewWalletService(repo, nil, nil)
	result, err := svc.Recharge(t.Context(), AdminWalletMutationInput{
		UserID: 7, Amount: "100.10", IdempotencyKey: "request-1", OperatorID: 3, Note: "bank receipt 8",
	})
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.Equal(t, WalletTransactionRecharge, repo.applyInput.Type)
	require.Equal(t, "100.10000000", repo.applyInput.Amount.StringFixed(WalletAmountScale))
	require.Equal(t, int64(3), *repo.applyInput.OperatorID)
	require.Equal(t, "bank receipt 8", repo.applyInput.Note)
	require.Regexp(t, `^admin-wallet:[0-9a-f]{64}$`, repo.applyInput.IdempotencyKey)
}

func TestWalletServiceAdjustmentAllowsSignedAmountButNeverNegativeBalance(t *testing.T) {
	repo := &walletServiceRepoStub{applyErr: ErrWalletInsufficientBalance}
	svc := NewWalletService(repo, nil, nil)
	_, err := svc.Adjust(t.Context(), AdminWalletMutationInput{
		UserID: 7, Amount: "-12.50", IdempotencyKey: "request-2", OperatorID: 3, Note: "ticket 9",
	})
	require.Error(t, err)
	require.Equal(t, "WALLET_INSUFFICIENT_BALANCE", infraerrors.Reason(err))
	require.False(t, repo.applyInput.AllowNegativeBalance)
}

func TestWalletServiceAdjustmentPreservesEightDecimalPlaces(t *testing.T) {
	repo := &walletServiceRepoStub{}
	_, err := NewWalletService(repo, nil, nil).Adjust(t.Context(), AdminWalletMutationInput{
		UserID: 7, Amount: "-0.00000001", IdempotencyKey: "request-precise", OperatorID: 3, Note: "rounding correction",
	})
	require.NoError(t, err)
	require.Equal(t, "-0.00000001", repo.applyInput.Amount.StringFixed(WalletAmountScale))
}

func TestWalletServiceRejectsImpreciseOrUnauditedAdminMutations(t *testing.T) {
	tests := []struct {
		name   string
		input  AdminWalletMutationInput
		reason string
	}{
		{"too many decimals", AdminWalletMutationInput{UserID: 7, Amount: "1.001", IdempotencyKey: "k", OperatorID: 3, Note: "ticket"}, "WALLET_AMOUNT_INVALID"},
		{"scientific notation", AdminWalletMutationInput{UserID: 7, Amount: "1e2", IdempotencyKey: "k", OperatorID: 3, Note: "ticket"}, "WALLET_AMOUNT_INVALID"},
		{"missing idempotency", AdminWalletMutationInput{UserID: 7, Amount: "1.00", OperatorID: 3, Note: "ticket"}, "IDEMPOTENCY_KEY_REQUIRED"},
		{"missing operator", AdminWalletMutationInput{UserID: 7, Amount: "1.00", IdempotencyKey: "k", Note: "ticket"}, "WALLET_OPERATOR_REQUIRED"},
		{"missing note", AdminWalletMutationInput{UserID: 7, Amount: "1.00", IdempotencyKey: "k", OperatorID: 3}, "WALLET_NOTE_REQUIRED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWalletService(&walletServiceRepoStub{}, nil, nil).Recharge(t.Context(), tt.input)
			require.Error(t, err)
			require.Equal(t, tt.reason, infraerrors.Reason(err))
		})
	}
}

func TestWalletServiceRefundDerivesFullAmountFromOriginalUsage(t *testing.T) {
	original := &WalletTransaction{
		ID: 44, UserID: 7, Type: WalletTransactionUsage,
		Amount: decimal.RequireFromString("-3.12500000"), Currency: WalletCurrencyCNY,
	}
	repo := &walletServiceRepoStub{transaction: original}
	svc := NewWalletService(repo, nil, nil)
	result, err := svc.RefundUsage(t.Context(), AdminWalletRefundInput{
		UserID: 7, OriginalTransactionID: 44, IdempotencyKey: "refund-request", OperatorID: 3, Note: "approved ticket 10",
	})
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.Equal(t, WalletTransactionRefund, repo.applyInput.Type)
	require.Equal(t, "3.12500000", repo.applyInput.Amount.StringFixed(WalletAmountScale))
	require.Equal(t, int64(44), *repo.applyInput.ReversesTransactionID)
	require.False(t, repo.applyInput.AllowNegativeBalance)
}

func TestWalletServiceRefundRejectsWrongUserOrNonUsage(t *testing.T) {
	tests := []*WalletTransaction{
		{ID: 44, UserID: 8, Type: WalletTransactionUsage, Amount: decimal.NewFromInt(-1), Currency: WalletCurrencyCNY},
		{ID: 44, UserID: 7, Type: WalletTransactionRecharge, Amount: decimal.NewFromInt(1), Currency: WalletCurrencyCNY},
	}
	for _, original := range tests {
		repo := &walletServiceRepoStub{transaction: original}
		_, err := NewWalletService(repo, nil, nil).RefundUsage(t.Context(), AdminWalletRefundInput{
			UserID: 7, OriginalTransactionID: 44, IdempotencyKey: "refund", OperatorID: 3, Note: "ticket",
		})
		require.Equal(t, "WALLET_REFUND_TARGET_INVALID", infraerrors.Reason(err))
		require.Nil(t, repo.applyInput)
	}
}

func TestWalletServiceGetBalanceFailsClosedOnLedgerMismatch(t *testing.T) {
	repo := &walletServiceRepoStub{reconciliation: &WalletReconciliation{
		UserID: 7, CurrentBalance: decimal.RequireFromString("9.00"), LedgerBalance: decimal.RequireFromString("8.99"), Difference: decimal.RequireFromString("0.01"),
	}}
	_, err := NewWalletService(repo, nil, nil).GetBalance(t.Context(), 7)
	require.Equal(t, "WALLET_RECONCILIATION_MISMATCH", infraerrors.Reason(err))
}

func TestWalletServiceMapsRepositoryErrors(t *testing.T) {
	repo := &walletServiceRepoStub{reconciliationErr: ErrWalletUserNotFound}
	_, err := NewWalletService(repo, nil, nil).GetBalance(t.Context(), 99)
	require.Equal(t, "WALLET_USER_NOT_FOUND", infraerrors.Reason(err))

	repo = &walletServiceRepoStub{transactionErr: ErrWalletTransactionNotFound}
	_, err = NewWalletService(repo, nil, nil).RefundUsage(t.Context(), AdminWalletRefundInput{
		UserID: 7, OriginalTransactionID: 999, IdempotencyKey: "refund", OperatorID: 3, Note: "ticket",
	})
	require.Equal(t, "WALLET_TRANSACTION_NOT_FOUND", infraerrors.Reason(err))
	require.True(t, errors.Is(err, ErrWalletTransactionNotFound))
}

var _ WalletRepository = (*walletServiceRepoStub)(nil)
