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
// Two things sqlmock structurally cannot prove and this suite exists for:
//   - the `t.id::text = $2` cast. Without it Postgres has to compare the uuid
//     column against a placeholder lib/pq sends as text, and the statement fails
//     for every identifier — measured 42883 "operator does not exist: character
//     varying = uuid". Only a real driver raises that; sqlmock never type-checks
//     arguments, so its regex can pin the cast's presence but not its necessity.
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

// TestIntegrationTeam_ResolveByIdentifier_UUIDAndSlug is the 42P08 regression
// guard: a real driver executes the same statement with a UUID and with a slug
// bound to the same text placeholder. A query written `t.id = $2 OR t.slug = $2`
// fails here on the very first case.
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
// string is the one that would break a query comparing against a uuid column
// without the cast.
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
