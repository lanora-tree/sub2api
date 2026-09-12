package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterUserManagementRoutesIncludesWalletEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userHandler := adminhandler.NewUserHandler(nil, nil, nil, nil, nil, nil, nil)
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{User: userHandler}}
	router := gin.New()
	stepUp := middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() })
	registerUserManagementRoutes(router.Group("/api/v1/admin"), handlers, stepUp)

	tests := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{http.MethodGet, "/api/v1/admin/users/7/wallet", "", http.StatusInternalServerError},
		{http.MethodPost, "/api/v1/admin/users/7/wallet/transactions", `{}`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/admin/users/7/wallet/refunds", `{}`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/admin/users/7/balance", `{}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
		if tt.body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		router.ServeHTTP(recorder, request)
		require.Equal(t, tt.status, recorder.Code, "%s %s: %s", tt.method, tt.path, recorder.Body.String())
	}
}

func TestRegisterUserManagementRoutesGatesWalletWritesWithStepUp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userHandler := adminhandler.NewUserHandler(nil, nil, nil, nil, nil, nil, nil)
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{User: userHandler}}
	router := gin.New()
	stepUpCalls := 0
	stepUp := middleware.StepUpAuthMiddleware(func(c *gin.Context) {
		stepUpCalls++
		c.AbortWithStatus(http.StatusTeapot)
	})
	registerUserManagementRoutes(router.Group("/api/v1/admin"), handlers, stepUp)

	for _, path := range []string{
		"/api/v1/admin/users/7/wallet/transactions",
		"/api/v1/admin/users/7/wallet/refunds",
		"/api/v1/admin/users/7/balance",
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusTeapot, recorder.Code, path)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/7/wallet", nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Equal(t, 3, stepUpCalls)
}
