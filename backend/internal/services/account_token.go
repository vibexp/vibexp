package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// One-time account links (#1432, epic #1426).
//
// A password account is never self-registered. A person gets one through an
// invitation or an administrator and sets its password from a link; a forgotten
// password is recovered through a link too. This service issues those links
// and completes them.
//
// # The token
//
// A link carries a token of 32 random bytes, unpadded URL-safe base64. Only its
// SHA-256 is stored, so a copy of the table opens no account, and a fast hash
// is right because there is nothing to brute-force in 256 bits of randomness.
// The token travels in the URL FRAGMENT of the link and in request BODIES,
// never in a path or a query string, so it reaches no access log, proxy or
// Referer header. Nothing here logs a token; log lines carry the first
// characters of its hash (redactAccountToken).
//
// # Emailed and copied links
//
// A link is either emailed to the account's mailbox or copied by an
// administrator and handed over. The difference matters: completing an emailed
// link proves its holder reads the mailbox, so it may mark the account's email
// verified; a copied link proves nothing about who holds it. To keep that
// distinction true, a token issued for email is handed ONLY to the function
// that sends the mail (IssueByEmail) and is never returned to the caller. An
// administrator must not be able to copy an "emailed" token.
//
// One emailed and one copied link of the same purpose may be live together.
// Completing ANY link ends every other live link of the account, of either
// purpose: a setup link that still worked after a reset set the password
// would be a way back in for whoever holds it.
//
// # One answer for a bad link
//
// Unknown, malformed, expired, consumed and superseded tokens, a token of the
// other purpose, and a token whose account is gone or suspended are all
// ErrAccountTokenInvalid. A caller holding a guess learns nothing from it.

const (
	// accountTokenBytes is the token's entropy: 32 random bytes, encoded as 43
	// unpadded URL-safe base64 characters.
	accountTokenBytes = 32

	// accountTokenMaxLength bounds the candidate that is hashed, so an oversized
	// body costs nothing. It is far above the real token's length.
	accountTokenMaxLength = 256

	// accountLinkCompletionTimeout bounds the revocation of API access that may
	// follow a completion. It runs detached from the request (see complete),
	// so it needs a limit of its own.
	accountLinkCompletionTimeout = 30 * time.Second

	// maxAccountNameLength bounds the display name a setup may set, in
	// characters. It matches the limit the user profile has elsewhere.
	maxAccountNameLength = 255

	// accountSetupPagePath and accountResetPagePath are the SPA routes the
	// links point at. The token follows as the URL fragment.
	accountSetupPagePath = "/account/setup"
	accountResetPagePath = "/account/reset"
)

var (
	// ErrAccountTokenInvalid is the one rejection for every link that cannot be
	// completed; see the comment at the top of this file.
	ErrAccountTokenInvalid = errors.New("invalid or expired account link")
	// ErrAccountLinkSpent wraps a failure that happens after a link was
	// completed: the link is used up, the password is the new one and the
	// earlier sessions are ended, but the requested revocation of API access
	// did not happen. Nothing can be retried with the link.
	ErrAccountLinkSpent = errors.New("account link spent without completing")
	// ErrAccountLinkTooRecent is returned by IssueLink and IssueByEmail when
	// params.MinInterval is set and the user's live link of the same kind is
	// younger than it. No link was issued and the earlier one still works.
	ErrAccountLinkTooRecent = errors.New("an account link was issued too recently")
	// ErrInvalidAccountTokenRequest is returned by IssueLink and IssueByEmail
	// for a request that names no user, or a purpose that is not one.
	ErrInvalidAccountTokenRequest = errors.New("invalid account link request")
	// ErrInvalidAccountName is returned by CompleteSetup for a display name that
	// is too long.
	ErrInvalidAccountName = errors.New("invalid account name")
)

// IssueAccountLinkParams says who a link is for and what it does.
type IssueAccountLinkParams struct {
	// UserID is the account the link acts on.
	UserID string
	// Purpose is models.AccountTokenPurposeSetup or ...Reset.
	Purpose string
	// CreatedBy is the administrator issuing the link; empty for a link the
	// system issues on the user's own request (forgot password).
	CreatedBy string
	// MinInterval, when positive, refuses to issue (ErrAccountLinkTooRecent)
	// while the link this one would replace is younger than it. The check and
	// the issue are one transaction, so simultaneous requests cannot each
	// pass it. Zero issues unconditionally.
	MinInterval time.Duration
}

// IssuedAccountLink is a link an administrator copies and hands over.
type IssuedAccountLink struct {
	// URL is the complete link. It contains the token: show it once, never log
	// or store it.
	URL       string
	ExpiresAt time.Time
}

// LogValue keeps the link out of logs: a logged IssuedAccountLink shows its
// expiry only.
func (l IssuedAccountLink) LogValue() slog.Value {
	return slog.GroupValue(slog.Time("expires_at", l.ExpiresAt))
}

// String and GoString do the same for fmt, including %v and %+v and %#v.
func (l IssuedAccountLink) String() string {
	return "account link (expires " + l.ExpiresAt.Format(time.RFC3339) + ")"
}

// GoString implements fmt.GoStringer; see String.
func (l IssuedAccountLink) GoString() string { return l.String() }

// AccountLinkEmail is what the mailer needs to send a link. It contains the
// token inside URL: pass it to the mail provider and nowhere else.
type AccountLinkEmail struct {
	User      *models.User
	Purpose   string
	URL       string
	ExpiresAt time.Time
}

// LogValue keeps the link out of logs: a logged AccountLinkEmail shows whose
// it is, its purpose and its expiry, never the URL.
func (e AccountLinkEmail) LogValue() slog.Value {
	userID := ""
	if e.User != nil {
		userID = e.User.ID
	}
	return slog.GroupValue(
		slog.String("user_id", userID), slog.String("purpose", e.Purpose), slog.Time("expires_at", e.ExpiresAt))
}

// String and GoString do the same for fmt, including %v and %+v and %#v.
func (e AccountLinkEmail) String() string {
	return "account link email (" + e.Purpose + ", expires " + e.ExpiresAt.Format(time.RFC3339) + ")"
}

// GoString implements fmt.GoStringer; see String.
func (e AccountLinkEmail) GoString() string { return e.String() }

// AccountLinkSender sends one account link to the account's mailbox. An error
// it returns is passed on to the caller of IssueByEmail and may be logged, so
// it must not contain the URL.
type AccountLinkSender func(ctx context.Context, email AccountLinkEmail) error

// AccountTokenPreview is what a link's page shows before the person types a
// password: what the link does and whose account it is.
type AccountTokenPreview struct {
	Purpose   string
	Email     string
	Name      string
	ExpiresAt time.Time
}

// AccountTokenServiceInterface issues and completes one-time account links.
type AccountTokenServiceInterface interface {
	// IssueLink issues a link for an administrator to copy and hand over
	// (delivery "link") and returns it. It replaces the user's previous copied
	// link of the same purpose; an emailed one stays live. A setup link lasts 7
	// days and a reset link 24 hours. Completing such a link does NOT mark the
	// account's email verified.
	IssueLink(ctx context.Context, params IssueAccountLinkParams) (*IssuedAccountLink, error)
	// IssueByEmail issues a link and hands it to send, which must mail it to
	// the account's address; the link is never returned to the caller, so it
	// cannot be copied (see the comment at the top of the file). It replaces
	// the user's previous emailed link of the same purpose; a copied one stays
	// live. A setup link lasts 7 days and a reset link 1 hour. If send fails
	// the link is withdrawn and the error returned; the emailed link it
	// replaced is gone too, so the caller should offer to send again.
	// Completing such a link marks the account's email verified.
	IssueByEmail(ctx context.Context, params IssueAccountLinkParams, send AccountLinkSender) (time.Time, error)
	// Preview returns what the link with this token does, ErrAccountTokenInvalid,
	// or ErrAccessRestricted when an active allowlist does not admit the
	// account's address (completing the link would be refused the same way).
	// It changes nothing.
	Preview(ctx context.Context, token string) (*AccountTokenPreview, error)
	// CompleteSetup completes a setup link: it sets the account's password,
	// and its display name when name is not empty, ends the account's other
	// sessions, and returns the user to sign in (with the session epoch a new
	// cookie must carry). It returns ErrAccountTokenInvalid, ErrAccessRestricted
	// or a *password.PolicyError, all of which leave the link usable, or an
	// error wrapping ErrAccountLinkSpent when it failed after the link was
	// used up.
	CompleteSetup(ctx context.Context, token, plaintext, name string) (*models.User, error)
	// CompleteReset completes a reset link the same way, without touching the
	// name. With revokeAPIAccess it also deletes the account's API keys and
	// MCP/OAuth grants.
	CompleteReset(ctx context.Context, token, plaintext string, revokeAPIAccess bool) (*models.User, error)
	// Sweep deletes every consumed or expired link and returns how many.
	Sweep(ctx context.Context) (int64, error)
}

// AccountTokenDeps are the collaborators of AccountTokenService.
type AccountTokenDeps struct {
	Tokens repositories.AccountTokenRepository
	Users  repositories.UserRepository
	// Passwords validates and hashes the new password. The hash is stored by
	// Tokens.Complete, in the completion's transaction.
	Passwords PasswordAuthServiceInterface
	APIAccess APIAccessServiceInterface
	Allowlist AccessAllowlistResolver
	// Notifier is told when a completion has set a password. Nil means nobody.
	Notifier AccountNotifier
	// BaseURL is the web app's base URL (frontend.base_url); links point there.
	BaseURL string
	Logger  *slog.Logger
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// AccountTokenService implements AccountTokenServiceInterface.
type AccountTokenService struct {
	tokens    repositories.AccountTokenRepository
	users     repositories.UserRepository
	passwords PasswordAuthServiceInterface
	apiAccess APIAccessServiceInterface
	allowlist AccessAllowlistResolver
	baseURL   string
	logger    *slog.Logger
	now       func() time.Time
	notifier  AccountNotifier
}

var _ AccountTokenServiceInterface = (*AccountTokenService)(nil)

// NewAccountTokenService creates the account link service.
func NewAccountTokenService(deps AccountTokenDeps) *AccountTokenService {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	notifier := deps.Notifier
	if notifier == nil {
		notifier = noopAccountNotifier{}
	}
	return &AccountTokenService{
		tokens:    deps.Tokens,
		users:     deps.Users,
		passwords: deps.Passwords,
		apiAccess: deps.APIAccess,
		allowlist: deps.Allowlist,
		baseURL:   deps.BaseURL,
		logger:    deps.Logger,
		notifier:  notifier,
		now:       now,
	}
}

// AccountLinkURL returns the link for a token of the given purpose. The token
// is the URL fragment: a browser never sends a fragment to the server, so the
// token stays out of access logs, proxies and Referer headers. The page reads
// it and removes it from the address bar.
func AccountLinkURL(baseURL, purpose, token string) string {
	path := accountResetPagePath
	if purpose == models.AccountTokenPurposeSetup {
		path = accountSetupPagePath
	}
	return strings.TrimRight(baseURL, "/") + path + "#token=" + token
}

// newAccountToken returns a fresh token and the SHA-256 stored in its place.
func newAccountToken() (token string, hash []byte, err error) {
	raw := make([]byte, accountTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("failed to generate account token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, hashAccountToken(token), nil
}

func hashAccountToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// redactAccountToken is what a log line may carry about a token: the first
// characters of its hash, enough to correlate entries and useless to anyone
// who reads the log.
func redactAccountToken(hash []byte) string {
	return hex.EncodeToString(hash)[:8]
}

// IssueLink implements AccountTokenServiceInterface.
func (s *AccountTokenService) IssueLink(
	ctx context.Context, params IssueAccountLinkParams,
) (*IssuedAccountLink, error) {
	token, stored, err := s.issue(ctx, params, models.AccountTokenDeliveryLink)
	if err != nil {
		return nil, err
	}
	return &IssuedAccountLink{
		URL:       AccountLinkURL(s.baseURL, stored.Purpose, token),
		ExpiresAt: stored.ExpiresAt,
	}, nil
}

// IssueByEmail implements AccountTokenServiceInterface.
func (s *AccountTokenService) IssueByEmail(
	ctx context.Context, params IssueAccountLinkParams, send AccountLinkSender,
) (time.Time, error) {
	if send == nil {
		return time.Time{}, fmt.Errorf("%w: no mail sender", ErrInvalidAccountTokenRequest)
	}
	user, err := s.users.GetByID(ctx, params.UserID)
	if err != nil {
		return time.Time{}, fmt.Errorf("load the account to email a link: %w", err)
	}
	token, stored, err := s.issue(ctx, params, models.AccountTokenDeliveryEmail)
	if err != nil {
		return time.Time{}, err
	}

	err = send(ctx, AccountLinkEmail{
		User:      user,
		Purpose:   stored.Purpose,
		URL:       AccountLinkURL(s.baseURL, stored.Purpose, token),
		ExpiresAt: stored.ExpiresAt,
	})
	if err != nil {
		// A link nobody received must not stay live. The delete runs detached
		// from the request so a cancelled request still withdraws it.
		if _, delErr := s.tokens.Delete(context.WithoutCancel(ctx), stored.TokenHash); delErr != nil {
			s.logger.With("user_id", stored.UserID, "token", redactAccountToken(stored.TokenHash),
				"error", delErr.Error()).Error("Failed to withdraw an account link that could not be emailed")
		}
		return time.Time{}, fmt.Errorf("email the account link: %w", err)
	}
	return stored.ExpiresAt, nil
}

// issue validates the request, mints a token and stores its hash.
func (s *AccountTokenService) issue(
	ctx context.Context, params IssueAccountLinkParams, delivery string,
) (string, *models.AccountToken, error) {
	if strings.TrimSpace(params.UserID) == "" {
		return "", nil, fmt.Errorf("%w: no user", ErrInvalidAccountTokenRequest)
	}
	if !models.ValidAccountTokenPurpose(params.Purpose) {
		return "", nil, fmt.Errorf("%w: unknown purpose %q", ErrInvalidAccountTokenRequest, params.Purpose)
	}

	token, hash, err := newAccountToken()
	if err != nil {
		return "", nil, err
	}
	stored := &models.AccountToken{
		UserID:    params.UserID,
		Purpose:   params.Purpose,
		Delivery:  delivery,
		TokenHash: hash,
		ExpiresAt: s.now().Add(models.AccountTokenLifetime(params.Purpose, delivery)),
		CreatedBy: optionalActor(params.CreatedBy),
	}
	err = s.tokens.Replace(ctx, stored, params.MinInterval)
	if errors.Is(err, repositories.ErrAccountTokenTooRecent) {
		return "", nil, ErrAccountLinkTooRecent
	}
	if err != nil {
		return "", nil, fmt.Errorf("store the account link: %w", err)
	}
	s.logger.With(
		"user_id", stored.UserID, "purpose", stored.Purpose, "delivery", stored.Delivery,
		"token", redactAccountToken(hash), "expires_at", stored.ExpiresAt,
	).Info("Issued an account link")
	return token, stored, nil
}

// Preview implements AccountTokenServiceInterface.
func (s *AccountTokenService) Preview(ctx context.Context, token string) (*AccountTokenPreview, error) {
	stored, user, err := s.liveLink(ctx, token, "")
	if err != nil {
		return nil, err
	}
	if err = ensureEmailAllowed(ctx, s.allowlist, s.logger, user.Email, LocalSignInProvider, true); err != nil {
		return nil, err
	}
	return &AccountTokenPreview{
		Purpose:   stored.Purpose,
		Email:     user.Email,
		Name:      user.Name,
		ExpiresAt: stored.ExpiresAt,
	}, nil
}

// liveLink resolves token to its live link and its usable account. purpose,
// when not empty, must be the link's purpose. Every way this can fail that is
// about the link or the account is ErrAccountTokenInvalid.
func (s *AccountTokenService) liveLink(
	ctx context.Context, token, purpose string,
) (*models.AccountToken, *models.User, error) {
	if token == "" || len(token) > accountTokenMaxLength {
		return nil, nil, ErrAccountTokenInvalid
	}
	stored, err := s.tokens.GetLive(ctx, hashAccountToken(token), s.now())
	if errors.Is(err, repositories.ErrAccountTokenNotFound) {
		return nil, nil, ErrAccountTokenInvalid
	}
	if err != nil {
		return nil, nil, fmt.Errorf("look up the account link: %w", err)
	}
	if purpose != "" && stored.Purpose != purpose {
		return nil, nil, ErrAccountTokenInvalid
	}

	user, err := s.users.GetByID(ctx, stored.UserID)
	if errors.Is(err, repositories.ErrUserNotFound) {
		return nil, nil, ErrAccountTokenInvalid
	}
	if err != nil {
		return nil, nil, fmt.Errorf("load the account of an account link: %w", err)
	}
	if user.IsSuspended() {
		return nil, nil, ErrAccountTokenInvalid
	}
	return stored, user, nil
}

// CompleteSetup implements AccountTokenServiceInterface.
func (s *AccountTokenService) CompleteSetup(
	ctx context.Context, token, plaintext, name string,
) (*models.User, error) {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > maxAccountNameLength {
		return nil, fmt.Errorf("%w: at most %d characters", ErrInvalidAccountName, maxAccountNameLength)
	}
	return s.complete(ctx, token, plaintext, completion{purpose: models.AccountTokenPurposeSetup, name: name})
}

// CompleteReset implements AccountTokenServiceInterface.
func (s *AccountTokenService) CompleteReset(
	ctx context.Context, token, plaintext string, revokeAPIAccess bool,
) (*models.User, error) {
	return s.complete(ctx, token, plaintext,
		completion{purpose: models.AccountTokenPurposeReset, revokeAPIAccess: revokeAPIAccess})
}

// completion is what distinguishes completing a setup from completing a reset.
type completion struct {
	purpose         string
	name            string
	revokeAPIAccess bool
}

// complete runs a link to its end.
//
// Everything that can be checked is checked BEFORE the link is consumed (the
// link is live, the account exists and may sign in, the allowlist admits it,
// the password meets the policy), and the password is hashed before it too, so
// a request that fails any of those leaves the link usable and no slow work is
// left for afterwards.
//
// The completion itself is one transaction in the repository: the link is
// consumed, the user's other links are deleted, the password is stored, the
// earlier sessions end and the profile is updated, all or nothing, under the
// lock an account claim also takes (#1462). If it fails nothing was written
// and the link is still live, so the request can simply be repeated.
//
// The one write after it is the optional revocation of API access. It runs
// detached from the request, so a browser that disconnects at that moment does
// not skip it; if it fails the error wraps ErrAccountLinkSpent, because by
// then the link is used and the password is already the new one.
func (s *AccountTokenService) complete(
	ctx context.Context, token, plaintext string, c completion,
) (*models.User, error) {
	stored, user, err := s.liveLink(ctx, token, c.purpose)
	if err != nil {
		return nil, err
	}
	// The address must be admitted by an active access allowlist, as for every
	// sign-in. Provider verification is not required, for the reason given in
	// PasswordAuthService.Login.
	if err = ensureEmailAllowed(ctx, s.allowlist, s.logger, user.Email, LocalSignInProvider, true); err != nil {
		return nil, err
	}
	// Validates against the policy and hashes; a *password.PolicyError passes
	// through unchanged.
	hash, err := s.passwords.HashNewPassword(ctx, plaintext, user.Email)
	if err != nil {
		return nil, err
	}

	done, err := s.tokens.Complete(ctx, repositories.AccountLinkCompletion{
		TokenHash:    stored.TokenHash,
		Now:          s.now(),
		PasswordHash: hash,
		Name:         c.name,
	})
	if errors.Is(err, repositories.ErrAccountTokenNotFound) {
		// Another request completed this link or another link of the account,
		// or the account was claimed, between the check above and here.
		return nil, ErrAccountTokenInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("complete the account link: %w", err)
	}
	user.SessionEpoch = done.SessionEpoch
	user.Name = done.Name
	user.EmailVerifiedAt = done.EmailVerifiedAt
	user.UpdatedAt = done.UpdatedAt
	user.Version = done.Version
	log := s.logger.With("user_id", user.ID, "purpose", done.Token.Purpose, "delivery", done.Token.Delivery,
		"token", redactAccountToken(done.Token.TokenHash))

	// The password is the new one from here on, whatever happens below, so the
	// owner is told now. Never a reason to fail: the notifier sends in the
	// background. It is the mailbox owner's signal that a password was set,
	// which matters most when the link was a copied one.
	s.notifier.PasswordChanged(ctx, user.ID)

	if c.revokeAPIAccess {
		revokeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), accountLinkCompletionTimeout)
		defer cancel()
		if _, err = s.apiAccess.RevokeAPIAccess(revokeCtx, user.ID); err != nil {
			log.With("error", err.Error()).
				Error("An account link was completed but the account's API access could not be revoked")
			return nil, fmt.Errorf("%w: revoke the API access of a completed account link: %w", ErrAccountLinkSpent, err)
		}
	}
	log.Info("Completed an account link")
	return user, nil
}

// Sweep implements AccountTokenServiceInterface.
func (s *AccountTokenService) Sweep(ctx context.Context) (int64, error) {
	removed, err := s.tokens.DeleteDead(ctx, s.now())
	if err != nil {
		return 0, fmt.Errorf("sweep account links: %w", err)
	}
	return removed, nil
}
