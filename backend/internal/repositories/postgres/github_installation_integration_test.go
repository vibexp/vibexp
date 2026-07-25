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
// rows this suite seeds (teams hang off users; github_app_configs and
// github_installations hang off teams with ON DELETE CASCADE).
// resetIntegrationTables only covers users/api_keys/user_preferences, so the
// tables under test are listed explicitly.
func resetGitHubInstallationTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, teams, github_app_configs, github_installations CASCADE")
	require.NoError(t, err)
}

// insertTestGitHubAppConfig seeds a team's own GitHub App (#477). Every
// installation now hangs off one, so a test that creates an installation must
// create its App first. appID must be unique instance-wide (unique_github_app_id).
func insertTestGitHubAppConfig(t *testing.T, teamID, appID string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		`INSERT INTO github_app_configs
		 (id, team_id, app_id, app_slug, client_id,
		  private_key_encrypted, client_secret_encrypted, webhook_secret_encrypted, webhook_token)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		id, teamID, appID, "app-"+appID, "Iv1."+appID,
		"enc-pk", "enc-cs", "enc-ws", "tok-"+id)
	require.NoError(t, err)
	return id
}

func newIntegrationGitHubInstallationRepo() repositories.GitHubInstallationRepository {
	return NewGitHubInstallationRepository(integrationDB.DB, slog.New(slog.DiscardHandler))
}

// buildGitHubInstallation returns an installation row for teamID carrying a
// distinctive permissions/events payload so round-trip fidelity is observable.
func buildGitHubInstallation(teamID, appConfigID string, installationID int64) *models.GitHubInstallation {
	return &models.GitHubInstallation{
		ID:                   uuid.New().String(),
		TeamID:               teamID,
		AppConfigID:          appConfigID,
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
	appConfigID := insertTestGitHubAppConfig(t, teamID, "1001")

	inst := buildGitHubInstallation(teamID, appConfigID, 1001)
	require.NoError(t, repo.Create(ctx, inst))
	assert.False(t, inst.CreatedAt.IsZero(), "Create must write back the DB-assigned created_at")
	assert.False(t, inst.UpdatedAt.IsZero(), "Create must write back the DB-assigned updated_at")

	byTeam, err := repo.GetByTeamID(ctx, teamID)
	require.NoError(t, err)
	assert.Equal(t, inst.ID, byTeam.ID)
	assert.Equal(t, teamID, byTeam.TeamID)
	assert.Equal(t, appConfigID, byTeam.AppConfigID, "the App binding must survive the round-trip")
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

// TestGitHubInstallationRepositoryIntegration_InstallationIDIsUniquePerApp pins
// the constraint swap made by #477. The global UNIQUE(installation_id) is gone:
// with per-team Apps, two teams installing THEIR OWN App on the same GitHub org
// legitimately produce the same installation_id, and that must now succeed.
// Uniqueness moved to (app_config_id, installation_id).
func TestGitHubInstallationRepositoryIntegration_InstallationIDIsUniquePerApp(t *testing.T) {
	resetGitHubInstallationTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubInstallationRepo()

	userID := insertTestUser(t)
	teamA := insertTestTeam(t, userID)
	teamB := insertTestTeam(t, userID)
	appA := insertTestGitHubAppConfig(t, teamA, "2001")
	appB := insertTestGitHubAppConfig(t, teamB, "2002")

	require.NoError(t, repo.Create(ctx, buildGitHubInstallation(teamA, appA, 2001)))

	// Previously a UNIQUE(installation_id) violation; now the supported case.
	require.NoError(t, repo.Create(ctx, buildGitHubInstallation(teamB, appB, 2001)),
		"two teams may install their own Apps on the same GitHub account")

	// The same App may not record the same installation twice. Which constraint
	// name Postgres reports is not pinned here: one App per team
	// (unique_team_github_app) makes app_config_id functionally determine
	// team_id, so the pre-existing unique_team_installation is violated by the
	// same row and may be the index reported first. The schema assertion below
	// is what proves the constraint swap actually happened.
	err := repo.Create(ctx, buildGitHubInstallation(teamA, appA, 2001))
	require.Error(t, err, "one App must not record the same installation_id twice")
	require.NotNil(t, uniqueViolation(err), "expected a unique violation, got: %v", err)

	// Each team still resolves to its own row via the team-scoped read. The
	// un-scoped GetByInstallationID is now ambiguous across Apps by design;
	// making it App-aware belongs to the resolver work (#480).
	gotA, err := repo.GetByTeamID(ctx, teamA)
	require.NoError(t, err)
	assert.Equal(t, appA, gotA.AppConfigID)
	gotB, err := repo.GetByTeamID(ctx, teamB)
	require.NoError(t, err)
	assert.Equal(t, appB, gotB.AppConfigID)
}

// TestGitHubInstallationRepositoryIntegration_ConstraintSwap asserts the schema
// directly, because the two uniqueness rules overlap in practice: a behavioural
// test cannot distinguish unique_app_installation from the pre-existing
// unique_team_installation while one App per team is enforced. Reading
// pg_constraint is the only way to prove 013 both added the new constraint and
// removed the global one it replaces.
func TestGitHubInstallationRepositoryIntegration_ConstraintSwap(t *testing.T) {
	ctx := context.Background()

	constraintExists := func(name string) bool {
		var count int
		err := integrationDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM pg_constraint
			 WHERE conrelid = 'public.github_installations'::regclass AND conname = $1`,
			name).Scan(&count)
		require.NoError(t, err)
		return count > 0
	}

	assert.True(t, constraintExists("unique_app_installation"),
		"013 must add UNIQUE (app_config_id, installation_id)")
	assert.False(t, constraintExists("github_installations_installation_id_key"),
		"013 must drop the global UNIQUE (installation_id): two teams may install their own Apps on the same org")
	assert.True(t, constraintExists("fk_installations_app_config"),
		"installations must be bound to the App that created them")
}

// TestGitHubInstallationRepositoryIntegration_AppConfigCascade proves the
// ON DELETE CASCADE on fk_installations_app_config: removing a team's App
// disconnects the installations made through it, which is what makes deleting a
// config a complete disconnect rather than a half-broken state.
func TestGitHubInstallationRepositoryIntegration_AppConfigCascade(t *testing.T) {
	resetGitHubInstallationTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubInstallationRepo()

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	appConfigID := insertTestGitHubAppConfig(t, teamID, "2501")

	require.NoError(t, repo.Create(ctx, buildGitHubInstallation(teamID, appConfigID, 2501)))

	_, err := integrationDB.ExecContext(ctx, "DELETE FROM github_app_configs WHERE id = $1", appConfigID)
	require.NoError(t, err)

	_, err = repo.GetByTeamID(ctx, teamID)
	assert.ErrorIs(t, err, repositories.ErrGitHubInstallationNotFound,
		"deleting the App config must cascade to its installations")
}

// TestGitHubInstallationRepositoryIntegration_AppConfigIDIsRequired pins the
// NOT NULL on app_config_id: an installation with no owning App is exactly the
// orphan state #477 exists to make unrepresentable.
func TestGitHubInstallationRepositoryIntegration_AppConfigIDIsRequired(t *testing.T) {
	resetGitHubInstallationTables(t)
	ctx := context.Background()

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)

	_, err := integrationDB.ExecContext(ctx,
		`INSERT INTO github_installations
		 (id, team_id, installation_id, account_login, account_type, target_type,
		  encrypted_access_token, token_expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		uuid.New().String(), teamID, int64(2601), "octo-org", "Organization", "organization",
		"enc", time.Now().UTC().Add(time.Hour))
	require.Error(t, err, "app_config_id must be NOT NULL")
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
	appConfigID := insertTestGitHubAppConfig(t, teamID, "3001")

	first := buildGitHubInstallation(teamID, appConfigID, 3001)
	require.NoError(t, repo.Create(ctx, first))
	require.NoError(t, repo.Delete(ctx, teamID))

	second := buildGitHubInstallation(teamID, appConfigID, 3001)
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
	appConfigID := insertTestGitHubAppConfig(t, teamID, "4001")

	inst := buildGitHubInstallation(teamID, appConfigID, 4001)
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

	ghost := buildGitHubInstallation(uuid.New().String(), uuid.New().String(), 4999)
	err := repo.Update(context.Background(), ghost)
	assert.ErrorIs(t, err, repositories.ErrGitHubInstallationNotFound)
}

func TestGitHubInstallationRepositoryIntegration_DeleteThenRedeleteIsNotFound(t *testing.T) {
	resetGitHubInstallationTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubInstallationRepo()

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	appConfigID := insertTestGitHubAppConfig(t, teamID, "5001")

	require.NoError(t, repo.Create(ctx, buildGitHubInstallation(teamID, appConfigID, 5001)))
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
	installedApp := insertTestGitHubAppConfig(t, installedTeam, "6001")
	otherApp := insertTestGitHubAppConfig(t, otherTeam, "6002")

	inst := buildGitHubInstallation(installedTeam, installedApp, 6001)
	require.NoError(t, repo.Create(ctx, inst))

	// Another team never sees a foreign installation.
	_, err := repo.GetByTeamID(ctx, otherTeam)
	assert.ErrorIs(t, err, repositories.ErrGitHubInstallationNotFound)

	// An unknown installation id resolves to nothing.
	_, err = repo.GetByInstallationID(ctx, 6999)
	assert.ErrorIs(t, err, repositories.ErrGitHubInstallationNotFound)

	// Once the other team installs too, each team resolves only its own row.
	otherInst := buildGitHubInstallation(otherTeam, otherApp, 6002)
	require.NoError(t, repo.Create(ctx, otherInst))

	got, err := repo.GetByTeamID(ctx, installedTeam)
	require.NoError(t, err)
	assert.Equal(t, inst.ID, got.ID)
	gotOther, err := repo.GetByTeamID(ctx, otherTeam)
	require.NoError(t, err)
	assert.Equal(t, otherInst.ID, gotOther.ID)
}
