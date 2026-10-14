package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// Setup links for accounts, as an instance admin issues them (#1436).

const (
	setupAdminID = "admin-1"
	setupUserID  = "user-pending"
	rootEmail    = "root@example.com"
)

// rootAdminList is an InstanceAdminResolver over a fixed list of root emails.
type rootAdminList []string

func (r rootAdminList) IsRootAdmin(email string) bool {
	for _, root := range r {
		if strings.EqualFold(root, email) {
			return true
		}
	}
	return false
}

func (r rootAdminList) RootAdminEmails() []string { return r }

func (r rootAdminList) IsInstanceAdmin(context.Context, *models.User) (bool, error) {
	return false, nil
}

// adminSetupPolicy is an AuthPolicyResolver with password sign-in and email set by
// the test.
type adminSetupPolicy struct {
	on    bool
	email bool
	err   error
}

func (p *adminSetupPolicy) Policy(context.Context) (models.InstanceAuthPolicy, error) {
	return models.InstanceAuthPolicy{PasswordLoginEnabled: p.on}, p.err
}

func (p *adminSetupPolicy) PasswordResetChannel(context.Context) (string, error) {
	if p.email {
		return PasswordResetByEmail, nil
	}
	return PasswordResetByAdmin, nil
}

// adminSetupLinks records the links the service asked for.
type adminSetupLinks struct {
	AccountTokenServiceInterface
	issued []IssueAccountLinkParams
	err    error
}

func (l *adminSetupLinks) IssueLink(_ context.Context, params IssueAccountLinkParams) (*IssuedAccountLink, error) {
	if l.err != nil {
		return nil, l.err
	}
	l.issued = append(l.issued, params)
	return &IssuedAccountLink{URL: "https://app.example.com/account/setup#token=COPIED",
		ExpiresAt: time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)}, nil
}

// adminSetupMailer records the setup emails the service asked for.
type adminSetupMailer struct {
	AccountLinkMailerInterface
	emailed []string
	err     error
	// checkErr is what CheckResetEmail answers; log, when set, records calls
	// in order with the other fakes.
	checkErr error
	log      *[]string
}

func (m *adminSetupMailer) EmailSetupLink(_ context.Context, userID, createdBy string) (time.Time, error) {
	if m.err != nil {
		return time.Time{}, m.err
	}
	m.emailed = append(m.emailed, userID+" by "+createdBy)
	return time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC), nil
}

type adminSetupFixture struct {
	svc    *AdminAccountService
	admin  *repomocks.MockAdminRepository
	users  *repomocks.MockUserRepository
	policy *adminSetupPolicy
	links  *adminSetupLinks
	mailer *adminSetupMailer
	email  string
	status string
	signIn models.AdminUserSignIn
}

func newAdminSetupFixture(t *testing.T) *adminSetupFixture {
	t.Helper()
	logger, _ := logtest.New()
	f := &adminSetupFixture{
		admin:  repomocks.NewMockAdminRepository(t),
		users:  repomocks.NewMockUserRepository(t),
		policy: &adminSetupPolicy{on: true},
		links:  &adminSetupLinks{},
		mailer: &adminSetupMailer{},
		email:  "pending@example.com",
		status: models.UserStatusActive,
		signIn: models.AdminUserSignIn{PendingSetup: true, Methods: []string{}},
	}
	f.svc = NewAdminAccountService(AdminAccountDeps{
		Admin: f.admin, Users: f.users, Policy: f.policy, Tokens: f.links, Mailer: f.mailer,
		RootAdmins: rootAdminList{rootEmail}, Logger: logger,
	})
	f.users.On("GetByID", mock.Anything, setupUserID).
		Return(func(context.Context, string) (*models.User, error) {
			return &models.User{ID: setupUserID, Email: f.email}, nil
		}).Maybe()
	f.admin.On("GetUserDetail", mock.Anything, setupUserID).
		Return(func(context.Context, string) (*models.AdminUserDetail, error) {
			return &models.AdminUserDetail{ID: setupUserID, Email: f.email, Status: f.status, SignIn: f.signIn}, nil
		}).Maybe()
	f.admin.On("GetUserDetail", mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	return f
}

func TestAdminAccount_SetUpNewUser(t *testing.T) {
	ctx := context.Background()

	t.Run("with email working the link is emailed and none is made to copy", func(t *testing.T) {
		f := newAdminSetupFixture(t)
		f.policy.email = true

		setup := f.svc.SetUpNewUser(ctx, setupAdminID, setupUserID)

		assert.Equal(t, AdminLinkByEmail, setup.Delivery)
		assert.Empty(t, setup.LinkURL)
		assert.False(t, setup.ExpiresAt.IsZero())
		assert.Equal(t, []string{setupUserID + " by " + setupAdminID}, f.mailer.emailed)
		assert.Empty(t, f.links.issued, "no copied link exists that nobody asked for")
	})

	t.Run("with no email the copied link is returned", func(t *testing.T) {
		f := newAdminSetupFixture(t)

		setup := f.svc.SetUpNewUser(ctx, setupAdminID, setupUserID)

		assert.Equal(t, AdminLinkByCopy, setup.Delivery)
		assert.Equal(t, "https://app.example.com/account/setup#token=COPIED", setup.LinkURL)
		require.Len(t, f.links.issued, 1)
		assert.Equal(t, IssueAccountLinkParams{
			UserID: setupUserID, Purpose: models.AccountTokenPurposeSetup, CreatedBy: setupAdminID,
		}, f.links.issued[0])
		assert.Empty(t, f.mailer.emailed)
	})

	t.Run("a root admin's address gets no copied link", func(t *testing.T) {
		f := newAdminSetupFixture(t)
		f.email = "Root@Example.com"

		setup := f.svc.SetUpNewUser(ctx, setupAdminID, setupUserID)

		assert.Equal(t, AdminUserSetup{Delivery: adminLinkNone, Reason: AdminSetupReasonRootAdmin}, setup)
		assert.Empty(t, f.links.issued)
	})

	t.Run("a root admin's address is emailed like any other when email works", func(t *testing.T) {
		f := newAdminSetupFixture(t)
		f.email, f.policy.email = rootEmail, true

		setup := f.svc.SetUpNewUser(ctx, setupAdminID, setupUserID)

		assert.Equal(t, AdminLinkByEmail, setup.Delivery)
	})

	t.Run("with password sign-in off nothing is made", func(t *testing.T) {
		f := newAdminSetupFixture(t)
		f.policy.on = false

		setup := f.svc.SetUpNewUser(ctx, setupAdminID, setupUserID)

		assert.Equal(t, AdminUserSetup{Delivery: adminLinkNone, Reason: AdminSetupReasonLocalSignInOff}, setup)
		assert.Empty(t, f.links.issued)
		assert.Empty(t, f.mailer.emailed)
	})

	t.Run("a failure never fails the creation; it is reported", func(t *testing.T) {
		for name, tc := range map[string]struct {
			arrange func(f *adminSetupFixture)
			reason  string
		}{
			"the email": {func(f *adminSetupFixture) { f.policy.email, f.mailer.err = true, errors.New("smtp down") },
				AdminSetupReasonEmailFailed},
			"the link":   {func(f *adminSetupFixture) { f.links.err = errors.New("connection refused") }, AdminSetupReasonSetupFailed},
			"the policy": {func(f *adminSetupFixture) { f.policy.err = errors.New("connection refused") }, AdminSetupReasonSetupFailed},
		} {
			f := newAdminSetupFixture(t)
			tc.arrange(f)

			setup := f.svc.SetUpNewUser(ctx, setupAdminID, setupUserID)

			assert.Equal(t, AdminUserSetup{Delivery: adminLinkNone, Reason: tc.reason}, setup, name)
		}
	})
}

func TestAdminAccount_IssueSetupLink(t *testing.T) {
	ctx := context.Background()

	t.Run("a link to copy", func(t *testing.T) {
		f := newAdminSetupFixture(t)

		link, err := f.svc.IssueSetupLink(ctx, setupAdminID, setupUserID, AdminLinkByCopy)

		require.NoError(t, err)
		assert.Equal(t, AdminLinkByCopy, link.Delivery)
		assert.Equal(t, "https://app.example.com/account/setup#token=COPIED", link.LinkURL)
		assert.Equal(t, "pending@example.com", link.Email, "the audit record names the account")
		require.Len(t, f.links.issued, 1)
	})

	t.Run("by email", func(t *testing.T) {
		f := newAdminSetupFixture(t)
		f.policy.email = true

		link, err := f.svc.IssueSetupLink(ctx, setupAdminID, setupUserID, AdminLinkByEmail)

		require.NoError(t, err)
		assert.Equal(t, AdminLinkByEmail, link.Delivery)
		assert.Empty(t, link.LinkURL)
		assert.Equal(t, "pending@example.com", link.Email)
		assert.Len(t, f.mailer.emailed, 1)
		assert.Empty(t, f.links.issued)
	})

	refusals := map[string]struct {
		arrange  func(f *adminSetupFixture)
		actor    string
		user     string
		delivery AdminLinkDelivery
		want     error
	}{
		"your own account": {actor: setupUserID, delivery: AdminLinkByCopy, want: ErrAdminOwnAccount},
		"password sign-in off": {
			arrange: func(f *adminSetupFixture) { f.policy.on = false }, delivery: AdminLinkByCopy, want: ErrAdminLocalSignInOff,
		},
		"an unknown user": {user: "nobody", delivery: AdminLinkByCopy, want: ErrAdminUserNotFound},
		"an account that can already sign in": {
			arrange:  func(f *adminSetupFixture) { f.signIn = models.AdminUserSignIn{Methods: []string{"google"}} },
			delivery: AdminLinkByCopy, want: ErrAdminAccountHasSignInMethod,
		},
		"email with no provider": {
			arrange:  func(f *adminSetupFixture) { f.mailer.err = ErrAccountEmailUnavailable },
			delivery: AdminLinkByEmail, want: ErrAccountEmailUnavailable,
		},
		"a copied link for a root admin": {
			arrange: func(f *adminSetupFixture) { f.email = rootEmail }, delivery: AdminLinkByCopy,
			want: ErrAdminRootAdminLinkRefused,
		},
		"an unknown delivery": {delivery: "carrier-pigeon", want: ErrInvalidAccountTokenRequest},
		"a suspended account, by email": {
			arrange:  func(f *adminSetupFixture) { f.status, f.policy.email = models.UserStatusSuspended, true },
			delivery: AdminLinkByEmail, want: ErrAdminAccountSuspended,
		},
	}
	for name, tc := range refusals {
		t.Run("refuses "+name, func(t *testing.T) {
			f := newAdminSetupFixture(t)
			if tc.arrange != nil {
				tc.arrange(f)
			}
			actor, user := setupAdminID, setupUserID
			if tc.actor != "" {
				actor = tc.actor
			}
			if tc.user != "" {
				user = tc.user
			}

			link, err := f.svc.IssueSetupLink(ctx, actor, user, tc.delivery)

			assert.Nil(t, link)
			require.ErrorIs(t, err, tc.want)
			assert.Empty(t, f.links.issued, "no link was issued")
		})
	}

	t.Run("a store failure is an error, not a refusal", func(t *testing.T) {
		f := newAdminSetupFixture(t)
		f.links.err = errors.New("connection refused")

		_, err := f.svc.IssueSetupLink(ctx, setupAdminID, setupUserID, AdminLinkByCopy)

		require.Error(t, err)
		for _, refusal := range []error{ErrAdminOwnAccount, ErrAdminLocalSignInOff, ErrAdminUserNotFound,
			ErrAdminAccountHasSignInMethod, ErrAccountEmailUnavailable, ErrAdminRootAdminLinkRefused} {
			assert.NotErrorIs(t, err, refusal)
		}
	})

	t.Run("a detail read failure is an error", func(t *testing.T) {
		logger, _ := logtest.New()
		admin := repomocks.NewMockAdminRepository(t)
		admin.On("GetUserDetail", mock.Anything, setupUserID).Return(nil, errors.New("connection refused"))
		svc := NewAdminAccountService(AdminAccountDeps{
			Admin: admin, Policy: &adminSetupPolicy{on: true}, Logger: logger, RootAdmins: rootAdminList{},
		})

		_, err := svc.IssueSetupLink(ctx, setupAdminID, setupUserID, AdminLinkByCopy)

		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrAdminUserNotFound)
	})
}

var _ repositories.AdminRepository = (*repomocks.MockAdminRepository)(nil)
