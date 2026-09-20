package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apikeyhmac"
	"github.com/stretchr/testify/require"
)

type progressiveHMACRepo struct {
	APIKeyRepository
	byDigest map[string]*APIKey
	lookups  []string
	rotated  bool
}

func (r *progressiveHMACRepo) GetByKeyForAuth(_ context.Context, digest string) (*APIKey, error) {
	r.lookups = append(r.lookups, digest)
	key := r.byDigest[digest]
	if key == nil {
		return nil, ErrAPIKeyNotFound
	}
	clone := *key
	return &clone, nil
}

func (r *progressiveHMACRepo) RotateKeyHash(_ context.Context, id int64, expectedHash, newHash string, newVersion int) (bool, error) {
	key := r.byDigest[expectedHash]
	if key == nil || key.ID != id {
		return false, nil
	}
	delete(r.byDigest, expectedHash)
	clone := *key
	clone.KeyHash = newHash
	clone.KeyVersion = newVersion
	r.byDigest[newHash] = &clone
	r.rotated = true
	return true, nil
}

func TestAPIKeyServicePreviousDigestProgressivelyRehashes(t *testing.T) {
	raw := "sk-progressive-rehash-123456"
	activePepper := "active-pepper-0123456789abcdef-0001"
	previousPepper := "previous-pepper-0123456789abcd-0001"
	deadline := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	previousRing, err := apikeyhmac.NewKeyring(1, previousPepper, 0, "", "")
	require.NoError(t, err)
	previous := previousRing.Active(raw)

	repo := &progressiveHMACRepo{byDigest: map[string]*APIKey{
		previous.Digest: {
			ID: 11, UserID: 22, KeyHash: previous.Digest, KeyVersion: 1,
			Status: StatusActive,
			User:   &User{ID: 22, Status: StatusActive},
		},
	}}
	cfg := &config.Config{APIKeyHMAC: config.APIKeyHMACConfig{
		ActiveVersion:       2,
		ActivePepper:        activePepper,
		PreviousVersion:     1,
		PreviousPepper:      previousPepper,
		PreviousAcceptUntil: deadline,
	}}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)

	got, err := svc.GetByKey(context.Background(), raw)
	require.NoError(t, err)
	require.Equal(t, raw, got.Key)
	require.Equal(t, 2, got.KeyVersion)
	require.True(t, repo.rotated)
	require.Len(t, repo.lookups, 2)
	require.NotEqual(t, raw, repo.lookups[0])
	require.NotEqual(t, raw, repo.lookups[1])
	require.True(t, apikeyhmac.IsDigest(repo.lookups[0]))
	require.True(t, apikeyhmac.IsDigest(repo.lookups[1]))
}

func TestAPIKeyServiceExpiredPreviousPepperIsNotQueried(t *testing.T) {
	repo := &progressiveHMACRepo{byDigest: map[string]*APIKey{}}
	cfg := &config.Config{APIKeyHMAC: config.APIKeyHMACConfig{
		ActiveVersion:       2,
		ActivePepper:        "active-pepper-0123456789abcdef-0001",
		PreviousVersion:     1,
		PreviousPepper:      "previous-pepper-0123456789abcd-0001",
		PreviousAcceptUntil: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
	}}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)

	_, err := svc.GetByKey(context.Background(), "sk-expired-previous-123456")
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
	require.Len(t, repo.lookups, 1)
}

func TestAPIKeyServiceFailsClosedWithoutPepper(t *testing.T) {
	svc := NewAPIKeyService(&progressiveHMACRepo{}, nil, nil, nil, nil, nil, &config.Config{})
	_, err := svc.GetByKey(context.Background(), "sk-no-pepper-123456789")
	require.ErrorIs(t, err, ErrAPIKeyHMACUnavailable)
}
