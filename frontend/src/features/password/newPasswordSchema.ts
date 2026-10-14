import { z } from 'zod'

import { PASSWORD_MAX_LENGTH, PASSWORD_MIN_LENGTH } from '@/lib/passwordPolicy'

/**
 * The new-password pair of a form (epic #1426): `password` and its repeat,
 * `confirmPassword`. Spread `newPasswordFields` into a page's `z.object`, then
 * apply `passwordsMatch` to the object, so the page's own fields sit beside
 * them:
 *
 *   z.object({ name: z.string(), ...newPasswordFields }).superRefine(passwordsMatch)
 *
 * The server applies the full policy (common passwords, the email address);
 * this catches what a person can fix before a request.
 */
export const newPasswordFields = {
  password: z
    .string()
    .min(1, 'Enter a password.')
    .min(
      PASSWORD_MIN_LENGTH,
      `Use at least ${String(PASSWORD_MIN_LENGTH)} characters.`
    )
    .max(
      PASSWORD_MAX_LENGTH,
      `Use at most ${String(PASSWORD_MAX_LENGTH)} characters.`
    ),
  confirmPassword: z.string(),
}

/** Puts "The two passwords do not match." on `confirmPassword`. */
export function passwordsMatch(
  values: { password: string; confirmPassword: string },
  ctx: z.RefinementCtx
): void {
  if (values.confirmPassword !== values.password) {
    ctx.addIssue({
      code: 'custom',
      path: ['confirmPassword'],
      message: 'The two passwords do not match.',
    })
  }
}

/** The pair alone, for a form that has no other fields. */
export const newPasswordSchema = z
  .object(newPasswordFields)
  .superRefine(passwordsMatch)

export type NewPasswordValues = z.infer<typeof newPasswordSchema>
