package config

import (
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/accountcredential"
)

// AccountCredentialsConfig configures the independent runtime key domain used
// to encrypt upstream provider credentials. Keys are 32 random bytes encoded
// as 64 lowercase or uppercase hex characters and must remain outside the DB.
type AccountCredentialsConfig struct {
	ActiveVersion       int    `mapstructure:"active_version"`
	ActiveKey           string `mapstructure:"active_key"`
	PreviousVersion     int    `mapstructure:"previous_version"`
	PreviousKey         string `mapstructure:"previous_key"`
	PreviousAcceptUntil string `mapstructure:"previous_accept_until"`
}

func (c AccountCredentialsConfig) Keyring() (*accountcredential.Keyring, error) {
	keyring, err := accountcredential.NewKeyring(
		c.ActiveVersion,
		c.ActiveKey,
		c.PreviousVersion,
		c.PreviousKey,
		c.PreviousAcceptUntil,
	)
	if err != nil {
		return nil, fmt.Errorf("account_credentials: %w", err)
	}
	return keyring, nil
}

func (c *Config) ValidateAccountCredentialsForRuntime() error {
	if c == nil {
		return fmt.Errorf("account_credentials: config is required")
	}
	_, err := c.AccountCredentials.Keyring()
	return err
}

func (c AccountCredentialsConfig) PreviousAccepted(now time.Time) bool {
	keyring, err := c.Keyring()
	return err == nil && keyring.PreviousAccepted(now)
}
