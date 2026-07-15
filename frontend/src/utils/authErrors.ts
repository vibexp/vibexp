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
  actionLabel: 'Back to sign in',
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
 * Map the `?error=` query param on `/auth/callback` to the alert to render.
 * Unknown and absent codes fall through to {@link GENERIC_AUTH_ERROR}.
 */
export function mapAuthCallbackError(code: string | null): AuthErrorView {
  if (code === ACCESS_RESTRICTED_CODE) {
    return ACCESS_RESTRICTED_ERROR
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
