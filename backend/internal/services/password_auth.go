package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/vibexp/vibexp/internal/auth/password"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Email and password sign-in (#1431, epic #1426).
//
// # What a caller may learn
//
// Sign-in must not tell an anonymous caller which accounts exist. Every refusal
// that depends on the account (no such email, no password set, a disabled
// password, a locked account, a wrong password) is the same error,
// ErrInvalidCredentials, and each of those paths performs exactly one argon2id
// computation, so they also take about as long as each other. The computation
// dominates the request (tens of milliseconds against a few of database work),
// which is what "comparable timing" can honestly mean here.
//
// # The throttle
//
// An attempt is counted BEFORE its password is checked, by one statement that
// also refuses a locked or disabled credential
// (UserCredentialRepository.BeginAttempt). Checking the lock first and counting
// a failure afterwards would let every attempt of a simultaneous burst pass the
// check before any failure was stored, turning "five guesses" into as many as
// arrive together. A correct password undoes its own count.
//
// The refusals that say more are deliberate: the access allowlist is decided by
// the address alone, before any account is looked up, so it reveals nothing
// about accounts; and a suspended account is told so only after its password
// verified, so it tells nobody who does not already hold the password.
//
// # Secrets
//
// Neither a password nor a hash is ever logged, wrapped into an error, or
// returned. Errors from internal/auth/password are safe by construction; this
// file logs user ids and reasons only.

// LocalSignInProvider is the provider name password sign-in reports to the
// access allowlist's audit log and to the sign-in activity.
const LocalSignInProvider = "password"

var (
	// ErrPasswordLoginDisabled is returned by Login while the instance
	// authentication policy has password login switched off.
	ErrPasswordLoginDisabled = errors.New("password login is disabled")
	// ErrInvalidCredentials is the one refusal for every account-dependent
	// reason; see the comment at the top of this file. Login wraps it in a
	// *PasswordLoginError that says why, for the audit trail only.
	ErrInvalidCredentials = errors.New("invalid email or password")
	// ErrAccountSuspended is returned by Login, as an *AccountSuspendedError
	// naming the account, when the password was correct and the account is
	// suspended.
	ErrAccountSuspended = errors.New("account is suspended")
)

// AccountSuspendedError is the ErrAccountSuspended Login returns. It names the
// account so the refusal can be recorded against it: this is the one refusal
// where the caller proved they hold the password.
type AccountSuspendedError struct {
	UserID string
}

func (e *AccountSuspendedError) Error() string { return ErrAccountSuspended.Error() }

// Is makes errors.Is(err, ErrAccountSuspended) true.
func (e *AccountSuspendedError) Is(target error) bool { return target == ErrAccountSuspended }

// Reasons a password sign-in was refused. They go to logs, metrics and the
// account's activity record, never into a response. The constant names avoid
// the word for what the user typed on purpose: secret scanners read a constant
// named after a credential and holding a string as a hard-coded credential.
const (
	SignInRefusalUnknownEmail = "unknown_email"
	SignInRefusalNoCredential = "no_password"
	SignInRefusalDisabled     = "password_disabled"
	SignInRefusalLocked       = "locked"
	SignInRefusalMismatch     = "wrong_password"
	SignInRefusalUnreadable   = "unreadable_hash"
	// SignInRefusalSuspended is recorded for a sign-in refused with
	// ErrAccountSuspended.
	SignInRefusalSuspended = "account_suspended"
)

// PasswordLoginError is the ErrInvalidCredentials Login returns. It says why
// the sign-in was refused and, when the email named an account, which one, so
// the caller can record the failure against it. It must never shape the
// response: every PasswordLoginError is answered identically.
type PasswordLoginError struct {
	// Reason is one of the SignInRefusal* values.
	Reason string
	// UserID is empty when the email named no account.
	UserID string
}

func (e *PasswordLoginError) Error() string {
	return ErrInvalidCredentials.Error() + ": " + e.Reason
}

// Is makes errors.Is(err, ErrInvalidCredentials) true for every reason.
func (e *PasswordLoginError) Is(target error) bool {
	return target == ErrInvalidCredentials
}

// PasswordAuthServiceInterface is password sign-in and the management of a
// user's password credential.
type PasswordAuthServiceInterface interface {
	// Login verifies email and password and returns the user to sign in. It
	// returns ErrPasswordLoginDisabled, ErrAccessRestricted,
	// ErrInvalidCredentials (as a *PasswordLoginError) or ErrAccountSuspended
	// for a refusal, and any other error when the sign-in could not be decided
	// (the policy, the allowlist or the database could not be read).
	Login(ctx context.Context, email, plaintext string) (*models.User, error)
	// SetPassword gives the user a password, replacing any they had, after
	// checking it against the password policy (a *password.PolicyError when it
	// fails). Besides the completion of a one-time account link, which stores
	// the hash HashNewPassword made inside its own transaction, it is the one
	// way a password is stored. A new password
	// clears the sign-in throttle and a disabled mark. An unknown user returns
	// repositories.ErrUserNotFound.
	SetPassword(ctx context.Context, userID, plaintext string) error
	// HashNewPassword checks plaintext against the password policy for an
	// account with email and returns its encoded hash, storing nothing. It is
	// for a caller that must write the credential as part of a larger
	// statement or transaction (the completion of a one-time account link,
	// #1462); every other caller uses SetPassword. A password that breaks the
	// policy is a *password.PolicyError.
	HashNewPassword(ctx context.Context, plaintext, email string) (string, error)
	// RemovePassword deletes the user's password and reports whether they had
	// one.
	RemovePassword(ctx context.Context, userID string) (removed bool, err error)
	// HasPassword reports whether the user has a password, disabled or not.
	HasPassword(ctx context.Context, userID string) (bool, error)
	// VerifyPassword checks plaintext against the user's CURRENT password, for
	// an already authenticated session that is about to change it (#1437). It
	// goes through the same per-account throttle as Login: a wrong password is
	// a counted failed attempt and returns ErrCurrentPasswordInvalid, and while
	// the account is locked it returns ErrPasswordAttemptsExceeded (as a
	// *PasswordLockedError) without checking. An account with no password, or a
	// disabled one, is ErrCurrentPasswordInvalid too. The sign-in gates
	// (policy, allowlist, suspension) are not applied; the caller's session has
	// passed them.
	VerifyPassword(ctx context.Context, userID, plaintext string) error
	// PasswordStatus reports whether the user has a password, whether it is
	// usable (not disabled by an administrator), and when it last changed.
	PasswordStatus(ctx context.Context, userID string) (PasswordStatus, error)
	// HashNewPassword checks plaintext against the password policy for an
	// account with email and returns its encoded hash, storing nothing. It is
	// for the one caller that must write the credential in the same statement
	// as a new account (UserRepository.CreateWithCredential, #1435); every
	// other caller uses SetPassword. A password that breaks the policy is a
	// *password.PolicyError.
	HashNewPassword(ctx context.Context, plaintext, email string) (string, error)
}

// PasswordHasher makes and checks password hashes. *password.Hasher is the
// implementation; the interface exists so a test can count the work done.
type PasswordHasher interface {
	Hash(ctx context.Context, plaintext string) (string, error)
	Verify(ctx context.Context, encoded, plaintext string) (ok, needsRehash bool, err error)
	DummyVerify(ctx context.Context, plaintext string) error
}

// PasswordAuthDeps are the collaborators of PasswordAuthService.
type PasswordAuthDeps struct {
	Users       repositories.UserRepository
	Credentials repositories.UserCredentialRepository
	Policy      AuthPolicyResolver
	Allowlist   AccessAllowlistResolver
	// Hasher makes and checks the hashes. Production wiring passes
	// password.NewHasher(password.DefaultParams); tests pass cheap parameters.
	Hasher PasswordHasher
	// Lockout is the per-account throttle schedule. The zero value means
	// models.DefaultPasswordLockout.
	Lockout models.PasswordLockout
	Logger  *slog.Logger
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// PasswordAuthService implements PasswordAuthServiceInterface.
type PasswordAuthService struct {
	users       repositories.UserRepository
	credentials repositories.UserCredentialRepository
	policy      AuthPolicyResolver
	allowlist   AccessAllowlistResolver
	hasher      PasswordHasher
	lockout     models.PasswordLockout
	logger      *slog.Logger
	now         func() time.Time
}

var _ PasswordAuthServiceInterface = (*PasswordAuthService)(nil)

// NewPasswordAuthService creates the password sign-in service.
func NewPasswordAuthService(deps PasswordAuthDeps) *PasswordAuthService {
	lockout := deps.Lockout
	if lockout == (models.PasswordLockout{}) {
		lockout = models.DefaultPasswordLockout
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &PasswordAuthService{
		users:       deps.Users,
		credentials: deps.Credentials,
		policy:      deps.Policy,
		allowlist:   deps.Allowlist,
		hasher:      deps.Hasher,
		lockout:     lockout,
		logger:      deps.Logger,
		now:         now,
	}
}

// Login implements PasswordAuthServiceInterface. The gates run in the order the
// OAuth callback runs its own: policy, access allowlist, credential check,
// suspension.
func (s *PasswordAuthService) Login(ctx context.Context, email, plaintext string) (*models.User, error) {
	policy, err := s.policy.Policy(ctx)
	if err != nil {
		// Not "disabled": an unreadable policy must not look like a switch that
		// is off, or a database fault would silently close the only way in.
		return nil, fmt.Errorf("read the authentication policy: %w", err)
	}
	if !policy.PasswordLoginEnabled {
		return nil, ErrPasswordLoginDisabled
	}

	email = models.NormalizeEmail(email)

	// The address must match an active access allowlist exactly as it must for
	// every other sign-in method. What is waived is only the requirement that
	// an identity provider VERIFIED the address (#218), hence `true`: a
	// password account exists solely because an administrator or an inviter
	// created it for that address, and creation is where the allowlist is
	// enforced. Requiring provider verification here would make every invited
	// password account unusable on an allowlisted instance.
	err = ensureEmailAllowed(ctx, s.allowlist, s.logger, email, LocalSignInProvider, true)
	if err != nil {
		return nil, err
	}

	user, credential, refusal, err := s.admitAttempt(ctx, email)
	if err != nil {
		return nil, err
	}
	if refusal != nil {
		return nil, s.refuseWithoutAHash(ctx, plaintext, refusal)
	}

	ok, needsRehash, err := s.hasher.Verify(ctx, credential.PasswordHash, plaintext)
	switch {
	case errors.Is(err, password.ErrMalformedHash):
		// A stored hash this server cannot read. The error carries no part of it.
		s.logger.With("user_id", user.ID, "error", err.Error()).
			Error("Password sign-in refused: the stored hash is unreadable")
		return nil, s.refuseWithoutAHash(ctx, plaintext,
			&PasswordLoginError{Reason: SignInRefusalUnreadable, UserID: user.ID})
	case err != nil:
		// The request was abandoned while waiting to hash. Not a refusal.
		return nil, fmt.Errorf("verify the password: %w", err)
	case !ok:
		s.logIfLocked(user.ID, credential)
		return nil, &PasswordLoginError{Reason: SignInRefusalMismatch, UserID: user.ID}
	}

	if user.IsSuspended() {
		s.logger.With("user_id", user.ID).Info("Password sign-in refused: the account is suspended")
		return nil, &AccountSuspendedError{UserID: user.ID}
	}

	s.afterSuccess(ctx, user.ID, credential.PasswordHash, plaintext, needsRehash)
	return user, nil
}

// admitAttempt finds the account for email and admits one sign-in attempt
// against its credential, counting it (see the throttle note at the top of this
// file). It returns a non-nil refusal, and no credential, when there is nothing
// to check the password against: no such account, no password, a disabled one,
// or a lock still running. Such an attempt is not counted, so repeating it
// cannot extend the lock.
func (s *PasswordAuthService) admitAttempt(
	ctx context.Context, email string,
) (*models.User, *models.UserCredential, *PasswordLoginError, error) {
	user, err := s.users.GetByEmail(ctx, email)
	if errors.Is(err, repositories.ErrUserNotFound) {
		return nil, nil, &PasswordLoginError{Reason: SignInRefusalUnknownEmail}, nil
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("look up the account: %w", err)
	}

	credential, err := s.credentials.BeginAttempt(ctx, user.ID, s.now(), s.lockout)
	switch {
	case err == nil:
		return user, credential, nil, nil
	case errors.Is(err, repositories.ErrUserCredentialNotFound):
		return nil, nil, &PasswordLoginError{Reason: SignInRefusalNoCredential, UserID: user.ID}, nil
	case errors.Is(err, repositories.ErrUserCredentialDisabled):
		return nil, nil, &PasswordLoginError{Reason: SignInRefusalDisabled, UserID: user.ID}, nil
	case errors.Is(err, repositories.ErrUserCredentialUnavailable):
		return nil, nil, &PasswordLoginError{Reason: SignInRefusalLocked, UserID: user.ID}, nil
	default:
		return nil, nil, nil, fmt.Errorf("admit the sign-in attempt: %w", err)
	}
}

// refuseWithoutAHash spends one dummy computation, so a refusal that had no
// hash to check takes as long as one that did, and returns the refusal. If the
// request was abandoned while waiting to hash, that error is returned instead.
func (s *PasswordAuthService) refuseWithoutAHash(
	ctx context.Context, plaintext string, refusal *PasswordLoginError,
) error {
	if err := s.hasher.DummyVerify(ctx, plaintext); err != nil {
		return fmt.Errorf("refuse the sign-in: %w", err)
	}
	return refusal
}

// logIfLocked reports, once per lock, that the attempt just counted locked the
// account.
func (s *PasswordAuthService) logIfLocked(userID string, credential *models.UserCredential) {
	if !credential.IsLockedAt(s.now()) {
		return
	}
	s.logger.With(
		"user_id", userID,
		"failed_attempts", credential.FailedAttempts,
		"locked_until", credential.LockedUntil,
	).Warn("Password sign-in locked after repeated failures")
}

// afterSuccess undoes the count of the attempt that just succeeded (and any
// before it) and, when the hash was made under older parameters, replaces it
// with one made under the current ones. Neither is a reason to fail a sign-in
// whose password was correct, so failures are logged and the sign-in proceeds.
//
// The rehash is a compare-and-swap on verifiedHash: if the password was changed
// while this sign-in was in flight, the new password's hash is left alone.
func (s *PasswordAuthService) afterSuccess(
	ctx context.Context, userID, verifiedHash, plaintext string, needsRehash bool,
) {
	if err := s.credentials.RecordSuccess(ctx, userID); err != nil {
		s.logger.With("user_id", userID, "error", err.Error()).
			Error("Failed to reset the sign-in throttle after a successful password sign-in")
	}
	if !needsRehash {
		return
	}
	hash, err := s.hasher.Hash(ctx, plaintext)
	if err != nil {
		s.logger.With("user_id", userID, "error", err.Error()).
			Error("Failed to upgrade a password hash to the current parameters")
		return
	}
	updated, err := s.credentials.UpdateHash(ctx, userID, verifiedHash, hash)
	switch {
	case err != nil:
		s.logger.With("user_id", userID, "error", err.Error()).
			Error("Failed to upgrade a password hash to the current parameters")
	case updated:
		s.logger.With("user_id", userID).Info("Upgraded a password hash to the current parameters")
	default:
		s.logger.With("user_id", userID).
			Info("Skipped a password hash upgrade: the password was changed meanwhile")
	}
}

// SetPassword implements PasswordAuthServiceInterface.
func (s *PasswordAuthService) SetPassword(ctx context.Context, userID, plaintext string) error {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("load the account to set its password: %w", err)
	}
	hash, err := s.HashNewPassword(ctx, plaintext, user.Email)
	if err != nil {
		return err
	}
	if err = s.credentials.Upsert(ctx, userID, hash); err != nil {
		return fmt.Errorf("store the password credential: %w", err)
	}
	s.logger.With("user_id", userID).Info("Password set")
	return nil
}

// HashNewPassword implements PasswordAuthServiceInterface.
func (s *PasswordAuthService) HashNewPassword(ctx context.Context, plaintext, email string) (string, error) {
	if err := password.Validate(plaintext, email); err != nil {
		return "", err
	}
	hash, err := s.hasher.Hash(ctx, plaintext)
	if err != nil {
		return "", fmt.Errorf("hash the password: %w", err)
	}
	return hash, nil
}

// RemovePassword implements PasswordAuthServiceInterface.
func (s *PasswordAuthService) RemovePassword(ctx context.Context, userID string) (bool, error) {
	removed, err := s.credentials.Delete(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("remove the password credential: %w", err)
	}
	if removed {
		s.logger.With("user_id", userID).Info("Password removed")
	}
	return removed, nil
}

// HasPassword implements PasswordAuthServiceInterface.
func (s *PasswordAuthService) HasPassword(ctx context.Context, userID string) (bool, error) {
	_, err := s.credentials.Get(ctx, userID)
	if errors.Is(err, repositories.ErrUserCredentialNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load the password credential: %w", err)
	}
	return true, nil
}

// UnverifiedAccountCleanup is the UnverifiedAccountClaimHook of an instance
// with password sign-in (#1430, #1431). When a sign-in provider's VERIFIED
// email claims an account whose own email was never verified, whatever the
// holder of that account's one-time link set up before the mailbox's real owner
// arrived has to go: the account's outstanding one-time links (#1432), which
// would let their holder set a password again; the password; its sessions; and
// every API key and MCP/OAuth grant (#1449), which the link's holder could
// have minted while signed in.
type UnverifiedAccountCleanup struct {
	tokens    repositories.AccountTokenRepository
	apiAccess APIAccessServiceInterface
	logger    *slog.Logger
}

var _ UnverifiedAccountClaimHook = (*UnverifiedAccountCleanup)(nil)

// NewUnverifiedAccountCleanup creates the cleanup.
func NewUnverifiedAccountCleanup(
	tokens repositories.AccountTokenRepository, apiAccess APIAccessServiceInterface, logger *slog.Logger,
) *UnverifiedAccountCleanup {
	return &UnverifiedAccountCleanup{tokens: tokens, apiAccess: apiAccess, logger: logger}
}

// OnUnverifiedAccountClaimed deletes the account's one-time links and its
// password and ends its sessions, in one transaction, then revokes its API
// access.
//
// The transaction holds the lock the completion of a one-time link holds
// (#1462), so the two cannot interleave: a link completed before it has its
// password removed here, and one completed after it finds the link gone and
// sets nothing. Ending the sessions in the same transaction is what makes the
// revocation that follows complete: once it commits, nobody can sign in with
// the password, set a new one from a link, or use an open browser session to
// mint a new key or grant. (AuthService bumps the session epoch once more
// after the hook; that is harmless and keeps it correct with any hook.)
//
// Both steps are idempotent, as the hook's contract requires: a second run on
// an account already cleaned up changes nothing but the session epoch. A
// failed step is returned and aborts the sign-in, because linking the new
// owner to an account its previous holder can still enter would defeat the
// point.
func (c *UnverifiedAccountCleanup) OnUnverifiedAccountClaimed(ctx context.Context, userID string) error {
	cleared, err := c.tokens.ClearForClaim(ctx, userID)
	if err != nil {
		return fmt.Errorf("remove the one-time links and the password of a claimed account: %w", err)
	}
	if _, err := c.apiAccess.RevokeAPIAccess(ctx, userID); err != nil {
		return fmt.Errorf("revoke the API access of a claimed account: %w", err)
	}
	if cleared.Links > 0 || cleared.HadPassword {
		c.logger.With("user_id", userID, "links", cleared.Links, "had_password", cleared.HadPassword).
			Warn("Removed the one-time links and the password of an unverified account claimed by a verified sign-in")
	}
	return nil
}
