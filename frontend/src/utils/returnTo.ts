import { STORAGE_KEYS } from '@/constants/storageKeys'
import { sessionStore } from '@/utils/storage'

/**
 * Where to send a user after a login round-trip when no (or an unsafe)
 * `return_to` was supplied. The app root.
 */
export const DEFAULT_RETURN_TO = '/'

/**
 * Validate a `return_to` target to a same-origin INTERNAL path, defending
 * against open redirects. Only a single leading slash is accepted; a
 * protocol-relative (`//host`) or backslash (`/\host`) target — which browsers
 * resolve to an absolute, cross-origin URL — is rejected, as is any absolute
 * URL or non-path string. Anything rejected falls back to {@link DEFAULT_RETURN_TO}.
 */
export function sanitizeReturnTo(raw: string | null | undefined): string {
  if (typeof raw !== 'string') return DEFAULT_RETURN_TO
  // Browsers strip tab/newline/CR from a URL before parsing (WHATWG URL spec),
  // so e.g. "/\t/\tevil.com" would collapse to a protocol-relative "//evil.com"
  // after the checks below. Strip them first and validate the resolved value.
  const path = raw.replace(/[\t\n\r]/g, '')
  if (!path.startsWith('/')) return DEFAULT_RETURN_TO
  if (path.startsWith('//') || path.startsWith('/\\')) return DEFAULT_RETURN_TO
  return path
}

/**
 * Persist the validated `return_to` path so it survives the login round-trip
 * (the provider flow bounces to the IdP and back, and sessionStorage is the
 * only state that survives). Always stores a sanitized value.
 */
export function stashReturnTo(raw: string | null | undefined): void {
  sessionStore.set(STORAGE_KEYS.RETURN_TO, sanitizeReturnTo(raw))
}

/**
 * Read and clear the stashed `return_to` path, returning a sanitized value
 * (defaulting to {@link DEFAULT_RETURN_TO} when absent or unsafe). Single-use:
 * the key is removed so it cannot resume a second time.
 */
export function consumeReturnTo(): string {
  const stored = sessionStore.get(STORAGE_KEYS.RETURN_TO)
  sessionStore.remove(STORAGE_KEYS.RETURN_TO)
  return sanitizeReturnTo(stored)
}
