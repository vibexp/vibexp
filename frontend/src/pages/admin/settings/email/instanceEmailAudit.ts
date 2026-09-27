import type { AdminInstanceSettingsAuditEntry } from '@/services/adminService'

import {
  type AuditField,
  type AuditFieldChange,
  type AuditSnapshot,
  diffAuditFields,
} from '../instanceSettingsAudit'

/**
 * Presentation rules for the instance email provider's change history (#1191).
 *
 * Audit snapshots are the redacted stored row, diffed over an allowlist
 * (`instanceSettingsAudit.ts`) that also skips delivery health. The
 * credential itself is never in a snapshot; the `secret` marker (`changed` /
 * `unchanged`) is rendered separately.
 */

const FIELD_LABELS: readonly AuditField[] = [
  ['provider_type', 'Provider'],
  ['settings.host', 'SMTP host'],
  ['settings.port', 'SMTP port'],
  ['settings.username', 'SMTP username'],
  ['settings.domain', 'Mailgun domain'],
  ['settings.base_url', 'Mailgun API base URL'],
  ['settings.message_stream', 'Postmark message stream'],
  ['from_address', 'From address'],
  ['from_name', 'Display name'],
  ['reply_to', 'Reply-To'],
  ['contact_recipient_address', 'Contact recipient'],
  ['privacy_policy_url', 'Privacy policy URL'],
  ['has_credential', 'Credential stored'],
]

/** `settings` is the stored per-type block, flat (`{host, port}`), so one level. */
function read(snapshot: AuditSnapshot, key: string): unknown {
  if (!snapshot) return undefined
  if (key.startsWith('settings.')) {
    const settings = snapshot.settings
    if (settings === null || typeof settings !== 'object') return undefined
    return (settings as Record<string, unknown>)[key.slice('settings.'.length)]
  }
  return snapshot[key]
}

/** The allowlisted email fields whose displayed value differs. */
export function auditFieldChanges(
  entry: Pick<AdminInstanceSettingsAuditEntry, 'before' | 'after'>
): AuditFieldChange[] {
  return diffAuditFields(entry, FIELD_LABELS, read)
}

/** The credential marker of an upsert, or null when the entry has none. */
export function credentialChange(
  entry: Pick<AdminInstanceSettingsAuditEntry, 'after'>
): 'changed' | 'unchanged' | null {
  const marker = entry.after?.secret
  return marker === 'changed' || marker === 'unchanged' ? marker : null
}
