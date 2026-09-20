package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apikeyhmac"
)

const testAPIKeyHMACPepper = "middleware-test-api-key-pepper-0123456789"

func testAPIKeyConfig(cfg *config.Config) *config.Config {
	if cfg == nil {
		cfg = &config.Config{}
	}
	cfg.APIKeyHMAC = config.APIKeyHMACConfig{
		ActiveVersion: 1,
		ActivePepper:  testAPIKeyHMACPepper,
	}
	return cfg
}

func testAPIKeyDigest(raw string) string {
	keyring, err := apikeyhmac.NewKeyring(1, testAPIKeyHMACPepper, 0, "", "")
	if err != nil {
		panic(err)
	}
	return keyring.Active(raw).Digest
}
