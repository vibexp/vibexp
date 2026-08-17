package oauthserver

import (
	"context"
	"crypto"
)

// VerificationPublicKeys returns the Authorization Server's current signing
// public keys — the active key plus any retired-but-not-pruned keys, so a token
// signed just before a key rotation still verifies. It lets a same-process
// resource server (the MCP endpoint) verify AS-issued tokens in-process, without
// an HTTP JWKS round-trip to the public issuer URL. See authkit.NewLocalKeySet.
func (s *Service) VerificationPublicKeys(ctx context.Context) ([]crypto.PublicKey, error) {
	set, err := s.keys.PublicJWKS(ctx)
	if err != nil {
		return nil, err
	}
	pubs := make([]crypto.PublicKey, 0, len(set.Keys))
	for i := range set.Keys {
		pubs = append(pubs, set.Keys[i].Key)
	}
	return pubs, nil
}
