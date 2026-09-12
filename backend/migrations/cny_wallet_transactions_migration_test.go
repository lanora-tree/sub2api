package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCNYWalletTransactionsMigrationDefinesImmutableLedger(t *testing.T) {
	content, err := FS.ReadFile("240_cny_wallet_transactions.sql")
	require.NoError(t, err)
	sql := strings.ToUpper(string(content))

	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS WALLET_TRANSACTIONS",
		"AMOUNT NUMERIC(20,8) NOT NULL",
		"BALANCE_BEFORE NUMERIC(20,8) NOT NULL",
		"BALANCE_AFTER NUMERIC(20,8) NOT NULL",
		"CHECK (CURRENCY = 'CNY')",
		"WALLET_TRANSACTIONS_AMOUNT_SIGN_CHECK",
		"TYPE = 'USAGE' AND AMOUNT <= 0",
		"CHECK (BALANCE_AFTER = BALANCE_BEFORE + AMOUNT)",
		"WALLET_TRANSACTIONS_REQUIRED_TEXT_CHECK",
		"WALLET_TRANSACTIONS_FINGERPRINT_CHECK",
		"UNIQUE (IDEMPOTENCY_KEY)",
		"BEFORE UPDATE OR DELETE ON WALLET_TRANSACTIONS",
		"'M6-OPENING:USER:' || U.ID::TEXT",
		"ON CONFLICT (IDEMPOTENCY_KEY) DO NOTHING",
	} {
		require.Contains(t, sql, fragment)
	}
}
