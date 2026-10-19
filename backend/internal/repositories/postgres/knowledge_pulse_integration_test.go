//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// One pass over every Knowledge Pulse query against the real schema: titles
// and slugs resolve, rows come back oldest first after the cursor, and another
// team's activity never leaks in.
func TestIntegrationKnowledgePulse_QueriesResolveAndStayInTeam(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	at := seedAccessTeam(t)
	other := seedAccessTeam(t)
	artifactID := insertAccessArtifact(t, at, at.ownerID, "pulse-runbook")
	blueprintID := insertAccessBlueprint(t, at, at.ownerID, "pulse-rules")
	otherArtifact := insertAccessArtifact(t, other, other.ownerID, "other-artifact")

	keyID := uuid.New().String()
	_, err := integrationDB.ExecContext(ctx,
		`INSERT INTO api_keys (id, user_id, name, key_hash, key_prefix) VALUES ($1, $2, 'Laptop key', $3, 'vx_')`,
		keyID, at.ownerID, "hash-"+keyID)
	require.NoError(t, err)

	since := time.Now().Add(-time.Hour)
	exec := func(q string, args ...any) {
		t.Helper()
		_, err := integrationDB.ExecContext(ctx, q, args...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO resource_access_events (team_id, user_id, resource_type, resource_id, source, api_key_id, user_agent, created_at)
	      VALUES ($1, $2, 'artifact', $3, 'mcp', $4, 'claude-code/2.0', now() - interval '2 minutes'),
	             ($1, $2, 'blueprint', $5, 'web', NULL, NULL, now() - interval '1 minute'),
	             ($1, $2, 'project', $6, 'web', NULL, NULL, now()),
	             ($7, $8, 'artifact', $9, 'mcp', NULL, NULL, now())`,
		at.teamID, at.ownerID, artifactID, keyID, blueprintID, at.projectID, other.teamID, other.ownerID, otherArtifact)
	exec(`INSERT INTO resource_write_events (team_id, project_id, user_id, resource_type, resource_id, action, source, assistant_name)
	      VALUES ($1, $2, $3, 'artifact', $4, 'update', 'mcp', 'claude-code')`,
		at.teamID, at.projectID, at.ownerID, artifactID)
	exec(`INSERT INTO resource_relations (team_id, project_id, from_type, from_id, to_type, to_id, relation_type, origin, status, created_by)
	      VALUES ($1, $2, 'artifact', $3, 'blueprint', $4, 'supersedes', 'ai', 'suggested', $5)`,
		at.teamID, at.projectID, artifactID, blueprintID, at.ownerID)
	feedID := uuid.New().String()
	exec(`INSERT INTO feeds (id, team_id, name, created_by_user_id) VALUES ($1, $2, 'General', $3)`,
		feedID, at.teamID, at.ownerID)
	exec(`INSERT INTO feed_items (team_id, feed_id, title, content, excerpt, ai_assistant_name, posted_by_user_id)
	      VALUES ($1, $2, 'Shipped pulse', 'body', 'body', 'claude-code', $3)`,
		at.teamID, feedID, at.ownerID)

	repo := NewKnowledgePulseRepository(integrationDB)

	reads, err := repo.ListReadsSince(ctx, at.teamID, since, 10)
	require.NoError(t, err)
	require.Len(t, reads, 2, "project reads and the other team's reads are excluded")
	assert.Equal(t, artifactID, reads[0].Resource.ID, "oldest first")
	assert.Equal(t, "Artifact Title", reads[0].Resource.Title)
	assert.Equal(t, "pulse-runbook", reads[0].Resource.Slug)
	assert.Equal(t, at.projectID, reads[0].Resource.ProjectID)
	assert.Equal(t, "Laptop key", reads[0].Actor.APIKeyName)
	assert.Equal(t, "claude-code/2.0", reads[0].Actor.UserAgent)
	assert.Equal(t, "Integration Test User", reads[1].Actor.UserName)

	after, err := repo.ListReadsSince(ctx, at.teamID, reads[0].At, 10)
	require.NoError(t, err)
	require.Len(t, after, 1, "since is exclusive")

	writes, err := repo.ListWritesSince(ctx, at.teamID, since, 10)
	require.NoError(t, err)
	require.Len(t, writes, 1)
	assert.Equal(t, "claude-code", writes[0].Actor.AssistantName)
	assert.Equal(t, "update", writes[0].Action)

	links, err := repo.ListLinksSince(ctx, at.teamID, since, 10)
	require.NoError(t, err)
	require.Len(t, links, 1)
	assert.Equal(t, "Blueprint Title", links[0].To.Title)
	assert.Equal(t, "pulse-rules", links[0].To.Slug)

	posts, err := repo.ListFeedPostsSince(ctx, at.teamID, since, 10)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	assert.Equal(t, "Shipped pulse", posts[0].Title)

	snap, err := repo.GetSnapshot(ctx, at.teamID, since, 150)
	require.NoError(t, err)
	require.Len(t, snap.Nodes, 2)
	assert.Equal(t, 1, snap.Nodes[0].Degree)
	require.Len(t, snap.Edges, 1)
	assert.Equal(t, artifactID, snap.Edges[0].FromID)
	assert.NotEmpty(t, snap.Actors)

	stats, err := repo.GetDayStats(ctx, at.teamID, since)
	require.NoError(t, err)
	assert.Equal(t, 2, stats.Reads)
	assert.Equal(t, 1, stats.Writes)
	assert.Equal(t, 1, stats.Links)
	assert.Equal(t, 1, stats.Supersedes)
	assert.Equal(t, 1, stats.LinkedUpdates)

	otherReads, err := repo.ListReadsSince(ctx, other.teamID, since, 10)
	require.NoError(t, err)
	assert.Len(t, otherReads, 1)
}
