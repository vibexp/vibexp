//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Behavior-level suite for team_settings_audit (#828) against real Postgres. It
// asserts what only a real database can prove: the ON DELETE CASCADE on the
// destination team, the deliberate ABSENCE of one on the source team, ON DELETE
// SET NULL on the actor, jsonb round-tripping, and that the ordering plus its id
// tiebreaker page a set of identically-stamped rows without repeating or
// skipping one. Query text is never asserted here — that is the sqlmock suite's
// job.

func resetTeamSettingsAuditTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, teams, team_settings_audit CASCADE")
	require.NoError(t, err)
}

func newTeamSettingsAuditRepo() repositories.TeamSettingsAuditRepository {
	return NewTeamSettingsAuditRepository(integrationDB)
}

// seedTeamSettingsAuditScope creates the actor plus a destination and a source
// team — a copy is by definition a two-team operation.
func seedTeamSettingsAuditScope(t *testing.T) (actorID, destTeamID, sourceTeamID string) {
	t.Helper()
	actorID = insertTestUser(t)
	return actorID, insertTestTeam(t, actorID), insertTestTeam(t, actorID)
}

func TestIntegrationTeamSettingsAudit_AppendRoundTrip(t *testing.T) {
	resetTeamSettingsAuditTables(t)
	ctx := context.Background()
	repo := newTeamSettingsAuditRepo()
	actorID, destTeamID, sourceTeamID := seedTeamSettingsAuditScope(t)

	sourceResourceID := uuid.New().String()
	createdResourceID := uuid.New().String()
	entry := &models.TeamSettingsAudit{
		TeamID:            destTeamID,
		ActorUserID:       &actorID,
		Surface:           models.SettingsAuditSurfaceModelProvider,
		SourceTeamID:      &sourceTeamID,
		SourceResourceID:  &sourceResourceID,
		CreatedResourceID: &createdResourceID,
		Detail:            json.RawMessage(`{"name":"OpenAI (copy)","provider_type":"openai"}`),
	}

	require.NoError(t, repo.Append(ctx, entry))
	assert.NotEmpty(t, entry.ID, "the database assigns the id")
	assert.False(t, entry.CreatedAt.IsZero(), "the database assigns created_at")

	entries, total, err := repo.ListByTeam(ctx, destTeamID, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, entry.ID, entries[0].ID)
	assert.Equal(t, models.SettingsAuditSurfaceModelProvider, entries[0].Surface)
	require.NotNil(t, entries[0].SourceTeamID)
	assert.Equal(t, sourceTeamID, *entries[0].SourceTeamID)
	assert.JSONEq(t,
		`{"name":"OpenAI (copy)","provider_type":"openai"}`, string(entries[0].Detail),
		"jsonb must round-trip through lib/pq as an object, not as bytea")
}

// An entry written with no detail must land as an empty OBJECT. The column is
// NOT NULL and a nil json.RawMessage binds as SQL NULL rather than falling back
// to the column default, so this is the assertion that proves the repository's
// substitution is load-bearing: remove it and the INSERT fails outright.
func TestIntegrationTeamSettingsAudit_AbsentDetailStoresEmptyObject(t *testing.T) {
	resetTeamSettingsAuditTables(t)
	ctx := context.Background()
	repo := newTeamSettingsAuditRepo()
	actorID, destTeamID, sourceTeamID := seedTeamSettingsAuditScope(t)

	entry := &models.TeamSettingsAudit{
		TeamID:       destTeamID,
		ActorUserID:  &actorID,
		Surface:      models.SettingsAuditSurfaceCustomTypes,
		SourceTeamID: &sourceTeamID,
	}

	require.NoError(t, repo.Append(ctx, entry))
	assert.JSONEq(t, `{}`, string(entry.Detail))
	assert.Nil(t, entry.SourceResourceID, "a whole-set copy names no single source row")
	assert.Nil(t, entry.CreatedResourceID)
}

// Deleting the DESTINATION team removes its entries: an entry about a team that
// no longer exists is unreadable, and team_id is the only column with a cascade.
func TestIntegrationTeamSettingsAudit_CascadesOnDestinationTeamDelete(t *testing.T) {
	resetTeamSettingsAuditTables(t)
	ctx := context.Background()
	repo := newTeamSettingsAuditRepo()
	actorID, destTeamID, sourceTeamID := seedTeamSettingsAuditScope(t)

	require.NoError(t, repo.Append(ctx, &models.TeamSettingsAudit{
		TeamID:       destTeamID,
		ActorUserID:  &actorID,
		Surface:      models.SettingsAuditSurfaceEmbeddingProvider,
		SourceTeamID: &sourceTeamID,
	}))
	require.Equal(t, 1, countRows(t, integrationDB.DB,
		"SELECT COUNT(*) FROM team_settings_audit WHERE team_id = $1", destTeamID))

	_, err := integrationDB.ExecContext(ctx, "DELETE FROM teams WHERE id = $1", destTeamID)
	require.NoError(t, err)

	assert.Equal(t, 0, countRows(t, integrationDB.DB,
		"SELECT COUNT(*) FROM team_settings_audit WHERE team_id = $1", destTeamID),
		"the entry must go with its destination team")
}

// Deleting the SOURCE team must leave the entry intact with source_team_id
// still populated. This is the one that a well-meaning foreign key would break:
// a CASCADE would erase the record of the copy, and a SET NULL would blank the
// only field that says where the credential came from — which is the question
// the log exists to answer.
func TestIntegrationTeamSettingsAudit_SurvivesSourceTeamDelete(t *testing.T) {
	resetTeamSettingsAuditTables(t)
	ctx := context.Background()
	repo := newTeamSettingsAuditRepo()
	actorID, destTeamID, sourceTeamID := seedTeamSettingsAuditScope(t)

	require.NoError(t, repo.Append(ctx, &models.TeamSettingsAudit{
		TeamID:       destTeamID,
		ActorUserID:  &actorID,
		Surface:      models.SettingsAuditSurfaceEmbeddingProvider,
		SourceTeamID: &sourceTeamID,
	}))

	_, err := integrationDB.ExecContext(ctx, "DELETE FROM teams WHERE id = $1", sourceTeamID)
	require.NoError(t, err)

	entries, total, err := repo.ListByTeam(ctx, destTeamID, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	require.NotNil(t, entries[0].SourceTeamID)
	assert.Equal(t, sourceTeamID, *entries[0].SourceTeamID,
		"the source team id must outlive the source team")
}

// Deleting the ACTOR blanks actor_user_id and keeps the entry: deleting an
// account must not erase the record of what that account did.
func TestIntegrationTeamSettingsAudit_ActorDeleteSetsNull(t *testing.T) {
	resetTeamSettingsAuditTables(t)
	ctx := context.Background()
	repo := newTeamSettingsAuditRepo()
	actorID, destTeamID, sourceTeamID := seedTeamSettingsAuditScope(t)

	// A second user owns the teams, so removing the actor does not cascade the
	// teams (and therefore the entries) away underneath the assertion.
	ownerID := insertTestUser(t)
	_, err := integrationDB.ExecContext(ctx,
		"UPDATE teams SET owner_id = $1 WHERE id IN ($2, $3)", ownerID, destTeamID, sourceTeamID)
	require.NoError(t, err)

	require.NoError(t, repo.Append(ctx, &models.TeamSettingsAudit{
		TeamID:       destTeamID,
		ActorUserID:  &actorID,
		Surface:      models.SettingsAuditSurfaceModelProvider,
		SourceTeamID: &sourceTeamID,
	}))

	_, err = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", actorID)
	require.NoError(t, err)

	entries, total, err := repo.ListByTeam(ctx, destTeamID, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	assert.Nil(t, entries[0].ActorUserID, "the entry outlives its actor")
}

// Paging over entries written in ONE transaction is the case the id tiebreaker
// exists for: `now()` is transaction-start time, so all five rows carry an
// identical created_at and `ORDER BY created_at DESC` alone leaves their order
// UNDEFINED. Walking the whole set one page at a time must visit each entry
// exactly once — a property sqlmock cannot observe, since it returns whatever
// rows the test declares.
//
// Honest about its strength: dropping `id DESC` does NOT make this fail today,
// because Postgres happens to return five rows of one small table in a stable
// order anyway. Undefined is not the same as unstable, and no fixture size makes
// the planner reliably reorder on demand — so this pins the observable property
// (a full walk yields each entry once) rather than pretending to force the
// tiebreaker. The tiebreaker is in the SQL and in the index for the case that
// does reorder: a bigger table, a different plan, a later Postgres.
func TestIntegrationTeamSettingsAudit_PagesIdenticallyStampedEntries(t *testing.T) {
	resetTeamSettingsAuditTables(t)
	ctx := context.Background()
	repo := newTeamSettingsAuditRepo()
	actorID, destTeamID, sourceTeamID := seedTeamSettingsAuditScope(t)

	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	for i := 0; i < 5; i++ {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO team_settings_audit
			     (team_id, actor_user_id, surface, source_team_id)
			 VALUES ($1, $2, $3, $4)`,
			destTeamID, actorID, models.SettingsAuditSurfaceCustomTypes, sourceTeamID)
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())

	var stamps int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(DISTINCT created_at) FROM team_settings_audit WHERE team_id = $1",
		destTeamID).Scan(&stamps))
	require.Equal(t, 1, stamps, "one transaction must stamp every row identically")

	seen := map[string]int{}
	for offset := 0; offset < 5; offset += 2 {
		page, total, listErr := repo.ListByTeam(ctx, destTeamID, 2, offset)
		require.NoError(t, listErr)
		assert.Equal(t, 5, total)
		for _, entry := range page {
			seen[entry.ID]++
		}
	}

	assert.Len(t, seen, 5, "every entry is visited exactly once across the pages")
	for id, times := range seen {
		assert.Equal(t, 1, times, "entry %s appeared on more than one page", id)
	}
}

// Entries are tenancy-scoped: another team's log is never visible, and the
// total count describes the same set as the page.
func TestIntegrationTeamSettingsAudit_ListByTeamIsTenancyScoped(t *testing.T) {
	resetTeamSettingsAuditTables(t)
	ctx := context.Background()
	repo := newTeamSettingsAuditRepo()
	actorID, destTeamID, sourceTeamID := seedTeamSettingsAuditScope(t)
	otherTeamID := insertTestTeam(t, actorID)

	require.NoError(t, repo.Append(ctx, &models.TeamSettingsAudit{
		TeamID: destTeamID, ActorUserID: &actorID,
		Surface: models.SettingsAuditSurfaceModelProvider, SourceTeamID: &sourceTeamID,
	}))
	require.NoError(t, repo.Append(ctx, &models.TeamSettingsAudit{
		TeamID: otherTeamID, ActorUserID: &actorID,
		Surface: models.SettingsAuditSurfaceModelProvider, SourceTeamID: &sourceTeamID,
	}))

	entries, total, err := repo.ListByTeam(ctx, destTeamID, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, destTeamID, entries[0].TeamID)

	empty, emptyTotal, err := repo.ListByTeam(ctx, uuid.New().String(), 10, 0)
	require.NoError(t, err)
	assert.Zero(t, emptyTotal)
	assert.Empty(t, empty)
}
