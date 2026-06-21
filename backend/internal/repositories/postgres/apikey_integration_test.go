//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Behavior-level parity suite for APIKeyRepository against real Postgres
// (#1612). These tests are the contract any reimplementation of the queries
// (e.g. the sqlc PoC, #1588) must satisfy — they assert rows in/out and error
// semantics, never SQL text.

func newIntegrationAPIKey(userID, name string, createdAt time.Time, integrations []string) *models.APIKey {
	return &models.APIKey{
		UserID:       userID,
		Name:         name,
		KeyHash:      "hash-" + uuid.New().String(),
		KeyPrefix:    "vxp_test",
		Integrations: integrations,
		CreatedAt:    createdAt,
		UpdatedAt:    createdAt,
	}
}

func TestIntegrationAPIKey_CreateAndGetByUserID(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAPIKeyRepository(integrationDB)
	userID := insertTestUser(t)
	ctx := context.Background()

	now := time.Now().UTC()
	older := newIntegrationAPIKey(userID, "older-key", now.Add(-time.Minute), []string{"ai_tools", "cli"})
	newer := newIntegrationAPIKey(userID, "newer-key", now, nil)

	require.NoError(t, repo.Create(ctx, older))
	require.NoError(t, repo.Create(ctx, newer))

	assert.NotEmpty(t, older.ID)
	assert.EqualValues(t, 1, older.Version)

	keys, err := repo.GetByUserID(ctx, userID)
	require.NoError(t, err)
	require.Len(t, keys, 2)

	assert.Equal(t, "newer-key", keys[0].Name, "keys must be ordered created_at DESC")
	assert.Equal(t, "older-key", keys[1].Name)

	got := keys[1]
	assert.Equal(t, older.ID, got.ID)
	assert.Equal(t, []string{"ai_tools", "cli"}, got.Integrations, "permissions must be hydrated in grant order")
	assert.Equal(t, "everything", got.UsageType, "column default must apply when Create omits usage_type")
	assert.False(t, got.IsLegacy)
	assert.Nil(t, got.MigrationNotes)
	assert.Nil(t, got.LastUsedAt)
	assert.Nil(t, got.ExpiresAt)

	assert.Empty(t, keys[0].Integrations, "a key created without integrations must hydrate to an empty slice")
}

func TestIntegrationAPIKey_Create_UnknownIntegrationRollsBack(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAPIKeyRepository(integrationDB)
	userID := insertTestUser(t)
	ctx := context.Background()

	key := newIntegrationAPIKey(userID, "doomed-key", time.Now().UTC(), []string{"not_in_catalog"})

	err := repo.Create(ctx, key)
	require.Error(t, err, "an integration code missing from the catalog must fail the create")

	keys, listErr := repo.GetByUserID(ctx, userID)
	require.NoError(t, listErr)
	assert.Empty(t, keys, "the api_keys insert must roll back with the failed permission insert")
}

func TestIntegrationAPIKey_GetByKeyHash(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAPIKeyRepository(integrationDB)
	userID := insertTestUser(t)
	ctx := context.Background()

	key := newIntegrationAPIKey(userID, "auth-key", time.Now().UTC(), []string{"mcp_server"})
	require.NoError(t, repo.Create(ctx, key))

	t.Run("found", func(t *testing.T) {
		got, err := repo.GetByKeyHash(ctx, key.KeyHash)
		require.NoError(t, err)
		assert.Equal(t, key.ID, got.ID)
		assert.Equal(t, userID, got.UserID)
		assert.Equal(t, []string{"mcp_server"}, got.Integrations)
	})

	t.Run("unknown hash", func(t *testing.T) {
		got, err := repo.GetByKeyHash(ctx, "hash-"+uuid.New().String())
		require.Error(t, err)
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "API key not found")
	})

	t.Run("expired key is rejected", func(t *testing.T) {
		_, err := integrationDB.ExecContext(ctx,
			"UPDATE api_keys SET expires_at = NOW() - INTERVAL '1 hour' WHERE id = $1", key.ID)
		require.NoError(t, err)

		got, lookupErr := repo.GetByKeyHash(ctx, key.KeyHash)
		require.Error(t, lookupErr)
		assert.Nil(t, got)
		assert.Contains(t, lookupErr.Error(), "API key not found")
	})

	t.Run("future expiry is still valid", func(t *testing.T) {
		_, err := integrationDB.ExecContext(ctx,
			"UPDATE api_keys SET expires_at = NOW() + INTERVAL '1 hour' WHERE id = $1", key.ID)
		require.NoError(t, err)

		got, lookupErr := repo.GetByKeyHash(ctx, key.KeyHash)
		require.NoError(t, lookupErr)
		require.NotNil(t, got.ExpiresAt)
		assert.Equal(t, key.ID, got.ID)
	})
}

func TestIntegrationAPIKey_Delete(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAPIKeyRepository(integrationDB)
	userID := insertTestUser(t)
	ctx := context.Background()

	key := newIntegrationAPIKey(userID, "revoke-me", time.Now().UTC(), []string{"cli"})
	require.NoError(t, repo.Create(ctx, key))

	t.Run("another user's delete does not revoke", func(t *testing.T) {
		otherUserID := insertTestUser(t)

		err := repo.Delete(ctx, otherUserID, key.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "API key not found")

		keys, listErr := repo.GetByUserID(ctx, userID)
		require.NoError(t, listErr)
		assert.Len(t, keys, 1, "the key must survive a delete attempt by a non-owner")
	})

	t.Run("owner delete revokes key and cascades permissions", func(t *testing.T) {
		require.NoError(t, repo.Delete(ctx, userID, key.ID))

		keys, listErr := repo.GetByUserID(ctx, userID)
		require.NoError(t, listErr)
		assert.Empty(t, keys)

		integrations, intErr := repo.GetIntegrationsByAPIKeyID(ctx, key.ID)
		require.NoError(t, intErr)
		assert.Empty(t, integrations, "permission rows must cascade-delete with the key")
	})

	t.Run("deleting an already-deleted key reports not found", func(t *testing.T) {
		err := repo.Delete(ctx, userID, key.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "API key not found")
	})
}

func TestIntegrationAPIKey_GetNamesByIDs(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAPIKeyRepository(integrationDB)
	userID := insertTestUser(t)
	otherUserID := insertTestUser(t)
	ctx := context.Background()

	now := time.Now().UTC()
	mine1 := newIntegrationAPIKey(userID, "mine-one", now, nil)
	mine2 := newIntegrationAPIKey(userID, "mine-two", now, nil)
	theirs := newIntegrationAPIKey(otherUserID, "theirs", now, nil)
	require.NoError(t, repo.Create(ctx, mine1))
	require.NoError(t, repo.Create(ctx, mine2))
	require.NoError(t, repo.Create(ctx, theirs))

	t.Run("empty input yields empty map without querying", func(t *testing.T) {
		got, err := repo.GetNamesByIDs(ctx, userID, nil)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("returns only the caller's keys, omitting unknown and foreign IDs", func(t *testing.T) {
		got, err := repo.GetNamesByIDs(ctx, userID, []string{
			mine1.ID, mine2.ID, theirs.ID, uuid.New().String(),
		})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			mine1.ID: "mine-one",
			mine2.ID: "mine-two",
		}, got)
	})
}
