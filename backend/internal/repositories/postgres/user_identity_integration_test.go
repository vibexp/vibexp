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
	"github.com/vibexp/vibexp/internal/repositories"
)

// Real-Postgres coverage for sign-in identities and case-insensitive email
// (#1427): UserIdentityRepository, the identity rows UserRepository.Create
// writes in the same statement as the user, and the identity-based lookup.

func strRef(s string) *string { return &s }

func TestIntegrationUserIdentity_CreateWritesIdentitiesWithTheUser(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	users := NewUserRepository(integrationDB)
	identities := NewUserIdentityRepository(integrationDB)
	now := time.Now().UTC().Truncate(time.Microsecond)

	t.Run("a Google-type user gets the slug identity and the legacy one", func(t *testing.T) {
		subject := "g-" + uuid.New().String()
		user := &models.User{
			GoogleID: &subject, IDPProvider: strRef("google-workspace"), IDPSubject: &subject,
			Email: uuid.New().String() + "@integration.test", Name: "G", EmailVerifiedAt: &now,
			CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, users.Create(ctx, user))

		got, err := identities.ListByUserID(ctx, user.ID)
		require.NoError(t, err)
		require.Len(t, got, 2)
		providers := []string{got[0].Provider, got[1].Provider}
		assert.ElementsMatch(t, []string{"google-workspace", models.LegacyGoogleIdentityProvider}, providers)
		for _, identity := range got {
			assert.Equal(t, subject, identity.Subject)
			assert.Equal(t, user.ID, identity.UserID)
			require.NotNil(t, identity.LastUsedAt, "signing up is a use")
		}

		for _, provider := range providers {
			found, lookupErr := users.GetByIDPSubject(ctx, provider, subject)
			require.NoError(t, lookupErr)
			assert.Equal(t, user.ID, found.ID)
			require.NotNil(t, found.EmailVerifiedAt)
			assert.True(t, now.Equal(*found.EmailVerifiedAt))
		}
	})

	t.Run("an account with no subject gets no identity", func(t *testing.T) {
		// Admin-created: a provider hint but no subject until the first sign-in.
		user := &models.User{
			IDPProvider: strRef("corp-sso"), Email: uuid.New().String() + "@integration.test", Name: "Pending",
			CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, users.Create(ctx, user))

		got, err := identities.ListByUserID(ctx, user.ID)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("the users.idp columns alone do not resolve a sign-in", func(t *testing.T) {
		id := uuid.New().String()
		_, err := integrationDB.ExecContext(ctx,
			"INSERT INTO users (id, email, name, idp_provider, idp_subject) VALUES ($1, $2, 'Inline', 'corp-sso', 'inline-only')",
			id, id+"@integration.test")
		require.NoError(t, err)

		_, err = users.GetByIDPSubject(ctx, "corp-sso", "inline-only")
		require.ErrorIs(t, err, repositories.ErrUserNotFound, "user_identities is the only lookup source")
	})

	t.Run("an identity someone else holds fails the whole create", func(t *testing.T) {
		first := &models.User{
			IDPProvider: strRef("corp-sso"), IDPSubject: strRef("shared-sub"),
			Email: uuid.New().String() + "@integration.test", Name: "First", CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, users.Create(ctx, first))

		secondEmail := uuid.New().String() + "@integration.test"
		second := &models.User{
			IDPProvider: strRef("corp-sso"), IDPSubject: strRef("shared-sub"),
			Email: secondEmail, Name: "Second", CreatedAt: now, UpdatedAt: now,
		}
		err := users.Create(ctx, second)
		require.Error(t, err)
		assert.NotErrorIs(t, err, repositories.ErrUserEmailTaken, "an identity conflict is not a taken email")

		_, err = users.GetByEmail(ctx, secondEmail)
		require.ErrorIs(t, err, repositories.ErrUserNotFound, "no user row may be left without its identity")
	})
}

func TestIntegrationUserIdentity_Upsert(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	identities := NewUserIdentityRepository(integrationDB)
	owner := insertTestUser(t)
	other := insertTestUser(t)

	require.NoError(t, identities.Upsert(ctx, owner, "corp-sso", "sub-1"), "first use creates the identity")
	first, err := identities.ListByUserID(ctx, owner)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.NotNil(t, first[0].LastUsedAt)

	// Age it, so the second use is visibly a new stamp and not the same instant.
	_, err = integrationDB.ExecContext(ctx,
		"UPDATE user_identities SET last_used_at = now() - interval '1 day' WHERE id = $1", first[0].ID)
	require.NoError(t, err)

	require.NoError(t, identities.Upsert(ctx, owner, "corp-sso", "sub-1"), "a later use is idempotent")
	second, err := identities.ListByUserID(ctx, owner)
	require.NoError(t, err)
	require.Len(t, second, 1, "no second row for the same pair")
	assert.Equal(t, first[0].ID, second[0].ID)
	assert.True(t, first[0].CreatedAt.Equal(second[0].CreatedAt), "created_at is not touched")
	assert.WithinDuration(t, time.Now(), *second[0].LastUsedAt, time.Minute, "last_used_at is stamped again")

	err = identities.Upsert(ctx, other, "corp-sso", "sub-1")
	require.ErrorIs(t, err, repositories.ErrUserIdentityTaken, "a pair held by another user is never reassigned")
	stillOwners, err := identities.ListByUserID(ctx, owner)
	require.NoError(t, err)
	require.Len(t, stillOwners, 1)
	othersIdentities, err := identities.ListByUserID(ctx, other)
	require.NoError(t, err)
	assert.Empty(t, othersIdentities)

	require.NoError(t, identities.Upsert(ctx, owner, "github", "gh-9"), "a user may hold several identities")
	both, err := identities.ListByUserID(ctx, owner)
	require.NoError(t, err)
	require.Len(t, both, 2)
	assert.Equal(t, "corp-sso", both[0].Provider, "oldest first")

	require.Error(t, identities.Upsert(ctx, uuid.New().String(), "corp-sso", "sub-2"), "the user must exist")
}

func TestIntegrationUserIdentity_DeletingTheUserDeletesItsIdentities(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	identities := NewUserIdentityRepository(integrationDB)
	userID := insertTestUser(t)
	require.NoError(t, identities.Upsert(ctx, userID, "corp-sso", "sub-del"))

	require.NoError(t, NewUserRepository(integrationDB).DeleteByID(ctx, userID))

	assert.Equal(t, 0, countRows(t, integrationDB.DB,
		"SELECT count(*) FROM user_identities WHERE provider = 'corp-sso' AND subject = 'sub-del'"))
}

// One address, one account, whatever casing each path is handed.
func TestIntegrationUser_EmailIsCaseInsensitive(t *testing.T) {
	resetIntegrationTables(t)
	ctx := context.Background()
	users := NewUserRepository(integrationDB)
	now := time.Now().UTC().Truncate(time.Microsecond)
	local := uuid.New().String()

	user := &models.User{Email: "  Alice-" + local + "@Example.COM ", Name: "Alice", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, users.Create(ctx, user))
	stored := "alice-" + local + "@example.com"
	assert.Equal(t, stored, user.Email, "the model carries what was stored")

	for _, spelling := range []string{stored, "ALICE-" + local + "@EXAMPLE.COM", " Alice-" + local + "@example.com "} {
		got, err := users.GetByEmail(ctx, spelling)
		require.NoError(t, err, "lookup by %q", spelling)
		assert.Equal(t, user.ID, got.ID)
		assert.Equal(t, stored, got.Email)
	}

	dup := &models.User{Email: "ALICE-" + local + "@example.com", Name: "Other Alice", CreatedAt: now, UpdatedAt: now}
	require.ErrorIs(t, users.Create(ctx, dup), repositories.ErrUserEmailTaken,
		"the same address in another casing is the same account")

	fresh, err := users.GetByID(ctx, user.ID)
	require.NoError(t, err)
	fresh.Email = "Alice.Renamed-" + local + "@Example.com"
	fresh.EmailVerifiedAt = &now
	require.NoError(t, users.Update(ctx, fresh))
	assert.Equal(t, "alice.renamed-"+local+"@example.com", fresh.Email)
	reread, err := users.GetByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "alice.renamed-"+local+"@example.com", reread.Email)
	require.NotNil(t, reread.EmailVerifiedAt)
	assert.True(t, now.Equal(*reread.EmailVerifiedAt))

	reread.EmailVerifiedAt = nil
	require.NoError(t, users.Update(ctx, reread))
	cleared, err := users.GetByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Nil(t, cleared.EmailVerifiedAt, "Update writes the model's value, including clearing it")

	_, err = integrationDB.ExecContext(ctx,
		"INSERT INTO users (email, name) VALUES ($1, 'Raw')", "Raw-"+local+"@Example.com")
	requirePQCode(t, err, pqCheckViolation)
}

// A restricted prompt share granted to an address typed with capitals still
// admits that account, whose users.email is lower case since migration
// 022_consolidated.
func TestIntegrationPromptShare_HasAccessIsCaseInsensitive(t *testing.T) {
	resetSearchTables(t)
	ctx := context.Background()
	owner := insertTestUser(t)
	teamID := insertTestTeam(t, owner)
	promptID := insertTestPrompt(t, owner, teamID, insertTestProject(t, owner, teamID), "Shared", "body", "published")
	var shareID string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO prompt_shares (prompt_id, share_token, share_type, created_by)
		 VALUES ($1, $2, 'restricted', $3) RETURNING id`,
		promptID, uuid.New().String(), owner).Scan(&shareID))
	_, err := integrationDB.ExecContext(ctx,
		"INSERT INTO prompt_share_access (share_id, email) VALUES ($1, 'Alice@Example.com')", shareID)
	require.NoError(t, err)
	repo := NewPromptShareRepository(integrationDB)

	for _, viewer := range []string{"alice@example.com", "Alice@Example.com", "ALICE@EXAMPLE.COM"} {
		ok, accessErr := repo.HasAccess(ctx, shareID, viewer)
		require.NoError(t, accessErr)
		assert.True(t, ok, "viewer %q", viewer)
	}
	ok, err := repo.HasAccess(ctx, shareID, "mallory@example.com")
	require.NoError(t, err)
	assert.False(t, ok)
}
