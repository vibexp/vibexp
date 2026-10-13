package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/password"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repo_mocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

const (
	linkTestBaseURL = "https://vibexp.example.com/"
	linkTestAdminID = "admin-1"
	linkTestNewPass = "a long enough new passphrase"
)

var linkTestNow = time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)

// memAccountTokens is an in-memory AccountTokenRepository with the semantics of
// the PostgreSQL one: one live link per user, purpose and delivery, and a
// consume that ends every other live link of the same user.
type memAccountTokens struct {
	mu     sync.Mutex
	rows   []*models.AccountToken
	events *[]string
	// failing, when set, is returned by every method named in failOn.
	failing error
	failOn  map[string]bool
	// afterConsume, when set, runs once a link has been consumed.
	afterConsume func()
}

func (m *memAccountTokens) fail(method string) error {
	if m.failing != nil && m.failOn[method] {
		return m.failing
	}
	return nil
}

func (m *memAccountTokens) Replace(_ context.Context, token *models.AccountToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.fail("Replace"); err != nil {
		return err
	}
	kept := m.rows[:0]
	for _, row := range m.rows {
		superseded := row.ConsumedAt == nil && row.UserID == token.UserID &&
			row.Purpose == token.Purpose && row.Delivery == token.Delivery
		if !superseded {
			kept = append(kept, row)
		}
	}
	token.ID = fmt.Sprintf("token-%d", len(m.rows)+1)
	stored := *token
	kept = append(kept, &stored)
	m.rows = kept
	return nil
}

func (m *memAccountTokens) GetLive(_ context.Context, hash []byte, now time.Time) (*models.AccountToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.fail("GetLive"); err != nil {
		return nil, err
	}
	for _, row := range m.rows {
		if bytes.Equal(row.TokenHash, hash) && row.IsLiveAt(now) {
			found := *row
			return &found, nil
		}
	}
	return nil, repositories.ErrAccountTokenNotFound
}

func (m *memAccountTokens) Consume(_ context.Context, hash []byte, now time.Time) (*models.AccountToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.fail("Consume"); err != nil {
		return nil, err
	}
	var won *models.AccountToken
	for _, row := range m.rows {
		if bytes.Equal(row.TokenHash, hash) && row.IsLiveAt(now) {
			won = row
		}
	}
	if won == nil {
		return nil, repositories.ErrAccountTokenNotFound
	}
	won.ConsumedAt = &now
	kept := m.rows[:0]
	for _, row := range m.rows {
		other := row != won && row.ConsumedAt == nil && row.UserID == won.UserID
		if !other {
			kept = append(kept, row)
		}
	}
	m.rows = kept
	if m.events != nil {
		*m.events = append(*m.events, "link consumed")
	}
	if m.afterConsume != nil {
		m.afterConsume()
	}
	consumed := *won
	return &consumed, nil
}

func (m *memAccountTokens) Delete(_ context.Context, hash []byte) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.fail("Delete"); err != nil {
		return false, err
	}
	for i, row := range m.rows {
		if bytes.Equal(row.TokenHash, hash) {
			m.rows = append(m.rows[:i], m.rows[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (m *memAccountTokens) DeleteForUser(_ context.Context, userID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var removed int64
	kept := m.rows[:0]
	for _, row := range m.rows {
		if row.UserID == userID {
			removed++
			continue
		}
		kept = append(kept, row)
	}
	m.rows = kept
	return removed, nil
}

func (m *memAccountTokens) DeleteDead(_ context.Context, now time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.fail("DeleteDead"); err != nil {
		return 0, err
	}
	var removed int64
	kept := m.rows[:0]
	for _, row := range m.rows {
		if !row.IsLiveAt(now) {
			removed++
			continue
		}
		kept = append(kept, row)
	}
	m.rows = kept
	return removed, nil
}

// all returns a copy of every stored row.
func (m *memAccountTokens) all() []models.AccountToken {
	m.mu.Lock()
	defer m.mu.Unlock()
	rows := make([]models.AccountToken, 0, len(m.rows))
	for _, row := range m.rows {
		rows = append(rows, *row)
	}
	return rows
}

var _ repositories.AccountTokenRepository = (*memAccountTokens)(nil)

// linkPasswords is a PasswordAuthServiceInterface that records the passwords
// it was asked to set. (The generated mock lives in a package that imports this
// one.) err, when set, fails SetPassword.
type linkPasswords struct {
	PasswordAuthServiceInterface
	mu     sync.Mutex
	set    []string
	events *[]string
	err    error
}

func (p *linkPasswords) SetPassword(ctx context.Context, userID, plaintext string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Like the real service, which gives up hashing when its context ends.
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.err != nil {
		return p.err
	}
	p.set = append(p.set, userID+":"+plaintext)
	if p.events != nil {
		*p.events = append(*p.events, "password set")
	}
	return nil
}

func (p *linkPasswords) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.set)
}

// accountLinkFixture is an AccountTokenService over an in-memory link store, mocks of
// the user store and the password service, a settable clock and a recording
// logger. events records the order of the writes a completion performs.
type accountLinkFixture struct {
	svc       *AccountTokenService
	tokens    *memAccountTokens
	users     *repo_mocks.MockUserRepository
	passwords *linkPasswords
	apiAccess *recordingAPIAccess
	logs      *logtest.Recorder
	user      *models.User
	now       time.Time
	events    []string
	// saved is the user as last written by Update.
	saved *models.User
}

func newAccountLinkFixture(t *testing.T, allowedEmails ...string) *accountLinkFixture {
	t.Helper()
	logger, logs := logtest.New()
	f := &accountLinkFixture{
		users: repo_mocks.NewMockUserRepository(t),
		logs:  logs,
		user:  &models.User{ID: pwTestUserID, Email: pwTestEmail, Name: "Ada", Status: models.UserStatusActive},
		now:   linkTestNow,
	}
	f.tokens = &memAccountTokens{events: &f.events}
	f.passwords = &linkPasswords{events: &f.events}
	f.apiAccess = &recordingAPIAccess{events: &f.events}
	f.users.On("GetByID", mock.Anything, pwTestUserID).
		Return(func(context.Context, string) (*models.User, error) {
			if f.user == nil {
				return nil, repositories.ErrUserNotFound
			}
			u := *f.user
			return &u, nil
		}).Maybe()
	f.svc = NewAccountTokenService(AccountTokenDeps{
		Tokens:    f.tokens,
		Users:     f.users,
		Passwords: f.passwords,
		APIAccess: f.apiAccess,
		Allowlist: newStaticAllowlistResolver(logger, allowedEmails),
		BaseURL:   linkTestBaseURL,
		Logger:    logger,
		Now:       func() time.Time { return f.now },
	})
	return f
}

// expectCompletion sets up the writes of a completion that goes through.
func (f *accountLinkFixture) expectCompletion() {
	f.users.On("BumpSessionEpoch", mock.Anything, pwTestUserID).
		Run(func(mock.Arguments) { f.events = append(f.events, "sessions ended") }).Return(int64(7), nil).Once()
	f.users.On("Update", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			u := *args.Get(1).(*models.User)
			f.saved = &u
			f.events = append(f.events, "account updated")
		}).Return(nil).Maybe()
}

// tokenIn returns the token a link carries.
func tokenIn(t *testing.T, link string) string {
	t.Helper()
	_, token, found := strings.Cut(link, "#token=")
	require.True(t, found, "the token is the link's fragment: %s", link)
	return token
}

// copied issues a link an administrator copies and returns its token.
func (f *accountLinkFixture) copied(t *testing.T, purpose string) string {
	t.Helper()
	link, err := f.svc.IssueLink(context.Background(),
		IssueAccountLinkParams{UserID: pwTestUserID, Purpose: purpose, CreatedBy: linkTestAdminID})
	require.NoError(t, err)
	return tokenIn(t, link.URL)
}

// emailed issues a link by email and returns the token the mailer was handed.
func (f *accountLinkFixture) emailed(t *testing.T, purpose string) string {
	t.Helper()
	var token string
	_, err := f.svc.IssueByEmail(context.Background(), IssueAccountLinkParams{UserID: pwTestUserID, Purpose: purpose},
		func(_ context.Context, email AccountLinkEmail) error {
			token = tokenIn(t, email.URL)
			return nil
		})
	require.NoError(t, err)
	return token
}

func TestAccountTokenService_IssueLink(t *testing.T) {
	ctx := context.Background()

	t.Run("a setup link lasts 7 days and only its hash is stored", func(t *testing.T) {
		f := newAccountLinkFixture(t)

		link, err := f.svc.IssueLink(ctx, IssueAccountLinkParams{
			UserID: pwTestUserID, Purpose: models.AccountTokenPurposeSetup, CreatedBy: linkTestAdminID,
		})

		require.NoError(t, err)
		assert.Equal(t, linkTestNow.Add(7*24*time.Hour), link.ExpiresAt)
		assert.True(t, strings.HasPrefix(link.URL, "https://vibexp.example.com/account/setup#token="), link.URL)
		assert.NotContains(t, link.URL, "?", "the token is never a query parameter")

		token := tokenIn(t, link.URL)
		raw, err := base64.RawURLEncoding.DecodeString(token)
		require.NoError(t, err, "unpadded URL-safe base64, so nothing in it needs encoding")
		assert.Len(t, raw, 32)

		rows := f.tokens.all()
		require.Len(t, rows, 1)
		assert.Equal(t, hashAccountToken(token), rows[0].TokenHash)
		assert.NotContains(t, string(rows[0].TokenHash), token)
		assert.Equal(t, models.AccountTokenDeliveryLink, rows[0].Delivery)
		require.NotNil(t, rows[0].CreatedBy)
		assert.Equal(t, linkTestAdminID, *rows[0].CreatedBy)
	})

	t.Run("a copied reset link lasts 24 hours", func(t *testing.T) {
		f := newAccountLinkFixture(t)

		link, err := f.svc.IssueLink(ctx,
			IssueAccountLinkParams{UserID: pwTestUserID, Purpose: models.AccountTokenPurposeReset})

		require.NoError(t, err)
		assert.Equal(t, linkTestNow.Add(24*time.Hour), link.ExpiresAt)
		assert.True(t, strings.HasPrefix(link.URL, "https://vibexp.example.com/account/reset#token="), link.URL)
		assert.Nil(t, f.tokens.all()[0].CreatedBy, "no actor: issued by the system")
	})

	t.Run("a new link replaces the previous one of the same kind", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		first := f.copied(t, models.AccountTokenPurposeReset)
		second := f.copied(t, models.AccountTokenPurposeReset)

		_, err := f.svc.Preview(ctx, first)
		require.ErrorIs(t, err, ErrAccountTokenInvalid)
		_, err = f.svc.Preview(ctx, second)
		require.NoError(t, err)
	})

	t.Run("every token is different", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		seen := map[string]bool{}
		for range 50 {
			seen[f.copied(t, models.AccountTokenPurposeSetup)] = true
		}
		assert.Len(t, seen, 50)
	})

	for name, params := range map[string]IssueAccountLinkParams{
		"no user":         {UserID: " ", Purpose: models.AccountTokenPurposeSetup},
		"unknown purpose": {UserID: pwTestUserID, Purpose: "verify"},
	} {
		t.Run("refuses a request with "+name, func(t *testing.T) {
			f := newAccountLinkFixture(t)

			_, err := f.svc.IssueLink(ctx, params)

			require.ErrorIs(t, err, ErrInvalidAccountTokenRequest)
			assert.Empty(t, f.tokens.all())
		})
	}

	t.Run("a store failure is returned", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		boom := errors.New("connection refused")
		f.tokens.failing, f.tokens.failOn = boom, map[string]bool{"Replace": true}

		_, err := f.svc.IssueLink(ctx,
			IssueAccountLinkParams{UserID: pwTestUserID, Purpose: models.AccountTokenPurposeSetup})

		require.ErrorIs(t, err, boom)
	})
}

func TestAccountTokenService_IssueByEmail(t *testing.T) {
	ctx := context.Background()
	reset := IssueAccountLinkParams{UserID: pwTestUserID, Purpose: models.AccountTokenPurposeReset}

	t.Run("hands the link to the mailer only, and an emailed reset lasts 1 hour", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		var sent AccountLinkEmail

		expiresAt, err := f.svc.IssueByEmail(ctx, reset, func(_ context.Context, email AccountLinkEmail) error {
			sent = email
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, linkTestNow.Add(time.Hour), expiresAt)
		assert.Equal(t, expiresAt, sent.ExpiresAt)
		assert.Equal(t, models.AccountTokenPurposeReset, sent.Purpose)
		assert.Equal(t, pwTestEmail, sent.User.Email)
		rows := f.tokens.all()
		require.Len(t, rows, 1)
		assert.Equal(t, models.AccountTokenDeliveryEmail, rows[0].Delivery)
		assert.Equal(t, hashAccountToken(tokenIn(t, sent.URL)), rows[0].TokenHash)
	})

	t.Run("an emailed setup link lasts 7 days", func(t *testing.T) {
		f := newAccountLinkFixture(t)

		expiresAt, err := f.svc.IssueByEmail(ctx,
			IssueAccountLinkParams{UserID: pwTestUserID, Purpose: models.AccountTokenPurposeSetup},
			func(context.Context, AccountLinkEmail) error { return nil })

		require.NoError(t, err)
		assert.Equal(t, linkTestNow.Add(7*24*time.Hour), expiresAt)
	})

	t.Run("a link that could not be sent is withdrawn", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		boom := errors.New("smtp: connection refused")
		var token string

		_, err := f.svc.IssueByEmail(ctx, reset, func(_ context.Context, email AccountLinkEmail) error {
			token = tokenIn(t, email.URL)
			return boom
		})

		require.ErrorIs(t, err, boom)
		assert.Empty(t, f.tokens.all())
		_, err = f.svc.Preview(ctx, token)
		require.ErrorIs(t, err, ErrAccountTokenInvalid)
	})

	t.Run("a link that could be neither sent nor withdrawn is reported", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		boom := errors.New("smtp: connection refused")
		f.tokens.failing, f.tokens.failOn = errors.New("connection reset"), map[string]bool{"Delete": true}

		_, err := f.svc.IssueByEmail(ctx, reset, func(context.Context, AccountLinkEmail) error { return boom })

		require.ErrorIs(t, err, boom)
		require.NotNil(t, f.logs.LastEntry())
		assert.Contains(t, f.logs.LastEntry().Message, "Failed to withdraw")
	})

	t.Run("an emailed and a copied link of one purpose are both live", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		emailed := f.emailed(t, models.AccountTokenPurposeReset)
		copied := f.copied(t, models.AccountTokenPurposeReset)

		_, err := f.svc.Preview(ctx, emailed)
		require.NoError(t, err, "issuing a copied link leaves the emailed one live")
		_, err = f.svc.Preview(ctx, copied)
		require.NoError(t, err)

		again := f.emailed(t, models.AccountTokenPurposeReset)
		_, err = f.svc.Preview(ctx, emailed)
		require.ErrorIs(t, err, ErrAccountTokenInvalid, "a second emailed link replaces the first")
		_, err = f.svc.Preview(ctx, copied)
		require.NoError(t, err, "and leaves the copied one live")
		_, err = f.svc.Preview(ctx, again)
		require.NoError(t, err)
	})

	t.Run("refuses a missing sender, and an unknown account", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		_, err := f.svc.IssueByEmail(ctx, reset, nil)
		require.ErrorIs(t, err, ErrInvalidAccountTokenRequest)

		f.user = nil
		_, err = f.svc.IssueByEmail(ctx, reset, func(context.Context, AccountLinkEmail) error {
			t.Fatal("nothing is sent for an account that does not exist")
			return nil
		})
		require.ErrorIs(t, err, repositories.ErrUserNotFound)
		assert.Empty(t, f.tokens.all())
	})
}

func TestAccountTokenService_Preview(t *testing.T) {
	ctx := context.Background()

	t.Run("says what the link does and whose account it is, and changes nothing", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeSetup)

		for range 3 {
			preview, err := f.svc.Preview(ctx, token)
			require.NoError(t, err)
			assert.Equal(t, AccountTokenPreview{
				Purpose: models.AccountTokenPurposeSetup, Email: pwTestEmail, Name: "Ada",
				ExpiresAt: linkTestNow.Add(7 * 24 * time.Hour),
			}, *preview)
		}
		assert.Nil(t, f.tokens.all()[0].ConsumedAt)
	})

	t.Run("every unusable link is the same error", func(t *testing.T) {
		cases := map[string]func(f *accountLinkFixture, token string) string{
			"empty":     func(*accountLinkFixture, string) string { return "" },
			"unknown":   func(*accountLinkFixture, string) string { return "bm90LWEtcmVhbC10b2tlbg" },
			"oversized": func(*accountLinkFixture, string) string { return strings.Repeat("a", accountTokenMaxLength+1) },
			"expired": func(f *accountLinkFixture, token string) string {
				f.now = f.now.Add(7*24*time.Hour + time.Second)
				return token
			},
			"expiring this instant": func(f *accountLinkFixture, token string) string { f.now = f.now.Add(7 * 24 * time.Hour); return token },
			"account gone":          func(f *accountLinkFixture, token string) string { f.user = nil; return token },
			"account suspended": func(f *accountLinkFixture, token string) string {
				f.user.Status = models.UserStatusSuspended
				return token
			},
			"token with a changed character": func(_ *accountLinkFixture, token string) string {
				return token[:len(token)-1] + map[bool]string{true: "A", false: "B"}[token[len(token)-1] != 'A']
			},
		}
		for name, arrange := range cases {
			t.Run(name, func(t *testing.T) {
				f := newAccountLinkFixture(t)
				token := arrange(f, f.copied(t, models.AccountTokenPurposeSetup))

				preview, err := f.svc.Preview(ctx, token)

				assert.Nil(t, preview)
				require.Equal(t, ErrAccountTokenInvalid, err, "the bare error, with nothing wrapped around it")
			})
		}
	})

	t.Run("an address the allowlist does not admit is refused, as completing would be", func(t *testing.T) {
		f := newAccountLinkFixture(t, "someone.else@example.com")
		token := f.copied(t, models.AccountTokenPurposeSetup)

		preview, err := f.svc.Preview(ctx, token)

		assert.Nil(t, preview)
		require.ErrorIs(t, err, ErrAccessRestricted)
	})

	t.Run("a link is live until the instant before it expires", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeSetup)
		f.now = f.now.Add(7*24*time.Hour - time.Nanosecond)

		_, err := f.svc.Preview(ctx, token)
		require.NoError(t, err)
	})

	t.Run("a store failure is not reported as a bad link", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeSetup)
		boom := errors.New("connection refused")
		f.tokens.failing, f.tokens.failOn = boom, map[string]bool{"GetLive": true}

		_, err := f.svc.Preview(ctx, token)

		require.ErrorIs(t, err, boom)
		require.NotErrorIs(t, err, ErrAccountTokenInvalid)
	})
}

func TestAccountTokenService_CompleteSetup(t *testing.T) {
	ctx := context.Background()

	t.Run("sets the password, ends other sessions and returns the user under the new epoch", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeSetup)
		f.expectCompletion()

		user, err := f.svc.CompleteSetup(ctx, token, linkTestNewPass, "  Ada Lovelace ")

		require.NoError(t, err)
		assert.Equal(t, pwTestUserID, user.ID)
		assert.Equal(t, int64(7), user.SessionEpoch, "the epoch a new cookie must carry")
		assert.Equal(t, "Ada Lovelace", user.Name)
		assert.Equal(t, []string{"link consumed", "password set", "sessions ended", "account updated"}, f.events)
		assert.Empty(t, f.apiAccess.revoked, "API keys and grants are kept")
		require.NotNil(t, f.saved)
		assert.Equal(t, "Ada Lovelace", f.saved.Name)
		assert.Nil(t, f.saved.EmailVerifiedAt, "a copied link proves nothing about the mailbox")
	})

	t.Run("works once", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeSetup)
		f.expectCompletion()
		_, err := f.svc.CompleteSetup(ctx, token, linkTestNewPass, "")
		require.NoError(t, err)

		_, err = f.svc.CompleteSetup(ctx, token, "another long enough passphrase", "")

		require.ErrorIs(t, err, ErrAccountTokenInvalid)
		_, err = f.svc.Preview(ctx, token)
		require.ErrorIs(t, err, ErrAccountTokenInvalid)
	})

	t.Run("an emailed link verifies the address; no name leaves the name alone", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.emailed(t, models.AccountTokenPurposeSetup)
		f.expectCompletion()

		user, err := f.svc.CompleteSetup(ctx, token, linkTestNewPass, "")

		require.NoError(t, err)
		require.NotNil(t, f.saved)
		require.NotNil(t, f.saved.EmailVerifiedAt)
		assert.Equal(t, linkTestNow, *f.saved.EmailVerifiedAt)
		assert.Equal(t, "Ada", f.saved.Name)
		assert.NotNil(t, user.EmailVerifiedAt)
	})

	t.Run("an address that is already verified keeps its date, and nothing is written", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		verifiedAt := linkTestNow.Add(-48 * time.Hour)
		f.user.EmailVerifiedAt = &verifiedAt
		token := f.emailed(t, models.AccountTokenPurposeSetup)
		f.expectCompletion()

		_, err := f.svc.CompleteSetup(ctx, token, linkTestNewPass, "Ada")

		require.NoError(t, err)
		assert.Nil(t, f.saved, "the same name and an already verified address need no write")
	})

	t.Run("a name that is too long is refused and the link stays usable", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeSetup)

		_, err := f.svc.CompleteSetup(ctx, token, linkTestNewPass, strings.Repeat("é", maxAccountNameLength+1))
		require.ErrorIs(t, err, ErrInvalidAccountName)

		f.expectCompletion()
		_, err = f.svc.CompleteSetup(ctx, token, linkTestNewPass, strings.Repeat("é", maxAccountNameLength))
		require.NoError(t, err, "the limit is in characters, not bytes")
	})

	t.Run("a reset link does not complete a setup", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeReset)

		_, err := f.svc.CompleteSetup(ctx, token, linkTestNewPass, "")

		require.Equal(t, ErrAccountTokenInvalid, err)
		assert.Empty(t, f.events)
		_, err = f.svc.Preview(ctx, token)
		require.NoError(t, err, "and is not used up by the attempt")
	})
}

func TestAccountTokenService_CompleteReset(t *testing.T) {
	ctx := context.Background()

	t.Run("replaces the password and keeps API access unless asked", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeReset)
		f.expectCompletion()

		user, err := f.svc.CompleteReset(ctx, token, linkTestNewPass, false)

		require.NoError(t, err)
		assert.Equal(t, int64(7), user.SessionEpoch)
		assert.Equal(t, []string{"link consumed", "password set", "sessions ended"}, f.events)
		assert.Empty(t, f.apiAccess.revoked)
		assert.Nil(t, f.saved, "a copied reset link changes nothing on the profile")
	})

	t.Run("revokes API access when asked, after the sessions end", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.emailed(t, models.AccountTokenPurposeReset)
		f.expectCompletion()

		_, err := f.svc.CompleteReset(ctx, token, linkTestNewPass, true)

		require.NoError(t, err)
		assert.Equal(t, []string{pwTestUserID}, f.apiAccess.revoked)
		assert.Equal(t,
			[]string{"link consumed", "password set", "sessions ended", "API access revoked", "account updated"},
			f.events)
		require.NotNil(t, f.saved.EmailVerifiedAt, "an emailed reset link verifies the address")
	})

	t.Run("completing any link ends every other link of the account", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		emailed := f.emailed(t, models.AccountTokenPurposeReset)
		copied := f.copied(t, models.AccountTokenPurposeReset)
		setup := f.copied(t, models.AccountTokenPurposeSetup)
		f.expectCompletion()

		_, err := f.svc.CompleteReset(ctx, copied, linkTestNewPass, false)
		require.NoError(t, err)

		_, err = f.svc.Preview(ctx, emailed)
		require.ErrorIs(t, err, ErrAccountTokenInvalid, "the emailed reset link died with the copied one")
		_, err = f.svc.CompleteReset(ctx, emailed, "another long enough passphrase", false)
		require.ErrorIs(t, err, ErrAccountTokenInvalid)
		_, err = f.svc.Preview(ctx, setup)
		require.ErrorIs(t, err, ErrAccountTokenInvalid, "a setup link is no way back in after a reset")
		_, err = f.svc.CompleteSetup(ctx, setup, "another long enough passphrase", "")
		require.ErrorIs(t, err, ErrAccountTokenInvalid)
		assert.Equal(t, 1, f.passwords.count(), "one password was set, by the reset")
	})

	t.Run("a setup link does not complete a reset", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeSetup)

		_, err := f.svc.CompleteReset(ctx, token, linkTestNewPass, true)

		require.Equal(t, ErrAccountTokenInvalid, err)
		assert.Empty(t, f.events)
	})

	t.Run("a password that fails the policy leaves the link usable", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeReset)

		for _, weak := range []string{"short", "password1234", pwTestEmail} {
			_, err := f.svc.CompleteReset(ctx, token, weak, false)
			var policy *password.PolicyError
			require.ErrorAs(t, err, &policy, weak)
		}
		assert.Empty(t, f.events, "nothing was consumed or written")

		f.expectCompletion()
		_, err := f.svc.CompleteReset(ctx, token, linkTestNewPass, false)
		require.NoError(t, err)
	})

	t.Run("an address the allowlist does not admit is refused and the link stays usable", func(t *testing.T) {
		f := newAccountLinkFixture(t, "someone.else@example.com")
		token := f.copied(t, models.AccountTokenPurposeReset)

		_, err := f.svc.CompleteReset(ctx, token, linkTestNewPass, false)

		require.ErrorIs(t, err, ErrAccessRestricted)
		assert.Empty(t, f.events)
		rows := f.tokens.all()
		require.Len(t, rows, 1)
		assert.Nil(t, rows[0].ConsumedAt, "the link is not used up")
	})

	t.Run("a suspended account's link is an invalid link", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeReset)
		f.user.Status = models.UserStatusSuspended

		_, err := f.svc.CompleteReset(ctx, token, linkTestNewPass, false)

		require.Equal(t, ErrAccountTokenInvalid, err)
		assert.Empty(t, f.events)
	})

	t.Run("of many requests completing one link at once, one wins", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		f.tokens.events, f.passwords.events = nil, nil // appended from many goroutines otherwise
		token := f.copied(t, models.AccountTokenPurposeReset)
		f.users.On("BumpSessionEpoch", mock.Anything, pwTestUserID).Return(int64(7), nil).Once()

		const callers = 40
		results := make(chan error, callers)
		var start sync.WaitGroup
		start.Add(1)
		for i := range callers {
			go func() {
				start.Wait()
				_, err := f.svc.CompleteReset(ctx, token, fmt.Sprintf("a long enough passphrase %d", i), false)
				results <- err
			}()
		}
		start.Done()

		won := 0
		for range callers {
			if err := <-results; err == nil {
				won++
			} else {
				require.ErrorIs(t, err, ErrAccountTokenInvalid)
			}
		}
		assert.Equal(t, 1, won)
		assert.Equal(t, 1, f.passwords.count(), "exactly one password was set")
	})

	t.Run("a failure after the link is consumed is an error, and the link is spent", func(t *testing.T) {
		boom := errors.New("connection refused")
		steps := map[string]func(f *accountLinkFixture){
			"setting the password": func(f *accountLinkFixture) {
				f.passwords.err = boom
			},
			"ending the sessions": func(f *accountLinkFixture) {
				f.users.On("BumpSessionEpoch", mock.Anything, pwTestUserID).Return(int64(0), boom).Once()
			},
			"revoking API access": func(f *accountLinkFixture) {
				f.users.On("BumpSessionEpoch", mock.Anything, pwTestUserID).Return(int64(7), nil).Once()
				f.apiAccess.err = boom
			},
			"updating the account": func(f *accountLinkFixture) {
				f.users.On("BumpSessionEpoch", mock.Anything, pwTestUserID).Return(int64(7), nil).Once()
				f.users.On("Update", mock.Anything, mock.Anything).Return(boom).Once()
			},
		}
		for name, arrange := range steps {
			t.Run(name, func(t *testing.T) {
				f := newAccountLinkFixture(t)
				token := f.emailed(t, models.AccountTokenPurposeReset)
				arrange(f)

				user, err := f.svc.CompleteReset(ctx, token, linkTestNewPass, true)

				assert.Nil(t, user, "nobody is signed in by a completion that failed")
				require.ErrorIs(t, err, boom)
				require.ErrorIs(t, err, ErrAccountLinkSpent, "the caller can tell the link is used up")
				require.NotErrorIs(t, err, ErrAccountTokenInvalid)
				assert.Contains(t, f.logs.LastEntry().Message, "consumed but could not be completed")
				_, err = f.svc.Preview(ctx, token)
				require.ErrorIs(t, err, ErrAccountTokenInvalid, "the link is spent")
			})
		}
	})

	// The moment after the consume is the one place an abandoned request
	// would cost the person their link, so the writes do not follow the
	// request's context.
	t.Run("a request abandoned right after the consume still completes", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeReset)
		requestCtx, abandon := context.WithCancel(ctx)
		defer abandon()
		f.tokens.afterConsume = abandon
		f.users.On("BumpSessionEpoch", mock.MatchedBy(func(c context.Context) bool { return c.Err() == nil }),
			pwTestUserID).Return(int64(7), nil).Once()

		user, err := f.svc.CompleteReset(requestCtx, token, linkTestNewPass, false)

		require.NoError(t, err)
		require.Error(t, requestCtx.Err(), "the request really was abandoned")
		assert.Equal(t, int64(7), user.SessionEpoch)
		assert.Equal(t, 1, f.passwords.count(), "the password was stored")
	})

	t.Run("a request abandoned before the consume spends nothing", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeReset)
		f.tokens.failing, f.tokens.failOn = context.Canceled, map[string]bool{"Consume": true}

		_, err := f.svc.CompleteReset(ctx, token, linkTestNewPass, false)

		require.ErrorIs(t, err, context.Canceled)
		require.NotErrorIs(t, err, ErrAccountLinkSpent)
		_, err = f.svc.Preview(ctx, token)
		require.NoError(t, err, "the link is still usable")
	})

	t.Run("a store failure while consuming is not reported as a bad link", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeReset)
		boom := errors.New("connection refused")
		f.tokens.failing, f.tokens.failOn = boom, map[string]bool{"Consume": true}

		_, err := f.svc.CompleteReset(ctx, token, linkTestNewPass, false)

		require.ErrorIs(t, err, boom)
		require.NotErrorIs(t, err, ErrAccountTokenInvalid)
	})
}

func TestAccountTokenService_Sweep(t *testing.T) {
	ctx := context.Background()
	f := newAccountLinkFixture(t)
	live := f.copied(t, models.AccountTokenPurposeSetup)
	f.emailed(t, models.AccountTokenPurposeReset) // expires in an hour
	f.now = f.now.Add(2 * time.Hour)

	removed, err := f.svc.Sweep(ctx)

	require.NoError(t, err)
	assert.Equal(t, int64(1), removed)
	_, err = f.svc.Preview(ctx, live)
	require.NoError(t, err, "a live link survives the sweep")

	boom := errors.New("connection refused")
	f.tokens.failing, f.tokens.failOn = boom, map[string]bool{"DeleteDead": true}
	_, err = f.svc.Sweep(ctx)
	require.ErrorIs(t, err, boom)
}

// Nothing the service logs or returns as an error carries a token or a
// password, whatever happens to the link.
func TestAccountTokenService_NothingSecretIsLoggedOrReturned(t *testing.T) {
	ctx := context.Background()
	f := newAccountLinkFixture(t)
	copied := f.copied(t, models.AccountTokenPurposeReset)
	emailed := f.emailed(t, models.AccountTokenPurposeReset)
	var withdrawn string
	_, sendErr := f.svc.IssueByEmail(ctx,
		IssueAccountLinkParams{UserID: pwTestUserID, Purpose: models.AccountTokenPurposeSetup},
		func(_ context.Context, email AccountLinkEmail) error {
			withdrawn = tokenIn(t, email.URL)
			return errors.New("smtp: connection refused")
		})
	require.Error(t, sendErr)

	var errs []error
	_, err := f.svc.Preview(ctx, copied)
	errs = append(errs, err)
	_, err = f.svc.CompleteReset(ctx, copied, "short", false)
	errs = append(errs, err)
	_, err = f.svc.CompleteSetup(ctx, copied, linkTestNewPass, "")
	errs = append(errs, err)
	f.passwords.err = errors.New("connection refused")
	_, err = f.svc.CompleteReset(ctx, copied, linkTestNewPass, true)
	errs = append(errs, err, sendErr)
	_, err = f.svc.CompleteReset(ctx, emailed, linkTestNewPass, false)
	errs = append(errs, err)

	secrets := []string{copied, emailed, withdrawn, linkTestNewPass}
	var seen strings.Builder
	for _, err := range errs {
		if err != nil {
			seen.WriteString(err.Error() + "\n")
		}
	}
	require.NotEmpty(t, f.logs.AllEntries())
	for _, entry := range f.logs.AllEntries() {
		fmt.Fprintf(&seen, "%s %v\n", entry.Message, entry.Data)
	}
	for _, secret := range secrets {
		assert.NotContains(t, seen.String(), secret)
	}
	assert.Contains(t, seen.String(), redactAccountToken(hashAccountToken(copied)),
		"log lines identify a link by the start of its hash")
}

// A link that ends up in a log call by accident shows no token.
func TestAccountLinkValuesDoNotLogTheLink(t *testing.T) {
	logger, logs := logtest.New()
	link := "https://app.example.com/account/reset#token=the-secret-token"
	expires := linkTestNow.Add(time.Hour)

	logger.Info("issued", "link", IssuedAccountLink{URL: link, ExpiresAt: expires})
	logger.Info("sending", "email", AccountLinkEmail{
		User: &models.User{ID: pwTestUserID}, Purpose: models.AccountTokenPurposeReset, URL: link, ExpiresAt: expires,
	})
	logger.Info("sending", "email", AccountLinkEmail{URL: link})

	require.Len(t, logs.AllEntries(), 3)
	for _, entry := range logs.AllEntries() {
		assert.NotContains(t, fmt.Sprintf("%v", entry.Data), "the-secret-token")
		assert.NotEmpty(t, entry.Data)
	}

	// Nor does fmt print it, by value or through a pointer.
	issued := IssuedAccountLink{URL: link, ExpiresAt: expires}
	email := AccountLinkEmail{Purpose: models.AccountTokenPurposeReset, URL: link, ExpiresAt: expires}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		printed := fmt.Sprintf(verb+" "+verb+" "+verb+" "+verb, issued, &issued, email, &email)
		assert.NotContains(t, printed, "the-secret-token", verb)
		assert.Contains(t, printed, "expires", verb)
	}
}

func TestAccountLinkURL(t *testing.T) {
	assert.Equal(t, "https://app.example.com/account/setup#token=abc",
		AccountLinkURL("https://app.example.com", models.AccountTokenPurposeSetup, "abc"))
	assert.Equal(t, "https://app.example.com/account/reset#token=abc",
		AccountLinkURL("https://app.example.com//", models.AccountTokenPurposeReset, "abc"))
}
