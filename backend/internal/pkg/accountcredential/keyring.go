// Package accountcredential provides versioned authenticated encryption for
// upstream account credential documents. Master keys are supplied only by the
// runtime secret store and must never be persisted with the ciphertext.
package accountcredential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	masterKeyBytes       = 32
	FingerprintHexLength = sha256.Size * 2
	kdfDomain            = "sub2api:account-credentials:kdf:v1\x00"
	aadDomain            = "sub2api:account-credentials:aad:v1\x00"
	fingerprintDomain    = "sub2api:account-credentials:fingerprint:v1\x00"
	lookupDomain         = "sub2api:account-credentials:lookup:v1\x00"
	cacheAADDomain       = "sub2api:account-credentials:cache-aad:v1\x00"
)

// Sealed contains database-safe encrypted credential material. Ciphertext is
// base64(nonce || AES-GCM ciphertext || tag); the key version is stored in a
// separate database column and is also authenticated through AAD.
type Sealed struct {
	Ciphertext  string
	KeyVersion  int
	Fingerprint string
}

// FingerprintCandidate is one keyed digest that may match a stored credential
// document during the bounded active/previous rotation window.
type FingerprintCandidate struct {
	Version     int
	Fingerprint string
	Active      bool
}

type keyVersion struct {
	version        int
	aeadKey        []byte
	fingerprintKey []byte
	lookupKey      []byte
	cacheAEADKey   []byte
}

// Keyring retains one active master key and, during a bounded rotation, at
// most one previous key. Derived subkeys keep encryption and HMAC key domains
// separate even though they originate from the same runtime master secret.
type Keyring struct {
	active              keyVersion
	previous            *keyVersion
	previousAcceptUntil time.Time
}

// NewKeyring validates hex-encoded 256-bit master keys. Previous version, key,
// and RFC3339 cutoff must either all be configured or all be absent.
func NewKeyring(activeVersion int, activeKeyHex string, previousVersion int, previousKeyHex, previousAcceptUntil string) (*Keyring, error) {
	active, err := parseKeyVersion("active", activeVersion, activeKeyHex)
	if err != nil {
		return nil, err
	}
	keyring := &Keyring{active: active}

	previousConfigured := previousVersion != 0 || previousKeyHex != "" || previousAcceptUntil != ""
	if !previousConfigured {
		return keyring, nil
	}
	if previousVersion == activeVersion {
		return nil, fmt.Errorf("previous_version must differ from active_version")
	}
	previous, err := parseKeyVersion("previous", previousVersion, previousKeyHex)
	if err != nil {
		return nil, err
	}
	if hmac.Equal(active.aeadKey, previous.aeadKey) {
		return nil, fmt.Errorf("previous_key must differ from active_key")
	}
	cutoff, err := time.Parse(time.RFC3339, previousAcceptUntil)
	if err != nil {
		return nil, fmt.Errorf("previous_accept_until must be RFC3339: %w", err)
	}
	keyring.previous = &previous
	keyring.previousAcceptUntil = cutoff
	return keyring, nil
}

func parseKeyVersion(label string, version int, encoded string) (keyVersion, error) {
	if version <= 0 {
		return keyVersion{}, fmt.Errorf("%s_version must be positive", label)
	}
	if strings.TrimSpace(encoded) != encoded || encoded == "" {
		return keyVersion{}, fmt.Errorf("%s_key must be exactly %d hex-encoded bytes with no surrounding whitespace", label, masterKeyBytes)
	}
	master, err := hex.DecodeString(encoded)
	if err != nil || len(master) != masterKeyBytes {
		return keyVersion{}, fmt.Errorf("%s_key must be exactly %d hex-encoded bytes", label, masterKeyBytes)
	}
	return keyVersion{
		version:        version,
		aeadKey:        derive(master, "aead"),
		fingerprintKey: derive(master, "fingerprint"),
		lookupKey:      derive(master, "lookup"),
		cacheAEADKey:   derive(master, "scheduler-cache-aead"),
	}, nil
}

// SealCache encrypts an ephemeral scheduler-cache payload under a subkey that
// is independent from the database credential cipher. The cache identity is
// authenticated so ciphertext cannot be moved between account cache keys.
func (k *Keyring) SealCache(plaintext []byte, cacheIdentity string) (string, int, error) {
	if k == nil {
		return "", 0, fmt.Errorf("account credential keyring is required")
	}
	if strings.TrimSpace(cacheIdentity) != cacheIdentity || cacheIdentity == "" {
		return "", 0, fmt.Errorf("account credential cache identity is required and must contain no surrounding whitespace")
	}
	payload, err := sealWithKey(k.active.cacheAEADKey, plaintext, buildCacheAAD(cacheIdentity, k.active.version))
	if err != nil {
		return "", 0, err
	}
	return base64.StdEncoding.EncodeToString(payload), k.active.version, nil
}

// OpenCache authenticates and decrypts an ephemeral scheduler-cache payload.
func (k *Keyring) OpenCache(ciphertext string, keyVersion int, cacheIdentity string, now time.Time) ([]byte, error) {
	if k == nil {
		return nil, fmt.Errorf("account credential keyring is required")
	}
	if strings.TrimSpace(cacheIdentity) != cacheIdentity || cacheIdentity == "" {
		return nil, fmt.Errorf("account credential cache identity is required and must contain no surrounding whitespace")
	}
	key, err := k.keyForVersion(keyVersion, now)
	if err != nil {
		return nil, err
	}
	payload, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode account credential cache ciphertext: %w", err)
	}
	return openWithKey(key.cacheAEADKey, payload, buildCacheAAD(cacheIdentity, keyVersion), "account credential cache")
}

func sealWithKey(key, plaintext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create account credential cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create account credential gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate account credential nonce: %w", err)
	}
	return append(nonce, gcm.Seal(nil, nonce, plaintext, aad)...), nil
}

func openWithKey(key, payload, aad []byte, label string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create %s cipher: %w", label, err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create %s gcm: %w", label, err)
	}
	if len(payload) < gcm.NonceSize()+gcm.Overhead() {
		return nil, fmt.Errorf("%s ciphertext is too short", label)
	}
	plaintext, err := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], aad)
	if err != nil {
		return nil, fmt.Errorf("authenticate %s ciphertext: %w", label, err)
	}
	return plaintext, nil
}

func derive(master []byte, label string) []byte {
	mac := hmac.New(sha256.New, master)
	_, _ = mac.Write([]byte(kdfDomain))
	_, _ = mac.Write([]byte(label))
	return mac.Sum(nil)
}

// Seal encrypts one canonical credential document using the active key.
func (k *Keyring) Seal(plaintext []byte, aadID, platform string) (Sealed, error) {
	if k == nil {
		return Sealed{}, fmt.Errorf("account credential keyring is required")
	}
	if err := validateContext(aadID, platform); err != nil {
		return Sealed{}, err
	}
	payload, err := sealWithKey(k.active.aeadKey, plaintext, buildAAD(aadID, platform, k.active.version))
	if err != nil {
		return Sealed{}, err
	}
	return Sealed{
		Ciphertext:  base64.StdEncoding.EncodeToString(payload),
		KeyVersion:  k.active.version,
		Fingerprint: fingerprint(k.active.fingerprintKey, plaintext),
	}, nil
}

// Open authenticates and decrypts one credential document. A previous key is
// accepted only before its explicit cutoff; unknown or expired versions fail
// closed.
func (k *Keyring) Open(ciphertext string, keyVersion int, aadID, platform string, now time.Time) ([]byte, error) {
	if k == nil {
		return nil, fmt.Errorf("account credential keyring is required")
	}
	if err := validateContext(aadID, platform); err != nil {
		return nil, err
	}
	key, err := k.keyForVersion(keyVersion, now)
	if err != nil {
		return nil, err
	}
	payload, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode account credential ciphertext: %w", err)
	}
	return openWithKey(key.aeadKey, payload, buildAAD(aadID, platform, keyVersion), "account credential")
}

// FingerprintCandidates returns the active keyed fingerprint followed by the
// previous candidate while the rotation window is open. These values preserve
// compare-and-swap semantics without storing plaintext or an unkeyed digest.
func (k *Keyring) FingerprintCandidates(plaintext []byte, now time.Time) ([]FingerprintCandidate, error) {
	if k == nil {
		return nil, fmt.Errorf("account credential keyring is required")
	}
	out := []FingerprintCandidate{{
		Version:     k.active.version,
		Fingerprint: fingerprint(k.active.fingerprintKey, plaintext),
		Active:      true,
	}}
	if k.PreviousAccepted(now) {
		out = append(out, FingerprintCandidate{
			Version:     k.previous.version,
			Fingerprint: fingerprint(k.previous.fingerprintKey, plaintext),
		})
	}
	return out, nil
}

// LookupCandidates creates keyed, cross-row lookup digests for the small set
// of repository operations that must locate accounts by one credential value.
// Purpose is domain-separated (for example "api_key") and never stored with
// the raw value.
func (k *Keyring) LookupCandidates(purpose, raw string, now time.Time) ([]FingerprintCandidate, error) {
	if k == nil {
		return nil, fmt.Errorf("account credential keyring is required")
	}
	if strings.TrimSpace(purpose) != purpose || purpose == "" {
		return nil, fmt.Errorf("account credential lookup purpose is required and must contain no surrounding whitespace")
	}
	if raw == "" {
		return nil, fmt.Errorf("account credential lookup value is required")
	}
	out := []FingerprintCandidate{{
		Version:     k.active.version,
		Fingerprint: lookupDigest(k.active.lookupKey, purpose, raw),
		Active:      true,
	}}
	if k.PreviousAccepted(now) {
		out = append(out, FingerprintCandidate{
			Version:     k.previous.version,
			Fingerprint: lookupDigest(k.previous.lookupKey, purpose, raw),
		})
	}
	return out, nil
}

func (k *Keyring) ActiveVersion() int {
	if k == nil {
		return 0
	}
	return k.active.version
}

func (k *Keyring) PreviousVersion() int {
	if k == nil || k.previous == nil {
		return 0
	}
	return k.previous.version
}

func (k *Keyring) PreviousAccepted(now time.Time) bool {
	return k != nil && k.previous != nil && now.Before(k.previousAcceptUntil)
}

func (k *Keyring) keyForVersion(version int, now time.Time) (keyVersion, error) {
	if version == k.active.version {
		return k.active, nil
	}
	if k.previous != nil && version == k.previous.version {
		if !k.PreviousAccepted(now) {
			return keyVersion{}, fmt.Errorf("account credential key version %d is past its acceptance cutoff", version)
		}
		return *k.previous, nil
	}
	return keyVersion{}, fmt.Errorf("unknown account credential key version %d", version)
}

func validateContext(aadID, platform string) error {
	if strings.TrimSpace(aadID) != aadID || aadID == "" {
		return fmt.Errorf("account credential aad_id is required and must contain no surrounding whitespace")
	}
	if strings.TrimSpace(platform) != platform || platform == "" {
		return fmt.Errorf("account credential platform is required and must contain no surrounding whitespace")
	}
	return nil
}

func buildAAD(aadID, platform string, version int) []byte {
	out := make([]byte, 0, len(aadDomain)+len(aadID)+len(platform)+12)
	out = append(out, aadDomain...)
	out = appendLengthPrefixed(out, aadID)
	out = appendLengthPrefixed(out, platform)
	versionBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(versionBytes, uint32(version))
	return append(out, versionBytes...)
}

func buildCacheAAD(cacheIdentity string, version int) []byte {
	out := make([]byte, 0, len(cacheAADDomain)+len(cacheIdentity)+8)
	out = append(out, cacheAADDomain...)
	out = appendLengthPrefixed(out, cacheIdentity)
	versionBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(versionBytes, uint32(version))
	return append(out, versionBytes...)
}

func appendLengthPrefixed(dst []byte, value string) []byte {
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(value)))
	dst = append(dst, length...)
	return append(dst, value...)
}

func fingerprint(key, plaintext []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(fingerprintDomain))
	_, _ = mac.Write(plaintext)
	return hex.EncodeToString(mac.Sum(nil))
}

func lookupDigest(key []byte, purpose, raw string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(lookupDomain))
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(raw))
	return hex.EncodeToString(mac.Sum(nil))
}

func IsFingerprint(value string) bool {
	if len(value) != FingerprintHexLength || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
