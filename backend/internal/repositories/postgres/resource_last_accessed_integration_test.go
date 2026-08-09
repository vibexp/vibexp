//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/repositories"
)

// Behavior-level suite for the last-accessed denormalization (#730) against
// real Postgres. The two properties that only a database can prove are
// monotonicity (GREATEST, including over NULL) and that the write leaves
// `updated_at` alone — the latter depending on migration 014 for `memories`,
// whose BEFORE UPDATE trigger would otherwise make every read look like an edit.

// seedLastAccessedResources creates one row in each of the four resource
// tables and returns resourceType -> id.
func seedLastAccessedResources(t *testing.T) map[string]string {
	t.Helper()
	ctx := context.Background()

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	projectID := insertTestProject(t, userID, teamID)

	ids := map[string]string{
		"prompt": insertTestPrompt(t, userID, teamID, projectID, "Prompt", "body", "published"),
	}

	artifactID := uuid.New().String()
	_, err := integrationDB.ExecContext(ctx,
		"INSERT INTO artifacts (id, user_id, team_id, project_id, slug, title, content) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7)",
		artifactID, userID, teamID, projectID, "artifact-"+artifactID[:8], "Artifact", "content")
	require.NoError(t, err)
	ids["artifact"] = artifactID

	blueprintID := uuid.New().String()
	_, err = integrationDB.ExecContext(ctx,
		"INSERT INTO blueprints (id, user_id, team_id, project_id, slug, title, content, path) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
		blueprintID, userID, teamID, projectID, "blueprint-"+blueprintID[:8], "Blueprint", "content",
		".claude/blueprint-"+blueprintID[:8]+".md")
	require.NoError(t, err)
	ids["blueprint"] = blueprintID

	memoryID := uuid.New().String()
	_, err = integrationDB.ExecContext(ctx,
		"INSERT INTO memories (id, user_id, team_id, project_id, text) VALUES ($1, $2, $3, $4, $5)",
		memoryID, userID, teamID, projectID, "text")
	require.NoError(t, err)
	ids["memory"] = memoryID

	return ids
}

// lastAccessedValue reads one per-medium column.
func lastAccessedValue(t *testing.T, table, column, id string) *time.Time {
	t.Helper()
	var got *time.Time
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT "+column+" FROM "+table+" WHERE id = $1", id).Scan(&got))
	return got
}

func resourceUpdatedAt(t *testing.T, table, id string) time.Time {
	t.Helper()
	var got time.Time
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT updated_at FROM "+table+" WHERE id = $1", id).Scan(&got))
	return got
}

func TestIntegrationLastAccessed_WritesEveryTypeAndMedium(t *testing.T) {
	resetFreshnessTables(t)
	ids := seedLastAccessedResources(t)
	repo := NewResourceLastAccessedRepository(integrationDB)
	tables, columns := LastAccessedTargets()
	ctx := context.Background()

	at := time.Now().UTC().Truncate(time.Microsecond)

	for resourceType, table := range tables {
		for source, column := range columns {
			t.Run(resourceType+"/"+source, func(t *testing.T) {
				require.NoError(t, repo.UpdateLastAccessed(ctx, resourceType, ids[resourceType], source, at))

				got := lastAccessedValue(t, table, column, ids[resourceType])
				require.NotNil(t, got, "%s.%s must be set", table, column)
				assert.True(t, got.UTC().Equal(at), "got %v, want %v", got.UTC(), at)
			})
		}
	}
}

// A late-arriving event must never make a resource look less recently accessed
// than it is. The access path submits events to a worker pool, so two reads can
// be persisted out of order.
func TestIntegrationLastAccessed_IsMonotonic(t *testing.T) {
	resetFreshnessTables(t)
	ids := seedLastAccessedResources(t)
	repo := NewResourceLastAccessedRepository(integrationDB)
	ctx := context.Background()

	newer := time.Now().UTC().Truncate(time.Microsecond)
	older := newer.Add(-time.Hour)

	require.NoError(t, repo.UpdateLastAccessed(ctx, "prompt", ids["prompt"], "web", newer))
	require.NoError(t, repo.UpdateLastAccessed(ctx, "prompt", ids["prompt"], "web", older))

	got := lastAccessedValue(t, "prompts", "last_accessed_web_at", ids["prompt"])
	require.NotNil(t, got)
	assert.True(t, got.UTC().Equal(newer),
		"an out-of-order older event must not move the value backwards: got %v, want %v", got.UTC(), newer)
}

// GREATEST ignores NULL, so the very first write on a never-accessed resource
// must store the timestamp rather than yielding NULL.
func TestIntegrationLastAccessed_FirstWriteOverNull(t *testing.T) {
	resetFreshnessTables(t)
	ids := seedLastAccessedResources(t)
	repo := NewResourceLastAccessedRepository(integrationDB)
	ctx := context.Background()

	require.Nil(t, lastAccessedValue(t, "artifacts", "last_accessed_cli_at", ids["artifact"]),
		"precondition: columns start NULL")

	at := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, repo.UpdateLastAccessed(ctx, "artifact", ids["artifact"], "cli", at))

	got := lastAccessedValue(t, "artifacts", "last_accessed_cli_at", ids["artifact"])
	require.NotNil(t, got)
	assert.True(t, got.UTC().Equal(at))
}

// The load-bearing guarantee of this issue: a read is not an edit. `memories`
// is the case that actually bites — it carries the only BEFORE UPDATE trigger
// among the four tables, narrowed by migration 014 so a last-accessed-only
// write does not fire it. Without 014 this test fails on memories alone.
func TestIntegrationLastAccessed_DoesNotTouchUpdatedAt(t *testing.T) {
	resetFreshnessTables(t)
	ids := seedLastAccessedResources(t)
	repo := NewResourceLastAccessedRepository(integrationDB)
	tables, _ := LastAccessedTargets()
	ctx := context.Background()

	before := make(map[string]time.Time, len(tables))
	for resourceType, table := range tables {
		before[resourceType] = resourceUpdatedAt(t, table, ids[resourceType])
	}

	// A separate transaction with a later clock, so an accidental bump would be
	// a visibly different value rather than a same-timestamp coincidence.
	at := time.Now().UTC().Add(time.Minute)
	for resourceType := range tables {
		require.NoError(t, repo.UpdateLastAccessed(ctx, resourceType, ids[resourceType], "mcp", at))
	}

	for resourceType, table := range tables {
		t.Run(resourceType, func(t *testing.T) {
			after := resourceUpdatedAt(t, table, ids[resourceType])
			assert.True(t, after.Equal(before[resourceType]),
				"%s.updated_at moved (%v -> %v): a read must not look like an edit",
				table, before[resourceType], after)
		})
	}
}

// Migration 014 narrowed the memories trigger; a REAL edit must still bump
// updated_at, or the fix would have broken the signal it was protecting.
func TestIntegrationLastAccessed_MemoriesTriggerStillFiresOnRealEdit(t *testing.T) {
	resetFreshnessTables(t)
	ids := seedLastAccessedResources(t)
	ctx := context.Background()

	before := resourceUpdatedAt(t, "memories", ids["memory"])

	_, err := integrationDB.ExecContext(ctx,
		"UPDATE memories SET text = $2 WHERE id = $1", ids["memory"], "edited text")
	require.NoError(t, err)

	after := resourceUpdatedAt(t, "memories", ids["memory"])
	assert.True(t, after.After(before),
		"a content edit must still bump memories.updated_at (%v -> %v)", before, after)
}

// "Any medium" is a GREATEST over the four columns — the shape rule evaluation
// (#732) will use. Writing different instants per medium must yield the newest.
func TestIntegrationLastAccessed_AnyMediumIsTheMax(t *testing.T) {
	resetFreshnessTables(t)
	ids := seedLastAccessedResources(t)
	repo := NewResourceLastAccessedRepository(integrationDB)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Microsecond).Add(-10 * time.Hour)
	newest := base.Add(3 * time.Hour)
	require.NoError(t, repo.UpdateLastAccessed(ctx, "blueprint", ids["blueprint"], "web", base))
	require.NoError(t, repo.UpdateLastAccessed(ctx, "blueprint", ids["blueprint"], "cli", base.Add(time.Hour)))
	require.NoError(t, repo.UpdateLastAccessed(ctx, "blueprint", ids["blueprint"], "mcp", newest))
	// api deliberately left NULL — GREATEST must ignore it.

	var anyMedium time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT GREATEST(last_accessed_web_at, last_accessed_cli_at, last_accessed_mcp_at, "+
			"last_accessed_api_at) FROM blueprints WHERE id = $1", ids["blueprint"]).Scan(&anyMedium))

	assert.True(t, anyMedium.UTC().Equal(newest),
		"any-medium max should be %v, got %v", newest, anyMedium.UTC())
}

// The resource may be deleted between serving the read and running this
// asynchronous write; that must be a silent no-op, not an error the caller logs.
func TestIntegrationLastAccessed_MissingResourceIsNoOp(t *testing.T) {
	resetFreshnessTables(t)
	repo := NewResourceLastAccessedRepository(integrationDB)

	err := repo.UpdateLastAccessed(
		context.Background(), "prompt", uuid.New().String(), "web", time.Now().UTC())

	assert.NoError(t, err)
}

func TestIntegrationLastAccessed_UnsupportedTypeNeverTouchesDatabase(t *testing.T) {
	resetFreshnessTables(t)
	repo := NewResourceLastAccessedRepository(integrationDB)

	err := repo.UpdateLastAccessed(
		context.Background(), "project", uuid.New().String(), "web", time.Now().UTC())

	assert.ErrorIs(t, err, repositories.ErrUnsupportedLastAccessedResource)
}
