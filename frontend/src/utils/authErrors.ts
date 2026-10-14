import { ApiError } from '../types/errors'

/**
 * Stable code the backend emits when the access allowlist denies a sign-in.
 *
 * Both denial paths carry it: the identity-provider callback redirects to
 * `/auth/callback?error=access_restricted`, and dev login answers 403 with the
 * same code in its RFC 9457 body. Lowercase by contract — mirrors
 * `CodeAccessRestricted` in `backend/internal/errors/codes.go`.
 */
export const ACCESS_RESTRICTED_CODE = 'access_restricted'

/** The recovery action of every sign-in refusal that sends the person back. */
const BACK_TO_SIGN_IN = 'Back to sign in'

/**
 * User-facing view for a failed sign-in: the alert wording plus the recovery
 * action rendered beneath it.
 */
export interface AuthErrorView {
  title: string
  description: string
  actionLabel: string
  actionHref: string
}

/**
 * Shared wording for an allowlist denial, so the callback alert and the
 * dev-login form read identically.
 */
export const ACCESS_RESTRICTED_MESSAGE =
  'Access to this application is restricted to certain users. Your account is not authorized to sign in.'

export const ACCESS_RESTRICTED_ERROR: AuthErrorView = {
  title: 'Access restricted',
  description: ACCESS_RESTRICTED_MESSAGE,
  actionLabel: BACK_TO_SIGN_IN,
  actionHref: '/login',
}

/**
 * Stable code the backend's identity-provider callback redirects with
 * (`/auth/callback?error=email_not_verified`) when the sign-in matched an
 * existing account by email but the provider did not verify that email, so the
 * sign-in was not linked to the account. Mirrors
 * `callbackErrorEmailNotVerified` in `backend/internal/server/auth_handlers.go`.
 */
export const EMAIL_NOT_VERIFIED_CODE = 'email_not_verified'

export const EMAIL_NOT_VERIFIED_ERROR: AuthErrorView = {
  title: 'Email not verified',
  description:
    'An account with this email already exists, but your sign-in provider has not verified the address. Verify your email with the provider and try again, or sign in the way you did before.',
  actionLabel: BACK_TO_SIGN_IN,
  actionHref: '/login',
}

/**
 * Stable code the callback redirects with
 * (`/auth/callback?error=existing_sign_in_required`) when the sign-in matched
 * an existing account by email, but that account's email was never verified
 * and it already has a sign-in method of its own. Signing in that way once
 * verifies the account; after that other providers with the same email are
 * accepted. Mirrors `callbackErrorExistingSignInRequired` in
 * `backend/internal/server/auth_handlers.go`.
 */
export const EXISTING_SIGN_IN_REQUIRED_CODE = 'existing_sign_in_required'

export const EXISTING_SIGN_IN_REQUIRED_ERROR: AuthErrorView = {
  title: 'Sign in the way you did before',
  description:
    'An account with this email already exists and uses a different sign-in method. Sign in with that method first. After that you can use this one too. If the account is not yours, contact your administrator.',
  actionLabel: BACK_TO_SIGN_IN,
  actionHref: '/login',
}

/**
 * Stable code the callback redirects with
 * (`/auth/callback?error=registration_closed`) when the sign-in would have
 * created a new account, but this instance only admits people who were invited
 * to a team or whose account an administrator created. Mirrors
 * `callbackErrorRegistrationClosed` in
 * `backend/internal/server/auth_handlers.go`.
 */
export const REGISTRATION_CLOSED_CODE = 'registration_closed'

export const REGISTRATION_CLOSED_ERROR: AuthErrorView = {
  title: 'Sign-up is by invitation only',
  description:
    'You do not have an account here yet, and new accounts need an invitation. Ask a team admin to invite this email address, or ask your administrator to create an account for you, then sign in again.',
  actionLabel: BACK_TO_SIGN_IN,
  actionHref: '/login',
}

/**
 * Fallback for a cancelled login or any error code we don't recognise — the
 * denial is not attributable, so we keep the pre-existing generic wording.
 */
export const GENERIC_AUTH_ERROR: AuthErrorView = {
  title: 'Authentication failed',
  description: 'Authentication was cancelled or failed',
  actionLabel: 'Try again',
  actionHref: '/',
}

/**
 * Stable code the callback redirects with
 * (`/auth/callback?error=identity_disconnected`) when the sign-in came through
 * a provider account the account's owner disconnected (#1460). Mirrors
 * `callbackErrorIdentityDisconnected` in `backend/internal/server/auth_handlers.go`.
 */
export const IDENTITY_DISCONNECTED_CODE = 'identity_disconnected'

export const IDENTITY_DISCONNECTED_ERROR: AuthErrorView = {
  title: 'This sign-in was disconnected',
  description:
    'That sign-in was disconnected from your account here, so it was not let in. Sign in another way, then allow it again under Sign-in methods in Account settings.',
  actionLabel: BACK_TO_SIGN_IN,
  actionHref: '/login',
}

/**
 * Map the `?error=` query param on `/auth/callback` to the alert to render.
 * Unknown and absent codes fall through to {@link GENERIC_AUTH_ERROR}.
 */
export function mapAuthCallbackError(code: string | null): AuthErrorView {
  if (code === ACCESS_RESTRICTED_CODE) {
    return ACCESS_RESTRICTED_ERROR
  }
  if (code === EMAIL_NOT_VERIFIED_CODE) {
    return EMAIL_NOT_VERIFIED_ERROR
  }
  if (code === EXISTING_SIGN_IN_REQUIRED_CODE) {
    return EXISTING_SIGN_IN_REQUIRED_ERROR
  }
  if (code === REGISTRATION_CLOSED_CODE) {
    return REGISTRATION_CLOSED_ERROR
  }
  if (code === IDENTITY_DISCONNECTED_CODE) {
    return IDENTITY_DISCONNECTED_ERROR
  }
  return GENERIC_AUTH_ERROR
}

/**
 * Map an error thrown by a sign-in call to the message to show in the form.
 *
 * An allowlist denial is mapped by code to the shared restriction wording; any
 * other `Error` keeps its own message (for an {@link ApiError} that is the
 * backend's `detail`), and a non-Error falls back to `fallback`.
 */
export function mapSignInError(err: unknown, fallback: string): string {
  if (err instanceof ApiError && err.code === ACCESS_RESTRICTED_CODE) {
    return ACCESS_RESTRICTED_MESSAGE
  }
  if (err instanceof Error) {
    return err.message
  }
  return fallback
}

/**
 * The one message for a failed password sign-in (#1431). A wrong password and
 * a locked account get the same 401, so the client cannot tell them apart and
 * must not try: the text covers both.
 */
export const PASSWORD_SIGN_IN_FAILED_MESSAGE =
  'Incorrect email or password. After several failed attempts, wait a few minutes before trying again.'

/**
 * What to tell a person whose password sign-in failed, by the response status.
 * Credential failures stay here and never reach `useErrorHandler`, which reads
 * `AUTH_*` codes as an expired session.
 */
export function passwordSignInErrorMessage(err: unknown): string {
  const status = err instanceof ApiError ? err.status : null
  switch (status) {
    case 401:
      return PASSWORD_SIGN_IN_FAILED_MESSAGE
    case 400:
      return 'Check the email address and password, then try again.'
    case 403:
      return ACCESS_RESTRICTED_MESSAGE
    case 404:
      return 'Password sign-in is switched off on this instance. Sign in with a provider instead.'
    case 429:
      return 'Too many sign-in attempts from this device. Wait a minute, then try again.'
    default:
      return 'Sign-in failed. Try again in a moment.'
  }
}
