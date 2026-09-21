package repository

import (
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/accountcredential"
	"github.com/google/uuid"
)

const accountCredentialLookupPurposeAPIKey = "api_key"

// accountCredentialColumns is the complete persistence projection for one
// decrypted Account.Credentials map. The legacy JSONB column is intentionally
// omitted because every encrypted write replaces it with an empty object.
type accountCredentialColumns struct {
	Ciphertext      string
	KeyVersion      int
	AADID           uuid.UUID
	Fingerprint     string
	APIKeyDigest    string
	HasRefreshToken bool
	Metadata        map[string]any
}

type accountCredentialStore struct {
	keyring *accountcredential.Keyring
}

func newAccountCredentialStore(keyring *accountcredential.Keyring) (*accountCredentialStore, error) {
	if keyring == nil {
		return nil, fmt.Errorf("account credential keyring is required")
	}
	return &accountCredentialStore{keyring: keyring}, nil
}

func (s *accountCredentialStore) seal(credentials map[string]any, aadID uuid.UUID, platform string) (accountCredentialColumns, error) {
	if s == nil || s.keyring == nil {
		return accountCredentialColumns{}, fmt.Errorf("account credential store is required")
	}
	if aadID == uuid.Nil {
		aadID = uuid.New()
	}
	canonical, err := json.Marshal(normalizeJSONMap(credentials))
	if err != nil {
		return accountCredentialColumns{}, fmt.Errorf("marshal account credentials: %w", err)
	}
	sealed, err := s.keyring.Seal(canonical, aadID.String(), platform)
	if err != nil {
		return accountCredentialColumns{}, err
	}
	columns := accountCredentialColumns{
		Ciphertext:      sealed.Ciphertext,
		KeyVersion:      sealed.KeyVersion,
		AADID:           aadID,
		Fingerprint:     sealed.Fingerprint,
		HasRefreshToken: nonEmptyCredentialString(credentials, "refresh_token"),
		Metadata:        accountCredentialMetadata(credentials),
	}
	if apiKey, ok := credentialString(credentials, "api_key"); ok && strings.TrimSpace(apiKey) != "" {
		candidates, err := s.keyring.LookupCandidates(accountCredentialLookupPurposeAPIKey, apiKey, time.Now())
		if err != nil {
			return accountCredentialColumns{}, err
		}
		columns.APIKeyDigest = candidates[0].Fingerprint
	}
	return columns, nil
}

func (s *accountCredentialStore) open(columns accountCredentialColumns, platform string, now time.Time) (map[string]any, bool, error) {
	if s == nil || s.keyring == nil {
		return nil, false, fmt.Errorf("account credential store is required")
	}
	if columns.AADID == uuid.Nil || columns.Ciphertext == "" || columns.KeyVersion <= 0 {
		return nil, false, fmt.Errorf("account credential encrypted storage is incomplete")
	}
	plaintext, err := s.keyring.Open(columns.Ciphertext, columns.KeyVersion, columns.AADID.String(), platform, now)
	if err != nil {
		return nil, false, err
	}
	candidates, err := s.keyring.FingerprintCandidates(plaintext, now)
	if err != nil {
		return nil, false, err
	}
	fingerprintValid := false
	for _, candidate := range candidates {
		if candidate.Version == columns.KeyVersion && hmac.Equal([]byte(candidate.Fingerprint), []byte(columns.Fingerprint)) {
			fingerprintValid = true
			break
		}
	}
	if !fingerprintValid {
		return nil, false, fmt.Errorf("account credential fingerprint mismatch for key version %d", columns.KeyVersion)
	}
	var credentials map[string]any
	if err := json.Unmarshal(plaintext, &credentials); err != nil {
		return nil, false, fmt.Errorf("decode account credential document: %w", err)
	}
	if credentials == nil {
		credentials = map[string]any{}
	}
	return credentials, columns.KeyVersion != s.keyring.ActiveVersion(), nil
}

func (s *accountCredentialStore) fingerprintCandidates(credentials map[string]any, now time.Time) ([]accountcredential.FingerprintCandidate, error) {
	canonical, err := json.Marshal(normalizeJSONMap(credentials))
	if err != nil {
		return nil, fmt.Errorf("marshal expected account credentials: %w", err)
	}
	return s.keyring.FingerprintCandidates(canonical, now)
}

func (s *accountCredentialStore) apiKeyLookupCandidates(apiKey string, now time.Time) ([]accountcredential.FingerprintCandidate, error) {
	return s.keyring.LookupCandidates(accountCredentialLookupPurposeAPIKey, apiKey, now)
}

// accountCredentialMetadata is deliberately allowlist-only. Unknown fields
// default to sensitive and remain solely inside authenticated ciphertext.
func accountCredentialMetadata(credentials map[string]any) map[string]any {
	if len(credentials) == 0 {
		return map[string]any{}
	}
	metadata := make(map[string]any)
	if value, ok := credentialString(credentials, "base_url"); ok {
		matched, _ := regexp.MatchString(ollamaCloudBaseURLRegexSQL, value)
		if !matched {
			return metadata
		}
		metadata["base_url"] = value
	}
	return metadata
}

func credentialString(credentials map[string]any, key string) (string, bool) {
	value, ok := credentials[key]
	if !ok || value == nil {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

func nonEmptyCredentialString(credentials map[string]any, key string) bool {
	value, ok := credentialString(credentials, key)
	return ok && strings.TrimSpace(value) != ""
}
