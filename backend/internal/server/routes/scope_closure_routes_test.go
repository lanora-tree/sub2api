package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func scopeClosureSettingService(values map[string]string) *service.SettingService {
	return service.NewSettingService(&channelMonitorRouteSettingRepoStub{values: values}, &config.Config{})
}

func passthroughJWT(c *gin.Context)   { c.Next() }
func passthroughAdmin(c *gin.Context) { c.Next() }
func passthroughAudit(c *gin.Context) { c.Next() }

func TestAuthScopeRoutesReturnStableClosedResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")

	RegisterAuthRoutes(
		v1,
		&handler.Handlers{Auth: &handler.AuthHandler{}, Setting: &handler.SettingHandler{}},
		servermiddleware.JWTAuthMiddleware(passthroughJWT),
		servermiddleware.AuditLogMiddleware(passthroughAudit),
		nil,
		scopeClosureSettingService(map[string]string{
			service.SettingKeyRegistrationEnabled:   "false",
			service.SettingKeyPromoCodeEnabled:      "false",
			service.SettingKeyInvitationCodeEnabled: "false",
		}),
		nil,
	)

	for _, path := range []string{
		"/api/v1/auth/register",
		"/api/v1/auth/validate-promo-code",
		"/api/v1/auth/validate-invitation-code",
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusNotFound, recorder.Code, "path=%s", path)
		require.Contains(t, recorder.Body.String(), `"reason":"FEATURE_DISABLED"`, "path=%s", path)
	}
}

func TestPaymentPublicAndWebhookRoutesReturnStableClosedResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")

	RegisterPaymentRoutes(
		v1,
		&handler.PaymentHandler{},
		&handler.PaymentWebhookHandler{},
		&adminhandler.PaymentHandler{},
		servermiddleware.JWTAuthMiddleware(passthroughJWT),
		servermiddleware.AdminAuthMiddleware(passthroughAdmin),
		servermiddleware.AuditLogMiddleware(passthroughAudit),
		scopeClosureSettingService(map[string]string{service.SettingPaymentEnabled: "false"}),
		nil,
	)

	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/v1/payment/public/orders/verify"},
		{method: http.MethodPost, path: "/api/v1/payment/webhook/stripe"},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(`{}`))
		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusNotFound, recorder.Code, "path=%s", test.path)
		require.Contains(t, recorder.Body.String(), `"reason":"FEATURE_DISABLED"`, "path=%s", test.path)
		require.Contains(t, recorder.Body.String(), `"feature":"payment"`, "path=%s", test.path)
	}
}

func TestUserSelfServiceRoutesReturnStableClosedResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")

	RegisterUserRoutes(
		v1,
		&handler.Handlers{
			User:             &handler.UserHandler{},
			APIKey:           &handler.APIKeyHandler{},
			Usage:            &handler.UsageHandler{},
			Redeem:           &handler.RedeemHandler{},
			Subscription:     &handler.SubscriptionHandler{},
			Announcement:     &handler.AnnouncementHandler{},
			ChannelMonitor:   &handler.ChannelMonitorUserHandler{},
			ChannelMonitorV2: &handler.ChannelMonitorV2Handler{},
			Totp:             &handler.TotpHandler{},
			Passkey:          &handler.PasskeyHandler{},
			AvailableChannel: &handler.AvailableChannelHandler{},
		},
		servermiddleware.JWTAuthMiddleware(passthroughJWT),
		servermiddleware.AuditLogMiddleware(passthroughAudit),
		scopeClosureSettingService(map[string]string{
			service.SettingPaymentEnabled:              "false",
			service.SettingKeyAffiliateEnabled:         "false",
			service.SettingKeyAvailableChannelsEnabled: "false",
			service.SettingKeyChannelMonitorEnabled:    "false",
		}),
		nil,
	)

	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/v1/redeem"},
		{method: http.MethodGet, path: "/api/v1/subscriptions"},
		{method: http.MethodGet, path: "/api/v1/user/aff"},
		{method: http.MethodGet, path: "/api/v1/channels/available"},
		{method: http.MethodGet, path: "/api/v1/channel-monitors"},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(`{}`))
		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusNotFound, recorder.Code, "path=%s", test.path)
		require.Contains(t, recorder.Body.String(), `"reason":"FEATURE_DISABLED"`, "path=%s", test.path)
	}
}
