package accountcredential

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testKeyHex(value string) string { return strings.Repeat(value, masterKeyBytes) }

func TestKeyringSealOpenAndRandomNonce(t *testing.T) {
	keyring, err := NewKeyring(7, testKeyHex("ab"), 0, "", "")
	require.NoError(t, err)

	plaintext := []byte(`{"access_token":"secret","expires_at":"2030-01-01T00:00:00Z"}`)
	first, err := keyring.Seal(plaintext, "018f6f52-4a41-7c2e-9cc6-8996f7123456", "openai")
	require.NoError(t, err)
	second, err := keyring.Seal(plaintext, "018f6f52-4a41-7c2e-9cc6-8996f7123456", "openai")
	require.NoError(t, err)

	require.NotEqual(t, first.Ciphertext, second.Ciphertext)
	require.Equal(t, first.Fingerprint, second.Fingerprint)
	require.True(t, IsFingerprint(first.Fingerprint))
	require.Equal(t, 7, first.KeyVersion)

	got, err := keyring.Open(first.Ciphertext, first.KeyVersion, "018f6f52-4a41-7c2e-9cc6-8996f7123456", "openai", time.Now())
	require.NoError(t, err)
	require.Equal(t, plaintext, got)
}

func TestKeyringOpenRejectsAADAndCiphertextTampering(t *testing.T) {
	keyring, err := NewKeyring(1, testKeyHex("11"), 0, "", "")
	require.NoError(t, err)
	sealed, err := keyring.Seal([]byte(`{"api_key":"secret"}`), "aad-1", "deepseek")
	require.NoError(t, err)

	_, err = keyring.Open(sealed.Ciphertext, 1, "aad-2", "deepseek", time.Now())
	require.Error(t, err)
	_, err = keyring.Open(sealed.Ciphertext, 1, "aad-1", "openai", time.Now())
	require.Error(t, err)
	_, err = keyring.Open(sealed.Ciphertext, 2, "aad-1", "deepseek", time.Now())
	require.Error(t, err)

	raw, err := base64.StdEncoding.DecodeString(sealed.Ciphertext)
	require.NoError(t, err)
	raw[len(raw)-1] ^= 0xff
	_, err = keyring.Open(base64.StdEncoding.EncodeToString(raw), 1, "aad-1", "deepseek", time.Now())
	require.Error(t, err)
}

func TestKeyringBoundedPreviousVersion(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	cutoff := now.Add(time.Hour)
	rotating, err := NewKeyring(2, testKeyHex("22"), 1, testKeyHex("11"), cutoff.Format(time.RFC3339))
	require.NoError(t, err)
	oldOnly, err := NewKeyring(1, testKeyHex("11"), 0, "", "")
	require.NoError(t, err)

	old, err := oldOnly.Seal([]byte(`{"refresh_token":"old"}`), "aad-rotation", "openai")
	require.NoError(t, err)
	plaintext, err := rotating.Open(old.Ciphertext, old.KeyVersion, "aad-rotation", "openai", now)
	require.NoError(t, err)
	require.JSONEq(t, `{"refresh_token":"old"}`, string(plaintext))

	_, err = rotating.Open(old.Ciphertext, old.KeyVersion, "aad-rotation", "openai", cutoff)
	require.ErrorContains(t, err, "past its acceptance cutoff")

	candidates, err := rotating.FingerprintCandidates(plaintext, now)
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	require.True(t, candidates[0].Active)
	require.Equal(t, 2, candidates[0].Version)
	require.Equal(t, 1, candidates[1].Version)

	candidates, err = rotating.FingerprintCandidates(plaintext, cutoff)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
}

func TestKeyringLookupCandidatesArePurposeSeparated(t *testing.T) {
	now := time.Now()
	keyring, err := NewKeyring(4, testKeyHex("44"), 0, "", "")
	require.NoError(t, err)
	apiKey, err := keyring.LookupCandidates("api_key", "shared-secret", now)
	require.NoError(t, err)
	refresh, err := keyring.LookupCandidates("refresh_token", "shared-secret", now)
	require.NoError(t, err)
	require.Len(t, apiKey, 1)
	require.NotEqual(t, apiKey[0].Fingerprint, refresh[0].Fingerprint)
	require.True(t, IsFingerprint(apiKey[0].Fingerprint))
}

func TestNewKeyringRejectsInvalidConfiguration(t *testing.T) {
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	tests := []struct {
		name string
		args []any
	}{
		{name: "missing active version", args: []any{0, testKeyHex("11"), 0, "", ""}},
		{name: "short active key", args: []any{1, "abcd", 0, "", ""}},
		{name: "non hex active key", args: []any{1, strings.Repeat("z", 64), 0, "", ""}},
		{name: "partial previous", args: []any{2, testKeyHex("22"), 1, "", future}},
		{name: "same version", args: []any{2, testKeyHex("22"), 2, testKeyHex("11"), future}},
		{name: "same key", args: []any{2, testKeyHex("22"), 1, testKeyHex("22"), future}},
		{name: "invalid cutoff", args: []any{2, testKeyHex("22"), 1, testKeyHex("11"), "tomorrow"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewKeyring(
				test.args[0].(int), test.args[1].(string), test.args[2].(int),
				test.args[3].(string), test.args[4].(string),
			)
			require.Error(t, err)
		})
	}
}

func TestKeyringRejectsInvalidContextAndLookup(t *testing.T) {
	keyring, err := NewKeyring(1, testKeyHex("11"), 0, "", "")
	require.NoError(t, err)
	_, err = keyring.Seal([]byte("{}"), "", "openai")
	require.Error(t, err)
	_, err = keyring.Seal([]byte("{}"), "aad", " openai")
	require.Error(t, err)
	_, err = keyring.LookupCandidates("", "secret", time.Now())
	require.Error(t, err)
	_, err = keyring.LookupCandidates("api_key", "", time.Now())
	require.Error(t, err)
}

func TestKeyringCacheRoundTripAndIdentityBinding(t *testing.T) {
	keyring, err := NewKeyring(4, strings.Repeat("ab", 32), 0, "", "")
	require.NoError(t, err)

	plaintext := []byte(`{"id":42,"credentials":{"api_key":"secret"}}`)
	ciphertext, version, err := keyring.SealCache(plaintext, "42")
	require.NoError(t, err)
	require.Equal(t, 4, version)
	require.NotContains(t, ciphertext, "secret")

	opened, err := keyring.OpenCache(ciphertext, version, "42", time.Now())
	require.NoError(t, err)
	require.Equal(t, plaintext, opened)

	_, err = keyring.OpenCache(ciphertext, version, "43", time.Now())
	require.ErrorContains(t, err, "authenticate account credential cache ciphertext")
}
