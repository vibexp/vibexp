import { ApiError } from '@/types/errors'

import { messageOf } from './publicForms'

/**
 * Reading a failed request on the one-time account link pages (setup #1440,
 * reset #1439).
 */

export function statusOf(err: unknown): number | null {
  return err instanceof ApiError ? err.status : null
}

/** Why a link's preview failed, as the page shows it. */
export type PreviewFailure =
  /** Unusable (expired, used, unknown) or not a token at all (400). */
  | { kind: 'invalid' }
  /** The allowlist does not admit the account's email: retrying cannot help. */
  | { kind: 'restricted'; message: string }
  /** Password sign-in is off on the instance (404). */
  | { kind: 'password-off' }
  /** A passing failure: the link may still work. */
  | { kind: 'refused'; message: string }

export function previewFailure(err: unknown): PreviewFailure {
  const status = statusOf(err)
  // 400: a token that cannot be one (oversized, malformed).
  if (status === 401 || status === 400) return { kind: 'invalid' }
  if (status === 403)
    return {
      kind: 'restricted',
      message: messageOf(err, 'Access to this instance is restricted.'),
    }
  if (status === 404) return { kind: 'password-off' }
  return {
    kind: 'refused',
    message: messageOf(err, 'The link could not be checked'),
  }
}
