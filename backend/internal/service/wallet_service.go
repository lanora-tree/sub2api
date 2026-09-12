package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/shopspring/decimal"
)

var adminWalletAmountPattern = regexp.MustCompile(`^-?(?:0|[1-9][0-9]{0,11})(?:\.[0-9]{1,8})?$`)

type AdminWalletMutationInput struct {
	UserID         int64
	Amount         string
	IdempotencyKey string
	OperatorID     int64
	Note           string
}

type AdminWalletRefundInput struct {
	UserID                int64
	OriginalTransactionID int64
	IdempotencyKey        string
	OperatorID            int64
	Note                  string
}

type WalletBalance struct {
	UserID        int64
	Currency      string
	Balance       decimal.Decimal
	LedgerBalance decimal.Decimal
	Difference    decimal.Decimal
}

type WalletService struct {
	repo         WalletRepository
	billingCache *BillingCacheService
	authCache    APIKeyAuthCacheInvalidator
}

func NewWalletService(repo WalletRepository, billingCache *BillingCacheService, authCache APIKeyAuthCacheInvalidator) *WalletService {
	return &WalletService{repo: repo, billingCache: billingCache, authCache: authCache}
}

func (s *WalletService) GetBalance(ctx context.Context, userID int64) (*WalletBalance, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("WALLET_UNAVAILABLE", "wallet service is unavailable")
	}
	reconciliation, err := s.repo.ReconcileUser(ctx, userID)
	if err != nil {
		return nil, walletApplicationError(err)
	}
	if !reconciliation.Difference.IsZero() {
		return nil, infraerrors.InternalServer("WALLET_RECONCILIATION_MISMATCH", "wallet balance requires administrator review")
	}
	return &WalletBalance{
		UserID:        reconciliation.UserID,
		Currency:      WalletCurrencyCNY,
		Balance:       reconciliation.CurrentBalance,
		LedgerBalance: reconciliation.LedgerBalance,
		Difference:    reconciliation.Difference,
	}, nil
}

func (s *WalletService) Recharge(ctx context.Context, input AdminWalletMutationInput) (*WalletApplyResult, error) {
	amount, err := parseAdminWalletAmount(input.Amount, 2)
	if err != nil || !amount.IsPositive() {
		return nil, infraerrors.BadRequest("WALLET_AMOUNT_INVALID", "recharge amount must be a positive decimal string with at most two fractional digits")
	}
	return s.applyAdminMutation(ctx, WalletTransactionRecharge, amount, input)
}

func (s *WalletService) Adjust(ctx context.Context, input AdminWalletMutationInput) (*WalletApplyResult, error) {
	amount, err := parseAdminWalletAmount(input.Amount, 8)
	if err != nil || amount.IsZero() {
		return nil, infraerrors.BadRequest("WALLET_AMOUNT_INVALID", "adjustment amount must be a non-zero signed decimal string with at most eight fractional digits")
	}
	return s.applyAdminMutation(ctx, WalletTransactionAdjustment, amount, input)
}

func (s *WalletService) RefundUsage(ctx context.Context, input AdminWalletRefundInput) (*WalletApplyResult, error) {
	if err := validateAdminWalletRequest(input.UserID, input.OperatorID, input.IdempotencyKey, input.Note); err != nil {
		return nil, err
	}
	if input.OriginalTransactionID <= 0 {
		return nil, infraerrors.BadRequest("WALLET_REFUND_TARGET_INVALID", "original_transaction_id must be positive")
	}
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("WALLET_UNAVAILABLE", "wallet service is unavailable")
	}
	original, err := s.repo.GetTransaction(ctx, input.OriginalTransactionID)
	if err != nil {
		return nil, walletApplicationError(err)
	}
	if original.UserID != input.UserID || original.Type != WalletTransactionUsage || !original.Amount.IsNegative() || original.Currency != WalletCurrencyCNY {
		return nil, walletApplicationError(ErrWalletRefundTargetInvalid)
	}

	result, err := s.repo.Apply(ctx, &WalletMutation{
		UserID:                input.UserID,
		Type:                  WalletTransactionRefund,
		Amount:                original.Amount.Neg(),
		Currency:              WalletCurrencyCNY,
		ReferenceType:         "wallet_transaction",
		ReferenceID:           strconv.FormatInt(original.ID, 10),
		IdempotencyKey:        adminWalletRepositoryKey(WalletTransactionRefund, input.OperatorID, input.IdempotencyKey),
		OperatorID:            &input.OperatorID,
		ReversesTransactionID: &original.ID,
		Source:                "admin",
		Note:                  strings.TrimSpace(input.Note),
		Metadata: map[string]any{
			"original_transaction_id": original.ID,
		},
	})
	if err != nil {
		return nil, walletApplicationError(err)
	}
	s.invalidateBalanceCaches(ctx, input.UserID, result.Applied)
	return result, nil
}

func (s *WalletService) applyAdminMutation(ctx context.Context, transactionType WalletTransactionType, amount decimal.Decimal, input AdminWalletMutationInput) (*WalletApplyResult, error) {
	if err := validateAdminWalletRequest(input.UserID, input.OperatorID, input.IdempotencyKey, input.Note); err != nil {
		return nil, err
	}
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("WALLET_UNAVAILABLE", "wallet service is unavailable")
	}
	requestKey := adminWalletRepositoryKey(transactionType, input.OperatorID, input.IdempotencyKey)
	result, err := s.repo.Apply(ctx, &WalletMutation{
		UserID:         input.UserID,
		Type:           transactionType,
		Amount:         amount,
		Currency:       WalletCurrencyCNY,
		ReferenceType:  "admin_request",
		ReferenceID:    strings.TrimPrefix(requestKey, "admin-wallet:"),
		IdempotencyKey: requestKey,
		OperatorID:     &input.OperatorID,
		Source:         "admin",
		Note:           strings.TrimSpace(input.Note),
	})
	if err != nil {
		return nil, walletApplicationError(err)
	}
	s.invalidateBalanceCaches(ctx, input.UserID, result.Applied)
	return result, nil
}

func validateAdminWalletRequest(userID, operatorID int64, idempotencyKey, note string) error {
	if userID <= 0 {
		return infraerrors.BadRequest("WALLET_USER_INVALID", "user_id must be positive")
	}
	if operatorID <= 0 {
		return infraerrors.Unauthorized("WALLET_OPERATOR_REQUIRED", "authenticated administrator is required")
	}
	key := strings.TrimSpace(idempotencyKey)
	if key == "" || len(key) > 160 {
		return infraerrors.BadRequest("IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key is required and must not exceed 160 characters")
	}
	trimmedNote := strings.TrimSpace(note)
	if trimmedNote == "" || len([]rune(trimmedNote)) > 500 {
		return infraerrors.BadRequest("WALLET_NOTE_REQUIRED", "note is required and must not exceed 500 characters")
	}
	return nil
}

func parseAdminWalletAmount(raw string, maxScale int32) (decimal.Decimal, error) {
	raw = strings.TrimSpace(raw)
	if !adminWalletAmountPattern.MatchString(raw) {
		return decimal.Zero, errors.New("invalid admin wallet amount")
	}
	amount, err := decimal.NewFromString(raw)
	if err != nil || !WalletDecimalFits(amount) || amount.Exponent() < -maxScale {
		return decimal.Zero, errors.New("invalid admin wallet amount")
	}
	return amount, nil
}

func adminWalletRepositoryKey(transactionType WalletTransactionType, operatorID int64, rawKey string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s", transactionType, operatorID, strings.TrimSpace(rawKey))))
	return "admin-wallet:" + hex.EncodeToString(sum[:])
}

func (s *WalletService) invalidateBalanceCaches(ctx context.Context, userID int64, changed bool) {
	if !changed {
		return
	}
	if s.authCache != nil {
		s.authCache.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if s.billingCache != nil {
		if err := s.billingCache.InvalidateUserBalance(ctx, userID); err != nil {
			logger.LegacyPrintf("service.wallet", "invalidate user balance cache failed: user_id=%d err=%v", userID, err)
		}
	}
}

func walletApplicationError(err error) error {
	switch {
	case errors.Is(err, ErrWalletInvalidMutation):
		return infraerrors.BadRequest("WALLET_MUTATION_INVALID", "wallet mutation is invalid").WithCause(err)
	case errors.Is(err, ErrWalletUserNotFound):
		return infraerrors.NotFound("WALLET_USER_NOT_FOUND", "wallet user not found").WithCause(err)
	case errors.Is(err, ErrWalletTransactionNotFound):
		return infraerrors.NotFound("WALLET_TRANSACTION_NOT_FOUND", "wallet transaction not found").WithCause(err)
	case errors.Is(err, ErrWalletRefundTargetInvalid):
		return infraerrors.BadRequest("WALLET_REFUND_TARGET_INVALID", "refund target must be a negative usage transaction for the same user").WithCause(err)
	case errors.Is(err, ErrWalletAlreadyRefunded):
		return infraerrors.Conflict("WALLET_ALREADY_REFUNDED", "usage transaction has already been refunded").WithCause(err)
	case errors.Is(err, ErrWalletInsufficientBalance):
		return infraerrors.Conflict("WALLET_INSUFFICIENT_BALANCE", "adjustment would make the balance negative").WithCause(err)
	case errors.Is(err, ErrWalletIdempotencyConflict):
		return infraerrors.Conflict("WALLET_IDEMPOTENCY_CONFLICT", "Idempotency-Key was already used for a different wallet request").WithCause(err)
	default:
		return err
	}
}
