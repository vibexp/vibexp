//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/repositories"
)

// Regression suite for #517: SearchByMetadata scoped on user_id alone, with no
// team_id predicate and no read-access check, so it ignored the {team_id} path
// param and returned the caller's memories from EVERY team they belong to.
//
// The sqlmock test asserts the generated SQL; this asserts what Postgres
// actually returns, which is the part that matters for a tenancy bug.

func insertMemoryWithMetadata(t *testing.T, userID, teamID, projectID, metadataJSON string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		`INSERT INTO memories (id, user_id, team_id, project_id, text, status, metadata)
		 VALUES ($1, $2, $3, $4, 'remembered', 'active', $5::jsonb)`,
		id, userID, teamID, projectID, metadataJSON)
	require.NoError(t, err)
	return id
}

// TestIntegrationMemorySearch_DoesNotSpanTeams is the #517 regression: one user
// who belongs to two teams, with a matching memory in each. A search scoped to
// team A must return only team A's.
func TestIntegrationMemorySearch_DoesNotSpanTeams(t *testing.T) {
	resetMetadataCatalogTables(t)

	userID := insertTestUser(t)
	teamA := insertTestTeam(t, userID)
	projectA := insertTestProject(t, userID, teamA)
	teamB := insertTestTeam(t, userID)
	projectB := insertTestProject(t, userID, teamB)

	inA := insertMemoryWithMetadata(t, userID, teamA, projectA, `{"env":"prod"}`)
	insertMemoryWithMetadata(t, userID, teamB, projectB, `{"env":"prod"}`)

	repo := NewMemoryRepository(integrationDB).(*MemoryRepository)

	memories, total, err := repo.SearchByMetadata(
		context.Background(), userID, "env", "prod",
		repositories.MemoryFilters{TeamID: teamA, Page: 1, Limit: 50},
	)

	require.NoError(t, err)
	// Before the fix this returned 2 — the caller's memory from team B leaked
	// into a request scoped to team A.
	assert.Equal(t, 1, total)
	require.Len(t, memories, 1)
	assert.Equal(t, inA, memories[0].ID)
}

// TestIntegrationMemorySearch_NonMemberGetsNothing covers the read-access half
// of the predicate: naming a team you do not belong to returns nothing, rather
// than falling back to "your memories, anywhere".
func TestIntegrationMemorySearch_NonMemberGetsNothing(t *testing.T) {
	resetMetadataCatalogTables(t)

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	projectID := insertTestProject(t, userID, teamID)
	insertMemoryWithMetadata(t, userID, teamID, projectID, `{"env":"prod"}`)

	strangerTeam := insertTestTeam(t, insertTestUser(t))

	repo := NewMemoryRepository(integrationDB).(*MemoryRepository)

	memories, total, err := repo.SearchByMetadata(
		context.Background(), userID, "env", "prod",
		repositories.MemoryFilters{TeamID: strangerTeam, Page: 1, Limit: 50},
	)

	require.NoError(t, err)
	assert.Zero(t, total)
	assert.Empty(t, memories)
}
