//go:build integration

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

var walletIntegrationSchemaSequence atomic.Uint64

func TestWalletRepositoryConcurrentMutationsAndReconciliation(t *testing.T) {
	schemaName := createWalletIntegrationSchema(t, "")
	const mutations = 12

	start := make(chan struct{})
	type outcome struct {
		result *service.WalletApplyResult
		err    error
	}
	outcomes := make(chan outcome, mutations)
	var wg sync.WaitGroup
	for i := 0; i < mutations; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			result, err := applyWalletMutationInSchema(context.Background(), schemaName, &service.WalletMutation{
				UserID:         1,
				Type:           service.WalletTransactionAdjustment,
				Amount:         decimal.NewFromInt(1),
				ReferenceType:  "concurrency_test",
				ReferenceID:    fmt.Sprintf("mutation-%d", index),
				IdempotencyKey: fmt.Sprintf("wallet:concurrency:%s:%d", schemaName, index),
				Source:         "integration_test",
			})
			outcomes <- outcome{result: result, err: err}
		}(i)
	}
	close(start)
	wg.Wait()
	close(outcomes)

	for item := range outcomes {
		require.NoError(t, item.err)
		require.True(t, item.result.Applied)
	}

	tx := beginWalletSchemaTx(t, schemaName)
	defer tx.Rollback()
	reconciliation, err := reconcileWalletUser(context.Background(), tx, 1)
	require.NoError(t, err)
	require.True(t, reconciliation.CurrentBalance.Equal(decimal.NewFromInt(mutations)))
	require.True(t, reconciliation.LedgerBalance.Equal(decimal.NewFromInt(mutations)))
	require.True(t, reconciliation.Difference.IsZero())

	var count int
	require.NoError(t, tx.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM wallet_transactions`).Scan(&count))
	require.Equal(t, mutations, count)
}

func TestWalletRepositoryConcurrentIdempotentReplayAppliesOnce(t *testing.T) {
	schemaName := createWalletIntegrationSchema(t, "")
	cmd := &service.WalletMutation{
		UserID:         1,
		Type:           service.WalletTransactionRecharge,
		Amount:         decimal.RequireFromString("9.99000000"),
		ReferenceType:  "payment_order",
		ReferenceID:    "order-1",
		IdempotencyKey: "wallet:replay:" + schemaName,
		Source:         "integration_test",
	}

	start := make(chan struct{})
	results := make(chan *service.WalletApplyResult, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := applyWalletMutationInSchema(context.Background(), schemaName, cmd)
			results <- result
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	applied := 0
	var transactionID int64
	for result := range results {
		if result.Applied {
			applied++
		}
		if transactionID == 0 {
			transactionID = result.Transaction.ID
		} else {
			require.Equal(t, transactionID, result.Transaction.ID)
		}
	}
	require.Equal(t, 1, applied)

	tx := beginWalletSchemaTx(t, schemaName)
	defer tx.Rollback()
	reconciliation, err := reconcileWalletUser(context.Background(), tx, 1)
	require.NoError(t, err)
	require.True(t, reconciliation.CurrentBalance.Equal(decimal.RequireFromString("9.99")))
	require.True(t, reconciliation.Difference.IsZero())
}

func TestWalletRepositoryInsertFailureRollsBackBalance(t *testing.T) {
	schemaName := createWalletIntegrationSchema(t, "CHECK (amount <> 2)")
	tx, err := integrationDB.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	require.NoError(t, setWalletSchemaSearchPath(context.Background(), tx, schemaName))

	repo := &walletRepository{}
	_, err = repo.applyInTx(context.Background(), tx, &service.WalletMutation{
		UserID:         1,
		Type:           service.WalletTransactionAdjustment,
		Amount:         decimal.NewFromInt(2),
		ReferenceType:  "failure_test",
		ReferenceID:    "forced-check-failure",
		IdempotencyKey: "wallet:rollback:" + schemaName,
		Source:         "integration_test",
	})
	require.Error(t, err)
	require.NoError(t, tx.Rollback())

	verifyTx := beginWalletSchemaTx(t, schemaName)
	defer verifyTx.Rollback()
	var balance string
	require.NoError(t, verifyTx.QueryRowContext(context.Background(), `SELECT balance::text FROM users WHERE id = 1`).Scan(&balance))
	require.Equal(t, "0.00000000", balance)
	var count int
	require.NoError(t, verifyTx.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM wallet_transactions`).Scan(&count))
	require.Zero(t, count)
}

func TestWalletRepositoryConcurrentFullRefundAppliesOnceAcrossDifferentKeys(t *testing.T) {
	schemaName := createWalletIntegrationSchema(t, "")
	usage, err := applyWalletMutationInSchema(context.Background(), schemaName, &service.WalletMutation{
		UserID:               1,
		Type:                 service.WalletTransactionUsage,
		Amount:               decimal.RequireFromString("-4.25"),
		ReferenceType:        "usage_request",
		ReferenceID:          "request-refund-1",
		IdempotencyKey:       "wallet:usage:" + schemaName,
		Source:               "gateway",
		AllowNegativeBalance: true,
	})
	require.NoError(t, err)
	require.True(t, usage.Applied)
	originalID := usage.Transaction.ID

	type outcome struct {
		result *service.WalletApplyResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			result, applyErr := applyWalletMutationInSchema(context.Background(), schemaName, &service.WalletMutation{
				UserID:                1,
				Type:                  service.WalletTransactionRefund,
				Amount:                decimal.RequireFromString("4.25"),
				ReferenceType:         "wallet_transaction",
				ReferenceID:           fmt.Sprint(originalID),
				IdempotencyKey:        fmt.Sprintf("wallet:refund:%s:%d", schemaName, index),
				ReversesTransactionID: &originalID,
				Source:                "admin",
			})
			outcomes <- outcome{result: result, err: applyErr}
		}(i)
	}
	close(start)
	wg.Wait()
	close(outcomes)

	applied := 0
	alreadyRefunded := 0
	for item := range outcomes {
		switch {
		case item.err == nil:
			require.True(t, item.result.Applied)
			applied++
		case errors.Is(item.err, service.ErrWalletAlreadyRefunded):
			alreadyRefunded++
		default:
			require.NoError(t, item.err)
		}
	}
	require.Equal(t, 1, applied)
	require.Equal(t, 1, alreadyRefunded)

	tx := beginWalletSchemaTx(t, schemaName)
	defer tx.Rollback()
	reconciliation, err := reconcileWalletUser(context.Background(), tx, 1)
	require.NoError(t, err)
	require.True(t, reconciliation.CurrentBalance.IsZero())
	require.True(t, reconciliation.Difference.IsZero())
	var refundCount int
	require.NoError(t, tx.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM wallet_transactions WHERE type = 'refund' AND reverses_transaction_id = $1`, originalID).Scan(&refundCount))
	require.Equal(t, 1, refundCount)
}

func createWalletIntegrationSchema(t *testing.T, extraAmountConstraint string) string {
	t.Helper()
	schemaName := fmt.Sprintf("wallet_m6_%d_%d", time.Now().UnixNano(), walletIntegrationSchemaSequence.Add(1))
	quotedSchema := pq.QuoteIdentifier(schemaName)
	_, err := integrationDB.ExecContext(context.Background(), `CREATE SCHEMA `+quotedSchema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, cleanupErr := integrationDB.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+quotedSchema+` CASCADE`)
		require.NoError(t, cleanupErr)
	})

	_, err = integrationDB.ExecContext(context.Background(), `
		CREATE TABLE `+quotedSchema+`.users (
			id BIGINT PRIMARY KEY,
			balance NUMERIC(20,8) NOT NULL,
			deleted_at TIMESTAMPTZ NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE `+quotedSchema+`.wallet_transactions (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL REFERENCES `+quotedSchema+`.users(id),
			type VARCHAR(24) NOT NULL,
			amount NUMERIC(20,8) NOT NULL `+extraAmountConstraint+`,
			currency CHAR(3) NOT NULL,
			balance_before NUMERIC(20,8) NOT NULL,
			balance_after NUMERIC(20,8) NOT NULL,
			reference_type VARCHAR(32) NOT NULL,
			reference_id VARCHAR(128) NOT NULL,
			idempotency_key VARCHAR(160) NOT NULL UNIQUE,
			request_fingerprint VARCHAR(64) NOT NULL,
			operator_id BIGINT NULL,
			reverses_transaction_id BIGINT NULL,
			source VARCHAR(64) NOT NULL,
			note VARCHAR(500) NOT NULL,
			metadata JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		ALTER TABLE `+quotedSchema+`.wallet_transactions
			ADD CONSTRAINT wallet_refund_link_check CHECK (
				(type = 'refund' AND reverses_transaction_id IS NOT NULL)
				OR (type <> 'refund' AND reverses_transaction_id IS NULL)
			);
		ALTER TABLE `+quotedSchema+`.wallet_transactions
			ADD CONSTRAINT wallet_refund_link_fkey FOREIGN KEY (reverses_transaction_id)
			REFERENCES `+quotedSchema+`.wallet_transactions(id) ON DELETE RESTRICT;
		CREATE UNIQUE INDEX wallet_one_refund_per_usage
			ON `+quotedSchema+`.wallet_transactions(reverses_transaction_id) WHERE type = 'refund';
		INSERT INTO `+quotedSchema+`.users (id, balance) VALUES (1, 0);
	`)
	require.NoError(t, err)
	return schemaName
}

func applyWalletMutationInSchema(ctx context.Context, schemaName string, cmd *service.WalletMutation) (*service.WalletApplyResult, error) {
	tx, err := integrationDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	if err := setWalletSchemaSearchPath(ctx, tx, schemaName); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	repo := &walletRepository{}
	result, err := repo.applyInTx(ctx, tx, cmd)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func beginWalletSchemaTx(t *testing.T, schemaName string) *sql.Tx {
	t.Helper()
	tx, err := integrationDB.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	require.NoError(t, setWalletSchemaSearchPath(context.Background(), tx, schemaName))
	return tx
}

func setWalletSchemaSearchPath(ctx context.Context, tx *sql.Tx, schemaName string) error {
	_, err := tx.ExecContext(ctx, `SET LOCAL search_path TO `+pq.QuoteIdentifier(schemaName))
	return err
}
