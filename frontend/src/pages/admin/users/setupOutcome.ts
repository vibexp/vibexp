import { formatDateTime } from '@/lib/time'
import type { AdminUserSetup } from '@/services/adminService'
import { ApiError } from '@/types/errors'

/**
 * What the admin is told about a new account's setup link (#1443), from the
 * `setup` of `POST /admin/users`. The account exists in every case.
 */
export interface SetupOutcome {
  tone: 'info' | 'warning'
  title: string
  text: string
  /** The one-time link to show, for `delivery: link` only. */
  link: string | null
}

const NONE_REASONS: Readonly<Record<string, string>> = {
  password_login_disabled:
    'Password sign-in is switched off on this instance, so no setup link was made. They can sign in through an identity provider with this email address.',
  root_admin_link_refused:
    'This address is a root admin and no email provider is configured, so a setup link was not shown here: it would hand over a root admin account. Set up email, or give them a password from the setup page or the recovery command.',
  email_failed:
    'The setup email could not be sent. Get a setup link from the user page to try again.',
  setup_failed:
    'The setup link could not be made. Get one from the user page to try again.',
}

export function setupOutcome(
  email: string,
  setup: AdminUserSetup
): SetupOutcome {
  const until = setup.expires_at
    ? ` It works until ${formatDateTime(setup.expires_at)}.`
    : ''
  switch (setup.delivery) {
    case 'email':
      return {
        tone: 'info',
        title: 'Setup link emailed',
        text: `A link to set their password was emailed to ${email}.${until}`,
        link: null,
      }
    case 'link':
      return {
        tone: 'info',
        title: 'Setup link',
        text: `No email provider is configured, so send this link to ${email} yourself. It lets them set their password once.${until}`,
        link: setup.link_url ?? null,
      }
    default:
      return {
        tone: 'warning',
        title: 'No setup link',
        text:
          NONE_REASONS[setup.reason ?? ''] ??
          'No setup link was made. Get one from the user page.',
        link: null,
      }
  }
}

/**
 * The server's refusals of individual team rows on create, by row index: a
 * 400 whose `validation_errors` name `teams[i]`.
 */
export function teamRowErrors(err: unknown): Record<number, string> {
  const rows: Record<number, string> = {}
  if (!(err instanceof ApiError) || err.status !== 400) return rows
  for (const { field, message } of err.validationErrors ?? []) {
    const match = /^teams\[(\d+)\]/.exec(field)
    if (match) rows[Number(match[1])] ??= message
  }
  return rows
}
