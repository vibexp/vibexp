//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Real-Postgres coverage for UserRepository's read path (#652).
//
// The repository builds every SELECT from the single `userColumns` constant and
// scans it with the single `scanUser` helper, so the column list and the scan
// target list must stay positionally aligned. A mismatch COMPILES CLEANLY and
// fails only at runtime — either with a scan-type error or, far worse, by
// silently loading values shifted by one position into the wrong fields.
// sqlmock cannot catch that: it matches the query by regex and returns whatever
// canned columns the test itself declares, so a test and the code can drift
// together and still pass.
//
// These tests therefore run the real statements against a real table. They also
// deliberately run while the retired billing columns (stripe_customer_id,
// subscription_status, trial_ends_at, subscription_plan,
// subscription_canceled_at) are still PRESENT in the schema — #653 drops them
// separately — which is exactly the state this slice must work in: no longer
// selecting a column is unaffected by that column continuing to exist.

func TestIntegrationUser_CreateGetRoundTrip(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserRepository(integrationDB)
	ctx := context.Background()

	googleID := "google-" + uuid.New().String()
	avatarURL := "https://example.com/avatar.png"
	now := time.Now().UTC().Truncate(time.Microsecond)

	user := &models.User{
		GoogleID:  &googleID,
		Email:     uuid.New().String() + "@integration.test",
		Name:      "Round Trip User",
		AvatarURL: &avatarURL,
		CreatedAt: now,
		UpdatedAt: now,
	}

	require.NoError(t, repo.Create(ctx, user), "Create must succeed against the real schema")
	require.NotEmpty(t, user.ID, "Create must populate the id from RETURNING")

	// GetByID exercises userColumns + scanUser end to end.
	byID, err := repo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	assertUserRoundTrip(t, user, byID, googleID, avatarURL)

	// GetByEmail and GetByGoogleID use the same column list, so a misalignment
	// would surface identically — assert them too rather than trusting one path.
	byEmail, err := repo.GetByEmail(ctx, user.Email)
	require.NoError(t, err)
	assertUserRoundTrip(t, user, byEmail, googleID, avatarURL)

	byGoogleID, err := repo.GetByGoogleID(ctx, googleID)
	require.NoError(t, err)
	assertUserRoundTrip(t, user, byGoogleID, googleID, avatarURL)
}

// assertUserRoundTrip pins every field scanUser populates. Asserting the whole
// set is the point: a one-position scan shift typically leaves some fields
// looking right, so checking only the id would not detect it.
func assertUserRoundTrip(t *testing.T, want, got *models.User, googleID, avatarURL string) {
	t.Helper()

	assert.Equal(t, want.ID, got.ID)
	require.NotNil(t, got.GoogleID)
	assert.Equal(t, googleID, *got.GoogleID)
	assert.Nil(t, got.IDPProvider)
	assert.Nil(t, got.IDPSubject)
	assert.Equal(t, want.Email, got.Email)
	assert.Equal(t, "Round Trip User", got.Name)
	require.NotNil(t, got.AvatarURL)
	assert.Equal(t, avatarURL, *got.AvatarURL)
	assert.Nil(t, got.DefaultTeamID)
	assert.Equal(t, "active", got.Status, "status must come from the column default, not a shifted value")
	assert.False(t, got.OnboardingCompleted)
	assert.Nil(t, got.OnboardingCompletedAt)
	assert.False(t, got.CreatedAt.IsZero())
	assert.False(t, got.UpdatedAt.IsZero())
	assert.Positive(t, got.Version)
}

// TestIntegrationUser_UpdateRoundTrip covers the second RETURNING clause, whose
// column list and inline Scan are a separate pair from userColumns/scanUser.
func TestIntegrationUser_UpdateRoundTrip(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserRepository(integrationDB)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)
	user := &models.User{
		Email:     uuid.New().String() + "@integration.test",
		Name:      "Before Update",
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, repo.Create(ctx, user))

	// Create's RETURNING clause does not include `version`, so reload to pick up
	// the row's real version — Update's WHERE uses it for optimistic concurrency.
	user, err := repo.GetByID(ctx, user.ID)
	require.NoError(t, err)

	versionBefore := user.Version
	user.Name = "After Update"
	user.Email = uuid.New().String() + "@integration.test"
	require.NoError(t, repo.Update(ctx, user), "Update must succeed and scan its RETURNING row")

	assert.Greater(t, user.Version, versionBefore, "optimistic-concurrency version must be bumped")

	reloaded, err := repo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "After Update", reloaded.Name)
	assert.Equal(t, user.Email, reloaded.Email)
	assert.Equal(t, user.Version, reloaded.Version)
}
