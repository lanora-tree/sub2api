package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type userWalletRepoStub struct {
	queriedUserID int64
}

func (*userWalletRepoStub) Apply(context.Context, *service.WalletMutation) (*service.WalletApplyResult, error) {
	panic("unexpected wallet mutation")
}

func (*userWalletRepoStub) GetTransaction(context.Context, int64) (*service.WalletTransaction, error) {
	panic("unexpected transaction query")
}

func (s *userWalletRepoStub) ReconcileUser(_ context.Context, userID int64) (*service.WalletReconciliation, error) {
	s.queriedUserID = userID
	balance := decimal.RequireFromString("12.34000000")
	return &service.WalletReconciliation{UserID: userID, CurrentBalance: balance, LedgerBalance: balance, Difference: decimal.Zero}, nil
}

func TestUserWalletHandlerUsesAuthenticatedSubjectOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &userWalletRepoStub{}
	h := NewUserHandler(nil, nil, nil, nil, nil, nil)
	h.SetWalletService(service.NewWalletService(repo, nil, nil))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 17})
		c.Next()
	})
	router.GET("/api/v1/user/wallet", h.GetWallet)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/wallet?user_id=99", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, int64(17), repo.queriedUserID)
	require.Contains(t, rec.Body.String(), `"balance":"12.34000000"`)
	require.NotContains(t, rec.Body.String(), "99")
}

func TestUserWalletHandlerRejectsMissingIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewUserHandler(nil, nil, nil, nil, nil, nil)
	h.SetWalletService(service.NewWalletService(&userWalletRepoStub{}, nil, nil))
	router := gin.New()
	router.GET("/api/v1/user/wallet", h.GetWallet)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/user/wallet", nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

var _ service.WalletRepository = (*userWalletRepoStub)(nil)
