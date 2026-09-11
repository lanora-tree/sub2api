-- M2 / V1 scope closure.
-- These capabilities remain in source and can be restored by an administrator,
-- but this product profile intentionally starts with every out-of-scope switch off.
INSERT INTO settings (key, value, updated_at)
VALUES
    ('registration_enabled', 'false', NOW()),
    ('payment_enabled', 'false', NOW()),
    ('promo_code_enabled', 'false', NOW()),
    ('invitation_code_enabled', 'false', NOW()),
    ('affiliate_enabled', 'false', NOW()),
    ('purchase_subscription_enabled', 'false', NOW()),
    ('channel_monitor_enabled', 'false', NOW()),
    ('available_channels_enabled', 'false', NOW()),
    ('model_plaza_enabled', 'false', NOW()),
    ('plugin_management_enabled', 'false', NOW()),
    ('linuxdo_connect_enabled', 'false', NOW()),
    ('dingtalk_connect_enabled', 'false', NOW()),
    ('wechat_connect_enabled', 'false', NOW()),
    ('wechat_connect_open_enabled', 'false', NOW()),
    ('wechat_connect_mp_enabled', 'false', NOW()),
    ('wechat_connect_mobile_enabled', 'false', NOW()),
    ('oidc_connect_enabled', 'false', NOW()),
    ('github_oauth_enabled', 'false', NOW()),
    ('google_oauth_enabled', 'false', NOW())
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    updated_at = NOW();
