package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// One-time account links (#1432), driven through the router with the real
// AccountTokenService and PasswordAuthService over in-memory stores, so a link
// is followed from its issue to the session cookie it ends in, and the new
// password is then used to sign in. Every response is asserted against the spec.

const (
	linkPreviewPath = "/api/v1/auth/password/token/preview"
	linkSetupPath   = "/api/v1/auth/password/setup"
	linkResetPath   = "/api/v1/auth/password/reset"
	linkNewPassword = "a long enough new passphrase"
	linkAdminID     = "0d9c8b7a-6f5e-4d3c-8b2a-1f0e9d8c7b6a"
)

// memLinks is an in-memory AccountTokenRepository with the semantics of the
// PostgreSQL one.
type memLinks struct {
	mu   sync.Mutex
	rows []*models.AccountToken
	// err fails GetLive; completeErr fails Complete, which then writes nothing.
	err         error
	completeErr error
	// apply stands in for the credential and user writes of a completion.
	apply func(consumed *models.AccountToken, c repositories.AccountLinkCompletion) *repositories.CompletedAccountLink
}

func (m *memLinks) Replace(_ context.Context, token *models.AccountToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.rows[:0]
	for _, row := range m.rows {
		if row.ConsumedAt == nil && row.UserID == token.UserID &&
			row.Purpose == token.Purpose && row.Delivery == token.Delivery {
			continue
		}
		kept = append(kept, row)
	}
	stored := *token
	kept = append(kept, &stored)
	m.rows = kept
	return nil
}

func (m *memLinks) GetLive(_ context.Context, hash []byte, now time.Time) (*models.AccountToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	for _, row := range m.rows {
		if bytes.Equal(row.TokenHash, hash) && row.IsLiveAt(now) {
			found := *row
			return &found, nil
		}
	}
	return nil, repositories.ErrAccountTokenNotFound
}

func (m *memLinks) Complete(
	_ context.Context, c repositories.AccountLinkCompletion,
) (*repositories.CompletedAccountLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.completeErr != nil {
		return nil, m.completeErr
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
		if row != won && row.ConsumedAt == nil && row.UserID == won.UserID {
			continue
		}
		kept = append(kept, row)
	}
	m.rows = kept
	consumed := *won
	return m.apply(&consumed, c), nil
}

func (m *memLinks) ClearForClaim(context.Context, string) (*repositories.ClearedAccountClaim, error) {
	return &repositories.ClearedAccountClaim{}, nil
}

func (m *memLinks) Delete(context.Context, []byte) (bool, error) { return false, nil }

func (m *memLinks) DeleteDead(context.Context, time.Time) (int64, error) { return 0, nil }

// live reports how many unconsumed links are stored.
func (m *memLinks) live() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, row := range m.rows {
		if row.ConsumedAt == nil {
			n++
		}
	}
	return n
}

// countingAPIAccess is an APIAccessServiceInterface that records whose API
// access it was asked to revoke. err, when set, fails it.
type countingAPIAccess struct {
	mu      sync.Mutex
	revoked []string
	err     error
}

func (a *countingAPIAccess) RevokeAPIAccess(_ context.Context, userID string) (*models.APIAccessRevocation, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return nil, a.err
	}
	a.revoked = append(a.revoked, userID)
	return &models.APIAccessRevocation{}, nil
}

// linkFixture adds the account link service to the password sign-in fixture:
// the same server, user, credential store and clock.
type linkFixture struct {
	*pwLoginFixture
	links     *memLinks
	svc       services.AccountTokenServiceInterface
	apiAccess *countingAPIAccess
}

func newLinkFixture(t *testing.T, opts ...pwLoginOption) *linkFixture {
	t.Helper()
	f := &linkFixture{pwLoginFixture: newPwLoginFixture(t, opts...), links: &memLinks{}}
	f.apiAccess = &countingAPIAccess{}

	users := f.container.userRepository
	// What the repository's one transaction does: store the hash, end the
	// earlier sessions, and update the profile.
	f.links.apply = func(
		consumed *models.AccountToken, c repositories.AccountLinkCompletion,
	) *repositories.CompletedAccountLink {
		require.NoError(t, f.credentials.Upsert(context.Background(), consumed.UserID, c.PasswordHash))
		f.user.SessionEpoch++
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

	// The allowlist the options gave the password service applies here too.
	allowlist := staticAllowlistResolver(nil, nil)
	deps := services.PasswordAuthDeps{Allowlist: allowlist}
	for _, opt := range opts {
		opt(&pwLoginFixture{container: newMockAuthContainer(t)}, &deps)
	}
	f.svc = services.NewAccountTokenService(services.AccountTokenDeps{
		Tokens:    f.links,
		Users:     users,
		Passwords: f.container.passwordAuth,
		APIAccess: f.apiAccess,
		Allowlist: deps.Allowlist,
		BaseURL:   "https://vibexp.example.com",
		Logger:    f.srv.logger,
		Now:       func() time.Time { return f.now },
	})
	f.container.accountTokens = f.svc
	return f
}

// copied issues a link an administrator copies and returns its token.
func (f *linkFixture) copied(t *testing.T, purpose string) string {
	t.Helper()
	link, err := f.svc.IssueLink(context.Background(),
		services.IssueAccountLinkParams{UserID: pwLoginUserID, Purpose: purpose, CreatedBy: linkAdminID})
	require.NoError(t, err)
	_, token, found := strings.Cut(link.URL, "#token=")
	require.True(t, found)
	return token
}

// emailed issues a link by email and returns the token the mailer was handed.
func (f *linkFixture) emailed(t *testing.T, purpose string) string {
	t.Helper()
	var token string
	_, err := f.svc.IssueByEmail(context.Background(),
		services.IssueAccountLinkParams{UserID: pwLoginUserID, Purpose: purpose},
		func(_ context.Context, email services.AccountLinkEmail) error {
			_, token, _ = strings.Cut(email.URL, "#token=")
			return nil
		})
	require.NoError(t, err)
	require.NotEmpty(t, token)
	return token
}

// send posts a JSON body to path and asserts the response conforms to the spec.
func (f *linkFixture) send(t *testing.T, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, ok := body.([]byte)
	if !ok {
		var err error
		raw, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "account-link-test")
	rec := httptest.NewRecorder()
	f.srv.ServeHTTP(rec, req)
	specconformance.AssertConformsToSpec(t, req, rec)
	return rec
}

func (f *linkFixture) preview(t *testing.T, token string) *httptest.ResponseRecorder {
	t.Helper()
	return f.send(t, linkPreviewPath, map[string]any{"token": token})
}

// me requests the signed-in user's profile with the cookie a response set.
func (f *linkFixture) me(cookie *http.Cookie) int {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	f.srv.ServeHTTP(rec, req)
	return rec.Code
}

func TestAccountLinkPreview(t *testing.T) {
	f := newLinkFixture(t)
	token := f.copied(t, models.AccountTokenPurposeSetup)

	for range 2 {
		rec := f.preview(t, token)

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, map[string]any{
			"purpose": "setup", "email": pwLoginEmail, "name": "Ada Lovelace",
			"expires_at": f.now.Add(7 * 24 * time.Hour).Format(time.RFC3339),
		}, body, "exactly these fields: no user id, no providers, nothing about the token")
		assert.Nil(t, sessionCookieIn(rec), "looking at a link signs nobody in")
	}
	assert.Equal(t, 1, f.links.live(), "looking at a link does not use it up")
}

// Every link that cannot be used is the same response on all three operations,
// byte for byte apart from the request id and timestamp.
func TestAccountLink_UnusableLinksAreOneIdenticalResponse(t *testing.T) {
	type arrange func(t *testing.T, f *linkFixture) (token string)
	cases := map[string]arrange{
		"never existed": func(*testing.T, *linkFixture) string { return "bm90LWEtcmVhbC10b2tlbg" },
		"expired": func(t *testing.T, f *linkFixture) string {
			token := f.copied(t, models.AccountTokenPurposeReset)
			f.now = f.now.Add(24 * time.Hour)
			return token
		},
		"already used": func(t *testing.T, f *linkFixture) string {
			token := f.copied(t, models.AccountTokenPurposeReset)
			f.expectActivity(activities.ActivityTypeAuthLogin)
			used := f.send(t, linkResetPath, map[string]any{"token": token, "password": linkNewPassword})
			require.Equal(t, http.StatusOK, used.Code, used.Body.String())
			return token
		},
		"replaced by a newer link": func(t *testing.T, f *linkFixture) string {
			token := f.copied(t, models.AccountTokenPurposeReset)
			f.copied(t, models.AccountTokenPurposeReset)
			return token
		},
		"ended by the other link of its purpose": func(t *testing.T, f *linkFixture) string {
			token := f.emailed(t, models.AccountTokenPurposeReset)
			other := f.copied(t, models.AccountTokenPurposeReset)
			f.expectActivity(activities.ActivityTypeAuthLogin)
			used := f.send(t, linkResetPath, map[string]any{"token": other, "password": linkNewPassword})
			require.Equal(t, http.StatusOK, used.Code, used.Body.String())
			return token
		},
		"a setup link, after a reset was completed": func(t *testing.T, f *linkFixture) string {
			token := f.copied(t, models.AccountTokenPurposeSetup)
			reset := f.copied(t, models.AccountTokenPurposeReset)
			f.expectActivity(activities.ActivityTypeAuthLogin)
			used := f.send(t, linkResetPath, map[string]any{"token": reset, "password": linkNewPassword})
			require.Equal(t, http.StatusOK, used.Code, used.Body.String())
			return token
		},
		"account suspended": func(t *testing.T, f *linkFixture) string {
			token := f.copied(t, models.AccountTokenPurposeReset)
			f.user.Status = models.UserStatusSuspended
			return token
		},
	}

	var reference string
	for _, path := range []string{linkPreviewPath, linkSetupPath, linkResetPath} {
		for name, arrange := range cases {
			t.Run(path+"/"+name, func(t *testing.T) {
				f := newLinkFixture(t)
				token := arrange(t, f)
				hashedBefore := f.hasher.Derivations()

				rec := f.send(t, path, map[string]any{"token": token, "password": "another long passphrase"})

				require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
				assert.Nil(t, sessionCookieIn(rec))
				assert.Equal(t, hashedBefore, f.hasher.Derivations(), "no password was hashed for a bad link")
				// The problem's instance is the request path; everything else is
				// the same on all three operations.
				body := strings.ReplaceAll(stableBody(rec), path, "")
				if reference == "" {
					reference = body
				}
				assert.Equal(t, reference, body)
				assert.Contains(t, rec.Body.String(), accountLinkInvalidDetail)
			})
		}
	}
}

// A link of one purpose does not complete the other, and is not used up by the
// attempt.
func TestAccountLink_PurposeIsEnforced(t *testing.T) {
	f := newLinkFixture(t)
	setup := f.copied(t, models.AccountTokenPurposeSetup)
	reset := f.copied(t, models.AccountTokenPurposeReset)

	rec := f.send(t, linkResetPath, map[string]any{"token": setup, "password": linkNewPassword})
	assert.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
	rec = f.send(t, linkSetupPath, map[string]any{"token": reset, "password": linkNewPassword})
	assert.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())

	assert.Equal(t, 2, f.links.live())
	assert.Equal(t, f.hash, f.credentials.stored().PasswordHash, "the password is unchanged")
}

func TestAccountSetup_SetsThePasswordAndSignsIn(t *testing.T) {
	f := newLinkFixture(t)
	delete(f.credentials.rows, pwLoginUserID) // a new account has no password yet
	recorded := f.expectActivity(activities.ActivityTypeAuthLogin)
	token := f.emailed(t, models.AccountTokenPurposeSetup)
	epochBefore := f.user.SessionEpoch

	rec := f.send(t, linkSetupPath, map[string]any{"token": token, "password": linkNewPassword, "name": " Countess Ada "})

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, pwLoginUserID, body["id"])
	assert.Equal(t, "Countess Ada", body["name"])
	assert.NotContains(t, rec.Body.String(), token)

	cookie := sessionCookieIn(rec)
	require.NotNil(t, cookie, "completing a link signs the person in")
	assert.True(t, cookie.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	sess, err := f.srv.sessionManager.Read(req)
	require.NoError(t, err)
	assert.Equal(t, pwLoginUserID, sess.UserID)
	assert.Equal(t, sesslib.AuthMethodPassword, sess.AuthMethod)
	assert.Equal(t, epochBefore+1, sess.Epoch, "minted under the epoch the completion moved to")
	assert.Equal(t, http.StatusOK, f.me(cookie), "and the cookie is accepted")

	assert.Equal(t, services.LocalSignInProvider, recorded.get("provider"))
	assert.Equal(t, "setup", recorded.get("via"))
	require.NotNil(t, f.user.EmailVerifiedAt, "an emailed link verifies the address")
	assert.Equal(t, 0, f.links.live())

	// The password now signs in.
	login := f.post(t, pwLoginEmail, linkNewPassword)
	assert.Equal(t, http.StatusOK, login.Code, login.Body.String())
}

func TestAccountReset_ReplacesThePasswordAndEndsOtherSessions(t *testing.T) {
	f := newLinkFixture(t)
	recorded := f.expectActivity(activities.ActivityTypeAuthLogin)
	before := f.post(t, pwLoginEmail, pwLoginPassword)
	require.Equal(t, http.StatusOK, before.Code)
	earlier := sessionCookieIn(before)
	require.Equal(t, http.StatusOK, f.me(earlier))
	token := f.copied(t, models.AccountTokenPurposeReset)

	rec := f.send(t, linkResetPath, map[string]any{"token": token, "password": linkNewPassword})

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "reset", recorded.get("via"))
	assert.Equal(t, http.StatusOK, f.me(sessionCookieIn(rec)), "the new session works")
	assert.Equal(t, http.StatusUnauthorized, f.me(earlier), "a session from before the reset is ended")
	assert.Nil(t, f.user.EmailVerifiedAt, "a copied link verifies nothing")
	assert.Empty(t, f.apiAccess.revoked, "API keys and grants are kept")

	f.expectActivity(activities.ActivityTypeAuthFailure)
	assert.Equal(t, http.StatusUnauthorized, f.post(t, pwLoginEmail, pwLoginPassword).Code,
		"the old password no longer signs in")
	assert.Equal(t, http.StatusOK, f.post(t, pwLoginEmail, linkNewPassword).Code)
}

func TestAccountReset_RevokesAPIAccessOnlyWhenAsked(t *testing.T) {
	for name, tc := range map[string]struct {
		body    map[string]any
		revoked bool
	}{
		"asked":          {map[string]any{"revoke_api_access": true}, true},
		"explicitly not": {map[string]any{"revoke_api_access": false}, false},
		"field left out": {map[string]any{}, false},
	} {
		t.Run(name, func(t *testing.T) {
			f := newLinkFixture(t)
			f.expectActivity(activities.ActivityTypeAuthLogin)
			tc.body["token"] = f.copied(t, models.AccountTokenPurposeReset)
			tc.body["password"] = linkNewPassword

			rec := f.send(t, linkResetPath, tc.body)

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			if tc.revoked {
				assert.Equal(t, []string{pwLoginUserID}, f.apiAccess.revoked)
			} else {
				assert.Empty(t, f.apiAccess.revoked)
			}
		})
	}
}

// A password that fails the policy is a 400 naming each broken rule, and the
// link can be tried again.
func TestAccountLink_WeakPasswordIsFieldErrorsAndKeepsTheLink(t *testing.T) {
	for _, path := range []string{linkSetupPath, linkResetPath} {
		t.Run(path, func(t *testing.T) {
			f := newLinkFixture(t)
			f.expectActivity(activities.ActivityTypeAuthLogin)
			purpose := models.AccountTokenPurposeSetup
			if path == linkResetPath {
				purpose = models.AccountTokenPurposeReset
			}
			token := f.copied(t, purpose)

			for weak, code := range map[string]string{
				"tiny pw":      "too_short",
				"password1234": "common",
				pwLoginEmail:   "matches_email",
			} {
				rec := f.send(t, path, map[string]any{"token": token, "password": weak})

				require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
				var problem struct {
					Code   string `json:"code"`
					Errors []struct {
						Field string `json:"field"`
						Code  string `json:"code"`
					} `json:"validation_errors"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
				assert.Equal(t, "VALIDATION_FAILED", problem.Code)
				require.NotEmpty(t, problem.Errors, rec.Body.String())
				codes := []string{}
				for _, fieldError := range problem.Errors {
					assert.Equal(t, "password", fieldError.Field)
					codes = append(codes, fieldError.Code)
				}
				assert.Contains(t, codes, code)
				assert.NotContains(t, rec.Body.String(), weak+`"`, "the password is not echoed")
				assert.Nil(t, sessionCookieIn(rec))
			}
			assert.Equal(t, f.hash, f.credentials.stored().PasswordHash)

			rec := f.send(t, path, map[string]any{"token": token, "password": linkNewPassword})
			assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		})
	}
}

func TestAccountLink_OffAllowlistAccountIsRefusedAndKeepsTheLink(t *testing.T) {
	for path, purpose := range map[string]string{
		linkPreviewPath: models.AccountTokenPurposeSetup,
		linkSetupPath:   models.AccountTokenPurposeSetup,
		linkResetPath:   models.AccountTokenPurposeReset,
	} {
		t.Run(path, func(t *testing.T) {
			f := newLinkFixture(t, pwLoginWithAllowlist("someone-else@example.com"))
			token := f.copied(t, purpose)

			rec := f.send(t, path, map[string]any{"token": token, "password": linkNewPassword})

			require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
			code, _ := problemCodeAndReason(t, rec)
			assert.Equal(t, "ACCESS_RESTRICTED", strings.ToUpper(code))
			assert.Nil(t, sessionCookieIn(rec))
			assert.Equal(t, 1, f.links.live())
			assert.Equal(t, f.hash, f.credentials.stored().PasswordHash)
		})
	}
}

// A completion is all or nothing: when it cannot be stored, nothing was
// written, the link is still live, and the answer is the 503 that invites a
// retry.
func TestAccountLink_CompletionThatCannotBeStoredIs503AndKeepsTheLink(t *testing.T) {
	for path, purpose := range map[string]string{
		linkSetupPath: models.AccountTokenPurposeSetup,
		linkResetPath: models.AccountTokenPurposeReset,
	} {
		t.Run(path, func(t *testing.T) {
			f := newLinkFixture(t)
			f.expectActivity(activities.ActivityTypeAuthLogin)
			token := f.copied(t, purpose)
			f.links.completeErr = errors.New("pq: connection refused")

			rec := f.send(t, path, map[string]any{"token": token, "password": linkNewPassword})

			require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
			assert.NotContains(t, rec.Body.String(), "connection refused")
			assert.Nil(t, sessionCookieIn(rec), "nobody is signed in")
			assert.Equal(t, 1, f.links.live(), "the link is still live")
			assert.Equal(t, f.hash, f.credentials.stored().PasswordHash, "the password is unchanged")

			f.links.completeErr = nil
			again := f.send(t, path, map[string]any{"token": token, "password": linkNewPassword})
			assert.Equal(t, http.StatusOK, again.Code, "the same link completes on the next try")
		})
	}
}

// The one thing that can fail after a completion is the requested revocation
// of API access. That is a 500 saying the link is gone, never the 503 that
// invites a retry: the same link would only answer 401 from then on.
func TestAccountLink_FailureAfterTheLinkIsSpentIs500(t *testing.T) {
	f := newLinkFixture(t)
	token := f.copied(t, models.AccountTokenPurposeReset)
	f.apiAccess.err = errors.New("pq: connection refused")

	rec := f.send(t, linkResetPath,
		map[string]any{"token": token, "password": linkNewPassword, "revoke_api_access": true})

	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "can no longer be used")
	assert.Contains(t, rec.Body.String(), "Your password was changed", "the person is told what did happen")
	assert.NotContains(t, rec.Body.String(), "connection refused")
	assert.Nil(t, sessionCookieIn(rec), "nobody is signed in")
	assert.Equal(t, 0, f.links.live(), "the link is spent")

	f.apiAccess.err = nil
	again := f.send(t, linkResetPath, map[string]any{"token": token, "password": linkNewPassword})
	assert.Equal(t, http.StatusUnauthorized, again.Code, "and answers like any dead link afterwards")
}

func TestAccountLink_BadBodiesAre400(t *testing.T) {
	long := strings.Repeat("a", 257)
	for _, tc := range []struct{ name, path, body string }{
		{"preview: no token", linkPreviewPath, `{}`},
		{"preview: empty token", linkPreviewPath, `{"token":""}`},
		{"preview: oversized token", linkPreviewPath, `{"token":"` + long + `"}`},
		{"preview: malformed", linkPreviewPath, `{not json`},
		{"setup: no password", linkSetupPath, `{"token":"abc"}`},
		{"setup: no token", linkSetupPath, `{"password":"` + linkNewPassword + `"}`},
		{"setup: oversized token", linkSetupPath, `{"token":"` + long + `","password":"` + linkNewPassword + `"}`},
		{"setup: oversized password", linkSetupPath, `{"token":"abc","password":"` + strings.Repeat("p", 1025) + `"}`},
		{"setup: oversized name", linkSetupPath,
			`{"token":"abc","password":"` + linkNewPassword + `","name":"` + strings.Repeat("n", 1021) + `"}`},
		{"setup: empty body", linkSetupPath, ``},
		{"reset: no password", linkResetPath, `{"token":"abc"}`},
		{"reset: revoke is not a boolean", linkResetPath,
			`{"token":"abc","password":"` + linkNewPassword + `","revoke_api_access":"yes"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLinkFixture(t)

			rec := f.send(t, tc.path, []byte(tc.body))

			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Nil(t, sessionCookieIn(rec))
		})
	}
}

// A name longer than the limit in characters (though within the byte bound) is
// a 400 and leaves the link usable.
func TestAccountSetup_NameTooLongInCharacters(t *testing.T) {
	f := newLinkFixture(t)
	token := f.copied(t, models.AccountTokenPurposeSetup)

	rec := f.send(t, linkSetupPath,
		map[string]any{"token": token, "password": linkNewPassword, "name": strings.Repeat("n", 256)})

	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Equal(t, 1, f.links.live())
}

// Completing a link sets a session cookie, so like sign-in it must not be
// reachable from an HTML form on another site.
func TestAccountLink_RefusesABodyNotDeclaredAsJSON(t *testing.T) {
	for _, path := range []string{linkPreviewPath, linkSetupPath, linkResetPath} {
		for _, contentType := range []string{"text/plain", "application/x-www-form-urlencoded", ""} {
			t.Run(path+" "+contentType, func(t *testing.T) {
				f := newLinkFixture(t)
				token := f.copied(t, models.AccountTokenPurposeReset)
				body := fmt.Sprintf(`{"token":%q,"password":%q,"x":"="}`, token, linkNewPassword)
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				if contentType != "" {
					req.Header.Set("Content-Type", contentType)
				}
				rec := httptest.NewRecorder()

				f.srv.ServeHTTP(rec, req)

				assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
				assert.Nil(t, sessionCookieIn(rec))
				assert.Equal(t, 1, f.links.live())
			})
		}
	}
}

// With password login switched off none of the operations exists, and a link
// issued earlier is not used up by trying.
func TestAccountLink_DisabledIs404(t *testing.T) {
	off := staticAuthPolicy{policy: models.DefaultInstanceAuthPolicy(), reset: services.PasswordResetByAdmin}
	for _, path := range []string{linkPreviewPath, linkSetupPath, linkResetPath} {
		t.Run(path, func(t *testing.T) {
			f := newLinkFixture(t, pwLoginWithPolicy(off))
			token := f.copied(t, models.AccountTokenPurposeReset)

			rec := f.send(t, path, map[string]any{"token": token, "password": linkNewPassword})

			assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
			assert.Equal(t, 1, f.links.live())
		})
	}
}

func TestAccountLink_UnreadablePolicyIs503(t *testing.T) {
	f := newLinkFixture(t, pwLoginWithPolicy(staticAuthPolicy{err: errors.New("connection refused")}))

	rec := f.preview(t, "abc")

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "connection refused")
}

// A store that cannot be read is a 503, never the 401 of a bad link: a
// database fault must not tell someone their good link is dead.
func TestAccountLink_StoreFailureIs503(t *testing.T) {
	for _, path := range []string{linkPreviewPath, linkSetupPath, linkResetPath} {
		t.Run(path, func(t *testing.T) {
			f := newLinkFixture(t)
			token := f.copied(t, models.AccountTokenPurposeReset)
			f.links.err = errors.New("pq: connection refused")

			rec := f.send(t, path, map[string]any{"token": token, "password": linkNewPassword})

			assert.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
			assert.NotContains(t, rec.Body.String(), "connection refused")
			assert.Nil(t, sessionCookieIn(rec))
		})
	}
}

// A server that could not issue a session must not spend the link finding out.
func TestAccountLink_NoSessionManagerSpendsNoLink(t *testing.T) {
	f := newLinkFixture(t)
	token := f.copied(t, models.AccountTokenPurposeReset)
	f.srv.sessionManager = nil

	rec := f.send(t, linkResetPath, map[string]any{"token": token, "password": linkNewPassword})

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "not the 500 that means the link is used up")
	assert.Equal(t, 1, f.links.live())
	assert.Equal(t, f.hash, f.credentials.stored().PasswordHash)
}

func TestAccountLink_NothingSecretLeaves(t *testing.T) {
	f := newLinkFixture(t)
	var activityMetadata []map[string]interface{}
	f.container.activityService.On("RecordAuthActivity", mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			activityMetadata = append(activityMetadata, args.Get(4).(map[string]interface{}))
		}).Return(nil)
	setup := f.emailed(t, models.AccountTokenPurposeSetup)

	var responses []string
	record := func(rec *httptest.ResponseRecorder) {
		responses = append(responses, rec.Body.String(), fmt.Sprintf("%v", rec.Header()))
	}
	record(f.preview(t, setup))
	record(f.send(t, linkSetupPath, map[string]any{"token": setup, "password": "short"}))
	record(f.send(t, linkResetPath, map[string]any{"token": setup, "password": linkNewPassword}))
	record(f.send(t, linkSetupPath, map[string]any{"token": setup, "password": linkNewPassword}))
	record(f.send(t, linkSetupPath, map[string]any{"token": setup, "password": linkNewPassword}))
	// Issued now: completing the setup link above ended every earlier link.
	reset := f.copied(t, models.AccountTokenPurposeReset)
	record(f.send(t, linkResetPath, map[string]any{"token": reset, "password": "another long passphrase"}))
	record(f.send(t, linkResetPath, []byte(`{"token":"`+reset+`","password":"`+linkNewPassword+`"`)))

	secrets := []string{setup, reset, linkNewPassword, "another long passphrase", f.credentials.stored().PasswordHash}
	require.NotEmpty(t, f.logs.AllEntries())
	for _, entry := range f.logs.AllEntries() {
		line := entry.Message + " " + fmt.Sprintf("%v", entry.Data)
		for _, secret := range secrets {
			assert.NotContains(t, line, secret, "log entry %q", entry.Message)
		}
	}
	require.Len(t, activityMetadata, 2)
	for _, metadata := range activityMetadata {
		for _, secret := range secrets {
			assert.NotContains(t, fmt.Sprintf("%v", metadata), secret)
		}
	}
	for _, response := range responses {
		for _, secret := range secrets {
			assert.NotContains(t, response, secret)
		}
	}
}
