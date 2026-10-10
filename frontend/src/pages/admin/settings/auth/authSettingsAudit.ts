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

/** The setup events the server records itself, with no user involved. */
const SERVER_SETUP_EVENTS: readonly unknown[] = ['token_minted', 'rearmed']

/**
 * Who made an authentication settings change, saying only what the entry
 * supports. An entry has no actor for one of four reasons, and they cannot
 * always be told apart:
 *
 * - the break-glass CLI wrote it, which stamps `source: cli`;
 * - the server minted or re-armed a setup token (`auth_setup`), which no user
 *   does;
 * - it was written on a setup session, which has no user. Only the providers
 *   and the allowlist can be, and nothing in the entry marks it;
 * - the admin who made it has since been deleted (the audit row keeps the
 *   change and drops the reference), which nothing marks either.
 *
 * So an unmarked actorless provider or allowlist entry is one of the last two
 * and is labelled as both, never as a deleted user alone or a setup session
 * alone; for the other settings it can only be a deleted user.
 */
export function authAuditActorLabel(
  entry: Pick<
    AdminInstanceSettingsAuditEntry,
    'setting' | 'action' | 'actor_name' | 'before' | 'after'
  >
): string {
  if (entry.action === 'import' || entry.actor_name !== null) {
    return auditActorLabel(entry)
  }
  if (madeFromCli(entry)) return 'Server CLI'
  switch (entry.setting) {
    case 'auth_setup':
      return SERVER_SETUP_EVENTS.includes(entry.after?.event)
        ? 'Server'
        : auditActorLabel(entry)
    case 'auth_providers':
    case 'auth_allowlist':
      return 'Setup session or deleted user'
    default:
      return auditActorLabel(entry)
  }
}
