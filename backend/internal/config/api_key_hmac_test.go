package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadAPIKeyHMACFromEnvironment(t *testing.T) {
	resetViperWithJWTSecret(t)
	activePepper := strings.Repeat("a", 32)
	previousPepper := strings.Repeat("b", 32)
	cutoff := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	t.Setenv("API_KEY_HMAC_ACTIVE_VERSION", "9")
	t.Setenv("API_KEY_HMAC_ACTIVE_PEPPER", activePepper)
	t.Setenv("API_KEY_HMAC_PREVIOUS_VERSION", "8")
	t.Setenv("API_KEY_HMAC_PREVIOUS_PEPPER", previousPepper)
	t.Setenv("API_KEY_HMAC_PREVIOUS_ACCEPT_UNTIL", cutoff)

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 9, cfg.APIKeyHMAC.ActiveVersion)
	require.Equal(t, activePepper, cfg.APIKeyHMAC.ActivePepper)
	require.Equal(t, 8, cfg.APIKeyHMAC.PreviousVersion)
	require.Equal(t, previousPepper, cfg.APIKeyHMAC.PreviousPepper)
	require.Equal(t, cutoff, cfg.APIKeyHMAC.PreviousAcceptUntil)
	require.NoError(t, cfg.ValidateAPIKeyHMACForRuntime())
}

func TestValidateAPIKeyHMACForRuntimeFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		cfg  APIKeyHMACConfig
	}{
		{name: "missing active Pepper", cfg: APIKeyHMACConfig{ActiveVersion: 1}},
		{name: "short active Pepper", cfg: APIKeyHMACConfig{ActiveVersion: 1, ActivePepper: "short"}},
		{name: "partial previous config", cfg: APIKeyHMACConfig{
			ActiveVersion: 2, ActivePepper: strings.Repeat("a", 32), PreviousVersion: 1,
		}},
		{name: "same versions", cfg: APIKeyHMACConfig{
			ActiveVersion: 2, ActivePepper: strings.Repeat("a", 32),
			PreviousVersion: 2, PreviousPepper: strings.Repeat("b", 32),
			PreviousAcceptUntil: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := (&Config{APIKeyHMAC: test.cfg}).ValidateAPIKeyHMACForRuntime()
			require.Error(t, err)
		})
	}
}
