package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apikeyhmac"
)

// InvalidateAuthCacheByKey 清除指定 API Key 的认证缓存
func (s *APIKeyService) InvalidateAuthCacheByKey(ctx context.Context, key string) {
	if key == "" {
		return
	}
	keyring, err := s.requireKeyring()
	if err != nil {
		return
	}
	for _, candidate := range keyring.Candidates(key, time.Now()) {
		s.deleteAuthCache(ctx, candidate.Digest)
	}
}

// InvalidateAuthCacheByDigest clears an internal HMAC-addressed cache entry.
// It intentionally rejects non-digests so public raw credentials cannot be
// confused with internal cache references.
func (s *APIKeyService) InvalidateAuthCacheByDigest(ctx context.Context, keyHash string) {
	if !apikeyhmac.IsDigest(keyHash) {
		return
	}
	s.deleteAuthCache(ctx, keyHash)
}

// InvalidateAuthCacheByUserID 清除用户相关的 API Key 认证缓存
func (s *APIKeyService) InvalidateAuthCacheByUserID(ctx context.Context, userID int64) {
	if userID <= 0 {
		return
	}
	keys, err := s.apiKeyRepo.ListKeysByUserID(ctx, userID)
	if err != nil {
		return
	}
	s.deleteAuthCacheByDigests(ctx, keys)
}

// InvalidateAuthCacheByGroupID 清除分组相关的 API Key 认证缓存
func (s *APIKeyService) InvalidateAuthCacheByGroupID(ctx context.Context, groupID int64) {
	if groupID <= 0 {
		return
	}
	keys, err := s.apiKeyRepo.ListKeysByGroupID(ctx, groupID)
	if err != nil {
		return
	}
	s.deleteAuthCacheByDigests(ctx, keys)
}

func (s *APIKeyService) deleteAuthCacheByDigests(ctx context.Context, keyHashes []string) {
	if len(keyHashes) == 0 {
		return
	}
	for _, keyHash := range keyHashes {
		s.InvalidateAuthCacheByDigest(ctx, keyHash)
	}
}
