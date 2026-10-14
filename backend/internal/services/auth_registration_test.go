package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	idpmocks "github.com/vibexp/vibexp/internal/auth/idp/mocks"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repo_mocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// Which account email the access allowlist judges at sign-in, and the
// registration mode gate (#1434).

// recordingGate answers AllowNewAccount with err and records what it was asked.
type recordingGate struct {
	err   error
	calls []string
}

func (g *recordingGate) AllowNewAccount(_ context.Context, email string, verified bool) error {
	state := "unverified"
	if verified {
		state = "verified"
	}
	g.calls = append(g.calls, email+" "+state)
	return g.err
}

// signInFixture is an AuthService with an allowlist of exact emails (nil for an
// open instance), a recording gate, and strict repository mocks.
type signInFixture struct {
	svc        *AuthService
	users      *repo_mocks.MockUserRepository
	identities *repo_mocks.MockUserIdentityRepository
	gate       *recordingGate
	logs       *logtest.Recorder
}

func newSignInFixture(t *testing.T, allowedEmails []string, gateErr error) *signInFixture {
	t.Helper()
	logger, recorder := logtest.New()
	f := &signInFixture{
		users:      &repo_mocks.MockUserRepository{},
		identities: &repo_mocks.MockUserIdentityRepository{},
		gate:       &recordingGate{err: gateErr},
		logs:       recorder,
	}
	f.svc = NewAuthService(f.users, f.identities, staticIDPResolver{reg: idp.NewRegistry()}, nil, logger,
		newStaticAllowlistResolver(logger, allowedEmails), WithRegistrationGate(f.gate))
	return f
}

func (f *signInFixture) signIn(claims idp.Claims) (*models.User, bool, error) {
	return f.svc.createOrUpdateUserFromClaims(context.Background(), "corp-sso", idp.ProviderOIDC, &claims)
}

// assertNothingWritten fails if the sign-in created, updated or linked anything.
func (f *signInFixture) assertNothingWritten(t *testing.T) {
	t.Helper()
	f.users.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	f.users.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	f.users.AssertNotCalled(t, "BumpSessionEpoch", mock.Anything, mock.Anything)
	f.identities.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// An existing account is admitted or refused by its STORED email, the email the
// per-request check uses. The account email no longer follows the provider
// (#1430), so judging the claim email would let the two checks disagree.
func TestAuthService_AllowlistSubject_ExistingAccount(t *testing.T) {
	ctx := context.Background()
	verifiedAt := time.Now().Add(-24 * time.Hour)

	tests := []struct {
		name       string
		allowlist  []string
		stored     models.User
		claims     idp.Claims
		wantReason string // "" = admitted
	}{
		{
			name:      "stored email on the list, provider now reports another address: admitted",
			allowlist: []string{"me@corp.example"},
			stored:    models.User{ID: "user-1", Email: "me@corp.example", EmailVerifiedAt: &verifiedAt},
			claims:    idp.Claims{Subject: "sub-1", Email: "me@gmail.example", EmailVerified: true},
		},
		{
			name:       "stored email NOT on the list, provider reports a listed address: refused",
			allowlist:  []string{"me@corp.example"},
			stored:     models.User{ID: "user-1", Email: "me@gmail.example", EmailVerifiedAt: &verifiedAt},
			claims:     idp.Claims{Subject: "sub-1", Email: "me@corp.example", EmailVerified: true},
			wantReason: "not_on_allowlist",
		},
		{
			// The path of every user from before email verification was recorded
			// (migration 022_consolidated left email_verified_at NULL).
			name:      "never-verified account, provider verifies the stored address now: admitted",
			allowlist: []string{"me@corp.example"},
			stored:    models.User{ID: "user-1", Email: "me@corp.example"},
			claims:    idp.Claims{Subject: "sub-1", Email: "Me@Corp.example", EmailVerified: true},
		},
		{
			name:       "never-verified account, unverified claim: refused",
			allowlist:  []string{"me@corp.example"},
			stored:     models.User{ID: "user-1", Email: "me@corp.example"},
			claims:     idp.Claims{Subject: "sub-1", Email: "me@corp.example", EmailVerified: false},
			wantReason: "unverified_email",
		},
		{
			// A verified claim for some OTHER address says nothing about the
			// stored one, which may have been claimed unverified at sign-up.
			name:       "never-verified account, provider verifies a different address: refused",
			allowlist:  []string{"me@corp.example"},
			stored:     models.User{ID: "user-1", Email: "me@corp.example"},
			claims:     idp.Claims{Subject: "sub-1", Email: "other@gmail.example", EmailVerified: true},
			wantReason: "unverified_email",
		},
		{
			name:   "no allowlist: admitted whatever the claim says",
			stored: models.User{ID: "user-1", Email: "me@corp.example"},
			claims: idp.Claims{Subject: "sub-1", Email: "other@gmail.example", EmailVerified: false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newSignInFixture(t, tt.allowlist, nil)
			stored := tt.stored
			f.users.On("GetByIDPSubject", ctx, "corp-sso", "sub-1").Return(&stored, nil)
			f.users.On("Update", ctx, mock.Anything).Return(nil).Maybe()
			f.identities.On("Upsert", ctx, "user-1", "corp-sso", "sub-1").Return(nil).Maybe()

			user, _, err := f.signIn(tt.claims)

			assert.Empty(t, f.gate.calls, "the registration mode never applies to an existing account")
			if tt.wantReason != "" {
				require.ErrorIs(t, err, ErrAccessRestricted)
				assert.Nil(t, user)
				assert.Equal(t, tt.wantReason, denialReason(t, f.logs))
				f.assertNothingWritten(t)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "user-1", user.ID)
			assert.Equal(t, tt.stored.Email, user.Email, "the stored email is kept")
		})
	}
}

// A user from before email verification was recorded signs in with their usual
// provider on an allowlisted instance, and that sign-in records the
// verification.
func TestAuthService_AllowlistSubject_PreVerificationUserSignsInAndIsVerified(t *testing.T) {
	ctx := context.Background()
	f := newSignInFixture(t, []string{"me@corp.example"}, nil)
	f.users.On("GetByIDPSubject", ctx, "corp-sso", "sub-1").
		Return(&models.User{ID: "user-1", Email: "me@corp.example", Name: "Me"}, nil)
	var written *models.User
	f.users.On("Update", ctx, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		written = args.Get(1).(*models.User)
	})
	f.identities.On("Upsert", ctx, "user-1", "corp-sso", "sub-1").Return(nil)

	user, isNew, err := f.signIn(idp.Claims{Subject: "sub-1", Email: "me@corp.example", EmailVerified: true})

	require.NoError(t, err)
	assert.False(t, isNew)
	assert.Equal(t, "user-1", user.ID)
	require.NotNil(t, written)
	require.NotNil(t, written.EmailVerifiedAt, "the sign-in records that the stored address is verified")
	assert.WithinDuration(t, time.Now(), *written.EmailVerifiedAt, time.Minute)
}

// A link by email is judged by the same address either way; the allowlist's
// verification rule still comes first, as before.
func TestAuthService_AllowlistSubject_LinkByEmail(t *testing.T) {
	ctx := context.Background()

	t.Run("listed and verified: linked", func(t *testing.T) {
		f := newSignInFixture(t, []string{"ada@corp.example"}, nil)
		account := &models.User{ID: "pre-1", Email: "ada@corp.example"}
		f.users.On("GetByIDPSubject", ctx, "corp-sso", "sub-new").Return(nil, repositories.ErrUserNotFound)
		f.users.On("GetByEmail", ctx, "Ada@Corp.example").Return(account, nil)
		f.identities.On("ListByUserID", ctx, "pre-1").Return([]models.UserIdentity{}, nil)
		f.users.On("BumpSessionEpoch", ctx, "pre-1").Return(int64(1), nil)
		f.users.On("Update", ctx, mock.Anything).Return(nil)
		f.identities.On("Upsert", ctx, "pre-1", "corp-sso", "sub-new").Return(nil)

		user, isNew, err := f.signIn(idp.Claims{Subject: "sub-new", Email: "Ada@Corp.example", EmailVerified: true})

		require.NoError(t, err)
		assert.False(t, isNew)
		assert.Equal(t, "pre-1", user.ID)
		assert.Empty(t, f.gate.calls, "a pre-created account is existing: the registration mode does not apply")
	})

	t.Run("not listed: refused before anything is linked", func(t *testing.T) {
		f := newSignInFixture(t, []string{"someone@corp.example"}, nil)
		f.users.On("GetByIDPSubject", ctx, "corp-sso", "sub-new").Return(nil, repositories.ErrUserNotFound)
		f.users.On("GetByEmail", ctx, "ada@corp.example").
			Return(&models.User{ID: "pre-1", Email: "ada@corp.example"}, nil)

		_, _, err := f.signIn(idp.Claims{Subject: "sub-new", Email: "ada@corp.example", EmailVerified: true})

		require.ErrorIs(t, err, ErrAccessRestricted)
		f.assertNothingWritten(t)
	})

	t.Run("listed but unverified: the allowlist refuses first", func(t *testing.T) {
		f := newSignInFixture(t, []string{"ada@corp.example"}, nil)
		f.users.On("GetByIDPSubject", ctx, "corp-sso", "sub-new").Return(nil, repositories.ErrUserNotFound)
		f.users.On("GetByEmail", ctx, "ada@corp.example").
			Return(&models.User{ID: "pre-1", Email: "ada@corp.example"}, nil)

		_, _, err := f.signIn(idp.Claims{Subject: "sub-new", Email: "ada@corp.example", EmailVerified: false})

		require.ErrorIs(t, err, ErrAccessRestricted)
		assert.Equal(t, "unverified_email", denialReason(t, f.logs))
		f.assertNothingWritten(t)
	})
}

// A new account is judged by the claim email: allowlist first, then the
// registration mode, then the insert.
func TestAuthService_NewAccount_AllowlistThenRegistrationGate(t *testing.T) {
	ctx := context.Background()
	newcomer := idp.Claims{Subject: "sub-new", Email: "new@corp.example", Name: "New", EmailVerified: true}
	noAccount := func(f *signInFixture) {
		f.users.On("GetByIDPSubject", ctx, "corp-sso", "sub-new").Return(nil, repositories.ErrUserNotFound)
		f.users.On("GetByEmail", ctx, "new@corp.example").Return(nil, repositories.ErrUserNotFound)
	}

	t.Run("admitted by both: created", func(t *testing.T) {
		f := newSignInFixture(t, []string{"new@corp.example"}, nil)
		noAccount(f)
		f.users.On("Create", ctx, mock.Anything).Return(nil)

		user, isNew, err := f.signIn(newcomer)

		require.NoError(t, err)
		assert.True(t, isNew)
		assert.NotNil(t, user)
		assert.Equal(t, []string{"new@corp.example verified"}, f.gate.calls,
			"the gate is asked once, with the claim email and whether the provider verified it")
	})

	t.Run("registration closed: refused and nothing is created", func(t *testing.T) {
		f := newSignInFixture(t, nil, ErrRegistrationClosed)
		noAccount(f)

		user, isNew, err := f.signIn(newcomer)

		require.ErrorIs(t, err, ErrRegistrationClosed)
		assert.Nil(t, user)
		assert.False(t, isNew)
		f.assertNothingWritten(t)
	})

	t.Run("the allowlist refuses before the gate is asked", func(t *testing.T) {
		f := newSignInFixture(t, []string{"someone@corp.example"}, ErrRegistrationClosed)
		noAccount(f)

		_, _, err := f.signIn(newcomer)

		require.ErrorIs(t, err, ErrAccessRestricted)
		assert.Empty(t, f.gate.calls)
		f.assertNothingWritten(t)
	})

	t.Run("an unverified newcomer is passed to the gate as unverified", func(t *testing.T) {
		f := newSignInFixture(t, nil, nil)
		noAccount(f)
		f.users.On("Create", ctx, mock.Anything).Return(nil)
		unverified := newcomer
		unverified.EmailVerified = false

		_, _, err := f.signIn(unverified)

		require.NoError(t, err, "an open instance still creates an account from an unverified claim")
		assert.Equal(t, []string{"new@corp.example unverified"}, f.gate.calls)
	})

	t.Run("a gate that cannot decide fails the sign-in and creates nothing", func(t *testing.T) {
		boom := errors.New("policy unreadable")
		f := newSignInFixture(t, nil, boom)
		noAccount(f)

		_, _, err := f.signIn(newcomer)

		require.ErrorIs(t, err, boom)
		f.assertNothingWritten(t)
	})

	t.Run("ProvisionFromClaims enforces both too", func(t *testing.T) {
		f := newSignInFixture(t, nil, ErrRegistrationClosed)
		noAccount(f)

		_, err := f.svc.ProvisionFromClaims(ctx, "corp-sso", &newcomer)

		require.ErrorIs(t, err, ErrRegistrationClosed)
		f.assertNothingWritten(t)
	})
}

// HandleCallback returns each policy refusal as is, recognizably, and never
// reports one as a failure in the error log.
func TestAuthService_HandleCallback_RefusalsAreNotErrors(t *testing.T) {
	ctx := context.Background()
	claims := &idp.Claims{Subject: "sub-new", Email: "new@corp.example", EmailVerified: true}

	for name, tc := range map[string]struct {
		allowlist []string
		gateErr   error
		want      error
	}{
		"registration closed":  {nil, ErrRegistrationClosed, ErrRegistrationClosed},
		"not on the allowlist": {[]string{"someone@corp.example"}, nil, ErrAccessRestricted},
	} {
		t.Run(name, func(t *testing.T) {
			logger, recorder := logtest.New()
			provider := &idpmocks.MockIdentityProvider{}
			provider.On("Name").Return(idp.ProviderOIDC)
			provider.On("ExchangeCode", ctx, "code", "").Return(&idp.Tokens{AccessToken: "a"}, claims, nil)
			users := &repo_mocks.MockUserRepository{}
			noAccountYet(users)
			svc := NewAuthService(users, &repo_mocks.MockUserIdentityRepository{},
				newTestRegistry(provider), nil, logger, newStaticAllowlistResolver(logger, tc.allowlist),
				WithRegistrationGate(&recordingGate{err: tc.gateErr}))

			user, tokens, isNew, err := svc.HandleCallback(ctx, "code", string(idp.ProviderOIDC))

			assert.Equal(t, tc.want, err, "returned as is, so the handler can map it to its own code")
			assert.Nil(t, user)
			assert.Nil(t, tokens)
			assert.False(t, isNew)
			users.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
			for _, entry := range recorder.AllEntries() {
				assert.NotEqual(t, slog.LevelError, entry.Level, "unexpected error log: %s", entry.Message)
			}
		})
	}
}
