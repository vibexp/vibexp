package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Reset links and password removal, as an instance admin issues them (#1436).

func (m *adminSetupMailer) CheckResetEmail(context.Context, string) error {
	return m.checkErr
}

func (m *adminSetupMailer) EmailResetLink(_ context.Context, userID, createdBy string) (time.Time, error) {
	if m.log != nil {
		*m.log = append(*m.log, "email reset")
	}
	if m.err != nil {
		return time.Time{}, m.err
	}
	m.emailed = append(m.emailed, userID+" by "+createdBy)
	return time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC), nil
}

// orderedLinks issues copied links and records the call in order.
type orderedLinks struct {
	AccountTokenServiceInterface
	log    *[]string
	issued []IssueAccountLinkParams
	err    error
}

func (l *orderedLinks) IssueLink(_ context.Context, params IssueAccountLinkParams) (*IssuedAccountLink, error) {
	*l.log = append(*l.log, "issue "+params.Purpose)
	if l.err != nil {
		return nil, l.err
	}
	l.issued = append(l.issued, params)
	return &IssuedAccountLink{URL: "https://app.example.com/account/reset#token=COPIED",
		ExpiresAt: time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)}, nil
}

// disablingStore records DisablePassword.
type disablingStore struct {
	repositories.AccountTokenRepository
	log         *[]string
	hadPassword bool
	err         error
}

func (d *disablingStore) DisablePassword(context.Context, string) (*repositories.DisabledAccountPassword, error) {
	*d.log = append(*d.log, "disable")
	if d.err != nil {
		return nil, d.err
	}
	return &repositories.DisabledAccountPassword{HadPassword: d.hadPassword, Links: 2, SessionEpoch: 3}, nil
}

// removingAccounts records AdminRemovePassword.
type removingAccounts struct {
	AccountServiceInterface
	allowLast []bool
	err       error
}

func (a *removingAccounts) AdminRemovePassword(_ context.Context, _ string, allowLast bool) error {
	a.allowLast = append(a.allowLast, allowLast)
	return a.err
}

type adminAccessFixture struct {
	*adminSetupFixture
	log      []string
	links    *orderedLinks
	store    *disablingStore
	accounts *removingAccounts
	notifier *recordingNotifier
}

func newAdminAccessFixture(t *testing.T) *adminAccessFixture {
	t.Helper()
	f := &adminAccessFixture{adminSetupFixture: newAdminSetupFixture(t)}
	f.signIn = models.AdminUserSignIn{Methods: []string{models.LocalSignInMethod}, HasPassword: true}
	f.links = &orderedLinks{log: &f.log}
	f.store = &disablingStore{log: &f.log, hadPassword: true}
	f.accounts = &removingAccounts{}
	f.notifier = &recordingNotifier{}
	f.mailer.log = &f.log
	logger, _ := logtest.New()
	f.svc = NewAdminAccountService(AdminAccountDeps{
		Admin: f.admin, Users: f.users, Policy: f.policy, Tokens: f.links, Mailer: f.mailer,
		Store: f.store, Accounts: f.accounts, Notifier: f.notifier,
		RootAdmins: rootAdminList{rootEmail}, Logger: logger,
	})
	return f
}

func TestAdminAccount_IssueResetLink(t *testing.T) {
	ctx := context.Background()

	t.Run("a link to copy leaves the password working", func(t *testing.T) {
		f := newAdminAccessFixture(t)

		link, err := f.svc.IssueResetLink(ctx, setupAdminID, setupUserID, AdminResetLinkRequest{Delivery: AdminLinkByCopy})

		require.NoError(t, err)
		assert.Equal(t, "https://app.example.com/account/reset#token=COPIED", link.LinkURL)
		assert.Equal(t, "pending@example.com", link.Email)
		assert.False(t, link.Invalidated)
		assert.Equal(t, []string{"issue " + models.AccountTokenPurposeReset}, f.log, "nothing is stopped")
		assert.Equal(t, setupAdminID, f.links.issued[0].CreatedBy)
		assert.Empty(t, f.notifier.kinds)
	})

	t.Run("by email", func(t *testing.T) {
		f := newAdminAccessFixture(t)

		link, err := f.svc.IssueResetLink(ctx, setupAdminID, setupUserID, AdminResetLinkRequest{Delivery: AdminLinkByEmail})

		require.NoError(t, err)
		assert.Equal(t, AdminLinkByEmail, link.Delivery)
		assert.Empty(t, link.LinkURL)
		assert.Equal(t, []string{"email reset"}, f.log)
		assert.Equal(t, []string{setupUserID + " by " + setupAdminID}, f.mailer.emailed)
	})

	t.Run("invalidate now stops everything, tells the owner, then issues", func(t *testing.T) {
		for _, delivery := range []AdminLinkDelivery{AdminLinkByCopy, AdminLinkByEmail} {
			f := newAdminAccessFixture(t)

			link, err := f.svc.IssueResetLink(ctx, setupAdminID, setupUserID,
				AdminResetLinkRequest{Delivery: delivery, InvalidateNow: true})

			require.NoError(t, err, delivery)
			assert.True(t, link.Invalidated, delivery)
			require.Len(t, f.log, 2, delivery)
			assert.Equal(t, "disable", f.log[0], "the old password is stopped before the new link exists")
			assert.Equal(t, []AccountChangeKind{AccountChangeDisabledByAdmin}, f.notifier.kinds, delivery)
		}
	})

	t.Run("with no password to stop, nobody is told about a disable", func(t *testing.T) {
		f := newAdminAccessFixture(t)
		f.store.hadPassword = false

		_, err := f.svc.IssueResetLink(ctx, setupAdminID, setupUserID,
			AdminResetLinkRequest{Delivery: AdminLinkByCopy, InvalidateNow: true})

		require.NoError(t, err)
		assert.Empty(t, f.notifier.kinds)
	})

	refusals := map[string]struct {
		arrange  func(f *adminAccessFixture)
		actor    string
		delivery AdminLinkDelivery
		want     error
	}{
		"your own account": {actor: setupUserID, delivery: AdminLinkByCopy, want: ErrAdminOwnAccount},
		"password sign-in off": {
			arrange: func(f *adminAccessFixture) { f.policy.on = false }, delivery: AdminLinkByCopy,
			want: ErrAdminLocalSignInOff,
		},
		"a suspended account": {
			arrange: func(f *adminAccessFixture) { f.status = models.UserStatusSuspended }, delivery: AdminLinkByCopy,
			want: ErrAdminAccountSuspended,
		},
		"a copied link for a root admin": {
			arrange: func(f *adminAccessFixture) { f.email = rootEmail }, delivery: AdminLinkByCopy,
			want: ErrAdminRootAdminLinkRefused,
		},
		"email with no provider": {
			arrange:  func(f *adminAccessFixture) { f.mailer.checkErr = ErrAccountEmailUnavailable },
			delivery: AdminLinkByEmail, want: ErrAccountEmailUnavailable,
		},
		"email to an unverified address someone else signs in to": {
			arrange:  func(f *adminAccessFixture) { f.mailer.checkErr = ErrAccountEmailUnverifiedShared },
			delivery: AdminLinkByEmail, want: ErrAccountEmailUnverifiedShared,
		},
		"an unknown delivery": {delivery: "carrier-pigeon", want: ErrInvalidAccountTokenRequest},
	}
	for name, tc := range refusals {
		t.Run("refuses "+name+" and changes nothing, even with invalidate now", func(t *testing.T) {
			f := newAdminAccessFixture(t)
			if tc.arrange != nil {
				tc.arrange(f)
			}
			actor := setupAdminID
			if tc.actor != "" {
				actor = tc.actor
			}

			link, err := f.svc.IssueResetLink(ctx, actor, setupUserID,
				AdminResetLinkRequest{Delivery: tc.delivery, InvalidateNow: true})

			assert.Nil(t, link)
			require.ErrorIs(t, err, tc.want)
			assert.Empty(t, f.log, "nothing stopped, nothing issued")
			assert.Empty(t, f.notifier.kinds)
		})
	}

	t.Run("an unknown user", func(t *testing.T) {
		f := newAdminAccessFixture(t)

		_, err := f.svc.IssueResetLink(ctx, setupAdminID, "nobody", AdminResetLinkRequest{Delivery: AdminLinkByCopy})

		require.ErrorIs(t, err, ErrAdminUserNotFound)
		assert.Empty(t, f.log)
	})

	t.Run("a link that fails after the stop comes back with the stop, for the audit", func(t *testing.T) {
		for name, arrange := range map[string]func(f *adminAccessFixture) AdminLinkDelivery{
			"copied": func(f *adminAccessFixture) AdminLinkDelivery {
				f.links.err = errors.New("pq: down")
				return AdminLinkByCopy
			},
			"emailed": func(f *adminAccessFixture) AdminLinkDelivery {
				f.mailer.err = errors.New("smtp down")
				return AdminLinkByEmail
			},
		} {
			f := newAdminAccessFixture(t)
			delivery := arrange(f)

			link, err := f.svc.IssueResetLink(ctx, setupAdminID, setupUserID,
				AdminResetLinkRequest{Delivery: delivery, InvalidateNow: true})

			require.Error(t, err, name)
			require.NotNil(t, link, name)
			assert.True(t, link.Invalidated, name)
			assert.Empty(t, link.LinkURL, name)
			assert.Equal(t, "pending@example.com", link.Email, name)

			f2 := newAdminAccessFixture(t)
			arrange(f2)
			link, err = f2.svc.IssueResetLink(ctx, setupAdminID, setupUserID, AdminResetLinkRequest{Delivery: delivery})
			require.Error(t, err, name)
			assert.Nil(t, link, "nothing changed, nothing to audit: "+name)
		}
	})

	t.Run("a failed stop issues nothing", func(t *testing.T) {
		f := newAdminAccessFixture(t)
		f.store.err = errors.New("pq: down")

		_, err := f.svc.IssueResetLink(ctx, setupAdminID, setupUserID,
			AdminResetLinkRequest{Delivery: AdminLinkByCopy, InvalidateNow: true})

		require.Error(t, err)
		assert.Equal(t, []string{"disable"}, f.log)
		assert.Empty(t, f.notifier.kinds)
	})
}

func TestAdminAccount_RemovePassword(t *testing.T) {
	ctx := context.Background()

	t.Run("removed; the address comes back for the audit", func(t *testing.T) {
		f := newAdminAccessFixture(t)

		email, err := f.svc.RemovePassword(ctx, setupAdminID, setupUserID, true)

		require.NoError(t, err)
		assert.Equal(t, "pending@example.com", email)
		assert.Equal(t, []bool{true}, f.accounts.allowLast)
	})

	for name, tc := range map[string]struct {
		actor, user string
		accountsErr error
		want        error
	}{
		"your own account": {actor: setupUserID, user: setupUserID, want: ErrAdminOwnAccount},
		"an unknown user":  {actor: setupAdminID, user: "nobody", want: ErrAdminUserNotFound},
		"no password":      {actor: setupAdminID, user: setupUserID, accountsErr: ErrAccountPasswordNotSet, want: ErrAccountPasswordNotSet},
		"the last way in":  {actor: setupAdminID, user: setupUserID, accountsErr: ErrLastSignInMethod, want: ErrLastSignInMethod},
	} {
		t.Run("refuses "+name, func(t *testing.T) {
			f := newAdminAccessFixture(t)
			f.accounts.err = tc.accountsErr

			_, err := f.svc.RemovePassword(ctx, tc.actor, tc.user, false)

			require.ErrorIs(t, err, tc.want)
		})
	}
}
