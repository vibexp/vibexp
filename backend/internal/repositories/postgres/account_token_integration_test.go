//go:build integration

package postgres

import (
	"context"
	"crypto/sha256"
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
	}))
	return hash
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

	require.NoError(t, repo.Replace(ctx, token))
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

	consumed, err := repo.Consume(ctx, hash, accountTokenTestNow)
	require.NoError(t, err)
	require.NotNil(t, consumed.ConsumedAt)
	assert.True(t, consumed.ConsumedAt.Equal(accountTokenTestNow))
	assert.Equal(t, token.ID, consumed.ID)

	_, err = repo.GetLive(ctx, hash, accountTokenTestNow)
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound, "a consumed link is not live")
	_, err = repo.Consume(ctx, hash, accountTokenTestNow)
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound, "a link is consumed once")
}

func TestIntegrationAccountToken_ConsumeRefusesDeadAndUnknownLinks(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	ctx := context.Background()
	userID := insertTestUser(t)
	hash := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail)

	_, err := repo.Consume(ctx, accountTokenHash("unknown"), accountTokenTestNow)
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound)
	_, err = repo.Consume(ctx, hash, accountTokenTestNow.Add(time.Hour))
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
func TestIntegrationAccountToken_ConsumeEndsEveryOtherLinkOfTheUser(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	ctx := context.Background()
	userID, otherUser := insertTestUser(t), insertTestUser(t)
	resetEmail := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail)
	resetLink := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryLink)
	setupLink := issueAccountToken(t, repo, userID, models.AccountTokenPurposeSetup, models.AccountTokenDeliveryLink)
	others := issueAccountToken(t, repo, otherUser, models.AccountTokenPurposeReset, models.AccountTokenDeliveryLink)

	consumed, err := repo.Consume(ctx, resetLink, accountTokenTestNow)
	require.NoError(t, err)
	assert.Equal(t, models.AccountTokenDeliveryLink, consumed.Delivery)

	_, err = repo.GetLive(ctx, resetEmail, accountTokenTestNow)
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound, "the emailed reset link ended with the copied one")
	_, err = repo.Consume(ctx, resetEmail, accountTokenTestNow)
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound)
	_, err = repo.GetLive(ctx, setupLink, accountTokenTestNow)
	assert.ErrorIs(t, err, repositories.ErrAccountTokenNotFound, "a setup link does not outlive a completed reset")
	_, err = repo.Consume(ctx, setupLink, accountTokenTestNow)
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
func TestIntegrationAccountToken_ConcurrentConsumeOfOneLinkHasOneWinner(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewAccountTokenRepository(integrationDB)
	ctx := context.Background()
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
			_, err := repo.Consume(ctx, hash, accountTokenTestNow)
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
func TestIntegrationAccountToken_ConcurrentConsumeOfBothLinksHasOneWinner(t *testing.T) {
	repo := NewAccountTokenRepository(integrationDB)
	ctx := context.Background()

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
				_, err := repo.Consume(ctx, hashes[i%2], accountTokenTestNow)
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
			}))
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

	// Every link of the user, consumed ones included.
	consumedHash := issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryLink)
	_, err = repo.Consume(ctx, consumedHash, accountTokenTestNow)
	require.NoError(t, err)
	issueAccountToken(t, repo, userID, models.AccountTokenPurposeSetup, models.AccountTokenDeliveryEmail)
	removed, err := repo.DeleteForUser(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), removed)
	removed, err = repo.DeleteForUser(ctx, userID)
	require.NoError(t, err)
	assert.Zero(t, removed)
	_, err = repo.GetLive(ctx, others, accountTokenTestNow)
	assert.NoError(t, err, "another user's link is untouched")
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
	}))
	issueAccountToken(t, repo, userID, models.AccountTokenPurposeReset, models.AccountTokenDeliveryEmail) // expires in 1h
	consumed := issueAccountToken(t, repo, otherUser, models.AccountTokenPurposeSetup, models.AccountTokenDeliveryLink)
	_, err := repo.Consume(ctx, consumed, accountTokenTestNow)
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
	})
	assert.ErrorIs(t, err, repositories.ErrUserNotFound, "a link needs a user")

	// Deleting the issuing administrator keeps the link; deleting the user
	// removes it.
	hash := accountTokenHash("f")
	require.NoError(t, repo.Replace(ctx, &models.AccountToken{
		UserID: userID, Purpose: models.AccountTokenPurposeReset, Delivery: models.AccountTokenDeliveryLink,
		TokenHash: hash, ExpiresAt: accountTokenTestNow.Add(time.Hour), CreatedBy: &adminID,
	}))
	_, err = integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, adminID)
	require.NoError(t, err)
	got, err := repo.GetLive(ctx, hash, accountTokenTestNow)
	require.NoError(t, err)
	assert.Nil(t, got.CreatedBy)
	_, err = integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	require.NoError(t, err)
	assert.Equal(t, 0, countAccountTokens(t, "true"))
}
