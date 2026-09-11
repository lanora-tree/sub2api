package routes

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type featureEnabledFunc func(context.Context) bool
type settingFeatureEnabledFunc func(*service.SettingService, context.Context) bool

// featureEnabledGuard is the server-side boundary for switch-controlled routes.
// Disabled, missing, or unreadable settings all produce the same stable response.
func featureEnabledGuard(feature string, enabled featureEnabledFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if enabled != nil && enabled(c.Request.Context()) {
			c.Next()
			return
		}

		response.ErrorWithDetails(
			c,
			http.StatusNotFound,
			"feature is disabled",
			"FEATURE_DISABLED",
			map[string]string{"feature": feature},
		)
		c.Abort()
	}
}

func settingFeatureEnabledGuard(
	feature string,
	settingService *service.SettingService,
	enabled settingFeatureEnabledFunc,
) gin.HandlerFunc {
	return featureEnabledGuard(feature, func(ctx context.Context) bool {
		return settingService != nil && enabled != nil && enabled(settingService, ctx)
	})
}
