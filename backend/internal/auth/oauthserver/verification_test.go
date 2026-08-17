package oauthserver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerificationPublicKeys_IncludesRotatedKey pins the doc guarantee that
// VerificationPublicKeys returns the active key plus any retired-but-not-pruned
// key — the property that keeps a token signed just before a rotation verifiable
// by the same-process MCP resource server (see authkit.NewLocalKeySet). It
// mirrors TestKeyManager_RotationRetainsOldKeyInJWKS.
func TestVerificationPublicKeys_IncludesRotatedKey(t *testing.T) {
	// Zero interval forces rotation on every MaybeRotate call.
	km, _ := newTestKeyManager(0)
	ctx := context.Background()
	require.NoError(t, km.EnsureActiveKey(ctx))
	svc := &Service{keys: km}

	pubs, err := svc.VerificationPublicKeys(ctx)
	require.NoError(t, err)
	require.Len(t, pubs, 1, "one active key before rotation")
	require.NotNil(t, pubs[0])

	require.NoError(t, km.MaybeRotate(ctx))

	pubs, err = svc.VerificationPublicKeys(ctx)
	require.NoError(t, err)
	assert.Len(t, pubs, 2,
		"rotated-out key must remain so a token signed just before rotation still verifies")
	for i, k := range pubs {
		assert.NotNil(t, k, "verification key %d must be a non-nil public key", i)
	}
}
