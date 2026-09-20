// Package apikeyhmac provides versioned, domain-separated HMAC digests for
// user API key credentials. Peppers are deliberately supplied by runtime
// configuration and must never be persisted in the application database.
package apikeyhmac

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	DigestHexLength  = sha256.Size * 2
	minimumPepperLen = 32
	digestDomain     = "sub2api:user-api-key:hmac-sha256:v1\x00"
)

// Candidate identifies one digest that may authenticate a raw API key.
type Candidate struct {
	Version int
	Digest  string
	Active  bool
}

// Keyring contains the active credential pepper and, during a bounded
// rotation window, one previous pepper. Pepper bytes remain unexported.
type Keyring struct {
	activeVersion       int
	activePepper        []byte
	previousVersion     int
	previousPepper      []byte
	previousAcceptUntil time.Time
}

// NewKeyring validates and constructs a versioned HMAC keyring. The previous
// version, pepper, and RFC3339 deadline must either all be configured or all
// be absent.
func NewKeyring(activeVersion int, activePepper string, previousVersion int, previousPepper, previousAcceptUntil string) (*Keyring, error) {
	if activeVersion <= 0 {
		return nil, fmt.Errorf("active_version must be positive")
	}
	if strings.TrimSpace(activePepper) != activePepper || len([]byte(activePepper)) < minimumPepperLen {
		return nil, fmt.Errorf("active_pepper must be at least %d bytes and contain no surrounding whitespace", minimumPepperLen)
	}

	previousConfigured := previousVersion != 0 || previousPepper != "" || previousAcceptUntil != ""
	keyring := &Keyring{
		activeVersion: activeVersion,
		activePepper:  []byte(activePepper),
	}
	if !previousConfigured {
		return keyring, nil
	}
	if previousVersion <= 0 || previousVersion == activeVersion {
		return nil, fmt.Errorf("previous_version must be positive and differ from active_version")
	}
	if strings.TrimSpace(previousPepper) != previousPepper || len([]byte(previousPepper)) < minimumPepperLen {
		return nil, fmt.Errorf("previous_pepper must be at least %d bytes and contain no surrounding whitespace", minimumPepperLen)
	}
	if hmac.Equal([]byte(previousPepper), []byte(activePepper)) {
		return nil, fmt.Errorf("previous_pepper must differ from active_pepper")
	}
	deadline, err := time.Parse(time.RFC3339, previousAcceptUntil)
	if err != nil {
		return nil, fmt.Errorf("previous_accept_until must be RFC3339: %w", err)
	}
	keyring.previousVersion = previousVersion
	keyring.previousPepper = []byte(previousPepper)
	keyring.previousAcceptUntil = deadline
	return keyring, nil
}

func digest(pepper []byte, raw string) string {
	mac := hmac.New(sha256.New, pepper)
	_, _ = mac.Write([]byte(digestDomain))
	_, _ = mac.Write([]byte(raw))
	return hex.EncodeToString(mac.Sum(nil))
}

// Active returns the digest and version used for all new writes.
func (k *Keyring) Active(raw string) Candidate {
	return Candidate{Version: k.activeVersion, Digest: digest(k.activePepper, raw), Active: true}
}

// Candidates returns the active digest first and the previous digest only
// while its explicit acceptance window remains open.
func (k *Keyring) Candidates(raw string, now time.Time) []Candidate {
	out := []Candidate{k.Active(raw)}
	if k.PreviousAccepted(now) {
		out = append(out, Candidate{
			Version: k.previousVersion,
			Digest:  digest(k.previousPepper, raw),
		})
	}
	return out
}

func (k *Keyring) ActiveVersion() int {
	return k.activeVersion
}

func (k *Keyring) PreviousVersion() int {
	return k.previousVersion
}

func (k *Keyring) PreviousAccepted(now time.Time) bool {
	return k != nil && k.previousVersion > 0 && now.Before(k.previousAcceptUntil)
}

// DisplayMetadata returns bounded, non-secret identification material for a
// credential. It is safe for list/detail responses but not authentication.
func DisplayMetadata(raw string) (prefix, lastFour string) {
	if raw == "" {
		return "", ""
	}
	prefixLen := 8
	if len(raw) < prefixLen {
		prefixLen = len(raw)
	}
	prefix = raw[:prefixLen]
	lastStart := len(raw) - 4
	if lastStart < 0 {
		lastStart = 0
	}
	return prefix, raw[lastStart:]
}

func IsDigest(value string) bool {
	if len(value) != DigestHexLength {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(value) == value
}
