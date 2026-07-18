//go:build integration

package postgres

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// resetGitHubInstallationTables clears github_installations plus the parent
// rows this suite seeds (teams hang off users; github_installations hangs off
// teams with ON DELETE CASCADE). resetIntegrationTables only covers
// users/api_keys/user_preferences, so the table under test is listed
// explicitly.
func resetGitHubInstallationTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, teams, github_installations CASCADE")
	require.NoError(t, err)
}

func newIntegrationGitHubInstallationRepo() repositories.GitHubInstallationRepository {
	return NewGitHubInstallationRepository(integrationDB.DB, slog.New(slog.DiscardHandler))
}

// buildGitHubInstallation returns an installation row for teamID carrying a
// distinctive permissions/events payload so round-trip fidelity is observable.
func buildGitHubInstallation(teamID string, installationID int64) *models.GitHubInstallation {
	return &models.GitHubInstallation{
		ID:                   uuid.New().String(),
		TeamID:               teamID,
		InstallationID:       installationID,
		AccountLogin:         "octo-org",
		AccountType:          "Organization",
		TargetType:           "organization",
		EncryptedAccessToken: "encrypted-token-" + uuid.New().String(),
		TokenExpiresAt:       time.Now().UTC().Add(time.Hour),
		Permissions:          map[string]interface{}{"contents": "read", "metadata": "read"},
		Events:               []string{"push", "pull_request"},
	}
}

func TestGitHubInstallationRepositoryIntegration_CreateAndGetRoundTrip(t *testing.T) {
	resetGitHubInstallationTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubInstallationRepo()

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)

	inst := buildGitHubInstallation(teamID, 1001)
	require.NoError(t, repo.Create(ctx, inst))
	assert.False(t, inst.CreatedAt.IsZero(), "Create must write back the DB-assigned created_at")
	assert.False(t, inst.UpdatedAt.IsZero(), "Create must write back the DB-assigned updated_at")

	byTeam, err := repo.GetByTeamID(ctx, teamID)
	require.NoError(t, err)
	assert.Equal(t, inst.ID, byTeam.ID)
	assert.Equal(t, teamID, byTeam.TeamID)
	assert.Equal(t, int64(1001), byTeam.InstallationID)
	assert.Equal(t, inst.AccountLogin, byTeam.AccountLogin)
	assert.Equal(t, inst.AccountType, byTeam.AccountType)
	assert.Equal(t, inst.TargetType, byTeam.TargetType)
	assert.Equal(t, inst.EncryptedAccessToken, byTeam.EncryptedAccessToken)
	assert.WithinDuration(t, inst.TokenExpiresAt, byTeam.TokenExpiresAt, time.Second)
	assert.Equal(t, map[string]interface{}{"contents": "read", "metadata": "read"}, byTeam.Permissions,
		"permissions must survive the JSONB round-trip")
	assert.Equal(t, []string{"push", "pull_request"}, byTeam.Events,
		"events must survive the text[] round-trip")
	assert.Nil(t, byTeam.SuspendedAt)

	byInstallation, err := repo.GetByInstallationID(ctx, 1001)
	require.NoError(t, err)
	assert.Equal(t, byTeam, byInstallation, "both lookups must return the same row")
}

func TestGitHubInstallationRepositoryIntegration_UniqueInstallationID(t *testing.T) {
	resetGitHubInstallationTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubInstallationRepo()

	userID := insertTestUser(t)
	teamA := insertTestTeam(t, userID)
	teamB := insertTestTeam(t, userID)

	require.NoError(t, repo.Create(ctx, buildGitHubInstallation(teamA, 2001)))

	err := repo.Create(ctx, buildGitHubInstallation(teamB, 2001))
	require.Error(t, err, "the same installation_id must not attach to a second team")
	pqErr := uniqueViolation(err)
	require.NotNil(t, pqErr, "the failure must be the real UNIQUE(installation_id) constraint, got: %v", err)
	assert.Equal(t, "github_installations_installation_id_key", pqErr.Constraint)

	// The winner is untouched.
	got, err := repo.GetByInstallationID(ctx, 2001)
	require.NoError(t, err)
	assert.Equal(t, teamA, got.TeamID)
}

// TestGitHubInstallationRepositoryIntegration_ReinstallFlow exercises the
// DB-level reinstall sequence: after deleting a team's installation, the same
// GitHub installation_id can be created again without tripping
// UNIQUE(installation_id) or unique_team_installation.
func TestGitHubInstallationRepositoryIntegration_ReinstallFlow(t *testing.T) {
	resetGitHubInstallationTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubInstallationRepo()

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)

	first := buildGitHubInstallation(teamID, 3001)
	require.NoError(t, repo.Create(ctx, first))
	require.NoError(t, repo.Delete(ctx, teamID))

	second := buildGitHubInstallation(teamID, 3001)
	second.AccountLogin = "octo-org-reinstalled"
	require.NoError(t, repo.Create(ctx, second), "reinstall with the same installation_id must succeed after delete")

	got, err := repo.GetByInstallationID(ctx, 3001)
	require.NoError(t, err)
	assert.Equal(t, second.ID, got.ID, "the lookup must resolve to the new row, not the deleted one")
	assert.Equal(t, "octo-org-reinstalled", got.AccountLogin)
}

func TestGitHubInstallationRepositoryIntegration_UpdateRoundTrip(t *testing.T) {
	resetGitHubInstallationTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubInstallationRepo()

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)

	inst := buildGitHubInstallation(teamID, 4001)
	require.NoError(t, repo.Create(ctx, inst))
	createdUpdatedAt := inst.UpdatedAt

	suspendedAt := time.Now().UTC().Truncate(time.Second)
	inst.InstallationID = 4002
	inst.AccountLogin = "renamed-user"
	inst.AccountType = "User"
	inst.TargetType = "user"
	inst.EncryptedAccessToken = "rotated-token"
	inst.TokenExpiresAt = inst.TokenExpiresAt.Add(2 * time.Hour)
	inst.Permissions = map[string]interface{}{"contents": "write", "issues": "write"}
	inst.Events = []string{"push"}
	inst.SuspendedAt = &suspendedAt
	require.NoError(t, repo.Update(ctx, inst))

	got, err := repo.GetByTeamID(ctx, teamID)
	require.NoError(t, err)
	assert.Equal(t, inst.ID, got.ID)
	assert.Equal(t, int64(4002), got.InstallationID)
	assert.Equal(t, "renamed-user", got.AccountLogin)
	assert.Equal(t, "User", got.AccountType)
	assert.Equal(t, "user", got.TargetType)
	assert.Equal(t, "rotated-token", got.EncryptedAccessToken)
	assert.WithinDuration(t, inst.TokenExpiresAt, got.TokenExpiresAt, time.Second)
	assert.Equal(t, map[string]interface{}{"contents": "write", "issues": "write"}, got.Permissions)
	assert.Equal(t, []string{"push"}, got.Events)
	require.NotNil(t, got.SuspendedAt)
	assert.WithinDuration(t, suspendedAt, *got.SuspendedAt, time.Second)
	assert.False(t, got.UpdatedAt.Before(createdUpdatedAt), "Update must advance updated_at via NOW()")

	// The old installation_id no longer resolves; the new one does.
	_, err = repo.GetByInstallationID(ctx, 4001)
	assert.ErrorIs(t, err, repositories.ErrGitHubInstallationNotFound)
	byNewID, err := repo.GetByInstallationID(ctx, 4002)
	require.NoError(t, err)
	assert.Equal(t, inst.ID, byNewID.ID)
}

func TestGitHubInstallationRepositoryIntegration_UpdateMissingRowIsNotFound(t *testing.T) {
	resetGitHubInstallationTables(t)
	repo := newIntegrationGitHubInstallationRepo()

	ghost := buildGitHubInstallation(uuid.New().String(), 4999)
	err := repo.Update(context.Background(), ghost)
	assert.ErrorIs(t, err, repositories.ErrGitHubInstallationNotFound)
}

func TestGitHubInstallationRepositoryIntegration_DeleteThenRedeleteIsNotFound(t *testing.T) {
	resetGitHubInstallationTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubInstallationRepo()

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)

	require.NoError(t, repo.Create(ctx, buildGitHubInstallation(teamID, 5001)))
	require.NoError(t, repo.Delete(ctx, teamID))

	_, err := repo.GetByTeamID(ctx, teamID)
	assert.ErrorIs(t, err, repositories.ErrGitHubInstallationNotFound)

	err = repo.Delete(ctx, teamID)
	assert.ErrorIs(t, err, repositories.ErrGitHubInstallationNotFound, "re-delete must report not-found")
}

func TestGitHubInstallationRepositoryIntegration_TeamTenancy(t *testing.T) {
	resetGitHubInstallationTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubInstallationRepo()

	userID := insertTestUser(t)
	installedTeam := insertTestTeam(t, userID)
	otherTeam := insertTestTeam(t, userID)

	inst := buildGitHubInstallation(installedTeam, 6001)
	require.NoError(t, repo.Create(ctx, inst))

	// Another team never sees a foreign installation.
	_, err := repo.GetByTeamID(ctx, otherTeam)
	assert.ErrorIs(t, err, repositories.ErrGitHubInstallationNotFound)

	// An unknown installation id resolves to nothing.
	_, err = repo.GetByInstallationID(ctx, 6999)
	assert.ErrorIs(t, err, repositories.ErrGitHubInstallationNotFound)

	// Once the other team installs too, each team resolves only its own row.
	otherInst := buildGitHubInstallation(otherTeam, 6002)
	require.NoError(t, repo.Create(ctx, otherInst))

	got, err := repo.GetByTeamID(ctx, installedTeam)
	require.NoError(t, err)
	assert.Equal(t, inst.ID, got.ID)
	gotOther, err := repo.GetByTeamID(ctx, otherTeam)
	require.NoError(t, err)
	assert.Equal(t, otherInst.ID, gotOther.ID)
}
