package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadAccountCredentialsFromEnvironment(t *testing.T) {
	resetViperWithJWTSecret(t)
	activeKey := strings.Repeat("ab", 32)
	previousKey := strings.Repeat("cd", 32)
	cutoff := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	t.Setenv("ACCOUNT_CREDENTIALS_ACTIVE_VERSION", "9")
	t.Setenv("ACCOUNT_CREDENTIALS_ACTIVE_KEY", activeKey)
	t.Setenv("ACCOUNT_CREDENTIALS_PREVIOUS_VERSION", "8")
	t.Setenv("ACCOUNT_CREDENTIALS_PREVIOUS_KEY", previousKey)
	t.Setenv("ACCOUNT_CREDENTIALS_PREVIOUS_ACCEPT_UNTIL", cutoff)

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 9, cfg.AccountCredentials.ActiveVersion)
	require.Equal(t, activeKey, cfg.AccountCredentials.ActiveKey)
	require.Equal(t, 8, cfg.AccountCredentials.PreviousVersion)
	require.Equal(t, previousKey, cfg.AccountCredentials.PreviousKey)
	require.Equal(t, cutoff, cfg.AccountCredentials.PreviousAcceptUntil)
	require.NoError(t, cfg.ValidateAccountCredentialsForRuntime())
}

func TestValidateAccountCredentialsForRuntimeFailsClosed(t *testing.T) {
	validKey := strings.Repeat("ab", 32)
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	tests := []AccountCredentialsConfig{
		{ActiveVersion: 1},
		{ActiveVersion: 1, ActiveKey: "short"},
		{ActiveVersion: 2, ActiveKey: validKey, PreviousVersion: 1},
		{ActiveVersion: 2, ActiveKey: validKey, PreviousVersion: 2, PreviousKey: strings.Repeat("cd", 32), PreviousAcceptUntil: future},
	}
	for _, cfg := range tests {
		require.Error(t, (&Config{AccountCredentials: cfg}).ValidateAccountCredentialsForRuntime())
	}
}
