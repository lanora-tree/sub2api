package config

import (
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apikeyhmac"
)

// APIKeyHMACConfig configures database-safe user API key digests. Peppers
// must be injected by the runtime secret store, never written to PostgreSQL.
type APIKeyHMACConfig struct {
	ActiveVersion       int    `mapstructure:"active_version"`
	ActivePepper        string `mapstructure:"active_pepper"`
	PreviousVersion     int    `mapstructure:"previous_version"`
	PreviousPepper      string `mapstructure:"previous_pepper"`
	PreviousAcceptUntil string `mapstructure:"previous_accept_until"`
}

func (c APIKeyHMACConfig) Keyring() (*apikeyhmac.Keyring, error) {
	keyring, err := apikeyhmac.NewKeyring(
		c.ActiveVersion,
		c.ActivePepper,
		c.PreviousVersion,
		c.PreviousPepper,
		c.PreviousAcceptUntil,
	)
	if err != nil {
		return nil, fmt.Errorf("api_key_hmac: %w", err)
	}
	return keyring, nil
}

// ValidateAPIKeyHMACForRuntime is intentionally separate from Config.Validate:
// repository/bootstrap tests construct partial Config values, while every real
// server startup calls this stricter gate before credential backfill.
func (c *Config) ValidateAPIKeyHMACForRuntime() error {
	if c == nil {
		return fmt.Errorf("api_key_hmac: config is required")
	}
	_, err := c.APIKeyHMAC.Keyring()
	return err
}

func (c APIKeyHMACConfig) PreviousAccepted(now time.Time) bool {
	keyring, err := c.Keyring()
	return err == nil && keyring.PreviousAccepted(now)
}
