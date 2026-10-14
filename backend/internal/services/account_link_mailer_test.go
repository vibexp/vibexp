package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repo_mocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// channelPolicy is an AuthPolicyResolver whose reset channel a test sets.
type channelPolicy struct {
	channel string
	err     error
}

func (p *channelPolicy) Policy(context.Context) (models.InstanceAuthPolicy, error) {
	return models.InstanceAuthPolicy{PasswordLoginEnabled: true}, nil
}

func (p *channelPolicy) PasswordResetChannel(context.Context) (string, error) {
	return p.channel, p.err
}

// sentAccountEmails is an AccountEmailSender that records what it was asked to
// send. err, when set, fails every send. hold, when set, blocks every reset
// send until it is closed, and entered is signalled when one starts.
type sentAccountEmails struct {
	mu      sync.Mutex
	setups  []AccountLinkMessage
	resets  []AccountLinkMessage
	changed []PasswordChangedNotice
	err     error
	hold    chan struct{}
	entered chan struct{}
}

func (s *sentAccountEmails) SendAccountSetup(_ context.Context, msg AccountLinkMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.setups = append(s.setups, msg)
	return nil
}

func (s *sentAccountEmails) SendPasswordReset(_ context.Context, msg AccountLinkMessage) error {
	if s.hold != nil {
		s.entered <- struct{}{}
		<-s.hold
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.resets = append(s.resets, msg)
	return nil
}

func (s *sentAccountEmails) SendPasswordChanged(_ context.Context, notice PasswordChangedNotice) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.changed = append(s.changed, notice)
	return nil
}

func (s *sentAccountEmails) counts() (setups, resets, changed int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.setups), len(s.resets), len(s.changed)
}

// mailerFixture is an AccountLinkMailer over the link fixture: the real token
// service on the in-memory link store, a recording sender, and a policy whose
// reset channel the test sets. The account has a usable password by default.
type mailerFixture struct {
	*accountLinkFixture
	mailer     *AccountLinkMailer
	policy     *channelPolicy
	sent       *sentAccountEmails
	runner     *BackgroundRunner
	identities *repo_mocks.MockUserIdentityRepository
	// linked is what the identity store answers for the account; identityErr
	// fails that read.
	linked      []models.UserIdentity
	identityErr error
}

func newMailerFixture(t *testing.T, allowedEmails ...string) *mailerFixture {
	t.Helper()
	f := &mailerFixture{
		accountLinkFixture: newAccountLinkFixture(t, allowedEmails...),
		policy:             &channelPolicy{channel: PasswordResetByEmail},
		sent:               &sentAccountEmails{},
	}
	f.passwords.status = PasswordStatus{Set: true, Usable: true}
	f.users.On("GetByEmail", mock.Anything, pwTestEmail).
		Return(func(context.Context, string) (*models.User, error) {
			f.userMu.Lock()
			defer f.userMu.Unlock()
			if f.user == nil {
				return nil, repositories.ErrUserNotFound
			}
			u := *f.user
			return &u, nil
		}).Maybe()
	f.users.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, repositories.ErrUserNotFound).Maybe()
	f.identities = repo_mocks.NewMockUserIdentityRepository(t)
	f.identities.On("ListByUserID", mock.Anything, pwTestUserID).
		Return(func(context.Context, string) ([]models.UserIdentity, error) {
			return f.linked, f.identityErr
		}).Maybe()
	f.runner = NewBackgroundRunner(4, 5*time.Second, f.svc.logger)
	t.Cleanup(func() { f.runner.Shutdown(context.Background()) })
	f.mailer = NewAccountLinkMailer(AccountLinkMailerDeps{
		Tokens: f.svc, Users: f.users, Identities: f.identities, Passwords: f.passwords, Policy: f.policy,
		Allowlist: f.svc.allowlist, Email: f.sent, Runner: f.runner, Logger: f.svc.logger,
		Now: func() time.Time { return f.now },
	})
	return f
}

// drain waits for every background task the fixture started.
func (f *mailerFixture) drain() {
	f.runner.Shutdown(context.Background())
}

func TestAccountLinkMailer_PasswordReset(t *testing.T) {
	ctx := context.Background()

	t.Run("an account with a usable password gets one email whose link completes", func(t *testing.T) {
		f := newMailerFixture(t)

		skipped, err := f.mailer.sendPasswordReset(ctx, "  Ada@Example.COM ")

		require.NoError(t, err)
		assert.Empty(t, skipped)
		require.Len(t, f.sent.resets, 1)
		msg := f.sent.resets[0]
		assert.Equal(t, pwTestEmail, msg.To)
		assert.Equal(t, "Ada", msg.Name)
		assert.Equal(t, time.Hour, msg.ValidFor, "an emailed reset is valid for 1 hour")
		assert.True(t, strings.HasPrefix(msg.LinkURL, "https://vibexp.example.com/account/reset#token="), msg.LinkURL)
		rows := f.tokens.all()
		require.Len(t, rows, 1)
		assert.Equal(t, models.AccountTokenDeliveryEmail, rows[0].Delivery, "so completing it proves the mailbox")
		assert.Nil(t, rows[0].CreatedBy, "issued by the system")

		user, err := f.svc.CompleteReset(ctx, tokenIn(t, msg.LinkURL), linkTestNewPass, false)
		require.NoError(t, err, "the emailed link works end to end")
		assert.NotNil(t, user.EmailVerifiedAt)
	})

	// An account with no password is eligible: a person whose setup link
	// expired, or who only ever signed in with a provider, has no other
	// self-service way to get a password, and the emailed link proves the
	// mailbox.
	t.Run("a pending account whose setup link expired gets one and can complete it", func(t *testing.T) {
		f := newMailerFixture(t)
		f.passwords.status = PasswordStatus{}
		expired := f.copied(t, models.AccountTokenPurposeSetup)
		f.now = f.now.Add(8 * 24 * time.Hour)
		_, err := f.svc.Preview(ctx, expired)
		require.ErrorIs(t, err, ErrAccountTokenInvalid, "the setup link is dead")

		skipped, err := f.mailer.sendPasswordReset(ctx, pwTestEmail)

		require.NoError(t, err)
		assert.Empty(t, skipped)
		require.Len(t, f.sent.resets, 1)
		user, err := f.svc.CompleteReset(ctx, tokenIn(t, f.sent.resets[0].LinkURL), linkTestNewPass, false)
		require.NoError(t, err)
		assert.NotNil(t, user.EmailVerifiedAt, "and the address is now verified")
	})

	t.Run("an account that only signs in with a provider gets one", func(t *testing.T) {
		f := newMailerFixture(t)
		f.passwords.status = PasswordStatus{}
		// A provider sign-in verified the address when it was linked.
		verifiedAt := linkTestNow.Add(-30 * 24 * time.Hour)
		f.user.EmailVerifiedAt = &verifiedAt
		f.linked = []models.UserIdentity{{UserID: pwTestUserID, Provider: "corp-sso", Subject: "sub-1"}}

		skipped, err := f.mailer.sendPasswordReset(ctx, pwTestEmail)

		require.NoError(t, err)
		assert.Empty(t, skipped)
		assert.Len(t, f.sent.resets, 1)
	})

	// The mailbox's owner and whoever signs in through the identity may be two
	// people. An emailed link would put the owner's password on the account
	// and mark it verified while the other person's way in stays attached.
	t.Run("no email for: an unverified account somebody already signs in to with a provider", func(t *testing.T) {
		for name, arrange := range map[string]func(f *mailerFixture){
			"an identity row": func(f *mailerFixture) {
				f.linked = []models.UserIdentity{{UserID: pwTestUserID, Provider: "corp-sso", Subject: "sub-1"}}
			},
			"the legacy subject on the user": func(f *mailerFixture) {
				provider, subject := "corp-sso", "sub-1"
				f.user.IDPProvider, f.user.IDPSubject = &provider, &subject
			},
		} {
			f := newMailerFixture(t)
			arrange(f)

			skipped, err := f.mailer.sendPasswordReset(ctx, pwTestEmail)

			require.NoError(t, err, name)
			assert.Equal(t, resetSkippedShared, skipped, name)
			assert.Empty(t, f.tokens.all(), name)
			_, resets, _ := f.sent.counts()
			assert.Zero(t, resets, name)
		}
	})

	// The cooldown is decided where the link is stored, so simultaneous
	// requests cannot each pass it.
	t.Run("simultaneous requests for one account send one email", func(t *testing.T) {
		f := newMailerFixture(t)
		f.tokens.events = nil // appended from many goroutines otherwise
		const callers = 12
		outcomes := make(chan string, callers)
		var start sync.WaitGroup
		start.Add(1)
		for range callers {
			go func() {
				start.Wait()
				skipped, err := f.mailer.sendPasswordReset(ctx, pwTestEmail)
				if err != nil {
					skipped = "error: " + err.Error()
				}
				outcomes <- skipped
			}()
		}
		start.Done()

		sent, cooled := 0, 0
		for range callers {
			switch outcome := <-outcomes; outcome {
			case "":
				sent++
			case resetSkippedCooldown:
				cooled++
			default:
				t.Errorf("unexpected outcome %q", outcome)
			}
		}
		assert.Equal(t, 1, sent)
		assert.Equal(t, callers-1, cooled)
		_, resets, _ := f.sent.counts()
		assert.Equal(t, 1, resets, "one email, whose link is the live one")
		require.Len(t, f.tokens.all(), 1)
	})

	// Every one of these issues no link and sends nothing.
	refusals := map[string]struct {
		arrange func(f *mailerFixture)
		email   string
		want    string
	}{
		"no email provider stored": {
			arrange: func(f *mailerFixture) { f.policy.channel = PasswordResetByAdmin }, want: resetSkippedNoEmail,
		},
		"unknown address": {email: "nobody@example.com", want: resetSkippedUnknown},
		"suspended account": {
			arrange: func(f *mailerFixture) { f.user.Status = models.UserStatusSuspended }, want: resetSkippedSuspended,
		},
		// Storing a new password clears the disabled mark, so a reset would let
		// the person undo what the administrator did.
		"password disabled by an administrator": {
			arrange: func(f *mailerFixture) { f.passwords.status = PasswordStatus{Set: true, Usable: false} },
			want:    resetSkippedDisabled,
		},
	}
	for name, tc := range refusals {
		t.Run("no email for: "+name, func(t *testing.T) {
			f := newMailerFixture(t)
			if tc.arrange != nil {
				tc.arrange(f)
			}
			email := tc.email
			if email == "" {
				email = pwTestEmail
			}

			skipped, err := f.mailer.sendPasswordReset(ctx, email)

			require.NoError(t, err)
			assert.Equal(t, tc.want, skipped)
			setups, resets, changed := f.sent.counts()
			assert.Zero(t, setups+resets+changed, "nothing was sent")
			assert.Empty(t, f.tokens.all(), "no link was issued")
		})
	}

	t.Run("no email for: an address the allowlist does not admit", func(t *testing.T) {
		f := newMailerFixture(t, "someone.else@example.com")

		skipped, err := f.mailer.sendPasswordReset(ctx, pwTestEmail)

		require.NoError(t, err)
		assert.Equal(t, resetSkippedNotAllowed, skipped)
		assert.Empty(t, f.tokens.all())
	})

	t.Run("at most one email per account in five minutes, and the first link keeps working", func(t *testing.T) {
		f := newMailerFixture(t)
		_, err := f.mailer.sendPasswordReset(ctx, pwTestEmail)
		require.NoError(t, err)
		first := tokenIn(t, f.sent.resets[0].LinkURL)

		f.now = f.now.Add(passwordResetEmailCooldown - time.Second)
		skipped, err := f.mailer.sendPasswordReset(ctx, pwTestEmail)
		require.NoError(t, err)
		assert.Equal(t, resetSkippedCooldown, skipped)
		assert.Len(t, f.sent.resets, 1)
		_, err = f.svc.Preview(ctx, first)
		require.NoError(t, err, "a request inside the cooldown does not replace the earlier link")

		f.now = f.now.Add(time.Second)
		skipped, err = f.mailer.sendPasswordReset(ctx, pwTestEmail)
		require.NoError(t, err)
		assert.Empty(t, skipped, "after five minutes a new one goes out")
		assert.Len(t, f.sent.resets, 2)
		_, err = f.svc.Preview(ctx, first)
		require.ErrorIs(t, err, ErrAccountTokenInvalid, "and replaces the earlier link")
	})

	t.Run("a copied reset link does not start the cooldown", func(t *testing.T) {
		f := newMailerFixture(t)
		f.copied(t, models.AccountTokenPurposeReset)

		skipped, err := f.mailer.sendPasswordReset(ctx, pwTestEmail)

		require.NoError(t, err)
		assert.Empty(t, skipped)
	})

	t.Run("a send that fails is an error and leaves no live link", func(t *testing.T) {
		f := newMailerFixture(t)
		boom := errors.New("smtp: connection refused")
		f.sent.err = boom

		_, err := f.mailer.sendPasswordReset(ctx, pwTestEmail)

		require.ErrorIs(t, err, boom)
		assert.Empty(t, f.tokens.all(), "the link nobody received is withdrawn")
	})

	t.Run("anything that cannot be read is an error, never a send", func(t *testing.T) {
		boom := errors.New("connection refused")
		for name, arrange := range map[string]func(f *mailerFixture){
			"channel":    func(f *mailerFixture) { f.policy.err = boom },
			"status":     func(f *mailerFixture) { f.passwords.statusErr = boom },
			"identities": func(f *mailerFixture) { f.identityErr = boom },
			"link store": func(f *mailerFixture) {
				f.tokens.failing, f.tokens.failOn = boom, map[string]bool{"Replace": true}
			},
		} {
			f := newMailerFixture(t)
			arrange(f)

			_, err := f.mailer.sendPasswordReset(ctx, pwTestEmail)

			require.ErrorIs(t, err, boom, name)
			assert.Empty(t, f.tokens.all(), name)
		}
	})
}

// A lookup that fails is an error too, and sends nothing.
func TestAccountLinkMailer_UnreadableAccountIsAnError(t *testing.T) {
	logger, _ := logtest.New()
	boom := errors.New("connection refused")
	users := repo_mocks.NewMockUserRepository(t)
	users.On("GetByEmail", mock.Anything, pwTestEmail).Return(nil, boom)
	sent := &sentAccountEmails{}
	mailer := NewAccountLinkMailer(AccountLinkMailerDeps{
		Users: users, Policy: &channelPolicy{channel: PasswordResetByEmail}, Email: sent, Logger: logger,
	})

	_, err := mailer.sendPasswordReset(context.Background(), pwTestEmail)

	require.ErrorIs(t, err, boom)
	_, resets, _ := sent.counts()
	assert.Zero(t, resets)
}

// The whole point of the background: RequestPasswordReset has returned before
// the lookup and the send have finished, for an address with an account
// exactly as for one without. With the work done inline, the call for the
// account would block on the send and this test would time out.
func TestAccountLinkMailer_RequestPasswordResetReturnsBeforeTheWorkIsDone(t *testing.T) {
	f := newMailerFixture(t)
	f.sent.hold, f.sent.entered = make(chan struct{}), make(chan struct{}, 1)

	returned := make(chan struct{})
	go func() {
		f.mailer.RequestPasswordReset(pwTestEmail)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("RequestPasswordReset waited for the send")
	}
	select {
	case <-f.sent.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the send never started in the background")
	}
	_, resets, _ := f.sent.counts()
	assert.Zero(t, resets, "the send is still in flight although the call returned")

	close(f.sent.hold)
	f.drain()
	_, resets, _ = f.sent.counts()
	assert.Equal(t, 1, resets)
}

// RequestPasswordReset returns at once whatever the address is, and the work
// happens in the background.
func TestAccountLinkMailer_RequestPasswordResetRunsInTheBackground(t *testing.T) {
	f := newMailerFixture(t)

	f.mailer.RequestPasswordReset(pwTestEmail)
	f.mailer.RequestPasswordReset("nobody@example.com")
	f.drain()

	_, resets, _ := f.sent.counts()
	assert.Equal(t, 1, resets, "one for the account, none for the unknown address")

	// Nothing logged names the unknown address or carries a link.
	for _, entry := range f.logs.AllEntries() {
		line := entry.Message + fmt.Sprintf(" %v", entry.Data)
		assert.NotContains(t, line, "nobody@example.com")
		assert.NotContains(t, line, "#token=")
		assert.NotContains(t, line, tokenIn(t, f.sent.resets[0].LinkURL))
	}
}

func TestAccountLinkMailer_EmailSetupLink(t *testing.T) {
	ctx := context.Background()

	t.Run("emails a setup link valid for 7 days, issued by the administrator", func(t *testing.T) {
		f := newMailerFixture(t)

		expiresAt, err := f.mailer.EmailSetupLink(ctx, pwTestUserID, linkTestAdminID)

		require.NoError(t, err)
		assert.Equal(t, linkTestNow.Add(7*24*time.Hour), expiresAt)
		require.Len(t, f.sent.setups, 1)
		msg := f.sent.setups[0]
		assert.Equal(t, pwTestEmail, msg.To)
		assert.Equal(t, 7*24*time.Hour, msg.ValidFor)
		assert.True(t, strings.HasPrefix(msg.LinkURL, "https://vibexp.example.com/account/setup#token="), msg.LinkURL)
		rows := f.tokens.all()
		require.Len(t, rows, 1)
		require.NotNil(t, rows[0].CreatedBy)
		assert.Equal(t, linkTestAdminID, *rows[0].CreatedBy)
		assert.Equal(t, models.AccountTokenDeliveryEmail, rows[0].Delivery)
	})

	t.Run("with no email provider it issues nothing and says so", func(t *testing.T) {
		f := newMailerFixture(t)
		f.policy.channel = PasswordResetByAdmin

		_, err := f.mailer.EmailSetupLink(ctx, pwTestUserID, linkTestAdminID)

		require.ErrorIs(t, err, ErrAccountEmailUnavailable)
		assert.Empty(t, f.tokens.all())
		setups, _, _ := f.sent.counts()
		assert.Zero(t, setups)
	})

	t.Run("a send that fails is returned and the link withdrawn", func(t *testing.T) {
		f := newMailerFixture(t)
		boom := errors.New("smtp: connection refused")
		f.sent.err = boom

		_, err := f.mailer.EmailSetupLink(ctx, pwTestUserID, linkTestAdminID)

		require.ErrorIs(t, err, boom)
		assert.Empty(t, f.tokens.all())
	})
}

func TestAccountLinkMailer_EmailResetLink(t *testing.T) {
	ctx := context.Background()

	t.Run("emails a reset link valid for 1 hour, issued by the administrator", func(t *testing.T) {
		f := newMailerFixture(t)
		// A disabled password does not stop an administrator's reset.
		f.passwords.status = PasswordStatus{Set: true, Usable: false}

		expiresAt, err := f.mailer.EmailResetLink(ctx, pwTestUserID, linkTestAdminID)

		require.NoError(t, err)
		assert.Equal(t, linkTestNow.Add(time.Hour), expiresAt)
		require.Len(t, f.sent.resets, 1)
		assert.Equal(t, pwTestEmail, f.sent.resets[0].To)
		rows := f.tokens.all()
		require.Len(t, rows, 1)
		assert.Equal(t, models.AccountTokenPurposeReset, rows[0].Purpose)
		require.NotNil(t, rows[0].CreatedBy)
		assert.Equal(t, linkTestAdminID, *rows[0].CreatedBy)
	})

	t.Run("refusals issue and send nothing", func(t *testing.T) {
		for name, tc := range map[string]struct {
			arrange func(f *mailerFixture)
			want    error
		}{
			"no email provider": {func(f *mailerFixture) { f.policy.channel = PasswordResetByAdmin }, ErrAccountEmailUnavailable},
			"an unverified address someone signs in to with a provider": {func(f *mailerFixture) {
				f.linked = []models.UserIdentity{{UserID: pwTestUserID, Provider: "corp-sso", Subject: "sub-1"}}
			}, ErrAccountEmailUnverifiedShared},
		} {
			f := newMailerFixture(t)
			tc.arrange(f)

			require.ErrorIs(t, f.mailer.CheckResetEmail(ctx, pwTestUserID), tc.want, name)
			_, err := f.mailer.EmailResetLink(ctx, pwTestUserID, linkTestAdminID)
			require.ErrorIs(t, err, tc.want, name)
			assert.Empty(t, f.tokens.all(), name)
			_, resets, _ := f.sent.counts()
			assert.Zero(t, resets, name)
		}
	})

	t.Run("a verified account with a provider identity is fine", func(t *testing.T) {
		f := newMailerFixture(t)
		verifiedAt := linkTestNow.Add(-24 * time.Hour)
		f.user.EmailVerifiedAt = &verifiedAt
		f.linked = []models.UserIdentity{{UserID: pwTestUserID, Provider: "corp-sso", Subject: "sub-1"}}

		require.NoError(t, f.mailer.CheckResetEmail(ctx, pwTestUserID))
	})
}

func TestAccountEmailNotifier(t *testing.T) {
	newNotifier := func(f *mailerFixture) AccountNotifier {
		return NewAccountEmailNotifier(AccountEmailNotifierDeps{
			Users: f.users, Policy: f.policy, Email: f.sent, Runner: f.runner, Logger: f.svc.logger,
			Now: func() time.Time { return f.now },
		})
	}

	t.Run("sends the notice in the background, with no link", func(t *testing.T) {
		f := newMailerFixture(t)

		newNotifier(f).PasswordChanged(context.Background(), pwTestUserID, AccountChangeChanged)
		f.drain()

		require.Len(t, f.sent.changed, 1)
		assert.Equal(t, PasswordChangedNotice{
			To: pwTestEmail, Name: "Ada", Kind: AccountChangeChanged, ChangedAt: linkTestNow, CanResetByEmail: true,
		}, f.sent.changed[0])
	})

	t.Run("a password removed by a claim is sent with its kind and the provider (#1493)", func(t *testing.T) {
		f := newMailerFixture(t)

		newNotifier(f).PasswordRemovedByClaim(context.Background(), pwTestUserID, "Corp SSO")
		f.drain()

		require.Len(t, f.sent.changed, 1)
		assert.Equal(t, PasswordChangedNotice{
			To: pwTestEmail, Name: "Ada", Kind: AccountChangeRemovedByClaim, Provider: "Corp SSO",
			ChangedAt: linkTestNow, CanResetByEmail: true,
		}, f.sent.changed[0])
	})

	t.Run("a password removed by a claim sends nothing when no email provider is stored", func(t *testing.T) {
		f := newMailerFixture(t)
		f.policy.channel = PasswordResetByAdmin

		newNotifier(f).PasswordRemovedByClaim(context.Background(), pwTestUserID, "Corp SSO")
		f.drain()

		assert.Empty(t, f.sent.changed)
	})

	t.Run("the caller's cancelled context does not stop the notice", func(t *testing.T) {
		f := newMailerFixture(t)
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()

		newNotifier(f).PasswordChanged(cancelled, pwTestUserID, AccountChangeChanged)
		f.drain()

		assert.Len(t, f.sent.changed, 1)
	})

	t.Run("sends nothing when no email provider is stored", func(t *testing.T) {
		f := newMailerFixture(t)
		f.policy.channel = PasswordResetByAdmin

		newNotifier(f).PasswordChanged(context.Background(), pwTestUserID, AccountChangeChanged)
		f.drain()

		assert.Empty(t, f.sent.changed)
		assert.Empty(t, f.logs.AllEntries(), "and that is not an error")
	})

	t.Run("a failed send is logged, never returned", func(t *testing.T) {
		f := newMailerFixture(t)
		f.sent.err = errors.New("smtp: connection refused")

		newNotifier(f).PasswordChanged(context.Background(), pwTestUserID, AccountChangeChanged)
		f.drain()

		require.NotNil(t, f.logs.LastEntry())
		assert.Contains(t, f.logs.LastEntry().Message, "could not be sent")
		assert.Equal(t, pwTestUserID, f.logs.LastEntry().Data["user_id"])
	})

	// A full runner drops the notice; the drop is logged with the account, so
	// an operator can see whose signal was lost.
	t.Run("a dropped notice is logged with the account", func(t *testing.T) {
		f := newMailerFixture(t)
		full := NewBackgroundRunner(1, 5*time.Second, f.svc.logger)
		release := make(chan struct{})
		require.True(t, full.Go("blocker", func(context.Context) { <-release }))
		notifier := NewAccountEmailNotifier(AccountEmailNotifierDeps{
			Users: f.users, Policy: f.policy, Email: f.sent, Runner: full, Logger: f.svc.logger,
		})

		notifier.PasswordChanged(context.Background(), pwTestUserID, AccountChangeChanged)

		require.NotNil(t, f.logs.LastEntry())
		assert.Contains(t, f.logs.LastEntry().Message, "was dropped")
		assert.Equal(t, pwTestUserID, f.logs.LastEntry().Data["user_id"])
		close(release)
		full.Shutdown(context.Background())
		assert.Empty(t, f.sent.changed)
	})

	t.Run("a deleted account is nobody to tell", func(t *testing.T) {
		f := newMailerFixture(t)
		f.user = nil

		newNotifier(f).PasswordChanged(context.Background(), pwTestUserID, AccountChangeChanged)
		f.drain()

		assert.Empty(t, f.sent.changed)
		assert.Empty(t, f.logs.AllEntries())
	})
}

// A completed link tells the notifier, after everything else, and only when
// the completion went through.
func TestAccountTokenService_CompletionNotifiesThePasswordChange(t *testing.T) {
	ctx := context.Background()
	f := newMailerFixture(t)
	f.svc.notifier = NewAccountEmailNotifier(AccountEmailNotifierDeps{
		Users: f.users, Policy: f.policy, Email: f.sent, Runner: f.runner, Logger: f.svc.logger,
		Now: func() time.Time { return f.now },
	})
	setup := f.copied(t, models.AccountTokenPurposeSetup)

	_, err := f.svc.CompleteSetup(ctx, setup, "short", "")
	require.Error(t, err)
	_, err = f.svc.CompleteSetup(ctx, "bm90LWEtcmVhbC10b2tlbg", linkTestNewPass, "")
	require.ErrorIs(t, err, ErrAccountTokenInvalid)
	_, err = f.svc.CompleteSetup(ctx, setup, linkTestNewPass, "")
	require.NoError(t, err)
	f.drain()

	require.Len(t, f.sent.changed, 1, "one notice, for the one completion that set a password")
	assert.Equal(t, pwTestEmail, f.sent.changed[0].To)
}

// The password is already the new one when the requested revocation of API
// access fails, so the owner is still told.
func TestAccountTokenService_NotifiesEvenWhenRevocationFails(t *testing.T) {
	ctx := context.Background()
	f := newMailerFixture(t)
	f.svc.notifier = NewAccountEmailNotifier(AccountEmailNotifierDeps{
		Users: f.users, Policy: f.policy, Email: f.sent, Runner: f.runner, Logger: f.svc.logger,
		Now: func() time.Time { return f.now },
	})
	f.apiAccess.err = errors.New("connection refused")
	reset := f.copied(t, models.AccountTokenPurposeReset)

	_, err := f.svc.CompleteReset(ctx, reset, linkTestNewPass, true)
	require.ErrorIs(t, err, ErrAccountLinkSpent)
	f.drain()

	assert.Len(t, f.sent.changed, 1)
}

// The two runners are separate, so reset requests filling theirs do not stop
// a password-changed notice.
func TestAccountEmailRunnersAreSeparate(t *testing.T) {
	logger, _ := logtest.New()
	runners := NewAccountEmailRunners(logger)
	release := make(chan struct{})
	for range backgroundAccountEmails {
		require.True(t, runners.Requests.Go("request", func(context.Context) { <-release }))
	}
	require.False(t, runners.Requests.Go("request", func(context.Context) {}), "the request runner is full")

	ran := make(chan struct{})
	require.True(t, runners.Notices.Go("notice", func(context.Context) { close(ran) }))

	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Fatal("the notice did not run")
	}
	close(release)
	runners.Shutdown(context.Background())
}
