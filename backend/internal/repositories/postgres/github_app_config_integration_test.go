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

// The sqlmock suite proves the repository MAPS a unique violation to the right
// sentinel; this suite proves the database actually RAISES one. Both are needed:
// a constraint missing from the migration would leave every sqlmock test green.

func resetGitHubAppConfigTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, teams, github_app_configs, github_installations CASCADE")
	require.NoError(t, err)
}

func newIntegrationGitHubAppConfigRepo() repositories.GitHubAppConfigRepository {
	return NewGitHubAppConfigRepository(integrationDB)
}

// buildGitHubAppConfig returns a config with values distinctive enough that a
// column transposed in the INSERT or the SELECT projection is visible.
func buildGitHubAppConfig(teamID, userID, appID string) *models.GitHubAppConfig {
	return &models.GitHubAppConfig{
		TeamID:                 teamID,
		UserID:                 &userID,
		AppID:                  appID,
		AppSlug:                "slug-" + appID,
		ClientID:               "Iv1." + appID,
		PrivateKeyEncrypted:    "enc-private-key-" + appID,
		ClientSecretEncrypted:  "enc-client-secret-" + appID,
		WebhookSecretEncrypted: "enc-webhook-secret-" + appID,
		WebhookToken:           "tok-" + uuid.New().String(),
	}
}

func TestGitHubAppConfigRepositoryIntegration_CreateAndGetRoundTrip(t *testing.T) {
	resetGitHubAppConfigTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubAppConfigRepo()

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)

	config := buildGitHubAppConfig(teamID, userID, "100001")
	require.NoError(t, repo.Create(ctx, config))
	assert.NotEmpty(t, config.ID, "Create must write back the DB-assigned id")
	assert.False(t, config.CreatedAt.IsZero())
	assert.Equal(t, int64(1), config.Version, "a new row starts at version 1")

	// Every field is asserted explicitly: a transposed column in the INSERT or
	// the SELECT projection still satisfies the schema and would otherwise pass.
	byTeam, err := repo.GetByTeamID(ctx, teamID)
	require.NoError(t, err)
	assert.Equal(t, config.ID, byTeam.ID)
	assert.Equal(t, teamID, byTeam.TeamID)
	require.NotNil(t, byTeam.UserID)
	assert.Equal(t, userID, *byTeam.UserID)
	assert.Equal(t, "100001", byTeam.AppID)
	assert.Equal(t, "slug-100001", byTeam.AppSlug)
	assert.Equal(t, "Iv1.100001", byTeam.ClientID)
	assert.Equal(t, "enc-private-key-100001", byTeam.PrivateKeyEncrypted)
	assert.Equal(t, "enc-client-secret-100001", byTeam.ClientSecretEncrypted)
	assert.Equal(t, "enc-webhook-secret-100001", byTeam.WebhookSecretEncrypted)
	assert.Equal(t, config.WebhookToken, byTeam.WebhookToken)

	byID, err := repo.GetByID(ctx, teamID, config.ID)
	require.NoError(t, err)
	assert.Equal(t, byTeam, byID, "all three reads must return the same row")

	byToken, err := repo.GetByWebhookToken(ctx, config.WebhookToken)
	require.NoError(t, err)
	assert.Equal(t, byTeam, byToken)
}

// user_id is nullable and informational; a config outlives the account that
// registered it, so a NULL must scan cleanly rather than blow up the read.
func TestGitHubAppConfigRepositoryIntegration_NullUserID(t *testing.T) {
	resetGitHubAppConfigTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubAppConfigRepo()

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)

	config := buildGitHubAppConfig(teamID, userID, "100002")
	config.UserID = nil
	require.NoError(t, repo.Create(ctx, config))

	got, err := repo.GetByTeamID(ctx, teamID)
	require.NoError(t, err)
	assert.Nil(t, got.UserID)
}

func TestGitHubAppConfigRepositoryIntegration_UniqueTeam(t *testing.T) {
	resetGitHubAppConfigTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubAppConfigRepo()

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)

	require.NoError(t, repo.Create(ctx, buildGitHubAppConfig(teamID, userID, "200001")))

	err := repo.Create(ctx, buildGitHubAppConfig(teamID, userID, "200002"))
	assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigTeamTaken,
		"unique_team_github_app must surface as the mapped sentinel, not a raw pq error")
}

func TestGitHubAppConfigRepositoryIntegration_UniqueAppID(t *testing.T) {
	resetGitHubAppConfigTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubAppConfigRepo()

	userID := insertTestUser(t)
	teamA := insertTestTeam(t, userID)
	teamB := insertTestTeam(t, userID)

	require.NoError(t, repo.Create(ctx, buildGitHubAppConfig(teamA, userID, "300001")))

	// A GitHub App has one hook_url, so team B registering the same App would
	// leave its webhook token permanently dead.
	err := repo.Create(ctx, buildGitHubAppConfig(teamB, userID, "300001"))
	assert.ErrorIs(t, err, repositories.ErrGitHubAppAlreadyRegistered,
		"unique_github_app_id must surface as the mapped sentinel, not a raw pq error")

	// The first registration is untouched.
	got, err := repo.GetByTeamID(ctx, teamA)
	require.NoError(t, err)
	assert.Equal(t, "300001", got.AppID)
}

func TestGitHubAppConfigRepositoryIntegration_UniqueWebhookToken(t *testing.T) {
	resetGitHubAppConfigTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubAppConfigRepo()

	userID := insertTestUser(t)
	teamA := insertTestTeam(t, userID)
	teamB := insertTestTeam(t, userID)

	first := buildGitHubAppConfig(teamA, userID, "400001")
	require.NoError(t, repo.Create(ctx, first))

	// A collision is astronomically unlikely with 32 bytes of crypto/rand, but
	// the unique index is what makes it a retryable error instead of two teams
	// silently sharing a webhook route.
	second := buildGitHubAppConfig(teamB, userID, "400002")
	second.WebhookToken = first.WebhookToken
	err := repo.Create(ctx, second)
	require.Error(t, err)
	pqErr := uniqueViolation(err)
	require.NotNil(t, pqErr, "expected a unique violation, got: %v", err)
	assert.Equal(t, "idx_github_app_configs_webhook_token", pqErr.Constraint)
}

// TestGitHubAppConfigRepositoryIntegration_EmptyStringsRejected pins the CHECK
// constraints. NOT NULL alone accepts the empty string, and for these two
// columns an empty value fails silently and cross-tenant rather than loudly:
// an empty webhook_token would let an empty URL segment on the public webhook
// route resolve a real team's config, and an empty app_id would trip
// unique_github_app_id and tell the second writer the App is already taken.
func TestGitHubAppConfigRepositoryIntegration_EmptyStringsRejected(t *testing.T) {
	resetGitHubAppConfigTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubAppConfigRepo()

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)

	blankToken := buildGitHubAppConfig(teamID, userID, "450001")
	blankToken.WebhookToken = ""
	require.Error(t, repo.Create(ctx, blankToken), "an empty webhook_token must be rejected")

	blankAppID := buildGitHubAppConfig(teamID, userID, "")
	require.Error(t, repo.Create(ctx, blankAppID), "an empty app_id must be rejected")

	// The guard must survive an update, which is the likelier way to blank one:
	// both columns are writable and a caller that omits one would otherwise
	// clear it without complaint.
	valid := buildGitHubAppConfig(teamID, userID, "450002")
	require.NoError(t, repo.Create(ctx, valid))
	valid.WebhookToken = ""
	require.Error(t, repo.Update(ctx, valid), "an update must not be able to blank webhook_token")
}

func TestGitHubAppConfigRepositoryIntegration_UpdateOptimisticLocking(t *testing.T) {
	resetGitHubAppConfigTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubAppConfigRepo()

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)

	config := buildGitHubAppConfig(teamID, userID, "500001")
	require.NoError(t, repo.Create(ctx, config))
	createdUpdatedAt := config.UpdatedAt

	config.AppSlug = "renamed-app"
	config.PrivateKeyEncrypted = "enc-rotated-key"
	require.NoError(t, repo.Update(ctx, config))
	assert.Equal(t, int64(2), config.Version, "a successful update bumps version")
	assert.False(t, config.UpdatedAt.Before(createdUpdatedAt))

	got, err := repo.GetByTeamID(ctx, teamID)
	require.NoError(t, err)
	assert.Equal(t, "renamed-app", got.AppSlug)
	assert.Equal(t, "enc-rotated-key", got.PrivateKeyEncrypted)

	// A second writer holding the pre-update version must lose, and lose without
	// mutating anything.
	stale := *config
	stale.Version = 1
	stale.AppSlug = "clobbered"
	err = repo.Update(ctx, &stale)
	assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigVersionConflict)

	unchanged, err := repo.GetByTeamID(ctx, teamID)
	require.NoError(t, err)
	assert.Equal(t, "renamed-app", unchanged.AppSlug, "the losing write must not have applied")
	assert.Equal(t, int64(2), unchanged.Version)
}

func TestGitHubAppConfigRepositoryIntegration_UpdateDuplicateAppID(t *testing.T) {
	resetGitHubAppConfigTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubAppConfigRepo()

	userID := insertTestUser(t)
	teamA := insertTestTeam(t, userID)
	teamB := insertTestTeam(t, userID)

	require.NoError(t, repo.Create(ctx, buildGitHubAppConfig(teamA, userID, "600001")))
	configB := buildGitHubAppConfig(teamB, userID, "600002")
	require.NoError(t, repo.Create(ctx, configB))

	// Editing into a taken App id must be rejected the same way creating one is.
	configB.AppID = "600001"
	err := repo.Update(ctx, configB)
	assert.ErrorIs(t, err, repositories.ErrGitHubAppAlreadyRegistered)
}

func TestGitHubAppConfigRepositoryIntegration_DeleteAndUnknownReads(t *testing.T) {
	resetGitHubAppConfigTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubAppConfigRepo()

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)

	config := buildGitHubAppConfig(teamID, userID, "700001")
	require.NoError(t, repo.Create(ctx, config))

	require.NoError(t, repo.Delete(ctx, teamID, config.ID))
	assert.ErrorIs(t, repo.Delete(ctx, teamID, config.ID), repositories.ErrGitHubAppConfigNotFound,
		"re-delete must report not-found")

	_, err := repo.GetByTeamID(ctx, teamID)
	assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigNotFound)

	_, err = repo.GetByWebhookToken(ctx, config.WebhookToken)
	assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigNotFound)

	_, err = repo.GetByWebhookToken(ctx, "tok-does-not-exist")
	assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigNotFound)
}

// TestGitHubAppConfigRepositoryIntegration_TeamTenancy is the load-bearing test
// of this issue: these rows hold a team's GitHub credentials, so a read that
// forgets its team_id predicate is a cross-tenant credential leak.
func TestGitHubAppConfigRepositoryIntegration_TeamTenancy(t *testing.T) {
	resetGitHubAppConfigTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubAppConfigRepo()

	userID := insertTestUser(t)
	owningTeam := insertTestTeam(t, userID)
	foreignTeam := insertTestTeam(t, userID)

	config := buildGitHubAppConfig(owningTeam, userID, "800001")
	require.NoError(t, repo.Create(ctx, config))

	// Reads: the foreign team gets not-found, never the row.
	_, err := repo.GetByID(ctx, foreignTeam, config.ID)
	assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigNotFound)
	_, err = repo.GetByTeamID(ctx, foreignTeam)
	assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigNotFound)

	// Writes: the foreign team cannot edit or delete it.
	hijack := *config
	hijack.TeamID = foreignTeam
	hijack.AppSlug = "hijacked"
	err = repo.Update(ctx, &hijack)
	assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigVersionConflict)

	err = repo.Delete(ctx, foreignTeam, config.ID)
	assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigNotFound)

	// The owner's row survived both attempts intact.
	survived, err := repo.GetByID(ctx, owningTeam, config.ID)
	require.NoError(t, err)
	assert.Equal(t, "slug-800001", survived.AppSlug)
	assert.Equal(t, int64(1), survived.Version)
}

// Deleting a team must take its App config with it, so credentials never
// outlive the tenant they belong to.
func TestGitHubAppConfigRepositoryIntegration_TeamDeleteCascade(t *testing.T) {
	resetGitHubAppConfigTables(t)
	ctx := context.Background()
	repo := newIntegrationGitHubAppConfigRepo()

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)

	config := buildGitHubAppConfig(teamID, userID, "900001")
	require.NoError(t, repo.Create(ctx, config))

	_, err := integrationDB.ExecContext(ctx, "DELETE FROM teams WHERE id = $1", teamID)
	require.NoError(t, err)

	_, err = repo.GetByWebhookToken(ctx, config.WebhookToken)
	assert.ErrorIs(t, err, repositories.ErrGitHubAppConfigNotFound)
}
