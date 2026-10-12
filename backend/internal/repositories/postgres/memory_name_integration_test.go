//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// The rule is SQL, so only Postgres can say what it evaluates to: the title
// when non-blank, otherwise a body excerpt without a leading heading marker.
func TestIntegrationMemoryDisplayNameExpr(t *testing.T) {
	long := strings.Repeat("word ", 60)

	cases := []struct {
		name  string
		title sql.NullString
		text  string
		want  string
	}{
		{"title wins over the body", sql.NullString{String: "Deploy checklist", Valid: true}, "# Other\n\nBody", "Deploy checklist"},
		{"title is trimmed", sql.NullString{String: "  Deploy checklist \n", Valid: true}, "Body", "Deploy checklist"},
		{"NULL title falls back to the body", sql.NullString{}, "Drain the node first.", "Drain the node first."},
		{"empty title falls back to the body", sql.NullString{String: "", Valid: true}, "Drain the node first.", "Drain the node first."},
		{"blank title falls back to the body", sql.NullString{String: " \t\n", Valid: true}, "Drain the node first.", "Drain the node first."},
		{"leading heading marker is dropped", sql.NullString{}, "## Runbook for deploys", "Runbook for deploys"},
		{"heading marker after leading whitespace is dropped", sql.NullString{}, "\n  # Runbook", "Runbook"},
		{"a hash that is not a heading is kept", sql.NullString{}, "#hashtag first", "#hashtag first"},
		{"whitespace and newlines collapse to one space", sql.NullString{}, "# Runbook\n\nDrain  the\tnode.", "Runbook Drain the node."},
		{"a long body is cut at the excerpt length", sql.NullString{}, long, strings.TrimSpace(long[:120])},
		{"a blank memory is an empty name, not NULL", sql.NullString{}, "  \n ", ""},
	}

	query := "SELECT " + memoryDisplayNameExpr("m") +
		" FROM (SELECT 1 AS id, $1::text AS title, $2::text AS text) m"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got sql.NullString
			err := integrationDB.QueryRowContext(context.Background(), query, tc.title, tc.text).Scan(&got)
			require.NoError(t, err)
			require.True(t, got.Valid, "a present memory never yields a NULL name")
			assert.Equal(t, tc.want, got.String)
		})
	}

	t.Run("a join miss stays NULL so the per-type COALESCE moves on", func(t *testing.T) {
		var got sql.NullString
		missQuery := "SELECT " + memoryDisplayNameExpr("m") +
			" FROM (SELECT NULL::int AS id, 'T'::text AS title, 'body'::text AS text) m"
		require.NoError(t, integrationDB.QueryRowContext(context.Background(), missQuery).Scan(&got))
		assert.False(t, got.Valid)
	})
}

// End to end through one of the labelling queries: a titled memory is named by
// its title, an untitled one by its body.
func TestIntegrationComment_ListRecentByTeam_NamesMemoryByTitle(t *testing.T) {
	resetIntegrationTables(t)
	at := seedAccessTeam(t)
	ctx := context.Background()
	titledID := insertCommentMemory(t, at, at.ownerID, "chi routes on `r.URL.RawPath` whenever the path is encoded")
	_, err := integrationDB.ExecContext(ctx,
		"UPDATE memories SET title = $1 WHERE id = $2", "chi returns percent-encoded path params", titledID)
	require.NoError(t, err)
	untitledID := insertCommentMemory(t, at, at.ownerID, "# Dependabot runs\n\nget no SONAR_TOKEN")

	repo := NewCommentRepository(integrationDB)
	require.NoError(t, repo.Create(ctx, newComment(at.teamID, models.CommentResourceTypeMemory, titledID, at.ownerID, "one")))
	require.NoError(t, repo.Create(ctx, newComment(at.teamID, models.CommentResourceTypeMemory, untitledID, at.ownerID, "two")))

	recent, err := repo.ListRecentByTeam(ctx, at.teamID, 20)
	require.NoError(t, err)
	require.Len(t, recent, 2)

	titles := map[string]string{}
	for _, activity := range recent {
		titles[activity.ResourceID] = activity.ResourceTitle
	}
	assert.Equal(t, "chi returns percent-encoded path params", titles[titledID])
	assert.Equal(t, "Dependabot runs get no SONAR_TOKEN", titles[untitledID])
}
