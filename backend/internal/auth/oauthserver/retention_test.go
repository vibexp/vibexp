package oauthserver

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

func TestStore_DeleteExpired(t *testing.T) {
	ctx := context.Background()
	codes := newMemRequestRepo()
	access := newMemRequestRepo()
	refresh := newMemRequestRepo()
	pkce := newMemRequestRepo()
	store := NewStore(newMemClientRepo(), codes, access, refresh, pkce)

	past := time.Now().Add(-time.Minute)
	future := time.Now().Add(time.Hour)

	require.NoError(t, codes.Create(ctx, &models.OAuthRequest{Signature: "c-expired", ExpiresAt: past}))
	require.NoError(t, codes.Create(ctx, &models.OAuthRequest{Signature: "c-live", ExpiresAt: future}))
	require.NoError(t, access.Create(ctx, &models.OAuthRequest{Signature: "a-expired", ExpiresAt: past}))
	// Zero ExpiresAt (unknown) must never be swept.
	require.NoError(t, refresh.Create(ctx, &models.OAuthRequest{Signature: "r-unknown"}))

	n, err := store.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n, "only the two past-expiry rows should be removed")

	_, err = codes.Get(ctx, "c-live")
	assert.NoError(t, err, "live row must survive")
	_, err = refresh.Get(ctx, "r-unknown")
	assert.NoError(t, err, "unknown-expiry row must survive")
	_, err = codes.Get(ctx, "c-expired")
	assert.Error(t, err, "expired row must be gone")
}

func TestKeyManager_PruneRetiredRemovesOldRetiredKeys(t *testing.T) {
	km, repo := newTestKeyManager(0) // zero interval forces rotation
	ctx := context.Background()
	require.NoError(t, km.EnsureActiveKey(ctx))
	require.NoError(t, km.MaybeRotate(ctx)) // retires the first key

	keys, err := repo.ListAll(ctx)
	require.NoError(t, err)
	require.Len(t, keys, 2)

	// cutoff = now - 0 is after the retired key's rotated_at, so it is pruned;
	// the active key (no rotated_at) is kept.
	removed, err := km.PruneRetired(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), removed)

	set, err := km.PublicJWKS(ctx)
	require.NoError(t, err)
	assert.Len(t, set.Keys, 1, "only the active key should remain after pruning")
}

func TestKeyManager_PruneRetiredKeepsRecentRetiredKeys(t *testing.T) {
	km, repo := newTestKeyManager(0)
	ctx := context.Background()
	require.NoError(t, km.EnsureActiveKey(ctx))
	require.NoError(t, km.MaybeRotate(ctx))

	// A long TTL pushes the cutoff far into the past, so the just-retired key is
	// still within the window and must be retained.
	removed, err := km.PruneRetired(ctx, 720*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(0), removed)

	keys, err := repo.ListAll(ctx)
	require.NoError(t, err)
	assert.Len(t, keys, 2, "recently-retired key must be retained while tokens may still reference it")
}

func TestKeyManager_MaybeRotateSkipsWhenLockContended(t *testing.T) {
	km, repo := newTestKeyManager(0) // rotation is due
	ctx := context.Background()
	require.NoError(t, km.EnsureActiveKey(ctx))

	repo.mu.Lock()
	repo.lockContended = true
	repo.mu.Unlock()

	require.NoError(t, km.MaybeRotate(ctx), "a contended lock is not an error")

	keys, err := repo.ListAll(ctx)
	require.NoError(t, err)
	assert.Len(t, keys, 1, "no new key must be minted while a peer holds the rotation lock")
}
