//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Behavior-level tests for TeamRepository.ResolveByIdentifier against real
// Postgres (#812).
//
// Three things sqlmock structurally cannot prove and this suite exists for:
//   - that the identifier's two bindings are type-correct. Comparing the uuid
//     column against the text placeholder (`t.id = $2`) fails for every
//     identifier — measured 42883 "operator does not exist: character varying =
//     uuid" — because lib/pq sends the argument as text. Only a real driver
//     raises that; sqlmock never type-checks arguments.
//   - that the query is index-SERVED and not merely correct. sqlmock plans
//     nothing, so the shape that seq-scans every team passes its regex happily.
//   - that the owner-OR-member predicate really scopes the query, so resolution
//     doubles as the membership check every MCP tool relies on.

// seedResolveTeam creates a team owned by a fresh user plus a second user who is
// a member, and a third who is neither. Returns (teamID, slug, ownerID, memberID,
// outsiderID).
func seedResolveTeam(t *testing.T) (string, string, string, string, string) {
	t.Helper()
	ctx := context.Background()

	ownerID := insertTestUser(t)
	memberID := insertTestUser(t)
	outsiderID := insertTestUser(t)
	teamID := uuid.New().String()
	slug := "resolve-team-" + teamID[:8]

	_, err := integrationDB.ExecContext(ctx,
		"INSERT INTO teams (id, owner_id, name, slug) VALUES ($1, $2, $3, $4)",
		teamID, ownerID, "Resolve Team", slug)
	require.NoError(t, err)

	_, err = integrationDB.ExecContext(ctx,
		"INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)",
		teamID, memberID, models.TeamMemberRoleMember)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM teams WHERE id = $1", teamID)
	})

	return teamID, slug, ownerID, memberID, outsiderID
}

// TestIntegrationTeam_ResolveByIdentifier_UUIDAndSlug is the type-correctness
// guard: a real driver executes the same statement for a UUID identifier (the
// uuid parameter binds) and a slug one (it binds NULL). A query comparing the
// uuid column against the text placeholder fails here on the very first case,
// 42883 "operator does not exist: character varying = uuid".
func TestIntegrationTeam_ResolveByIdentifier_UUIDAndSlug(t *testing.T) {
	resetIntegrationTables(t)
	teamID, slug, ownerID, memberID, _ := seedResolveTeam(t)
	repo := NewTeamRepository(integrationDB)
	ctx := context.Background()

	for _, tc := range []struct {
		name       string
		userID     string
		identifier string
	}{
		{"owner by uuid", ownerID, teamID},
		{"owner by slug", ownerID, slug},
		{"member by uuid", memberID, teamID},
		{"member by slug", memberID, slug},
	} {
		t.Run(tc.name, func(t *testing.T) {
			team, err := repo.ResolveByIdentifier(ctx, tc.userID, tc.identifier)
			require.NoError(t, err)
			require.NotNil(t, team)
			assert.Equal(t, teamID, team.ID, "every identifier form resolves to the canonical UUID")
			assert.Equal(t, slug, team.Slug)
			assert.Equal(t, ownerID, team.OwnerID)
		})
	}
}

// TestIntegrationTeam_ResolveByIdentifier_NonMemberRejected proves the predicate
// is enforced in SQL: a real, existing team is invisible to a user with no
// membership row, by either identifier form. This is what lets resolveTeam skip a
// separate membership check.
func TestIntegrationTeam_ResolveByIdentifier_NonMemberRejected(t *testing.T) {
	resetIntegrationTables(t)
	teamID, slug, _, _, outsiderID := seedResolveTeam(t)
	repo := NewTeamRepository(integrationDB)
	ctx := context.Background()

	for _, identifier := range []string{teamID, slug} {
		team, err := repo.ResolveByIdentifier(ctx, outsiderID, identifier)
		require.ErrorIs(t, err, repositories.ErrTeamNotFound)
		assert.Nil(t, team)
	}
}

// TestIntegrationTeam_ResolveByIdentifier_UnknownIdentifier covers the arm that
// must be indistinguishable from the non-member one: a slug nobody owns, and a
// well-formed UUID no team carries, both return ErrTeamNotFound. The non-UUID
// string also covers the branch where the uuid parameter binds NULL, so only the
// slug arm can match.
func TestIntegrationTeam_ResolveByIdentifier_UnknownIdentifier(t *testing.T) {
	resetIntegrationTables(t)
	_, _, ownerID, _, _ := seedResolveTeam(t)
	repo := NewTeamRepository(integrationDB)
	ctx := context.Background()

	for _, identifier := range []string{"no-such-team-slug", uuid.New().String()} {
		team, err := repo.ResolveByIdentifier(ctx, ownerID, identifier)
		require.ErrorIs(t, err, repositories.ErrTeamNotFound)
		assert.Nil(t, team)
	}
}

// TestIntegrationTeam_ResolveByIdentifier_UsesAnIndex is the guard for the whole
// point of #812: the lookup must be index-served, not a table scan. It is a
// separate concern from correctness — an unindexable predicate returns exactly
// the right rows, which is why every other test here passes with it.
//
// Test tables are tiny, so the planner would seq-scan any shape on cost alone;
// `enable_seqscan = off` removes that noise and leaves only the question of
// whether an index CAN serve the predicate. It discriminates: casting the column
// (`t.id::text = $2`) still plans "Seq Scan on teams" even with seq scans
// disabled, because no index matches the expression.
//
// The whole plan is inspected, not just the top line — a Bitmap Heap Scan names
// its indexes only in the child Bitmap Index Scan nodes.
func TestIntegrationTeam_ResolveByIdentifier_UsesAnIndex(t *testing.T) {
	resetIntegrationTables(t)
	teamID, slug, ownerID, _, _ := seedResolveTeam(t)
	ctx := context.Background()

	// EXPLAIN the production statement itself, so the assertion cannot drift from
	// what ResolveByIdentifier actually executes.
	explainQuery := "EXPLAIN " + resolveTeamByIdentifierQuery

	for _, tc := range []struct {
		name       string
		identifier string
	}{
		{"uuid identifier", teamID},
		{"slug identifier", slug},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A transaction with SET LOCAL, not a pooled SET: the setting must
			// reach the same session as the EXPLAIN (against the pool it can land
			// on a different connection and silently measure a session where seq
			// scans are still enabled), and SET LOCAL reverts on rollback rather
			// than riding a returned connection into someone else's test.
			tx, err := integrationDB.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()

			_, err = tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off")
			require.NoError(t, err)

			rows, err := tx.QueryContext(ctx, explainQuery, ownerID, tc.identifier, uuidOrNil(tc.identifier))
			require.NoError(t, err)
			defer func() { _ = rows.Close() }()

			var plan string
			for rows.Next() {
				var line string
				require.NoError(t, rows.Scan(&line))
				plan += line + "\n"
			}
			require.NoError(t, rows.Err())

			assert.NotContains(t, plan, "Seq Scan on teams",
				"team resolution must be index-served; plan was:\n%s", plan)
			assert.Contains(t, plan, "idx_teams_slug",
				"the slug arm must reach its index; plan was:\n%s", plan)
		})
	}
}
