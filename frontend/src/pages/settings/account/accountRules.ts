import type { Account, AccountIdentity } from '@/services/accountService'
import { ApiError } from '@/types/errors'

/**
 * Presentation rules for the account page (#1441). The server is the
 * authority on every one of them; these only let the page explain a refusal
 * before the person runs into it.
 */

/** The written password rule, the same wording as the setup page. */
export const PASSWORD_RULE =
  'At least 12 characters. Not a common password, and not your email address.'

export const PASSWORD_MIN_LENGTH = 12
/** The API's bound on a password field, in characters. */
export const PASSWORD_MAX_LENGTH = 1024
export const NAME_MAX_LENGTH = 255

/** Sent to the sign-in page so signing in again comes back here. */
export const SIGN_IN_AGAIN_HREF = '/login?return_to=/settings/account'

/**
 * Whether the account's password is a way to sign in: it has one and password
 * sign-in is on. The page cannot see a password an administrator disabled, so
 * the server's `last_sign_in_method` refusal is still handled.
 */
function passwordUsable(account: Account): boolean {
  return account.password.set && account.password.login_enabled
}

function usableIdentities(account: Account): AccountIdentity[] {
  return account.identities.filter(identity => identity.available)
}

/** The password can go while an enabled provider still signs the person in. */
export function canRemovePassword(account: Account): boolean {
  return usableIdentities(account).length > 0
}

/** An identity can go while the password or another enabled provider remains. */
export function canDisconnect(
  account: Account,
  identity: AccountIdentity
): boolean {
  return (
    passwordUsable(account) ||
    usableIdentities(account).some(other => other.id !== identity.id)
  )
}

/** The stable `code` of an API error, or null for anything else. */
export function errorCode(err: unknown): string | null {
  return err instanceof ApiError ? err.code : null
}

export function isReauthenticationRequired(err: unknown): boolean {
  return errorCode(err) === 'reauthentication_required'
}

/**
 * What to tell the person about a refused account change. The codes are the
 * ones the API documents as stable; anything else shows the server's own
 * message.
 */
export function accountErrorMessage(err: unknown, fallback: string): string {
  switch (errorCode(err)) {
    case 'reauthentication_required':
      return 'This needs a recent sign-in. Sign in again, then repeat the change.'
    case 'cookie_session_required':
      return 'Sign-in methods can only be changed from a browser session, not with an API key or a connected tool.'
    case 'password_login_disabled':
      return 'Password sign-in is switched off on this instance, so a password cannot be set or changed.'
    case 'last_sign_in_method':
      return 'This is the only way left to sign in to your account, so it cannot be removed. Add another sign-in method first.'
    case 'too_many_attempts':
      return 'Too many wrong passwords. Password sign-in for your account is locked for a short time; wait a few minutes and try again.'
    default:
      return err instanceof Error && err.message ? err.message : fallback
  }
}
