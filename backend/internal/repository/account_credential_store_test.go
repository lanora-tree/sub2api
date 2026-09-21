package repository

import (
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/accountcredential"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func testAccountCredentialStore(t *testing.T) *accountCredentialStore {
	t.Helper()
	keyring, err := accountcredential.NewKeyring(1, strings.Repeat("ab", 32), 0, "", "")
	require.NoError(t, err)
	store, err := newAccountCredentialStore(keyring)
	require.NoError(t, err)
	return store
}

func TestAccountCredentialStoreRoundTripAndSafeProjection(t *testing.T) {
	store := testAccountCredentialStore(t)
	aadID := uuid.New()
	credentials := map[string]any{
		"api_key":       "provider-secret",
		"refresh_token": "refresh-secret",
		"base_url":      "https://ollama.com",
		"model_mapping": map[string]any{"public": "upstream"},
		"unknown_token": "must-stay-encrypted",
	}

	columns, err := store.seal(credentials, aadID, "openai")
	require.NoError(t, err)
	require.NotContains(t, columns.Ciphertext, "provider-secret")
	require.NotEmpty(t, columns.APIKeyDigest)
	require.True(t, columns.HasRefreshToken)
	require.Equal(t, credentials["base_url"], columns.Metadata["base_url"])
	require.NotContains(t, columns.Metadata, "model_mapping")
	require.NotContains(t, columns.Metadata, "api_key")
	require.NotContains(t, columns.Metadata, "refresh_token")
	require.NotContains(t, columns.Metadata, "unknown_token")

	got, needsRewrap, err := store.open(columns, "openai", time.Now())
	require.NoError(t, err)
	require.False(t, needsRewrap)
	require.Equal(t, credentials, got)
}

func TestAccountCredentialStoreRejectsFingerprintMismatch(t *testing.T) {
	store := testAccountCredentialStore(t)
	columns, err := store.seal(map[string]any{"access_token": "secret"}, uuid.New(), "openai")
	require.NoError(t, err)
	columns.Fingerprint = strings.Repeat("0", 64)

	_, _, err = store.open(columns, "openai", time.Now())
	require.ErrorContains(t, err, "fingerprint mismatch")
}

func TestAccountCredentialMetadataDefaultsUnknownFieldsToSensitive(t *testing.T) {
	metadata := accountCredentialMetadata(map[string]any{
		"base_url":             "https://example.invalid",
		"service_account_json": "secret-json",
		"agent_private_key":    "secret-key",
		"cookie":               "secret-cookie",
	})
	require.Empty(t, metadata)
}
