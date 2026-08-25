//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Real-Postgres coverage for TeamRepository.GetNamesByIDs, the batched
// source-team name lookup behind the settings audit read path (#832).
//
// The sqlmock suite pins the SQL shape but cannot prove the one behaviour this
// endpoint's acceptance criteria turn on: that an id naming a team which no
// longer exists comes back absent rather than raising. sqlmock returns whatever
// rows the test declares, so "the row is missing" there is an assertion about
// the fixture; here it is an assertion about the database, after a real DELETE.
// team_settings_audit.source_team_id carries no foreign key precisely so that
// DELETE is possible with the audit row still standing.

func TestIntegrationTeam_GetNamesByIDs(t *testing.T) {
	resetTeamSettingsAuditTables(t)
	ctx := context.Background()
	repo := NewTeamRepository(integrationDB)

	ownerID := insertTestUser(t)
	teamA := insertTestTeam(t, ownerID)
	teamB := insertTestTeam(t, ownerID)

	t.Run("resolves every existing id", func(t *testing.T) {
		got, err := repo.GetNamesByIDs(ctx, []string{teamA, teamB})
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.NotEmpty(t, got[teamA])
		assert.NotEmpty(t, got[teamB])
	})

	t.Run("empty ids never reaches the database", func(t *testing.T) {
		got, err := repo.GetNamesByIDs(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{}, got)
	})

	t.Run("a never-existing id is omitted, not an error", func(t *testing.T) {
		absent := uuid.New().String()

		got, err := repo.GetNamesByIDs(ctx, []string{teamA, absent})
		require.NoError(t, err)
		assert.Contains(t, got, teamA)
		assert.NotContains(t, got, absent,
			"an unknown team id must be absent from the map — that absence is how the "+
				"audit read path renders source_team_name as null")
	})

	t.Run("a DELETED team is omitted, not an error", func(t *testing.T) {
		doomed := insertTestTeam(t, ownerID)

		before, err := repo.GetNamesByIDs(ctx, []string{doomed})
		require.NoError(t, err)
		require.Contains(t, before, doomed, "fixture must resolve before the delete")

		_, err = integrationDB.ExecContext(ctx, "DELETE FROM teams WHERE id = $1", doomed)
		require.NoError(t, err)

		after, err := repo.GetNamesByIDs(ctx, []string{teamA, doomed})
		require.NoError(t, err,
			"a deleted source team must not fail the lookup — the settings audit log "+
				"outlives the team it names")
		assert.Contains(t, after, teamA)
		assert.NotContains(t, after, doomed)
	})

	t.Run("duplicate ids collapse to one entry", func(t *testing.T) {
		got, err := repo.GetNamesByIDs(ctx, []string{teamA, teamA})
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})
}
