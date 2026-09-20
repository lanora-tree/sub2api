package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apikeyhmac"
)

func (s *APIKeyService) requireKeyring() (*apikeyhmac.Keyring, error) {
	if s == nil || s.keyring == nil || s.keyringErr != nil {
		if s != nil && s.keyringErr != nil {
			return nil, ErrAPIKeyHMACUnavailable.WithCause(s.keyringErr)
		}
		return nil, ErrAPIKeyHMACUnavailable
	}
	return s.keyring, nil
}

func (s *APIKeyService) activeKeyCredential(raw string) (apikeyhmac.Candidate, string, string, error) {
	keyring, err := s.requireKeyring()
	if err != nil {
		return apikeyhmac.Candidate{}, "", "", err
	}
	prefix, lastFour := apikeyhmac.DisplayMetadata(raw)
	return keyring.Active(raw), prefix, lastFour, nil
}

func (s *APIKeyService) apiKeyExists(ctx context.Context, raw string) (bool, error) {
	keyring, err := s.requireKeyring()
	if err != nil {
		return false, err
	}
	for _, candidate := range keyring.Candidates(raw, time.Now()) {
		exists, lookupErr := s.apiKeyRepo.ExistsByKey(ctx, candidate.Digest)
		if lookupErr != nil {
			return false, lookupErr
		}
		if exists {
			return true, nil
		}
	}
	return false, nil
}

func (s *APIKeyService) lookupAPIKeyByRawCredential(ctx context.Context, raw string) (*APIKey, error) {
	keyring, err := s.requireKeyring()
	if err != nil {
		return nil, err
	}
	candidates := keyring.Candidates(raw, time.Now())
	active := candidates[0]
	for _, candidate := range candidates {
		apiKey, lookupErr := s.lookupAPIKeyHashForAuth(ctx, candidate.Digest)
		if lookupErr != nil {
			if errors.Is(lookupErr, ErrAPIKeyNotFound) {
				continue
			}
			return nil, lookupErr
		}
		if candidate.Active {
			return apiKey, nil
		}

		rotator, ok := s.apiKeyRepo.(apiKeyHashRotator)
		if !ok {
			return nil, ErrAPIKeyHMACUnavailable.WithCause(fmt.Errorf("api key repository does not support digest rotation"))
		}
		rotated, rotateErr := rotator.RotateKeyHash(ctx, apiKey.ID, candidate.Digest, active.Digest, active.Version)
		if rotateErr != nil {
			return nil, fmt.Errorf("rotate api key HMAC digest: %w", rotateErr)
		}
		if !rotated {
			apiKey, lookupErr = s.lookupAPIKeyHashForAuth(ctx, active.Digest)
			if lookupErr != nil {
				return nil, lookupErr
			}
		}
		apiKey.KeyHash = active.Digest
		apiKey.KeyVersion = active.Version
		return apiKey, nil
	}
	return nil, ErrAPIKeyNotFound
}
