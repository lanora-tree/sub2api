package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type adminWalletRepoStub struct {
	lastMutation *service.WalletMutation
	transaction  *service.WalletTransaction
	balance      decimal.Decimal
}

func (s *adminWalletRepoStub) Apply(_ context.Context, cmd *service.WalletMutation) (*service.WalletApplyResult, error) {
	clone := *cmd
	s.lastMutation = &clone
	before := s.balance
	after := before.Add(cmd.Amount)
	s.balance = after
	reversalID := cmd.ReversesTransactionID
	return &service.WalletApplyResult{Applied: true, Transaction: &service.WalletTransaction{
		ID: 91, UserID: cmd.UserID, Type: cmd.Type, Amount: cmd.Amount,
		Currency: service.WalletCurrencyCNY, BalanceBefore: before, BalanceAfter: after,
		ReversesTransactionID: reversalID, CreatedAt: time.Date(2026, 9, 13, 4, 0, 0, 0, time.UTC),
	}}, nil
}

func (s *adminWalletRepoStub) GetTransaction(_ context.Context, _ int64) (*service.WalletTransaction, error) {
	return s.transaction, nil
}

func (s *adminWalletRepoStub) ReconcileUser(_ context.Context, userID int64) (*service.WalletReconciliation, error) {
	return &service.WalletReconciliation{UserID: userID, CurrentBalance: s.balance, LedgerBalance: s.balance, Difference: decimal.Zero}, nil
}

func setupAdminWalletRouter(t *testing.T, repo *adminWalletRepoStub, withAdmin bool) *gin.Engine {
	t.Helper()
	previous := service.DefaultIdempotencyCoordinator()
	service.SetDefaultIdempotencyCoordinator(nil)
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previous) })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if withAdmin {
		router.Use(func(c *gin.Context) {
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 3})
			c.Next()
		})
	}
	h := NewUserHandler(newStubAdminService(), nil, nil, nil, nil, nil, nil)
	h.SetWalletService(service.NewWalletService(repo, nil, nil))
	router.POST("/api/v1/admin/users/:id/balance", h.UpdateBalance)
	router.POST("/api/v1/admin/users/:id/wallet/refunds", h.RefundUsage)
	router.GET("/api/v1/admin/users/:id/wallet", h.GetWallet)
	return router
}

func TestAdminWalletHandlerRechargeRequiresStringAndReturnsFixedDecimal(t *testing.T) {
	repo := &adminWalletRepoStub{balance: decimal.RequireFromString("1.25")}
	router := setupAdminWalletRouter(t, repo, true)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/balance", bytes.NewBufferString(`{"amount":"2.50","operation":"recharge","note":"receipt 42"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "recharge-request-42")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Data walletTransactionResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "2.50000000", body.Data.Amount)
	require.Equal(t, "3.75000000", body.Data.BalanceAfter)
	require.Equal(t, service.WalletTransactionRecharge, repo.lastMutation.Type)
	require.Equal(t, int64(3), *repo.lastMutation.OperatorID)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/balance", bytes.NewBufferString(`{"amount":2.5,"operation":"recharge","note":"receipt 43"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "recharge-request-43")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminWalletHandlerRequiresIdempotencyKeyNoteAndAdministratorIdentity(t *testing.T) {
	tests := []struct {
		name      string
		withAdmin bool
		key       string
		body      string
		status    int
	}{
		{"missing key", true, "", `{"amount":"1.00","operation":"recharge","note":"ticket"}`, http.StatusBadRequest},
		{"missing note", true, "key", `{"amount":"1.00","operation":"recharge","note":""}`, http.StatusBadRequest},
		{"missing admin identity", false, "key", `{"amount":"1.00","operation":"recharge","note":"ticket"}`, http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupAdminWalletRouter(t, &adminWalletRepoStub{}, tt.withAdmin)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/balance", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.key != "" {
				req.Header.Set("Idempotency-Key", tt.key)
			}
			router.ServeHTTP(rec, req)
			require.Equal(t, tt.status, rec.Code, rec.Body.String())
		})
	}
}

func TestAdminWalletHandlerRefundAndBalanceQuery(t *testing.T) {
	repo := &adminWalletRepoStub{
		balance: decimal.RequireFromString("6.75"),
		transaction: &service.WalletTransaction{
			ID: 44, UserID: 7, Type: service.WalletTransactionUsage,
			Amount: decimal.RequireFromString("-3.25"), Currency: service.WalletCurrencyCNY,
		},
	}
	router := setupAdminWalletRouter(t, repo, true)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/wallet/refunds", bytes.NewBufferString(`{"original_transaction_id":44,"note":"approved ticket"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "refund-request-44")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, int64(44), *repo.lastMutation.ReversesTransactionID)
	require.Equal(t, "3.25000000", repo.lastMutation.Amount.StringFixed(service.WalletAmountScale))

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/7/wallet", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"currency":"CNY"`)
	require.Contains(t, rec.Body.String(), `"balance":"10.00000000"`)
}

var _ service.WalletRepository = (*adminWalletRepoStub)(nil)
