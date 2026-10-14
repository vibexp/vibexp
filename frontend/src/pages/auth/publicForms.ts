import { z } from 'zod'

import { type AuthProvider, authService } from '@/services/authService'

/**
 * What the public pages that create an account share (#1440): the optional
 * display name, the error wording and the provider list.
 */

/** The API's bound on a display name. */
export const NAME_MAX_LENGTH = 255

/** An optional display name: trimmed, empty means "leave it". */
export const optionalNameField = z
  .string()
  .trim()
  .max(NAME_MAX_LENGTH, `Use at most ${String(NAME_MAX_LENGTH)} characters.`)

export function messageOf(err: unknown, fallback: string): string {
  return err instanceof Error && err.message ? err.message : fallback
}

/**
 * The enabled sign-in providers. A failure only costs the alternative to a
 * password, so it is an empty list rather than a broken page.
 */
export async function enabledProviders(): Promise<AuthProvider[]> {
  try {
    return (await authService.getSignInMethods()).providers
  } catch {
    return []
  }
}
