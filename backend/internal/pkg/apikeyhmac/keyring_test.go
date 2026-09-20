package apikeyhmac

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestKeyringActiveDigestKnownVector(t *testing.T) {
	keyring, err := NewKeyring(7, "0123456789abcdef0123456789abcdef", 0, "", "")
	require.NoError(t, err)

	candidate := keyring.Active("sk-test-credential-1234567890")
	require.Equal(t, 7, candidate.Version)
	require.True(t, candidate.Active)
	require.Equal(t, "e65537928f806d6c76b1a148ee062d502a427f7039dd75e24544c5a6c550a1e9", candidate.Digest)
	require.True(t, IsDigest(candidate.Digest))
}

func TestKeyringPreviousWindowIsBounded(t *testing.T) {
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	keyring, err := NewKeyring(
		2,
		"active-pepper-0123456789abcdef-0001",
		1,
		"previous-pepper-0123456789abcd-0001",
		now.Add(time.Hour).Format(time.RFC3339),
	)
	require.NoError(t, err)
	require.Len(t, keyring.Candidates("sk-test-credential", now), 2)
	require.Len(t, keyring.Candidates("sk-test-credential", now.Add(time.Hour)), 1)
}

func TestNewKeyringRejectsUnsafeConfiguration(t *testing.T) {
	pepper := "0123456789abcdef0123456789abcdef"
	_, err := NewKeyring(0, pepper, 0, "", "")
	require.ErrorContains(t, err, "active_version")
	_, err = NewKeyring(1, "short", 0, "", "")
	require.ErrorContains(t, err, "active_pepper")
	_, err = NewKeyring(2, pepper, 1, pepper, time.Now().Add(time.Hour).Format(time.RFC3339))
	require.ErrorContains(t, err, "must differ")
	_, err = NewKeyring(2, pepper, 1, "fedcba9876543210fedcba9876543210", "not-a-time")
	require.ErrorContains(t, err, "RFC3339")
}

func TestDisplayMetadata(t *testing.T) {
	prefix, lastFour := DisplayMetadata("sk-1234567890abcdef")
	require.Equal(t, "sk-12345", prefix)
	require.Equal(t, "cdef", lastFour)
}
