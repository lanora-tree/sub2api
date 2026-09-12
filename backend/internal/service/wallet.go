package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

const (
	WalletCurrencyCNY = "CNY"
	WalletAmountScale = int32(8)
)

type WalletTransactionType string

const (
	WalletTransactionRecharge   WalletTransactionType = "recharge"
	WalletTransactionUsage      WalletTransactionType = "usage"
	WalletTransactionRefund     WalletTransactionType = "refund"
	WalletTransactionAdjustment WalletTransactionType = "adjustment"
)

var (
	ErrWalletInvalidMutation     = errors.New("invalid wallet mutation")
	ErrWalletIdempotencyConflict = errors.New("wallet idempotency key reused with different request")
	ErrWalletUserNotFound        = errors.New("wallet user not found")
	ErrWalletInsufficientBalance = errors.New("wallet balance would become negative")
)

// WalletMutation is one requested change to the CNY balance. Amount is signed:
// recharge/refund are positive, usage is negative, and adjustment can be either.
type WalletMutation struct {
	UserID               int64
	Type                 WalletTransactionType
	Amount               decimal.Decimal
	Currency             string
	ReferenceType        string
	ReferenceID          string
	IdempotencyKey       string
	OperatorID           *int64
	Source               string
	Note                 string
	Metadata             map[string]any
	AllowNegativeBalance bool
}

type WalletTransaction struct {
	ID                 int64
	UserID             int64
	Type               WalletTransactionType
	Amount             decimal.Decimal
	Currency           string
	BalanceBefore      decimal.Decimal
	BalanceAfter       decimal.Decimal
	ReferenceType      string
	ReferenceID        string
	IdempotencyKey     string
	RequestFingerprint string
	OperatorID         *int64
	Source             string
	Note               string
	Metadata           map[string]any
	CreatedAt          time.Time
}

type WalletApplyResult struct {
	Applied     bool
	Transaction *WalletTransaction
}

type WalletReconciliation struct {
	UserID         int64
	CurrentBalance decimal.Decimal
	LedgerBalance  decimal.Decimal
	Difference     decimal.Decimal
}

type WalletRepository interface {
	Apply(ctx context.Context, cmd *WalletMutation) (*WalletApplyResult, error)
	ReconcileUser(ctx context.Context, userID int64) (*WalletReconciliation, error)
}

type walletFingerprintPayload struct {
	UserID               int64                 `json:"user_id"`
	Type                 WalletTransactionType `json:"type"`
	Amount               string                `json:"amount"`
	Currency             string                `json:"currency"`
	ReferenceType        string                `json:"reference_type"`
	ReferenceID          string                `json:"reference_id"`
	IdempotencyKey       string                `json:"idempotency_key"`
	OperatorID           *int64                `json:"operator_id"`
	Source               string                `json:"source"`
	Note                 string                `json:"note"`
	Metadata             json.RawMessage       `json:"metadata"`
	AllowNegativeBalance bool                  `json:"allow_negative_balance"`
}

// PrepareWalletMutation normalizes and validates a mutation, then produces the
// canonical JSON metadata and SHA-256 request fingerprint used by the repository.
func PrepareWalletMutation(cmd *WalletMutation) (*WalletMutation, []byte, string, error) {
	if cmd == nil {
		return nil, nil, "", fmt.Errorf("%w: command is nil", ErrWalletInvalidMutation)
	}

	prepared := *cmd
	prepared.Currency = strings.ToUpper(strings.TrimSpace(prepared.Currency))
	if prepared.Currency == "" {
		prepared.Currency = WalletCurrencyCNY
	}
	prepared.ReferenceType = strings.TrimSpace(prepared.ReferenceType)
	prepared.ReferenceID = strings.TrimSpace(prepared.ReferenceID)
	prepared.IdempotencyKey = strings.TrimSpace(prepared.IdempotencyKey)
	prepared.Source = strings.TrimSpace(prepared.Source)
	prepared.Note = strings.TrimSpace(prepared.Note)
	if prepared.OperatorID != nil {
		operatorID := *prepared.OperatorID
		prepared.OperatorID = &operatorID
	}

	if err := validateWalletMutation(&prepared); err != nil {
		return nil, nil, "", err
	}
	prepared.Amount = decimal.RequireFromString(prepared.Amount.StringFixed(WalletAmountScale))

	metadata := prepared.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, nil, "", fmt.Errorf("%w: metadata: %v", ErrWalletInvalidMutation, err)
	}
	// Round-trip the value so the prepared command no longer aliases the caller's
	// map and exactly matches the JSON persisted by the repository.
	prepared.Metadata = map[string]any{}
	if err := json.Unmarshal(metadataJSON, &prepared.Metadata); err != nil {
		return nil, nil, "", fmt.Errorf("%w: metadata: %v", ErrWalletInvalidMutation, err)
	}

	payload, err := json.Marshal(walletFingerprintPayload{
		UserID:               prepared.UserID,
		Type:                 prepared.Type,
		Amount:               prepared.Amount.StringFixed(WalletAmountScale),
		Currency:             prepared.Currency,
		ReferenceType:        prepared.ReferenceType,
		ReferenceID:          prepared.ReferenceID,
		IdempotencyKey:       prepared.IdempotencyKey,
		OperatorID:           prepared.OperatorID,
		Source:               prepared.Source,
		Note:                 prepared.Note,
		Metadata:             metadataJSON,
		AllowNegativeBalance: prepared.AllowNegativeBalance,
	})
	if err != nil {
		return nil, nil, "", fmt.Errorf("%w: fingerprint: %v", ErrWalletInvalidMutation, err)
	}
	sum := sha256.Sum256(payload)
	return &prepared, metadataJSON, hex.EncodeToString(sum[:]), nil
}

func validateWalletMutation(cmd *WalletMutation) error {
	if cmd.UserID <= 0 {
		return fmt.Errorf("%w: user_id must be positive", ErrWalletInvalidMutation)
	}
	if cmd.OperatorID != nil && *cmd.OperatorID <= 0 {
		return fmt.Errorf("%w: operator_id must be positive", ErrWalletInvalidMutation)
	}
	if cmd.Currency != WalletCurrencyCNY {
		return fmt.Errorf("%w: currency must be CNY", ErrWalletInvalidMutation)
	}
	if !WalletDecimalFits(cmd.Amount) {
		return fmt.Errorf("%w: amount must fit NUMERIC(20,8)", ErrWalletInvalidMutation)
	}
	switch cmd.Type {
	case WalletTransactionRecharge, WalletTransactionRefund:
		if !cmd.Amount.IsPositive() {
			return fmt.Errorf("%w: %s amount must be positive", ErrWalletInvalidMutation, cmd.Type)
		}
	case WalletTransactionUsage:
		if cmd.Amount.IsPositive() {
			return fmt.Errorf("%w: usage amount must be zero or negative", ErrWalletInvalidMutation)
		}
	case WalletTransactionAdjustment:
		if cmd.Amount.IsZero() {
			return fmt.Errorf("%w: adjustment amount must be non-zero", ErrWalletInvalidMutation)
		}
	default:
		return fmt.Errorf("%w: unsupported transaction type", ErrWalletInvalidMutation)
	}

	for _, value := range []struct {
		name  string
		value string
		max   int
	}{
		{"reference_type", cmd.ReferenceType, 32},
		{"reference_id", cmd.ReferenceID, 128},
		{"idempotency_key", cmd.IdempotencyKey, 160},
		{"source", cmd.Source, 64},
		{"note", cmd.Note, 500},
	} {
		if value.name != "note" && value.value == "" {
			return fmt.Errorf("%w: %s is required", ErrWalletInvalidMutation, value.name)
		}
		if utf8.RuneCountInString(value.value) > value.max {
			return fmt.Errorf("%w: %s exceeds %d characters", ErrWalletInvalidMutation, value.name, value.max)
		}
	}
	return nil
}

// WalletDecimalFits reports whether a decimal is exactly representable by
// PostgreSQL NUMERIC(20,8): at most 12 integer and 8 fractional digits.
func WalletDecimalFits(value decimal.Decimal) bool {
	limit := decimal.New(1, 12)
	return value.Truncate(WalletAmountScale).Equal(value) && value.Abs().LessThan(limit)
}
