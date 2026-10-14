package services

import "context"

// UnverifiedAccountClaimHook runs when a sign-in provider's VERIFIED email
// claims an existing account whose email had never been verified and which has
// no sign-in identity yet (#1430).
//
// Such an account was set up by whoever held a link to it — an admin-created
// account's setup link and an invitation are bearer links — which need not be
// the mailbox's owner. The person who proves ownership takes the account over,
// so what the setup left behind has to go. AuthService itself ends the
// account's cookie sessions right after the hook; the hook is for credentials
// AuthService does not know about. The password credential (#1431) plugs in
// here.
//
// Contract:
//   - it is called BEFORE the account is marked verified and before the
//     identity is attached, so nothing is linked when it fails: an error
//     aborts the sign-in;
//   - it must be idempotent: a sign-in that fails after the hook ran calls it
//     again on the next attempt;
//   - it is not called for an account whose email is already verified, nor for
//     one that already has a sign-in identity (that link is refused, see
//     ErrExistingSignInRequired);
//   - the caller decides from a row read before the hook runs. A hook that
//     finds, under its own lock, that the account changed since (verified, or
//     given an identity) returns an error wrapping
//     repositories.ErrAccountNoLongerClaimable and changes nothing; the caller
//     re-reads the account and carries on from what is true now (#1466).
type UnverifiedAccountClaimHook interface {
	OnUnverifiedAccountClaimed(ctx context.Context, userID string) error
}

// noopUnverifiedAccountClaimHook is the default: until password credentials
// exist, an account with no sign-in identity holds nothing for it to remove.
type noopUnverifiedAccountClaimHook struct{}

func (noopUnverifiedAccountClaimHook) OnUnverifiedAccountClaimed(context.Context, string) error {
	return nil
}

// AuthServiceOption customizes an AuthService at construction.
type AuthServiceOption func(*AuthService)

// WithUnverifiedAccountClaimHook replaces the default no-op hook. A nil hook is
// ignored.
func WithUnverifiedAccountClaimHook(hook UnverifiedAccountClaimHook) AuthServiceOption {
	return func(as *AuthService) {
		if hook != nil {
			as.claimHook = hook
		}
	}
}

// DisconnectedIdentityChecker says whether an account's owner disconnected a
// provider account (#1460). SignInMethodRepository satisfies it.
type DisconnectedIdentityChecker interface {
	IsDisconnected(ctx context.Context, userID, provider, subject string) (bool, error)
}

// WithDisconnectedIdentities makes sign-in refuse to link, by email, a provider
// account the owner disconnected. Without it nothing is refused.
func WithDisconnectedIdentities(checker DisconnectedIdentityChecker) AuthServiceOption {
	return func(as *AuthService) {
		as.disconnected = checker
	}
}
