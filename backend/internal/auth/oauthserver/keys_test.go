package oauthserver

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestKeyManager(rotation time.Duration) (*KeyManager, *memSigningKeyRepo) {
	repo := newMemSigningKeyRepo()
	encKey := []byte("0123456789abcdef0123456789abcdef")
	return NewKeyManager(repo, encKey, rotation), repo
}

func TestKeyManager_EnsureActiveKeyIsIdempotent(t *testing.T) {
	km, repo := newTestKeyManager(time.Hour)
	ctx := context.Background()

	require.NoError(t, km.EnsureActiveKey(ctx))
	require.NoError(t, km.EnsureActiveKey(ctx))

	keys, err := repo.ListAll(ctx)
	require.NoError(t, err)
	assert.Len(t, keys, 1, "EnsureActiveKey must not create duplicate keys")
}

func TestKeyManager_PublicJWKSExposesActiveKey(t *testing.T) {
	km, _ := newTestKeyManager(time.Hour)
	ctx := context.Background()
	require.NoError(t, km.EnsureActiveKey(ctx))

	set, err := km.PublicJWKS(ctx)
	require.NoError(t, err)
	require.Len(t, set.Keys, 1)
	assert.NotEmpty(t, set.Keys[0].KeyID)
	assert.True(t, set.Keys[0].IsPublic(), "JWKS must publish public keys only")
	assert.True(t, set.Keys[0].Valid())
}

func TestKeyManager_RotationRetainsOldKeyInJWKS(t *testing.T) {
	// Zero interval forces rotation on every MaybeRotate call.
	km, repo := newTestKeyManager(0)
	ctx := context.Background()
	require.NoError(t, km.EnsureActiveKey(ctx))

	firstActive, err := repo.GetActive(ctx)
	require.NoError(t, err)

	require.NoError(t, km.MaybeRotate(ctx))

	secondActive, err := repo.GetActive(ctx)
	require.NoError(t, err)
	assert.NotEqual(t, firstActive.KID, secondActive.KID, "rotation must create a new active key")

	set, err := km.PublicJWKS(ctx)
	require.NoError(t, err)
	assert.Len(t, set.Keys, 2, "rotated-out key must remain in JWKS so old tokens still validate")
}
