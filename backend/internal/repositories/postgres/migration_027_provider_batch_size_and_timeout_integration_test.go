//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Migration 027 adds embedding_providers.batch_size and request_timeout_seconds
// (#1665). It runs on its OWN scratch database because it migrates DOWN, which
// would corrupt the shared integrationDB.

// migration027Previous is the version immediately before 027.
const migration027Previous = 26

// insertMigrationEmbeddingProvider inserts a provider with only the columns that
// exist before 027, so the new ones take their defaults.
func insertMigrationEmbeddingProvider(t *testing.T, db *sql.DB, teamID, name string) string {
	t.Helper()
	var id string
	require.NoError(t, db.QueryRow(
		`INSERT INTO embedding_providers (user_id, team_id, name, provider_type, model)
		 SELECT owner_id, id, $2, 'openai_compatible', 'm' FROM teams WHERE id = $1
		 RETURNING id`, teamID, name).Scan(&id))
	return id
}

func TestMigration027_ProviderBatchSizeAndTimeout(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()
	m := newMigrator(t, db)

	require.NoError(t, m.Migrate(migration027Previous), "migrate to the previous version")
	require.False(t, columnExists(t, db, "embedding_providers", "batch_size"))
	teamID := seedMigrationTeam(t, db)
	existing := insertMigrationEmbeddingProvider(t, db, teamID, "existing")

	require.NoError(t, m.Migrate(27), "migrate to 027")

	t.Run("an existing provider gets today's hardcoded values", func(t *testing.T) {
		var batch, timeout int
		require.NoError(t, db.QueryRow(
			"SELECT batch_size, request_timeout_seconds FROM embedding_providers WHERE id = $1", existing,
		).Scan(&batch, &timeout))
		assert.Equal(t, 32, batch)
		assert.Equal(t, 30, timeout)
	})

	t.Run("the CHECKs match the Go bounds", func(t *testing.T) {
		cases := []struct {
			column string
			value  int
			ok     bool
		}{
			{"batch_size", models.EmbeddingBatchSizeMin, true},
			{"batch_size", models.EmbeddingBatchSizeMax, true},
			{"batch_size", models.EmbeddingBatchSizeMin - 1, false},
			{"batch_size", models.EmbeddingBatchSizeMax + 1, false},
			{"request_timeout_seconds", models.EmbeddingRequestTimeoutSecondsMin, true},
			{"request_timeout_seconds", models.EmbeddingRequestTimeoutSecondsMax, true},
			{"request_timeout_seconds", models.EmbeddingRequestTimeoutSecondsMin - 1, false},
			{"request_timeout_seconds", models.EmbeddingRequestTimeoutSecondsMax + 1, false},
		}
		for _, tc := range cases {
			// #nosec G202 -- the column name comes from the fixed table above.
			_, err := db.Exec("UPDATE embedding_providers SET "+tc.column+" = $1 WHERE id = $2", tc.value, existing)
			if tc.ok {
				assert.NoError(t, err, "%s = %d must be accepted", tc.column, tc.value)
			} else {
				requirePQCode(t, err, pqCheckViolation)
			}
		}
	})

	require.NoError(t, m.Migrate(migration027Previous), "migrate back down")
	assert.False(t, columnExists(t, db, "embedding_providers", "batch_size"))
	assert.False(t, columnExists(t, db, "embedding_providers", "request_timeout_seconds"))
	assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM embedding_providers WHERE id = $1", existing),
		"the down migration keeps the provider rows")
}

// Both settings round-trip through every repository read and write.
func TestEmbeddingProviderRepository_BatchSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	teamID := seedMigrationTeam(t, integrationDB.DB)
	var ownerID string
	require.NoError(t, integrationDB.QueryRow("SELECT owner_id FROM teams WHERE id = $1", teamID).Scan(&ownerID))
	repo := NewEmbeddingProviderRepository(integrationDB)

	provider := &models.EmbeddingProvider{
		UserID: ownerID, TeamID: &teamID, Name: "slow tei", ProviderType: "openai_compatible", Model: "m",
		ChunkSize: 1000, ChunkOverlap: 200, Concurrency: 1, BatchSize: 8, RequestTimeoutSeconds: 120,
		IsDefault: true, Configuration: "{}",
	}
	require.NoError(t, repo.Create(ctx, provider))

	assertSettings := func(p *models.EmbeddingProvider, batch, timeout int, read string) {
		t.Helper()
		assert.Equal(t, batch, p.BatchSize, read)
		assert.Equal(t, timeout, p.RequestTimeoutSeconds, read)
	}

	got, err := repo.GetByID(ctx, teamID, provider.ID)
	require.NoError(t, err)
	assertSettings(got, 8, 120, "GetByID")

	got.BatchSize, got.RequestTimeoutSeconds = 4, 300
	require.NoError(t, repo.Update(ctx, got))

	active, err := repo.GetActiveProvider(ctx, teamID)
	require.NoError(t, err)
	assertSettings(active, 4, 300, "GetActiveProvider")

	def, err := repo.GetDefault(ctx, teamID)
	require.NoError(t, err)
	assertSettings(def, 4, 300, "GetDefault")

	list, _, err := repo.List(ctx, teamID, repositories.EmbeddingProviderFilters{Page: 1, Limit: 10})
	require.NoError(t, err)
	require.Len(t, list, 1)
	assertSettings(&list[0], 4, 300, "List")

	got.BatchSize = models.EmbeddingBatchSizeMax + 1
	require.Error(t, repo.Update(ctx, got), "the CHECK rejects an out-of-range write")
}
