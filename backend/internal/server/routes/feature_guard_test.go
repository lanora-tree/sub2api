package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFeatureEnabledGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		enabled    featureEnabledFunc
		wantStatus int
	}{
		{name: "nil fails closed", enabled: nil, wantStatus: http.StatusNotFound},
		{name: "disabled", enabled: func(context.Context) bool { return false }, wantStatus: http.StatusNotFound},
		{name: "enabled", enabled: func(context.Context) bool { return true }, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(featureEnabledGuard("payment", tt.enabled))
			router.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))

			require.Equal(t, tt.wantStatus, recorder.Code)
			if tt.wantStatus == http.StatusNotFound {
				require.Contains(t, recorder.Body.String(), `"reason":"FEATURE_DISABLED"`)
				require.Contains(t, recorder.Body.String(), `"feature":"payment"`)
			}
		})
	}
}

func TestSettingFeatureEnabledGuardFailsClosedWithoutService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(settingFeatureEnabledGuard(
		"registration",
		nil,
		func(s *service.SettingService, ctx context.Context) bool { return s.IsRegistrationEnabled(ctx) },
	))
	router.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))
	require.Equal(t, http.StatusNotFound, recorder.Code)
}
