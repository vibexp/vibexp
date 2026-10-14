//go:build integration

package postgres

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// An administrator's disable and the owner's own change (#1436, #1461).

func disabledAt(t *testing.T, userID string) *time.Time {
	t.Helper()
	var at *time.Time
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		`SELECT disabled_at FROM user_credentials WHERE user_id = $1`, userID).Scan(&at))
	return at
}

func TestIntegrationUserCredential_ReplaceUnlessDisabled(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserCredentialRepository(integrationDB)
	ctx := context.Background()

	t.Run("a first password is stored", func(t *testing.T) {
		userID := insertTestUser(t)
		require.NoError(t, repo.ReplaceUnlessDisabled(ctx, userID, credentialTestHash))
		got, err := repo.Get(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, credentialTestHash, got.PasswordHash)
	})

	t.Run("a usable password is replaced and the throttle cleared", func(t *testing.T) {
		userID := insertTestUser(t)
		require.NoError(t, repo.Upsert(ctx, userID, credentialTestHash))
		_, err := repo.BeginAttempt(ctx, userID, time.Now().UTC(), models.DefaultPasswordLockout)
		require.NoError(t, err)

		require.NoError(t, repo.ReplaceUnlessDisabled(ctx, userID, credentialTestHash+"new"))

		got, err := repo.Get(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, credentialTestHash+"new", got.PasswordHash)
		assert.Zero(t, got.FailedAttempts)
	})

	t.Run("a disabled password is left as it is", func(t *testing.T) {
		userID := insertTestUser(t)
		require.NoError(t, repo.Upsert(ctx, userID, credentialTestHash))
		_, err := integrationDB.ExecContext(ctx,
			`UPDATE user_credentials SET disabled_at = CURRENT_TIMESTAMP WHERE user_id = $1`, userID)
		require.NoError(t, err)

		err = repo.ReplaceUnlessDisabled(ctx, userID, credentialTestHash+"new")

		require.ErrorIs(t, err, repositories.ErrUserCredentialDisabled)
		got, err := repo.Get(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, credentialTestHash, got.PasswordHash)
		assert.True(t, got.IsDisabled())
	})

	t.Run("an unknown user", func(t *testing.T) {
		assert.ErrorIs(t, repo.ReplaceUnlessDisabled(ctx, uuid.NewString(), credentialTestHash), repositories.ErrUserNotFound)
	})
}

// The race the lead asked for: an administrator's disable commits while the
// owner's change is already under way. The order is forced: a transaction
// holds the credential row, the owner's write queues behind it, and the
// holder then disables the password and commits. The owner's write re-checks
// the row it waited for, finds the mark and changes nothing.
//
// The control case shows the test can fail: the plain Upsert (the statement
// the owner's change used before) waits the same way and then clears the mark.
func TestIntegrationUserCredential_DisableDuringTheOwnersChangeWins(t *testing.T) {
	for _, tc := range []struct {
		name     string
		write    func(repo *UserCredentialRepository, userID string) error
		wantErr  error
		wantMark bool
		wantHash string
	}{
		{"the owner's change", func(repo *UserCredentialRepository, userID string) error {
			return repo.ReplaceUnlessDisabled(context.Background(), userID, credentialTestHash+"owner")
		}, repositories.ErrUserCredentialDisabled, true, credentialTestHash},
		{"control: a plain upsert", func(repo *UserCredentialRepository, userID string) error {
			return repo.Upsert(context.Background(), userID, credentialTestHash+"owner")
		}, nil, false, credentialTestHash + "owner"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetIntegrationTables(t)
			repo := NewUserCredentialRepository(integrationDB).(*UserCredentialRepository)
			ctx := context.Background()
			userID := insertTestUser(t)
			require.NoError(t, repo.Upsert(ctx, userID, credentialTestHash))

			holder, err := integrationDB.BeginTx(ctx, nil)
			require.NoError(t, err)
			rollbackOnCleanup(t, holder)
			_, err = holder.ExecContext(ctx, `SELECT 1 FROM user_credentials WHERE user_id = $1 FOR UPDATE`, userID)
			require.NoError(t, err)

			var writeErr error
			var done sync.WaitGroup
			done.Go(func() { writeErr = tc.write(repo, userID) })
			require.Eventually(t, func() bool {
				var waiting int
				scanErr := integrationDB.QueryRowContext(ctx,
					`SELECT count(*) FROM pg_locks WHERE NOT granted AND `+thisDatabaseLocks).Scan(&waiting)
				return scanErr == nil && waiting >= 1
			}, 10*time.Second, 5*time.Millisecond, "the owner's write is waiting on the row")

			_, err = holder.ExecContext(ctx,
				`UPDATE user_credentials SET disabled_at = CURRENT_TIMESTAMP WHERE user_id = $1`, userID)
			require.NoError(t, err)
			require.NoError(t, holder.Commit())
			done.Wait()

			if tc.wantErr != nil {
				require.ErrorIs(t, writeErr, tc.wantErr)
			} else {
				require.NoError(t, writeErr)
			}
			assert.Equal(t, tc.wantMark, disabledAt(t, userID) != nil)
			assert.Equal(t, tc.wantHash, readAccountState(t, userID).passwordHash)
		})
	}
}

func TestIntegrationAccountToken_DisablePassword(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	credentials := NewUserCredentialRepository(integrationDB)
	ctx := context.Background()
	userID, otherUser := insertTestUser(t), insertTestUser(t)
	require.NoError(t, credentials.Upsert(ctx, userID, credentialTestHash))
	used := issueAccountToken(t, repo, userID, models.AccountTokenPurposeSetup, models.AccountTokenDeliveryLink)
	_, err := completeLink(repo, used, accountTokenTestNow)
	require.NoError(t, err)
	before := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail)
	issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryLink)
	others := issueAccountToken(t, repo, otherUser, models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail)
	start := readAccountState(t, userID)
	otherStart := readAccountState(t, otherUser)

	disabled, err := repo.DisablePassword(ctx, userID)

	require.NoError(t, err)
	assert.True(t, disabled.HadPassword)
	assert.Equal(t, int64(2), disabled.Links, "the live links; the used one is history")
	assert.Equal(t, start.sessionEpoch+1, disabled.SessionEpoch)
	mark := disabledAt(t, userID)
	require.NotNil(t, mark)
	_, err = repo.GetLive(ctx, before, accountTokenTestNow)
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound, "a link issued before the disable dies with it")
	assert.Equal(t, otherStart, readAccountState(t, otherUser), "another account is untouched")
	_, err = repo.GetLive(ctx, others, accountTokenTestNow)
	assert.NoError(t, err)

	t.Run("again keeps the first time and still ends sessions", func(t *testing.T) {
		again, err := repo.DisablePassword(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, disabled.SessionEpoch+1, again.SessionEpoch)
		assert.True(t, mark.Equal(*disabledAt(t, userID)))
	})

	t.Run("a link issued afterwards completes and lifts the mark", func(t *testing.T) {
		// The administrator's link: the owner's own reset is refused while
		// the mark is set (TestIntegrationAccountToken_SelfServiceResetRefusedForADisabledPassword).
		adminID := insertTestUser(t)
		fresh := accountTokenHash(uuid.NewString())
		require.NoError(t, repo.Replace(ctx, &models.AccountToken{
			UserID: userID, Purpose: models.AccountTokenPurposeReset, Delivery: models.AccountTokenDeliveryLink,
			TokenHash: fresh, ExpiresAt: accountTokenTestNow.Add(time.Hour), CreatedBy: &adminID,
		}, 0))
		_, err := completeLink(repo, fresh, accountTokenTestNow)
		require.NoError(t, err)
		assert.Nil(t, disabledAt(t, userID))
		assert.Equal(t, accountTokenTestHash, readAccountState(t, userID).passwordHash)
	})

	t.Run("an account with no password: sessions and links end, nothing to mark", func(t *testing.T) {
		noPassword := insertTestUser(t)
		issueAccountToken(t, repo, noPassword, models.AccountTokenPurposeSetup, models.AccountTokenDeliveryLink)
		st := readAccountState(t, noPassword)

		got, err := repo.DisablePassword(ctx, noPassword)

		require.NoError(t, err)
		assert.False(t, got.HadPassword)
		assert.Equal(t, int64(1), got.Links)
		assert.Equal(t, st.sessionEpoch+1, got.SessionEpoch)
	})

	t.Run("an unknown user", func(t *testing.T) {
		_, err := repo.DisablePassword(ctx, uuid.NewString())
		assert.ErrorIs(t, err, repositories.ErrUserNotFound)
	})
}

func TestIntegrationSignInMethod_RemovePasswordOptions(t *testing.T) {
	ctx := context.Background()
	disable := func(t *testing.T, userID string) {
		t.Helper()
		_, err := integrationDB.ExecContext(ctx,
			`UPDATE user_credentials SET disabled_at = CURRENT_TIMESTAMP WHERE user_id = $1`, userID)
		require.NoError(t, err)
	}

	t.Run("the owner cannot remove a disabled password", func(t *testing.T) {
		f := newSignInMethodFixture(t)
		userID := f.user(true, "google")
		disable(t, userID)

		err := f.methods.RemovePassword(ctx, userID, usableMethods(true), repositories.RemovePasswordOptions{})

		require.ErrorIs(t, err, repositories.ErrUserCredentialDisabled)
		assert.True(t, f.hasPassword(userID))
	})

	t.Run("an administrator can", func(t *testing.T) {
		f := newSignInMethodFixture(t)
		userID := f.user(true, "google")
		disable(t, userID)

		require.NoError(t, f.methods.RemovePassword(ctx, userID, usableMethods(true),
			repositories.RemovePasswordOptions{AllowDisabled: true}))
		assert.False(t, f.hasPassword(userID))
	})

	t.Run("the last way in only when allowed", func(t *testing.T) {
		f := newSignInMethodFixture(t)
		userID := f.user(true)

		err := f.methods.RemovePassword(ctx, userID, usableMethods(true), repositories.RemovePasswordOptions{AllowDisabled: true})
		require.ErrorIs(t, err, repositories.ErrLastSignInMethod)
		assert.True(t, f.hasPassword(userID))

		require.NoError(t, f.methods.RemovePassword(ctx, userID, usableMethods(true),
			repositories.RemovePasswordOptions{AllowLast: true, AllowDisabled: true}))
		assert.False(t, f.hasPassword(userID))
	})
}

// The owner's own reset request must not reach a disabled password: one that
// passed the mailer's screening before an administrator's disable is refused
// when it is stored, under the lock the disable takes. An administrator's link
// (CreatedBy set) is what lifts the mark, so it is stored.
func TestIntegrationAccountToken_SelfServiceResetRefusedForADisabledPassword(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	ctx := context.Background()
	userID, adminID := insertTestUser(t), insertTestUser(t)
	require.NoError(t, NewUserCredentialRepository(integrationDB).Upsert(ctx, userID, credentialTestHash))
	_, err := repo.DisablePassword(ctx, userID)
	require.NoError(t, err)
	token := func(createdBy *string, purpose string) *models.AccountToken {
		return &models.AccountToken{UserID: userID, Purpose: purpose, Delivery: models.AccountTokenDeliveryEmail,
			TokenHash: accountTokenHash(uuid.NewString()), ExpiresAt: accountTokenTestNow.Add(time.Hour), CreatedBy: createdBy}
	}

	err = repo.Replace(ctx, token(nil, models.AccountTokenPurposeReset), 0)
	require.ErrorIs(t, err, repositories.ErrUserCredentialDisabled)
	assert.Zero(t, countAccountTokens(t, "user_id = $1", userID), "nothing stored")

	require.NoError(t, repo.Replace(ctx, token(&adminID, models.AccountTokenPurposeReset), 0),
		"an administrator's link is stored")
	require.NoError(t, repo.Replace(ctx, token(nil, models.AccountTokenPurposeSetup), 0),
		"the rule is about the owner's reset only")
}
