package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestPrepareWalletMutationCanonicalFingerprint(t *testing.T) {
	first := &WalletMutation{
		UserID:         42,
		Type:           WalletTransactionRecharge,
		Amount:         decimal.RequireFromString("12.34000000"),
		ReferenceType:  " payment_order ",
		ReferenceID:    " order-1 ",
		IdempotencyKey: " wallet:order-1 ",
		Source:         " admin ",
		Metadata:       map[string]any{"z": 1, "a": "value"},
	}
	second := &WalletMutation{
		UserID:         42,
		Type:           WalletTransactionRecharge,
		Amount:         decimal.RequireFromString("12.34"),
		Currency:       " cny ",
		ReferenceType:  "payment_order",
		ReferenceID:    "order-1",
		IdempotencyKey: "wallet:order-1",
		Source:         "admin",
		Metadata:       map[string]any{"a": "value", "z": 1},
	}

	preparedFirst, metadataFirst, fingerprintFirst, err := PrepareWalletMutation(first)
	require.NoError(t, err)
	preparedSecond, metadataSecond, fingerprintSecond, err := PrepareWalletMutation(second)
	require.NoError(t, err)

	require.Equal(t, WalletCurrencyCNY, preparedFirst.Currency)
	require.Equal(t, preparedFirst, preparedSecond)
	require.JSONEq(t, string(metadataFirst), string(metadataSecond))
	require.Len(t, fingerprintFirst, 64)
	require.Equal(t, fingerprintFirst, fingerprintSecond)
}

func TestPrepareWalletMutationRejectsInvalidAmountsAndFields(t *testing.T) {
	valid := func() *WalletMutation {
		return &WalletMutation{
			UserID:         1,
			Type:           WalletTransactionUsage,
			Amount:         decimal.RequireFromString("-0.00000001"),
			Currency:       WalletCurrencyCNY,
			ReferenceType:  "usage_request",
			ReferenceID:    "req-1",
			IdempotencyKey: "usage:req-1",
			Source:         "gateway",
		}
	}

	tests := []struct {
		name   string
		mutate func(*WalletMutation)
	}{
		{"zero adjustment", func(c *WalletMutation) { c.Type = WalletTransactionAdjustment; c.Amount = decimal.Zero }},
		{"fraction overflow", func(c *WalletMutation) { c.Amount = decimal.RequireFromString("-0.000000001") }},
		{"integer overflow", func(c *WalletMutation) { c.Amount = decimal.RequireFromString("-1000000000000") }},
		{"usage sign", func(c *WalletMutation) { c.Amount = decimal.NewFromInt(1) }},
		{"recharge sign", func(c *WalletMutation) { c.Type = WalletTransactionRecharge }},
		{"wrong currency", func(c *WalletMutation) { c.Currency = "USD" }},
		{"missing reference", func(c *WalletMutation) { c.ReferenceID = "" }},
		{"bad operator", func(c *WalletMutation) { id := int64(0); c.OperatorID = &id }},
		{"refund without target", func(c *WalletMutation) { c.Type = WalletTransactionRefund; c.Amount = decimal.NewFromInt(1) }},
		{"non-refund with target", func(c *WalletMutation) { id := int64(1); c.ReversesTransactionID = &id }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := valid()
			tt.mutate(cmd)
			_, _, _, err := PrepareWalletMutation(cmd)
			require.Error(t, err)
			require.True(t, errors.Is(err, ErrWalletInvalidMutation))
		})
	}
}

func TestPrepareWalletMutationAllowsZeroUsageForAuditableRounding(t *testing.T) {
	cmd := &WalletMutation{
		UserID:         1,
		Type:           WalletTransactionUsage,
		Amount:         decimal.Zero,
		ReferenceType:  "usage_request",
		ReferenceID:    "req-zero",
		IdempotencyKey: "usage:req-zero",
		Source:         "gateway",
	}
	prepared, _, _, err := PrepareWalletMutation(cmd)
	require.NoError(t, err)
	require.Equal(t, "0.00000000", prepared.Amount.StringFixed(WalletAmountScale))
}

func TestPrepareWalletMutationFingerprintCoversMutationSemantics(t *testing.T) {
	base := &WalletMutation{
		UserID:         7,
		Type:           WalletTransactionAdjustment,
		Amount:         decimal.NewFromInt(-1),
		ReferenceType:  "manual",
		ReferenceID:    "ticket-1",
		IdempotencyKey: "adjustment:ticket-1",
		Source:         "admin",
	}
	_, _, first, err := PrepareWalletMutation(base)
	require.NoError(t, err)

	changed := *base
	changed.AllowNegativeBalance = true
	_, _, second, err := PrepareWalletMutation(&changed)
	require.NoError(t, err)
	require.NotEqual(t, first, second)
}

func TestPrepareWalletMutationUsesCharacterLimitsForUnicodeText(t *testing.T) {
	cmd := &WalletMutation{
		UserID:         9,
		Type:           WalletTransactionAdjustment,
		Amount:         decimal.NewFromInt(1),
		ReferenceType:  "manual",
		ReferenceID:    "ticket-unicode",
		IdempotencyKey: "adjustment:unicode",
		Source:         "admin",
		Note:           strings.Repeat("中", 500),
	}
	_, _, _, err := PrepareWalletMutation(cmd)
	require.NoError(t, err)

	cmd.Note += "文"
	_, _, _, err = PrepareWalletMutation(cmd)
	require.ErrorIs(t, err, ErrWalletInvalidMutation)
}
