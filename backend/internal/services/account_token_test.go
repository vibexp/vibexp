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
// the PostgreSQL one: one live link per user, purpose and delivery; a link an
// administrator issues (CreatedBy set) replaces every live link of its purpose
// (#1490); and a completion that ends every other live link of the same user.
type memAccountTokens struct {
	mu     sync.Mutex
	rows   []*models.AccountToken
	events *[]string
	// failing, when set, is returned by every method named in failOn.
	failing error
	failOn  map[string]bool
	// apply stands in for the user and credential writes of a completion and
	// returns what the repository would.
	apply func(consumed *models.AccountToken, c repositories.AccountLinkCompletion) *repositories.CompletedAccountLink
	// completions records what each completion was asked to store.
	completions []repositories.AccountLinkCompletion
	// afterComplete, when set, runs once a link has been completed.
	afterComplete func()
}

func (m *memAccountTokens) fail(method string) error {
	if m.failing != nil && m.failOn[method] {
		return m.failing
	}
	return nil
}

func (m *memAccountTokens) Replace(
	_ context.Context, token *models.AccountToken, unlessIssuedWithin time.Duration,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.fail("Replace"); err != nil {
		return err
	}
	// The new link is issued "now": its expiry minus its kind's lifetime.
	issuedAt := token.ExpiresAt.Add(-models.AccountTokenLifetime(token.Purpose, token.Delivery))
	for _, row := range m.rows {
		sameKind := row.ConsumedAt == nil && row.UserID == token.UserID &&
			row.Purpose == token.Purpose && row.Delivery == token.Delivery
		if sameKind && unlessIssuedWithin > 0 && issuedAt.Sub(row.CreatedAt) < unlessIssuedWithin {
			return repositories.ErrAccountTokenTooRecent
		}
	}
	kept := m.rows[:0]
	for _, row := range m.rows {
		superseded := row.ConsumedAt == nil && row.UserID == token.UserID &&
			row.Purpose == token.Purpose && (token.CreatedBy != nil || row.Delivery == token.Delivery)
		if !superseded {
			kept = append(kept, row)
		}
	}
	token.ID = fmt.Sprintf("token-%d", len(m.rows)+1)
	token.CreatedAt = issuedAt
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

func (m *memAccountTokens) Complete(
	_ context.Context, c repositories.AccountLinkCompletion,
) (*repositories.CompletedAccountLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.fail("Complete"); err != nil {
		return nil, err
	}
	var won *models.AccountToken
	for _, row := range m.rows {
		if bytes.Equal(row.TokenHash, c.TokenHash) && row.IsLiveAt(c.Now) {
			won = row
		}
	}
	if won == nil {
		return nil, repositories.ErrAccountTokenNotFound
	}
	now := c.Now
	won.ConsumedAt = &now
	kept := m.rows[:0]
	for _, row := range m.rows {
		other := row != won && row.ConsumedAt == nil && row.UserID == won.UserID
		if !other {
			kept = append(kept, row)
		}
	}
	m.rows = kept
	m.completions = append(m.completions, c)
	if m.events != nil {
		*m.events = append(*m.events, "link completed")
	}
	consumed := *won
	done := m.apply(&consumed, c)
	if m.afterComplete != nil {
		m.afterComplete()
	}
	return done, nil
}

func (m *memAccountTokens) ClearForClaim(context.Context, string) (*repositories.ClearedAccountClaim, error) {
	return &repositories.ClearedAccountClaim{}, nil
}

func (m *memAccountTokens) DisablePassword(context.Context, string) (*repositories.DisabledAccountPassword, error) {
	return &repositories.DisabledAccountPassword{}, nil
}

// completed returns what every completion stored.
func (m *memAccountTokens) completed() []repositories.AccountLinkCompletion {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]repositories.AccountLinkCompletion(nil), m.completions...)
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

// linkPasswords is a PasswordAuthServiceInterface that applies the real
// password policy and returns a recognizable stand-in for the hash. (The
// generated mock lives in a package that imports this one.) err, when set,
// fails HashNewPassword.
type linkPasswords struct {
	PasswordAuthServiceInterface
	mu     sync.Mutex
	events *[]string
	err    error
	// status and statusErr are what PasswordStatus answers.
	status    PasswordStatus
	statusErr error
}

func (p *linkPasswords) HashNewPassword(ctx context.Context, plaintext, email string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Like the real service, which gives up hashing when its context ends.
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if p.err != nil {
		return "", p.err
	}
	if err := password.Validate(plaintext, email); err != nil {
		return "", err
	}
	if p.events != nil {
		*p.events = append(*p.events, "password hashed")
	}
	return "hash-of:" + plaintext, nil
}

// PasswordStatus reports the status the test set; the zero value is "no
// password".
func (p *linkPasswords) PasswordStatus(context.Context, string) (PasswordStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status, p.statusErr
}

// revocations is an APIAccessServiceInterface that records, for each call,
// whether its context was still alive.
type revocations struct {
	mu    sync.Mutex
	alive []bool
}

func (r *revocations) RevokeAPIAccess(ctx context.Context, _ string) (*models.APIAccessRevocation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.alive = append(r.alive, ctx.Err() == nil)
	return &models.APIAccessRevocation{}, nil
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
	// userMu guards user: the fake repository writes it while the user mock
	// reads it.
	userMu sync.Mutex
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
	f.tokens.apply = f.applyCompletion
	f.users.On("GetByID", mock.Anything, pwTestUserID).
		Return(func(context.Context, string) (*models.User, error) {
			f.userMu.Lock()
			defer f.userMu.Unlock()
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

// applyCompletion does to the fixture's user what the repository's one
// transaction does to the stored one.
func (f *accountLinkFixture) applyCompletion(
	consumed *models.AccountToken, c repositories.AccountLinkCompletion,
) *repositories.CompletedAccountLink {
	f.userMu.Lock()
	defer f.userMu.Unlock()
	f.user.SessionEpoch = 7
	f.user.Version++
	if consumed.ProvesMailbox() && f.user.EmailVerifiedAt == nil {
		now := c.Now
		f.user.EmailVerifiedAt = &now
	}
	if c.Name != "" {
		f.user.Name = c.Name
	}
	return &repositories.CompletedAccountLink{
		Token: consumed, SessionEpoch: f.user.SessionEpoch, Name: f.user.Name,
		EmailVerifiedAt: f.user.EmailVerifiedAt, UpdatedAt: c.Now, Version: f.user.Version,
	}
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

	// #1490: an administrator's link replaces every earlier link of its
	// purpose; the owner's own emailed request replaces only the emailed one.
	t.Run("an administrator's link replaces the owner's; the owner's leaves the administrator's", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		emailed := f.emailed(t, models.AccountTokenPurposeReset)
		copied := f.copied(t, models.AccountTokenPurposeReset)

		_, err := f.svc.Preview(ctx, emailed)
		require.ErrorIs(t, err, ErrAccountTokenInvalid, "the administrator's copied link replaces the owner's emailed one")
		_, err = f.svc.Preview(ctx, copied)
		require.NoError(t, err)

		again := f.emailed(t, models.AccountTokenPurposeReset)
		_, err = f.svc.Preview(ctx, copied)
		require.NoError(t, err, "the owner's new emailed link leaves the copied one live")
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

	t.Run("stores the password, the name and the new epoch in one completion", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeSetup)

		user, err := f.svc.CompleteSetup(ctx, token, linkTestNewPass, "  Ada Lovelace ")

		require.NoError(t, err)
		assert.Equal(t, pwTestUserID, user.ID)
		assert.Equal(t, int64(7), user.SessionEpoch, "the epoch a new cookie must carry")
		assert.Equal(t, "Ada Lovelace", user.Name)
		assert.Nil(t, user.EmailVerifiedAt, "a copied link proves nothing about the mailbox")
		assert.Equal(t, []string{"password hashed", "link completed"}, f.events,
			"the password is hashed before the one transaction, and nothing is written after it")
		assert.Empty(t, f.apiAccess.revoked, "API keys and grants are kept")
		completions := f.tokens.completed()
		require.Len(t, completions, 1)
		assert.Equal(t, repositories.AccountLinkCompletion{
			TokenHash: hashAccountToken(token), Now: linkTestNow,
			PasswordHash: "hash-of:" + linkTestNewPass, Name: "Ada Lovelace",
		}, completions[0])
	})

	t.Run("works once", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeSetup)
		_, err := f.svc.CompleteSetup(ctx, token, linkTestNewPass, "")
		require.NoError(t, err)

		_, err = f.svc.CompleteSetup(ctx, token, "another long enough passphrase", "")

		require.ErrorIs(t, err, ErrAccountTokenInvalid)
		_, err = f.svc.Preview(ctx, token)
		require.ErrorIs(t, err, ErrAccountTokenInvalid)
		assert.Len(t, f.tokens.completed(), 1)
	})

	t.Run("an emailed link verifies the address; no name leaves the name alone", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.emailed(t, models.AccountTokenPurposeSetup)

		user, err := f.svc.CompleteSetup(ctx, token, linkTestNewPass, "")

		require.NoError(t, err)
		require.NotNil(t, user.EmailVerifiedAt)
		assert.Equal(t, linkTestNow, *user.EmailVerifiedAt)
		assert.Equal(t, "Ada", user.Name)
		assert.Empty(t, f.tokens.completed()[0].Name)
	})

	t.Run("an address that is already verified keeps its date", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		verifiedAt := linkTestNow.Add(-48 * time.Hour)
		f.user.EmailVerifiedAt = &verifiedAt
		token := f.emailed(t, models.AccountTokenPurposeSetup)

		user, err := f.svc.CompleteSetup(ctx, token, linkTestNewPass, "Ada")

		require.NoError(t, err)
		require.NotNil(t, user.EmailVerifiedAt)
		assert.Equal(t, verifiedAt, *user.EmailVerifiedAt)
	})

	t.Run("a name that is too long is refused and the link stays usable", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeSetup)

		_, err := f.svc.CompleteSetup(ctx, token, linkTestNewPass, strings.Repeat("é", maxAccountNameLength+1))
		require.ErrorIs(t, err, ErrInvalidAccountName)
		assert.Empty(t, f.events)

		_, err = f.svc.CompleteSetup(ctx, token, linkTestNewPass, strings.Repeat("é", maxAccountNameLength))
		require.NoError(t, err, "the limit is in characters, not bytes")
	})

	t.Run("a reset link does not complete a setup", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeReset)

		_, err := f.svc.CompleteSetup(ctx, token, linkTestNewPass, "")

		require.Equal(t, ErrAccountTokenInvalid, err)
		assert.Empty(t, f.events, "not even hashed")
		_, err = f.svc.Preview(ctx, token)
		require.NoError(t, err, "and is not used up by the attempt")
	})
}

func TestAccountTokenService_CompleteReset(t *testing.T) {
	ctx := context.Background()

	t.Run("replaces the password and keeps API access unless asked", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeReset)

		user, err := f.svc.CompleteReset(ctx, token, linkTestNewPass, false)

		require.NoError(t, err)
		assert.Equal(t, int64(7), user.SessionEpoch)
		assert.Equal(t, "Ada", user.Name)
		assert.Nil(t, user.EmailVerifiedAt, "a copied reset link verifies nothing")
		assert.Equal(t, []string{"password hashed", "link completed"}, f.events)
		assert.Empty(t, f.apiAccess.revoked)
		assert.Empty(t, f.tokens.completed()[0].Name, "a reset never sets the name")
	})

	t.Run("revokes API access when asked, after the completion", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.emailed(t, models.AccountTokenPurposeReset)

		user, err := f.svc.CompleteReset(ctx, token, linkTestNewPass, true)

		require.NoError(t, err)
		assert.Equal(t, []string{pwTestUserID}, f.apiAccess.revoked)
		assert.Equal(t, []string{"password hashed", "link completed", "API access revoked"}, f.events)
		require.NotNil(t, user.EmailVerifiedAt, "an emailed reset link verifies the address")
	})

	t.Run("completing any link ends every other link of the account", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		emailed := f.emailed(t, models.AccountTokenPurposeReset)
		copied := f.copied(t, models.AccountTokenPurposeReset)
		setup := f.copied(t, models.AccountTokenPurposeSetup)

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
		assert.Len(t, f.tokens.completed(), 1, "one password was stored, by the reset")
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
		assert.Empty(t, f.events, "nothing was hashed, consumed or written")

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
		assert.Len(t, f.tokens.completed(), 1, "exactly one password was stored")
	})

	// The completion is all or nothing in the repository, so when it fails
	// nothing was written and the same link can be used again.
	t.Run("a completion that fails writes nothing and leaves the link usable", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeReset)
		boom := errors.New("connection refused")
		f.tokens.failing, f.tokens.failOn = boom, map[string]bool{"Complete": true}

		user, err := f.svc.CompleteReset(ctx, token, linkTestNewPass, true)

		assert.Nil(t, user)
		require.ErrorIs(t, err, boom)
		require.NotErrorIs(t, err, ErrAccountTokenInvalid, "a store failure is not a bad link")
		require.NotErrorIs(t, err, ErrAccountLinkSpent, "and the link is not spent")
		assert.Empty(t, f.apiAccess.revoked, "nothing is revoked for a completion that did not happen")

		f.tokens.failing = nil
		_, err = f.svc.CompleteReset(ctx, token, linkTestNewPass, false)
		require.NoError(t, err, "the same link completes on the next try")
	})

	t.Run("a request abandoned while it waits to hash spends nothing", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeReset)
		abandoned, cancel := context.WithCancel(ctx)
		cancel()

		_, err := f.svc.CompleteReset(abandoned, token, linkTestNewPass, false)

		require.ErrorIs(t, err, context.Canceled)
		require.NotErrorIs(t, err, ErrAccountLinkSpent)
		assert.Empty(t, f.tokens.completed())
		_, err = f.svc.Preview(ctx, token)
		require.NoError(t, err, "the link is still usable")
	})

	// The one write after the completion is the revocation. It must not be
	// skipped because the browser went away in that moment, and if it fails the
	// caller has to know the link is used up.
	t.Run("a request abandoned right after the completion still revokes API access", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.copied(t, models.AccountTokenPurposeReset)
		revoked := &revocations{}
		f.svc.apiAccess = revoked
		requestCtx, abandon := context.WithCancel(ctx)
		defer abandon()
		f.tokens.afterComplete = abandon

		user, err := f.svc.CompleteReset(requestCtx, token, linkTestNewPass, true)

		require.NoError(t, err)
		require.Error(t, requestCtx.Err(), "the request really was abandoned")
		assert.Equal(t, int64(7), user.SessionEpoch)
		assert.Equal(t, []bool{true}, revoked.alive, "the revocation ran on a context that was still alive")
	})

	t.Run("a revocation that fails after the completion says the link is spent", func(t *testing.T) {
		f := newAccountLinkFixture(t)
		token := f.emailed(t, models.AccountTokenPurposeReset)
		boom := errors.New("connection refused")
		f.apiAccess.err = boom

		user, err := f.svc.CompleteReset(ctx, token, linkTestNewPass, true)

		assert.Nil(t, user, "nobody is signed in by a request that failed")
		require.ErrorIs(t, err, boom)
		require.ErrorIs(t, err, ErrAccountLinkSpent, "the caller can tell the link is used up")
		assert.Contains(t, f.logs.LastEntry().Message, "API access could not be revoked")
		assert.Len(t, f.tokens.completed(), 1, "the password was stored")
		_, err = f.svc.Preview(ctx, token)
		require.ErrorIs(t, err, ErrAccountTokenInvalid, "the link is spent")
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
