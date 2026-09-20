package repository

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apikeyhmac"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const testAPIKeyHMACPepper = "test-api-key-pepper-0123456789abcdef"

func withTestAPIKeyCredential(key *service.APIKey) *service.APIKey {
	if key == nil || apikeyhmac.IsDigest(key.KeyHash) {
		return key
	}
	keyring, err := apikeyhmac.NewKeyring(1, testAPIKeyHMACPepper, 0, "", "")
	if err != nil {
		panic(err)
	}
	candidate := keyring.Active(key.Key)
	key.KeyHash = candidate.Digest
	key.KeyVersion = candidate.Version
	key.KeyPrefix, key.KeyLastFour = apikeyhmac.DisplayMetadata(key.Key)
	return key
}

func TestInitEntRejectsMissingPepperBeforeDatabaseOpen(t *testing.T) {
	_, _, err := InitEnt(&config.Config{Timezone: "UTC"})
	if err == nil || !strings.Contains(err.Error(), "validate API key HMAC config") {
		t.Fatalf("expected pre-migration HMAC validation error, got %v", err)
	}
}

func TestValidateAPIKeyCredentialRejectsPlaintextOnly(t *testing.T) {
	err := validateAPIKeyCredential(&service.APIKey{Key: "sk-user-plaintext"})
	if err == nil {
		t.Fatal("expected plaintext-only credential to be rejected")
	}
}

func TestValidateAPIKeyCredentialAcceptsHMACMetadata(t *testing.T) {
	keyring, err := apikeyhmac.NewKeyring(7, "0123456789abcdef0123456789abcdef", 0, "", "")
	if err != nil {
		t.Fatalf("build keyring: %v", err)
	}
	raw := "sk-user-example-secret"
	candidate := keyring.Active(raw)
	prefix, lastFour := apikeyhmac.DisplayMetadata(raw)

	err = validateAPIKeyCredential(&service.APIKey{
		Key:         raw,
		KeyHash:     candidate.Digest,
		KeyPrefix:   prefix,
		KeyLastFour: lastFour,
		KeyVersion:  candidate.Version,
	})
	if err != nil {
		t.Fatalf("expected HMAC credential to pass validation: %v", err)
	}
}
