import type { AdminInstanceSettingsAuditEntry } from '@/services/adminService'
import type { AdminAuthSettingsAuditSetting } from '@/services/authSettingsService'

import {
  auditActionLabel,
  auditActorLabel,
  type AuditField,
  type AuditFieldChange,
  type AuditSnapshot,
  diffAuditFields,
} from '../instanceSettingsAudit'

/**
 * Presentation rules for the four authentication settings' change histories
 * (#1239). Each setting diffs an allowlist of the fields an admin actually
 * sets (`instanceSettingsAudit.ts`). A client secret is never in a snapshot:
 * only its `changed` / `unchanged` marker is, rendered as a badge.
 */

export interface AuthAuditSection {
  setting: AdminAuthSettingsAuditSetting
  /** The tab's label. */
  label: string
  description: string
  fields: readonly AuditField[]
  actionLabel: (action: AdminInstanceSettingsAuditEntry['action']) => string
}

type Action = AdminInstanceSettingsAuditEntry['action']

const labelled =
  (labels: Partial<Record<Action, string>>) =>
  (action: Action): string =>
    labels[action] ?? auditActionLabel(action)

export const AUTH_AUDIT_SECTIONS: readonly AuthAuditSection[] = [
  {
    setting: 'auth_providers',
    label: 'Providers',
    description:
      'Every change to the sign-in providers. Client secrets are never recorded, only whether one changed.',
    fields: [
      ['slug', 'Slug'],
      ['type', 'Type'],
      ['display_name', 'Display name'],
      ['enabled', 'Enabled'],
      ['sort_order', 'Position'],
      ['client_id', 'Client ID'],
      ['issuer_url', 'Issuer URL'],
    ],
    actionLabel: labelled({ delete: 'Deleted' }),
  },
  {
    setting: 'auth_allowlist',
    label: 'Allowlist',
    description: 'Every change to the access allowlist.',
    fields: [
      ['domains', 'Allowed domains'],
      ['emails', 'Allowed email addresses'],
    ],
    actionLabel: labelled({ delete: 'Reset to open access' }),
  },
  {
    setting: 'instance_admins',
    label: 'Admins',
    description: 'Every instance admin grant and revocation.',
    fields: [
      ['user_id', 'User ID'],
      ['granted_by', 'Granted by (user ID)'],
    ],
    actionLabel: labelled({ upsert: 'Granted', delete: 'Revoked' }),
  },
  {
    setting: 'auth_setup',
    label: 'Setup',
    description:
      'Setup mode events: a setup token minted or re-armed, and setup completed by a root admin sign-in.',
    fields: [
      ['event', 'Event'],
      ['rearmed', 'Re-armed'],
      ['expires_at', 'Token valid until'],
      ['consumed_at', 'Completed at'],
    ],
    actionLabel: labelled({ upsert: 'Recorded' }),
  },
]

/** A list is shown comma-separated, not as JSON. */
function read(snapshot: AuditSnapshot, key: string): unknown {
  const value = snapshot?.[key]
  return Array.isArray(value) ? value.join(', ') : value
}

/** The allowlisted fields of `section` whose displayed value differs. */
export function authAuditChanges(
  entry: Pick<AdminInstanceSettingsAuditEntry, 'before' | 'after'>,
  section: Pick<AuthAuditSection, 'fields'>
): AuditFieldChange[] {
  return diffAuditFields(entry, section.fields, read)
}

/** The client secret marker of a provider save, or null when it has none. */
export function clientSecretChange(
  entry: Pick<AdminInstanceSettingsAuditEntry, 'after'>
): 'changed' | 'unchanged' | null {
  const marker = entry.after?.client_secret
  return marker === 'changed' || marker === 'unchanged' ? marker : null
}

/**
 * Whether the change came from the break-glass CLI (`vibexp admin auth …`)
 * rather than the API. The marker is on `after` for a save and on `before` for
 * a removal, whose `after` is null.
 */
export function madeFromCli(
  entry: Pick<AdminInstanceSettingsAuditEntry, 'before' | 'after'>
): boolean {
  return entry.after?.source === 'cli' || entry.before?.source === 'cli'
}

/**
 * Who made an authentication settings change. Unlike the other instance
 * settings, a change here can have no actor by design: the break-glass CLI
 * runs on the server with no user, and a setup session (`/setup`) exists
 * precisely because nobody can sign in yet. Only those two write without one,
 * so a missing actor is never read as a deleted user.
 */
export function authAuditActorLabel(
  entry: Pick<
    AdminInstanceSettingsAuditEntry,
    'action' | 'actor_name' | 'before' | 'after'
  >
): string {
  if (entry.action === 'import' || entry.actor_name !== null) {
    return auditActorLabel(entry)
  }
  return madeFromCli(entry) ? 'Server CLI' : 'Setup session (no signed-in user)'
}
