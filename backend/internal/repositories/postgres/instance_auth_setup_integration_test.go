//go:build integration

package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Behavior-level suite for the auth setup repository (#1236) against real
// Postgres: the singleton, mint-unless-live and its two-replica race, forced
// re-arm, consumption, generation bumps, and the audit entry each write
// appends. The table is global to the shared test database, so no test runs in
// parallel.

func resetInstanceAuthSetup(t *testing.T) {
	t.Helper()
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, "instance_auth_setup")
}

func setupTokenHash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func TestIntegrationInstanceAuthSetup_Get_NoRow(t *testing.T) {
	resetInstanceAuthSetup(t)
	_, err := NewInstanceAuthSetupRepository(integrationDB).Get(context.Background())
	require.ErrorIs(t, err, repositories.ErrInstanceAuthSetupNotFound)
}

func TestIntegrationInstanceAuthSetup_MintIfAbsentOrExpired(t *testing.T) {
	resetInstanceAuthSetup(t)
	repo := NewInstanceAuthSetupRepository(integrationDB)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	first, second := setupTokenHash("first"), setupTokenHash("second")

	row, minted, err := repo.MintIfAbsentOrExpired(ctx, first, now.Add(24*time.Hour), now)
	require.NoError(t, err)
	require.True(t, minted, "nothing stored: mint")
	assert.Equal(t, first, row.TokenHash)
	assert.Equal(t, int64(1), row.Generation)
	assert.False(t, row.Rearmed)
	assert.True(t, row.ExpiresAt.Equal(now.Add(24*time.Hour)))

	t.Run("only the hash is stored", func(t *testing.T) {
		var stored []byte
		require.NoError(t, integrationDB.QueryRowContext(ctx,
			"SELECT token_hash FROM instance_auth_setup").Scan(&stored))
		assert.Equal(t, first, stored)
		assert.Len(t, stored, sha256.Size)
		assert.False(t, bytes.Contains(stored, []byte("first")))
	})

	t.Run("a live token is kept: a second boot mints nothing", func(t *testing.T) {
		row, minted, err := repo.MintIfAbsentOrExpired(ctx, second, now.Add(48*time.Hour), now.Add(time.Hour))
		require.NoError(t, err)
		assert.False(t, minted)
		assert.Equal(t, first, row.TokenHash, "the first replica's token is still the stored one")
		assert.Equal(t, int64(1), row.Generation)
		assert.Len(t, authAuditEntries(t, models.InstanceSettingAuthSetup), 1, "a no-op mint writes no audit entry")
	})

	t.Run("an expired token is replaced and the generation bumped", func(t *testing.T) {
		later := now.Add(24*time.Hour + time.Second)
		row, minted, err := repo.MintIfAbsentOrExpired(ctx, second, later.Add(24*time.Hour), later)
		require.NoError(t, err)
		require.True(t, minted)
		assert.Equal(t, second, row.TokenHash)
		assert.Equal(t, int64(2), row.Generation)
	})

	t.Run("a consumed token is replaced, and the consumption cleared", func(t *testing.T) {
		userID := insertTestUser(t)
		consumed, err := repo.Consume(ctx, userID)
		require.NoError(t, err)
		require.True(t, consumed)

		row, minted, err := repo.MintIfAbsentOrExpired(ctx, first, now.Add(72*time.Hour), now)
		require.NoError(t, err)
		require.True(t, minted)
		assert.Nil(t, row.ConsumedAt)
		assert.Nil(t, row.ConsumedBy)
		assert.Equal(t, int64(4), row.Generation, "consume and mint each bump it")
	})

	assert.Equal(t, 1, countRows(t, integrationDB.DB, "SELECT count(*) FROM instance_auth_setup"), "still one row")
}

// Two replicas booting together: the table lock makes the database the only
// arbiter, so exactly one mints and every replica ends up agreeing on its
// token. The row is empty at the start, where a row lock would lock nothing.
func TestIntegrationInstanceAuthSetup_ConcurrentMintHasOneWinner(t *testing.T) {
	resetInstanceAuthSetup(t)
	repo := NewInstanceAuthSetupRepository(integrationDB)
	ctx := context.Background()
	now := time.Now().UTC()

	const replicas = 6
	type result struct {
		hash   []byte
		stored []byte
		minted bool
		err    error
	}
	results := make([]result, replicas)
	var wg sync.WaitGroup
	begin := make(chan struct{})
	for i := range replicas {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hash := setupTokenHash(uuid.NewString())
			<-begin
			row, minted, err := repo.MintIfAbsentOrExpired(ctx, hash, now.Add(24*time.Hour), now)
			results[i] = result{hash: hash, minted: minted, err: err}
			if row != nil {
				results[i].stored = row.TokenHash
			}
		}()
	}
	close(begin)
	wg.Wait()

	final, err := repo.Get(ctx)
	require.NoError(t, err)
	var winners int
	for _, r := range results {
		require.NoError(t, r.err)
		assert.Equal(t, final.TokenHash, r.stored, "every replica is handed the one stored token")
		if r.minted {
			winners++
			assert.Equal(t, r.hash, final.TokenHash, "the winner's token is the stored one")
		}
	}
	assert.Equal(t, 1, winners, "exactly one replica mints")
	assert.Equal(t, int64(1), final.Generation)
	assert.Len(t, authAuditEntries(t, models.InstanceSettingAuthSetup), 1)
}

func TestIntegrationInstanceAuthSetup_ForceMint(t *testing.T) {
	resetInstanceAuthSetup(t)
	repo := NewInstanceAuthSetupRepository(integrationDB)
	ctx := context.Background()
	now := time.Now().UTC()

	row, err := repo.ForceMint(ctx, setupTokenHash("a"), now.Add(24*time.Hour))
	require.NoError(t, err)
	assert.True(t, row.Rearmed)
	assert.Equal(t, int64(1), row.Generation)

	// A live token does not stop a forced mint.
	row, err = repo.ForceMint(ctx, setupTokenHash("b"), now.Add(24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, setupTokenHash("b"), row.TokenHash)
	assert.Equal(t, int64(2), row.Generation)

	t.Run("a boot-time mint after expiry keeps the re-arm", func(t *testing.T) {
		later := now.Add(25 * time.Hour)
		row, minted, err := repo.MintIfAbsentOrExpired(ctx, setupTokenHash("c"), later.Add(24*time.Hour), later)
		require.NoError(t, err)
		require.True(t, minted)
		assert.True(t, row.Rearmed, "re-armed until consumed, not until the token expires")
	})

	t.Run("consumption clears it", func(t *testing.T) {
		consumed, err := repo.Consume(ctx, insertTestUser(t))
		require.NoError(t, err)
		require.True(t, consumed)
		row, err := repo.Get(ctx)
		require.NoError(t, err)
		assert.False(t, row.Rearmed)
	})
}

func TestIntegrationInstanceAuthSetup_Consume(t *testing.T) {
	resetInstanceAuthSetup(t)
	repo := NewInstanceAuthSetupRepository(integrationDB)
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("nothing to consume", func(t *testing.T) {
		consumed, err := repo.Consume(ctx, uuid.NewString())
		require.NoError(t, err, "no row: no write, so not even the user is checked")
		assert.False(t, consumed)
		assert.Empty(t, authAuditEntries(t, models.InstanceSettingAuthSetup))
	})

	_, _, err := repo.MintIfAbsentOrExpired(ctx, setupTokenHash("t"), now.Add(24*time.Hour), now)
	require.NoError(t, err)

	t.Run("an unknown user is ErrUserNotFound and consumes nothing", func(t *testing.T) {
		consumed, err := repo.Consume(ctx, uuid.NewString())
		require.ErrorIs(t, err, repositories.ErrUserNotFound)
		assert.False(t, consumed)
		row, err := repo.Get(ctx)
		require.NoError(t, err)
		assert.False(t, row.IsConsumed(), "the rejected consume rolled back")
		assert.NotNil(t, row.TokenHash)
	})

	userID := insertTestUser(t)
	consumed, err := repo.Consume(ctx, userID)
	require.NoError(t, err)
	require.True(t, consumed)

	row, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Nil(t, row.TokenHash, "the hash is dropped")
	require.NotNil(t, row.ConsumedAt)
	require.NotNil(t, row.ConsumedBy)
	assert.Equal(t, userID, *row.ConsumedBy)
	assert.Equal(t, int64(2), row.Generation, "outstanding setup sessions are invalidated")
	assert.False(t, row.HasLiveToken(now))

	t.Run("consuming twice is a no-op", func(t *testing.T) {
		consumed, err := repo.Consume(ctx, userID)
		require.NoError(t, err)
		assert.False(t, consumed)
		again, err := repo.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(2), again.Generation)
	})

	t.Run("deleting the consumer keeps the row", func(t *testing.T) {
		_, err := integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
		require.NoError(t, err)
		row, err := repo.Get(ctx)
		require.NoError(t, err)
		assert.Nil(t, row.ConsumedBy)
		assert.NotNil(t, row.ConsumedAt)
	})
}

// Every change is audited under auth_setup, and no snapshot ever carries the
// token hash.
func TestIntegrationInstanceAuthSetup_AuditIsRedacted(t *testing.T) {
	resetInstanceAuthSetup(t)
	repo := NewInstanceAuthSetupRepository(integrationDB)
	ctx := context.Background()
	now := time.Now().UTC()
	userID := insertTestUser(t)

	_, _, err := repo.MintIfAbsentOrExpired(ctx, setupTokenHash("a"), now.Add(24*time.Hour), now)
	require.NoError(t, err)
	_, err = repo.ForceMint(ctx, setupTokenHash("b"), now.Add(24*time.Hour))
	require.NoError(t, err)
	_, err = repo.Consume(ctx, userID)
	require.NoError(t, err)

	entries := authAuditEntries(t, models.InstanceSettingAuthSetup) // newest first
	require.Len(t, entries, 3)

	wantEvents := []string{"consumed", "rearmed", "token_minted"}
	for i, entry := range entries {
		assert.Equal(t, models.InstanceSettingsAuditActionUpsert, entry.Action)
		after := decodeAuditDoc(t, entry.After)
		assert.Equal(t, wantEvents[i], after["event"])
		for _, doc := range []map[string]any{decodeAuditDoc(t, entry.Before), after} {
			assert.NotContains(t, doc, "token_hash")
			assert.NotContains(t, doc, "TokenHash")
		}
		assert.NotContains(t, string(entry.After), "token_hash")
	}

	require.NotNil(t, entries[0].ActorUserID, "consumption names the root admin")
	assert.Equal(t, userID, *entries[0].ActorUserID)
	assert.Nil(t, entries[1].ActorUserID, "a re-arm has no acting user")
	assert.Nil(t, entries[2].ActorUserID, "a boot-time mint has no acting user")
	assert.Nil(t, entries[2].Before, "the first mint created the row")
	assert.Equal(t, true, decodeAuditDoc(t, entries[1].After)["rearmed"])
	assert.NotNil(t, decodeAuditDoc(t, entries[0].After)["consumed_at"])
}

// Setup state is not part of the provider or allowlist caches: no setup write
// bumps the shared auth settings version.
func TestIntegrationInstanceAuthSetup_DoesNotBumpTheSettingsVersion(t *testing.T) {
	resetInstanceAuthSetup(t)
	repo := NewInstanceAuthSetupRepository(integrationDB)
	ctx := context.Background()
	now := time.Now().UTC()
	start := authSettingsVersion(t)

	_, _, err := repo.MintIfAbsentOrExpired(ctx, setupTokenHash("a"), now.Add(24*time.Hour), now)
	require.NoError(t, err)
	_, err = repo.ForceMint(ctx, setupTokenHash("b"), now.Add(24*time.Hour))
	require.NoError(t, err)
	_, err = repo.Consume(ctx, insertTestUser(t))
	require.NoError(t, err)

	assert.Equal(t, start, authSettingsVersion(t))
}
