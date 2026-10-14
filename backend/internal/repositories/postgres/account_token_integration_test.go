//go:build integration

package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Real-Postgres coverage for account_tokens (#1432). The properties these
// tests pin are the ones only the database can give: one live link per user,
// purpose and delivery; a consume that exactly one of many simultaneous
// requests wins and that ends every other link of the user; and the
// constraints of migration 031.

var accountTokenTestNow = time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)

func accountTokenHash(seed string) []byte {
	sum := sha256.Sum256([]byte(seed))
	return sum[:]
}

// issueAccountToken stores a link for the user that expires an hour after
// accountTokenTestNow and returns its hash.
func issueAccountToken(
	t *testing.T, repo repositories.AccountTokenRepository, userID, purpose, delivery string,
) []byte {
	t.Helper()
	hash := accountTokenHash(uuid.NewString())
	require.NoError(t, repo.Replace(context.Background(), &models.AccountToken{
		UserID: userID, Purpose: purpose, Delivery: delivery, TokenHash: hash,
		ExpiresAt: accountTokenTestNow.Add(time.Hour),
	}, 0))
	return hash
}

// accountTokenTestHash is a hash-shaped fixture for the password a completion
// stores. The repository never interprets it.
const accountTokenTestHash = "$argon2id$v=19$m=64,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$bGlua2xpbmtsaW5rbGlua2xpbmtsaW5rbGlua2xpbms"

// completeLink completes the link with this hash at now, storing the fixture
// password hash and no name, and returns the consumed link.
func completeLink(repo repositories.AccountTokenRepository, hash []byte, now time.Time) (*models.AccountToken, error) {
	done, err := repo.Complete(context.Background(), repositories.AccountLinkCompletion{
		TokenHash: hash, Now: now, PasswordHash: accountTokenTestHash,
	})
	if err != nil {
		return nil, err
	}
	return done.Token, nil
}

func countAccountTokens(t *testing.T, where string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT count(*) FROM account_tokens WHERE "+where, args...).Scan(&n))
	return n
}

func TestIntegrationAccountToken_Lifecycle(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	ctx := context.Background()
	userID, adminID := insertTestUser(t), insertTestUser(t)
	hash := accountTokenHash("lifecycle")
	token := &models.AccountToken{
		UserID: userID, Purpose: models.AccountTokenPurposeSetup, Delivery: models.AccountTokenDeliveryLink,
		TokenHash: hash, ExpiresAt: accountTokenTestNow.Add(time.Hour), CreatedBy: &adminID,
	}

	require.NoError(t, repo.Replace(ctx, token, 0))
	assert.NotEmpty(t, token.ID)
	assert.False(t, token.CreatedAt.IsZero())

	got, err := repo.GetLive(ctx, hash, accountTokenTestNow)
	require.NoError(t, err)
	assert.Equal(t, token.ID, got.ID)
	assert.Equal(t, userID, got.UserID)
	assert.Equal(t, models.AccountTokenPurposeSetup, got.Purpose)
	assert.Equal(t, models.AccountTokenDeliveryLink, got.Delivery)
	assert.Equal(t, hash, got.TokenHash)
	assert.True(t, got.ExpiresAt.Equal(accountTokenTestNow.Add(time.Hour)))
	assert.Nil(t, got.ConsumedAt)
	require.NotNil(t, got.CreatedBy)
	assert.Equal(t, adminID, *got.CreatedBy)

	_, err = repo.GetLive(ctx, accountTokenHash("another"), accountTokenTestNow)
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound)
	_, err = repo.GetLive(ctx, hash, accountTokenTestNow.Add(time.Hour))
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound, "a link is dead at the instant it expires")
	_, err = repo.GetLive(ctx, hash, accountTokenTestNow.Add(time.Hour-time.Microsecond))
	assert.NoError(t, err, "and live until then")

	consumed, err := completeLink(repo, hash, accountTokenTestNow)
	require.NoError(t, err)
	require.NotNil(t, consumed.ConsumedAt)
	assert.True(t, consumed.ConsumedAt.Equal(accountTokenTestNow))
	assert.Equal(t, token.ID, consumed.ID)

	_, err = repo.GetLive(ctx, hash, accountTokenTestNow)
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound, "a consumed link is not live")
	_, err = completeLink(repo, hash, accountTokenTestNow)
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound, "a link is consumed once")
}

func TestIntegrationAccountToken_CompleteRefusesDeadAndUnknownLinks(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	userID := insertTestUser(t)
	hash := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail)

	_, err := completeLink(repo, accountTokenHash("unknown"), accountTokenTestNow)
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound)
	_, err = completeLink(repo, hash, accountTokenTestNow.Add(time.Hour))
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound, "an expired link cannot be consumed")

	assert.Equal(t, 1, countAccountTokens(t, "consumed_at IS NULL"), "and is left as it was")
}

// A new link replaces the previous live one of the same purpose AND delivery,
// and no other.
func TestIntegrationAccountToken_ReplaceSupersedesPerPurposeAndDelivery(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	ctx := context.Background()
	userID, otherUser := insertTestUser(t), insertTestUser(t)

	resetEmail := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail)
	resetLink := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryLink)
	setupEmail := issueAccountToken(t, repo, userID, models.AccountTokenPurposeSetup, models.AccountTokenDeliveryEmail)
	others := issueAccountToken(t, repo, otherUser, models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail)

	newer := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail)

	_, err := repo.GetLive(ctx, resetEmail, accountTokenTestNow)
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound, "the earlier emailed reset link is replaced")
	for name, hash := range map[string][]byte{
		"the newer emailed reset link": newer,
		"the copied reset link":        resetLink,
		"the emailed setup link":       setupEmail,
		"another user's link":          others,
	} {
		_, err = repo.GetLive(ctx, hash, accountTokenTestNow)
		assert.NoError(t, err, name)
	}
	assert.Equal(t, 4, countAccountTokens(t, "true"), "the replaced row is deleted, not kept")
}

// Consuming a link ends every other live link of the user, whatever its
// purpose or delivery, and no one else's.
func TestIntegrationAccountToken_CompleteEndsEveryOtherLinkOfTheUser(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	ctx := context.Background()
	userID, otherUser := insertTestUser(t), insertTestUser(t)
	resetEmail := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail)
	resetLink := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryLink)
	setupLink := issueAccountToken(t, repo, userID, models.AccountTokenPurposeSetup, models.AccountTokenDeliveryLink)
	others := issueAccountToken(t, repo, otherUser, models.AccountTokenPurposeReset, models.AccountTokenDeliveryLink)

	consumed, err := completeLink(repo, resetLink, accountTokenTestNow)
	require.NoError(t, err)
	assert.Equal(t, models.AccountTokenDeliveryLink, consumed.Delivery)

	_, err = repo.GetLive(ctx, resetEmail, accountTokenTestNow)
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound, "the emailed reset link ended with the copied one")
	_, err = completeLink(repo, resetEmail, accountTokenTestNow)
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound)
	_, err = repo.GetLive(ctx, setupLink, accountTokenTestNow)
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound, "a setup link does not outlive a completed reset")
	_, err = completeLink(repo, setupLink, accountTokenTestNow)
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound)
	_, err = repo.GetLive(ctx, others, accountTokenTestNow)
	assert.NoError(t, err, "another user's link is untouched")

	// A link of the same kind can be issued again after one was consumed: the
	// one-live-link index ignores consumed rows.
	again := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryLink)
	_, err = repo.GetLive(ctx, again, accountTokenTestNow)
	assert.NoError(t, err)
	assert.Equal(t, 1, countAccountTokens(t, "consumed_at IS NOT NULL"), "the consumed row is kept until the sweep")
}

// Of many requests completing the same link at once, exactly one wins.
func TestIntegrationAccountToken_ConcurrentCompletionOfOneLinkHasOneWinner(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	userID := insertTestUser(t)
	hash := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail)

	const callers = 24
	var won, lost atomic.Int32
	var start, done sync.WaitGroup
	start.Add(1)
	for range callers {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			_, err := completeLink(repo, hash, accountTokenTestNow)
			switch {
			case err == nil:
				won.Add(1)
			case assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound):
				lost.Add(1)
			}
		}()
	}
	start.Done()
	done.Wait()

	assert.Equal(t, int32(1), won.Load())
	assert.Equal(t, int32(callers-1), lost.Load())
}

// Two different links of one user (an emailed reset link and a copied setup
// link), completed at the same moment: one wins, the other is refused, and
// nothing deadlocks.
func TestIntegrationAccountToken_ConcurrentCompletionOfBothLinksHasOneWinner(t *testing.T) {
	repo := NewAccountTokenRepository(integrationDB)

	for round := range 20 {
		resetIntegrationTables(t)
		userID := insertTestUser(t)
		hashes := [][]byte{
			issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail),
			issueAccountToken(t, repo, userID, models.AccountTokenPurposeSetup, models.AccountTokenDeliveryLink),
		}

		var won atomic.Int32
		var start, done sync.WaitGroup
		start.Add(1)
		for i := range 8 {
			done.Add(1)
			go func() {
				defer done.Done()
				start.Wait()
				_, err := completeLink(repo, hashes[i%2], accountTokenTestNow)
				if err == nil {
					won.Add(1)
					return
				}
				assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound, "round %d", round)
			}()
		}
		start.Done()
		done.Wait()

		require.Equal(t, int32(1), won.Load(), "round %d", round)
		assert.Equal(t, 0, countAccountTokens(t, "consumed_at IS NULL"), "no live link is left")
	}
}

// Many requests issuing the same kind of link for one user at once all
// succeed, and exactly one link is left live.
func TestIntegrationAccountToken_ConcurrentReplaceLeavesOneLiveLink(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	userID := insertTestUser(t)

	var start, done sync.WaitGroup
	start.Add(1)
	for range 16 {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			assert.NoError(t, repo.Replace(context.Background(), &models.AccountToken{
				UserID: userID, Purpose: models.AccountTokenPurposeReset, Delivery: models.AccountTokenDeliveryEmail,
				TokenHash: accountTokenHash(uuid.NewString()), ExpiresAt: accountTokenTestNow.Add(time.Hour),
			}, 0))
		}()
	}
	start.Done()
	done.Wait()

	assert.Equal(t, 1, countAccountTokens(t, "user_id = $1", userID))
}

func TestIntegrationAccountToken_Delete(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	ctx := context.Background()
	userID, otherUser := insertTestUser(t), insertTestUser(t)
	hash := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail)
	issueAccountToken(t, repo, userID, models.AccountTokenPurposeSetup, models.AccountTokenDeliveryLink)
	others := issueAccountToken(t, repo, otherUser, models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail)

	deleted, err := repo.Delete(ctx, hash)
	require.NoError(t, err)
	assert.True(t, deleted)
	deleted, err = repo.Delete(ctx, hash)
	require.NoError(t, err)
	assert.False(t, deleted, "deleting nothing is not an error")

	_, err = repo.GetLive(ctx, others, accountTokenTestNow)
	assert.NoError(t, err, "another user's link is untouched")
}

// accountState is what a completion or a claim changes about an account.
type accountState struct {
	hasPassword  bool
	passwordHash string
	sessionEpoch int64
	verified     bool
	name         string
	version      int64
	links        int
}

func readAccountState(t *testing.T, userID string) accountState {
	t.Helper()
	ctx := context.Background()
	var st accountState
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT session_epoch, email_verified_at IS NOT NULL, name, version,
		       (SELECT count(*) FROM account_tokens WHERE user_id = users.id),
		       COALESCE((SELECT password_hash FROM user_credentials WHERE user_id = users.id), '')
		FROM users WHERE id = $1`, userID,
	).Scan(&st.sessionEpoch, &st.verified, &st.name, &st.version, &st.links, &st.passwordHash))
	st.hasPassword = st.passwordHash != ""
	return st
}

// A completion stores the password, ends the sessions and updates the profile
// together, and reports what it stored.
func TestIntegrationAccountToken_CompleteStoresEverything(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	credentials := NewUserCredentialRepository(integrationDB)
	ctx := context.Background()

	t.Run("an emailed setup link sets the password, the name and the verified mark", func(t *testing.T) {
		userID := insertTestUser(t)
		before := readAccountState(t, userID)
		hash := issueAccountToken(t, repo, userID, models.AccountTokenPurposeSetup, models.AccountTokenDeliveryEmail)

		done, err := repo.Complete(ctx, repositories.AccountLinkCompletion{
			TokenHash: hash, Now: accountTokenTestNow, PasswordHash: accountTokenTestHash, Name: "Ada Lovelace",
		})

		require.NoError(t, err)
		after := readAccountState(t, userID)
		assert.Equal(t, accountTokenTestHash, after.passwordHash)
		assert.Equal(t, before.sessionEpoch+1, after.sessionEpoch, "every earlier session is ended")
		assert.True(t, after.verified)
		assert.Equal(t, "Ada Lovelace", after.name)
		assert.Equal(t, before.version+1, after.version, "a stale profile update must not overwrite this")
		assert.Equal(t, 1, after.links, "only the consumed link is left, for the sweep")

		assert.Equal(t, hash, done.Token.TokenHash)
		require.NotNil(t, done.Token.ConsumedAt)
		assert.Equal(t, after.sessionEpoch, done.SessionEpoch)
		assert.Equal(t, "Ada Lovelace", done.Name)
		require.NotNil(t, done.EmailVerifiedAt)
		assert.True(t, done.EmailVerifiedAt.Equal(accountTokenTestNow))
		assert.Equal(t, after.version, done.Version)
		assert.False(t, done.UpdatedAt.IsZero())
	})

	t.Run("a copied link verifies nothing, and no name leaves the name alone", func(t *testing.T) {
		userID := insertTestUser(t)
		hash := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryLink)

		done, err := repo.Complete(ctx, repositories.AccountLinkCompletion{
			TokenHash: hash, Now: accountTokenTestNow, PasswordHash: accountTokenTestHash,
		})

		require.NoError(t, err)
		after := readAccountState(t, userID)
		assert.False(t, after.verified)
		assert.Nil(t, done.EmailVerifiedAt)
		assert.Equal(t, "Integration Test User", after.name)
		assert.Equal(t, after.name, done.Name)
	})

	t.Run("an address already verified keeps its date", func(t *testing.T) {
		userID := insertTestUser(t)
		earlier := accountTokenTestNow.Add(-48 * time.Hour)
		_, err := integrationDB.ExecContext(ctx, `UPDATE users SET email_verified_at = $2 WHERE id = $1`, userID, earlier)
		require.NoError(t, err)
		hash := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail)

		done, err := repo.Complete(ctx, repositories.AccountLinkCompletion{
			TokenHash: hash, Now: accountTokenTestNow, PasswordHash: accountTokenTestHash,
		})

		require.NoError(t, err)
		require.NotNil(t, done.EmailVerifiedAt)
		assert.True(t, done.EmailVerifiedAt.Equal(earlier))
	})

	t.Run("a new password is a fresh start for a locked credential", func(t *testing.T) {
		userID := insertTestUser(t)
		require.NoError(t, credentials.Upsert(ctx, userID, credentialTestHash))
		for range models.DefaultPasswordLockout.Threshold {
			_, err := credentials.BeginAttempt(ctx, userID, time.Now(), models.DefaultPasswordLockout)
			require.NoError(t, err)
		}
		hash := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryLink)

		_, err := completeLink(repo, hash, accountTokenTestNow)

		require.NoError(t, err)
		stored, err := credentials.Get(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, accountTokenTestHash, stored.PasswordHash)
		assert.Zero(t, stored.FailedAttempts)
		assert.Nil(t, stored.LockedUntil)
	})
}

// A completion is all or nothing: when a statement after the consume fails,
// the link is still live and nothing about the account has changed.
func TestIntegrationAccountToken_CompleteIsAllOrNothing(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	ctx := context.Background()
	userID := insertTestUser(t)
	hash := issueAccountToken(t, repo, userID, models.AccountTokenPurposeSetup, models.AccountTokenDeliveryEmail)
	other := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryLink)
	before := readAccountState(t, userID)

	// PostgreSQL refuses a NUL byte in text, so the LAST statement of the
	// transaction (the user update) fails after the link was consumed, the
	// other link deleted and the password stored.
	_, err := repo.Complete(ctx, repositories.AccountLinkCompletion{
		TokenHash: hash, Now: accountTokenTestNow, PasswordHash: accountTokenTestHash, Name: "Ada\x00",
	})

	require.Error(t, err)
	assert.NotErrorIs(t, err, repositories.ErrAccountTokenNotFound, "a failure is not a dead link")
	assert.Equal(t, before, readAccountState(t, userID), "nothing was written")
	_, err = repo.GetLive(ctx, hash, accountTokenTestNow)
	assert.NoError(t, err, "the link is still live")
	_, err = repo.GetLive(ctx, other, accountTokenTestNow)
	assert.NoError(t, err, "and so is the user's other link")

	_, err = completeLink(repo, hash, accountTokenTestNow)
	assert.NoError(t, err, "the same link completes on the next try")
}

func TestIntegrationAccountToken_ClearForClaim(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	credentials := NewUserCredentialRepository(integrationDB)
	ctx := context.Background()
	userID, otherUser := insertTestUser(t), insertTestUser(t)
	require.NoError(t, credentials.Upsert(ctx, userID, credentialTestHash))
	require.NoError(t, credentials.Upsert(ctx, otherUser, credentialTestHash))
	used := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryLink)
	_, err := completeLink(repo, used, accountTokenTestNow)
	require.NoError(t, err)
	issueAccountToken(t, repo, userID, models.AccountTokenPurposeSetup, models.AccountTokenDeliveryEmail)
	others := issueAccountToken(t, repo, otherUser, models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail)
	before := readAccountState(t, userID)
	otherBefore := readAccountState(t, otherUser)

	cleared, err := repo.ClearForClaim(ctx, userID)

	require.NoError(t, err)
	assert.Equal(t, int64(2), cleared.Links, "every link, consumed ones included")
	assert.True(t, cleared.HadPassword)
	assert.Equal(t, before.sessionEpoch+1, cleared.SessionEpoch)
	after := readAccountState(t, userID)
	assert.False(t, after.hasPassword)
	assert.Zero(t, after.links)
	assert.Equal(t, cleared.SessionEpoch, after.sessionEpoch)

	again, err := repo.ClearForClaim(ctx, userID)
	require.NoError(t, err, "clearing a cleared account is not an error")
	assert.Zero(t, again.Links)
	assert.False(t, again.HadPassword)

	assert.Equal(t, otherBefore, readAccountState(t, otherUser), "another account is untouched")
	_, err = repo.GetLive(ctx, others, accountTokenTestNow)
	assert.NoError(t, err)

	_, err = repo.ClearForClaim(ctx, uuid.NewString())
	assert.ErrorIs(t, err, repositories.ErrUserNotFound)
}

// thisDatabaseLocks restricts a pg_locks query to sessions of this test
// database: the integration packages share one PostgreSQL server. It filters
// by session and not by pg_locks.database, which is NULL for the transaction
// locks a row-lock waiter shows.
const thisDatabaseLocks = `pid IN (SELECT pid FROM pg_stat_activity WHERE datname = current_database())`

// waitForLockWaiters blocks until n sessions are waiting for an advisory lock.
func waitForLockWaiters(t *testing.T, n int) {
	t.Helper()
	require.Eventually(t, func() bool {
		var waiting int
		err := integrationDB.QueryRowContext(context.Background(),
			`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted AND `+thisDatabaseLocks,
		).Scan(&waiting)
		return err == nil && waiting >= n
	}, 10*time.Second, 5*time.Millisecond, "%d sessions waiting for the user's lock", n)
}

// rollbackOnCleanup releases whatever a test transaction holds if the test
// ends before committing it, so goroutines queued behind it are not left
// waiting.
func rollbackOnCleanup(t *testing.T, tx *sql.Tx) {
	t.Helper()
	t.Cleanup(func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Logf("failed to roll back a test transaction: %v", err)
		}
	})
}

// The password is written in the SAME transaction as the consume, under the
// user's lock. The test blocks the credential write (it holds the credential
// row) and looks at the completion while it is stuck there: the link must
// still be live to everyone else, because the consume has not committed, and
// the user's lock must still be held. A completion that committed the consume
// first and stored the password afterwards would show a dead link here.
func TestIntegrationAccountToken_PasswordIsStoredInsideTheCompletionTransaction(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	ctx := context.Background()
	userID := insertTestUser(t)
	require.NoError(t, NewUserCredentialRepository(integrationDB).Upsert(ctx, userID, credentialTestHash))
	hash := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryLink)

	holder, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	rollbackOnCleanup(t, holder)
	_, err = holder.ExecContext(ctx, `SELECT 1 FROM user_credentials WHERE user_id = $1 FOR UPDATE`, userID)
	require.NoError(t, err)

	var completeErr error
	var done sync.WaitGroup
	done.Go(func() { _, completeErr = completeLink(repo, hash, accountTokenTestNow) })

	require.Eventually(t, func() bool {
		var waiting int
		scanErr := integrationDB.QueryRowContext(ctx,
			`SELECT count(*) FROM pg_locks WHERE NOT granted AND locktype <> 'advisory' AND `+thisDatabaseLocks,
		).Scan(&waiting)
		return scanErr == nil && waiting >= 1
	}, 10*time.Second, 5*time.Millisecond, "the completion is waiting to write the credential")

	_, err = repo.GetLive(ctx, hash, accountTokenTestNow)
	require.NoError(t, err, "the consume has not committed while the password write is pending")
	var advisoryHeld int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND granted AND `+thisDatabaseLocks,
	).Scan(&advisoryHeld))
	// An advisory lock on a 64-bit key shows as one row.
	require.Equal(t, 1, advisoryHeld, "the user's lock is still held while the password write is pending")

	require.NoError(t, holder.Commit())
	done.Wait()
	require.NoError(t, completeErr)
	assert.Equal(t, accountTokenTestHash, readAccountState(t, userID).passwordHash)
	_, err = repo.GetLive(ctx, hash, accountTokenTestNow)
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound, "and only now is the link used up")
}

// A completion and a claim of one account at the same moment (#1462), in each
// order. The order is forced, not left to chance: the test holds the user's
// lock, lets the first operation queue on it, then the second, and releases;
// PostgreSQL grants a lock to its waiters in the order they arrived.
//
// Whichever goes first, the claimed account never ends up with a password:
// either the claim removes the password the completion stored, or the
// completion finds its link gone and stores nothing. This catches a claim that
// does not take the lock and a password stored before the lock is taken. It
// does NOT by itself catch a password stored just after the commit (that write
// is one statement and wins the race against the claim's three);
// TestIntegrationAccountToken_PasswordIsStoredInsideTheCompletionTransaction
// is the test for that.
func TestIntegrationAccountToken_CompletionAndClaimNeverInterleave(t *testing.T) {
	repo := NewAccountTokenRepository(integrationDB)
	ctx := context.Background()

	for _, order := range []string{"completion first", "claim first"} {
		t.Run(order, func(t *testing.T) {
			for round := range 20 {
				resetIntegrationTables(t)
				userID := insertTestUser(t)
				hash := issueAccountToken(t, repo, userID,
					models.AccountTokenPurposeSetup, models.AccountTokenDeliveryLink)
				before := readAccountState(t, userID)

				holder, err := integrationDB.BeginTx(ctx, nil)
				require.NoError(t, err)
				rollbackOnCleanup(t, holder)
				require.NoError(t, lockUserSignInMethods(ctx, holder, userID))

				var completeErr, claimErr error
				var cleared *repositories.ClearedAccountClaim
				var done sync.WaitGroup
				complete := func() {
					defer done.Done()
					_, completeErr = completeLink(repo, hash, accountTokenTestNow)
				}
				claim := func() {
					defer done.Done()
					cleared, claimErr = repo.ClearForClaim(ctx, userID)
				}
				first, second := complete, claim
				if order == "claim first" {
					first, second = claim, complete
				}
				done.Add(2)
				go first()
				waitForLockWaiters(t, 1)
				go second()
				waitForLockWaiters(t, 2)
				require.NoError(t, holder.Commit())
				done.Wait()

				require.NoError(t, claimErr, "round %d", round)
				after := readAccountState(t, userID)
				require.False(t, after.hasPassword, "round %d: a claimed account has no password", round)
				require.Zero(t, after.links, "round %d: nor any link", round)
				if order == "completion first" {
					require.NoError(t, completeErr, "round %d", round)
					require.True(t, cleared.HadPassword, "round %d: the claim removed the completed password", round)
					require.Equal(t, int64(1), cleared.Links, "round %d: and the consumed link", round)
					require.Equal(t, before.sessionEpoch+2, after.sessionEpoch, "round %d", round)
				} else {
					require.ErrorIs(t, completeErr, repositories.ErrAccountTokenNotFound, "round %d", round)
					require.False(t, cleared.HadPassword, "round %d", round)
					require.Equal(t, before.sessionEpoch+1, after.sessionEpoch, "round %d: only the claim wrote", round)
					require.Equal(t, before.version, after.version, "round %d: the completion wrote nothing", round)
				}
			}
		})
	}
}

// More simultaneous callers for one user than the pool has connections. Each
// waiter holds one connection and the lock holder needs no second one, so they
// all finish and the pool stays usable. A lock held on one connection while the
// work ran on others would starve the pool. This runs on a pool with the
// production cap; on the uncapped test pool it could never fail.
func TestIntegrationAccountToken_LockedOperationsDoNotStarveThePool(t *testing.T) {
	resetIntegrationTables(t)
	capped := openCappedDB(t)
	repo := NewAccountTokenRepository(capped)
	userID := insertTestUser(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	const callers = 4 * productionMaxOpenConns
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			var err error
			switch i % 3 {
			case 0:
				err = repo.Replace(ctx, &models.AccountToken{
					UserID: userID, Purpose: models.AccountTokenPurposeReset, Delivery: models.AccountTokenDeliveryLink,
					TokenHash: accountTokenHash(uuid.NewString()), ExpiresAt: accountTokenTestNow.Add(time.Hour),
				}, 0)
			case 1:
				_, err = repo.ClearForClaim(ctx, userID)
			default:
				hash := accountTokenHash(uuid.NewString())
				if err = repo.Replace(ctx, &models.AccountToken{
					UserID: userID, Purpose: models.AccountTokenPurposeSetup, Delivery: models.AccountTokenDeliveryEmail,
					TokenHash: hash, ExpiresAt: accountTokenTestNow.Add(time.Hour),
				}, 0); err == nil {
					_, err = completeLink(repo, hash, accountTokenTestNow)
				}
			}
			if err != nil && !errors.Is(err, repositories.ErrAccountTokenNotFound) {
				t.Errorf("caller %d: %v", i, err)
			}
		})
	}
	// An unrelated query on the same pool, while the callers are in flight.
	var one int
	bystander, bystanderCancel := context.WithTimeout(ctx, 10*time.Second)
	defer bystanderCancel()
	require.NoError(t, capped.QueryRowContext(bystander, "SELECT 1").Scan(&one), "the pool still answers")
	wg.Wait()

	require.NoError(t, ctx.Err(), "every caller finished well inside the timeout")
}

// Deleting a user while one of their links is being completed, issued or
// cleared. The delete locks the user row and cascades into the links; these
// operations lock the user row first too, so the two queue instead of
// deadlocking. Before that row lock was taken, about one round in five ended
// in "deadlock detected".
func TestIntegrationAccountToken_UserDeleteDoesNotDeadlock(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	ctx := context.Background()

	for round := range 150 {
		userID := insertTestUser(t)
		hash := issueAccountToken(t, repo, userID, models.AccountTokenPurposeSetup, models.AccountTokenDeliveryLink)

		var opErr, deleteErr error
		var start, done sync.WaitGroup
		start.Add(1)
		done.Add(2)
		go func() {
			defer done.Done()
			start.Wait()
			switch round % 3 {
			case 0:
				_, opErr = completeLink(repo, hash, accountTokenTestNow)
			case 1:
				_, opErr = repo.ClearForClaim(ctx, userID)
			default:
				opErr = repo.Replace(ctx, &models.AccountToken{
					UserID: userID, Purpose: models.AccountTokenPurposeSetup, Delivery: models.AccountTokenDeliveryLink,
					TokenHash: accountTokenHash(uuid.NewString()), ExpiresAt: accountTokenTestNow.Add(time.Hour),
				}, 0)
			}
		}()
		go func() {
			defer done.Done()
			start.Wait()
			_, deleteErr = integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
		}()
		start.Done()
		done.Wait()

		require.NoError(t, deleteErr, "round %d: the delete is never the deadlock's victim", round)
		if opErr != nil {
			// The user was deleted first: gone, never a deadlock.
			gone := errors.Is(opErr, repositories.ErrAccountTokenNotFound) || errors.Is(opErr, repositories.ErrUserNotFound)
			require.True(t, gone, "round %d: %v", round, opErr)
		}
	}
}

func TestIntegrationAccountToken_DeleteDead(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	ctx := context.Background()
	userID, otherUser := insertTestUser(t), insertTestUser(t)
	live := accountTokenHash("live")
	require.NoError(t, repo.Replace(ctx, &models.AccountToken{
		UserID: userID, Purpose: models.AccountTokenPurposeSetup, Delivery: models.AccountTokenDeliveryLink,
		TokenHash: live, ExpiresAt: accountTokenTestNow.Add(7 * 24 * time.Hour),
	}, 0))
	issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail) // expires in 1h
	consumed := issueAccountToken(t, repo, otherUser, models.AccountTokenPurposeSetup, models.AccountTokenDeliveryLink)
	_, err := completeLink(repo, consumed, accountTokenTestNow)
	require.NoError(t, err)

	removed, err := repo.DeleteDead(ctx, accountTokenTestNow)
	require.NoError(t, err)
	assert.Equal(t, int64(1), removed, "only the consumed link is dead now")

	removed, err = repo.DeleteDead(ctx, accountTokenTestNow.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(1), removed, "the reset link is dead at the instant it expires")

	_, err = repo.GetLive(ctx, live, accountTokenTestNow.Add(time.Hour))
	assert.NoError(t, err, "a live link survives the sweep")
	removed, err = repo.DeleteDead(ctx, accountTokenTestNow.Add(time.Hour))
	require.NoError(t, err)
	assert.Zero(t, removed)
}

// The constraints of migration 031.
func TestIntegrationAccountToken_Constraints(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	ctx := context.Background()
	userID, adminID := insertTestUser(t), insertTestUser(t)
	insert := func(purpose, delivery string, hash []byte) error {
		_, err := integrationDB.ExecContext(ctx, `
			INSERT INTO account_tokens (user_id, purpose, delivery, token_hash, expires_at)
			VALUES ($1, $2, $3, $4, $5)`, userID, purpose, delivery, hash, accountTokenTestNow)
		return err
	}

	assert.Error(t, insert("verify", models.AccountTokenDeliveryLink, accountTokenHash("a")), "unknown purpose")
	assert.Error(t, insert(models.AccountTokenPurposeSetup, "sms", accountTokenHash("b")), "unknown delivery")
	require.NoError(t, insert(models.AccountTokenPurposeSetup, models.AccountTokenDeliveryLink, accountTokenHash("c")))
	assert.Error(t, insert(models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail, accountTokenHash("c")),
		"a token hash is unique")
	assert.Error(t, insert(models.AccountTokenPurposeSetup, models.AccountTokenDeliveryLink, accountTokenHash("d")),
		"one live link per user, purpose and delivery")

	err := repo.Replace(ctx, &models.AccountToken{
		UserID: uuid.NewString(), Purpose: models.AccountTokenPurposeSetup, Delivery: models.AccountTokenDeliveryLink,
		TokenHash: accountTokenHash("e"), ExpiresAt: accountTokenTestNow,
	}, 0)
	assert.ErrorIs(t, err, repositories.ErrUserNotFound, "a link needs a user")

	// Deleting the issuing administrator keeps the link; deleting the user
	// removes it.
	hash := accountTokenHash("f")
	require.NoError(t, repo.Replace(ctx, &models.AccountToken{
		UserID: userID, Purpose: models.AccountTokenPurposeReset, Delivery: models.AccountTokenDeliveryLink,
		TokenHash: hash, ExpiresAt: accountTokenTestNow.Add(time.Hour), CreatedBy: &adminID,
	}, 0))
	_, err = integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, adminID)
	require.NoError(t, err)
	got, err := repo.GetLive(ctx, hash, accountTokenTestNow)
	require.NoError(t, err)
	assert.Nil(t, got.CreatedBy)
	_, err = integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	require.NoError(t, err)
	assert.Equal(t, 0, countAccountTokens(t, "true"))
}

// The spacing of emailed links is decided where the link is stored: Replace
// with an interval refuses while the live link of that kind is younger than
// it, and leaves that link in place.
func TestIntegrationAccountToken_ReplaceRefusesWhileTheLiveLinkIsTooRecent(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	ctx := context.Background()
	userID, otherUser := insertTestUser(t), insertTestUser(t)
	link := func(user, purpose, delivery string) *models.AccountToken {
		return &models.AccountToken{
			UserID: user, Purpose: purpose, Delivery: delivery,
			TokenHash: accountTokenHash(uuid.NewString()), ExpiresAt: accountTokenTestNow.Add(time.Hour),
		}
	}
	reset, email := models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail
	first := link(userID, reset, email)
	require.NoError(t, repo.Replace(ctx, first, 5*time.Minute), "nothing to be too recent yet")

	err := repo.Replace(ctx, link(userID, reset, email), 5*time.Minute)
	require.ErrorIs(t, err, repositories.ErrAccountTokenTooRecent)
	_, err = repo.GetLive(ctx, first.TokenHash, accountTokenTestNow)
	require.NoError(t, err, "the earlier link is still the live one")
	assert.Equal(t, 1, countAccountTokens(t, "user_id = $1", userID))

	// Other kinds, other users and an unconditional replace are not held back.
	require.NoError(t, repo.Replace(ctx, link(userID, reset, models.AccountTokenDeliveryLink), 5*time.Minute))
	require.NoError(t, repo.Replace(ctx, link(userID, models.AccountTokenPurposeSetup, email), 5*time.Minute))
	require.NoError(t, repo.Replace(ctx, link(otherUser, reset, email), 5*time.Minute))
	require.NoError(t, repo.Replace(ctx, link(userID, reset, email), 0), "no interval replaces at once")

	// Once the live link is older than the interval, it is replaced.
	_, err = integrationDB.ExecContext(ctx, `
		UPDATE account_tokens SET created_at = CURRENT_TIMESTAMP - interval '6 minutes'
		WHERE user_id = $1 AND purpose = $2 AND delivery = $3 AND consumed_at IS NULL`, userID, reset, email)
	require.NoError(t, err)
	require.NoError(t, repo.Replace(ctx, link(userID, reset, email), 5*time.Minute))

	// A used link does not hold the next one back.
	used := link(otherUser, models.AccountTokenPurposeSetup, email)
	require.NoError(t, repo.Replace(ctx, used, 5*time.Minute))
	_, err = completeLink(repo, used.TokenHash, accountTokenTestNow)
	require.NoError(t, err)
	require.NoError(t, repo.Replace(ctx, link(otherUser, models.AccountTokenPurposeSetup, email), 5*time.Minute))
}

// Simultaneous requests cannot each pass the check: one link is stored and
// every other request is refused, so one email goes out.
func TestIntegrationAccountToken_ConcurrentReplaceWithAnIntervalStoresOne(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	userID := insertTestUser(t)

	const callers = 16
	var stored, refused atomic.Int32
	var start, done sync.WaitGroup
	start.Add(1)
	for range callers {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			err := repo.Replace(context.Background(), &models.AccountToken{
				UserID: userID, Purpose: models.AccountTokenPurposeReset, Delivery: models.AccountTokenDeliveryEmail,
				TokenHash: accountTokenHash(uuid.NewString()), ExpiresAt: accountTokenTestNow.Add(time.Hour),
			}, 5*time.Minute)
			switch {
			case err == nil:
				stored.Add(1)
			case assert.ErrorIs(t, err, repositories.ErrAccountTokenTooRecent):
				refused.Add(1)
			}
		}()
	}
	start.Done()
	done.Wait()

	assert.EqualValues(t, 1, stored.Load())
	assert.EqualValues(t, callers-1, refused.Load())
	assert.Equal(t, 1, countAccountTokens(t, "user_id = $1", userID))
}
