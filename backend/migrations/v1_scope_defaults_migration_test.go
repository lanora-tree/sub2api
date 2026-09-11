package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestV1ScopeDefaultsDisableEveryOutOfScopeFeature(t *testing.T) {
	content, err := FS.ReadFile("239_v1_scope_defaults.sql")
	require.NoError(t, err)
	sql := string(content)

	for _, key := range []string{
		"registration_enabled",
		"payment_enabled",
		"promo_code_enabled",
		"invitation_code_enabled",
		"affiliate_enabled",
		"purchase_subscription_enabled",
		"channel_monitor_enabled",
		"available_channels_enabled",
		"model_plaza_enabled",
		"plugin_management_enabled",
		"linuxdo_connect_enabled",
		"dingtalk_connect_enabled",
		"wechat_connect_enabled",
		"wechat_connect_open_enabled",
		"wechat_connect_mp_enabled",
		"wechat_connect_mobile_enabled",
		"oidc_connect_enabled",
		"github_oauth_enabled",
		"google_oauth_enabled",
	} {
		require.Contains(t, sql, "('"+key+"', 'false', NOW())")
	}

	require.Contains(t, strings.ToUpper(sql), "ON CONFLICT (KEY) DO UPDATE")
}
