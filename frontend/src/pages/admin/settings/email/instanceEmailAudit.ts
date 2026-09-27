import type { AdminInstanceSettingsAuditEntry } from '@/services/adminService'

/**
 * Presentation rules for the instance email provider's change history (#1191).
 *
 * Audit snapshots are the redacted stored row: they also carry bookkeeping
 * (timestamps, delivery health, `updated_by`) that changes without anyone
 * changing the configuration, so the diff shows an ALLOWLIST of the fields an
 * admin actually sets. The credential itself is never in a snapshot; the
 * `secret` marker (`changed` / `unchanged`) is rendered separately.
 */

type Snapshot = Record<string, unknown> | null

const FIELD_LABELS: readonly (readonly [string, string])[] = [
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

export interface AuditFieldChange {
  key: string
  label: string
  before: string
  after: string
}

/** `settings` is the stored per-type block, flat (`{host, port}`), so one level. */
function read(snapshot: Snapshot, key: string): unknown {
  if (!snapshot) return undefined
  if (key.startsWith('settings.')) {
    const settings = snapshot.settings
    if (settings === null || typeof settings !== 'object') return undefined
    return (settings as Record<string, unknown>)[key.slice('settings.'.length)]
  }
  return snapshot[key]
}

export function formatAuditValue(value: unknown): string {
  if (value === undefined || value === null || value === '') return '—'
  if (typeof value === 'boolean') return value ? 'Yes' : 'No'
  if (typeof value === 'string') return value
  if (typeof value === 'number') return String(value)
  return JSON.stringify(value)
}

/**
 * The allowlisted fields whose displayed value differs between the two
 * snapshots. An import or first save (no `before`) lists every set field; a
 * delete (no `after`) lists every field that was set.
 */
export function auditFieldChanges(
  entry: Pick<AdminInstanceSettingsAuditEntry, 'before' | 'after'>
): AuditFieldChange[] {
  const changes: AuditFieldChange[] = []
  for (const [key, label] of FIELD_LABELS) {
    const before = formatAuditValue(read(entry.before, key))
    const after = formatAuditValue(read(entry.after, key))
    if (before !== after) changes.push({ key, label, before, after })
  }
  return changes
}

/** The credential marker of an upsert, or null when the entry has none. */
export function credentialChange(
  entry: Pick<AdminInstanceSettingsAuditEntry, 'after'>
): 'changed' | 'unchanged' | null {
  const marker = entry.after?.secret
  return marker === 'changed' || marker === 'unchanged' ? marker : null
}

export function auditActionLabel(
  action: AdminInstanceSettingsAuditEntry['action']
): string {
  switch (action) {
    case 'upsert':
      return 'Saved'
    case 'delete':
      return 'Removed'
    case 'import':
      return 'Imported'
  }
}

/** Who made the change: the boot-time import has no actor by design. */
export function auditActorLabel(
  entry: Pick<AdminInstanceSettingsAuditEntry, 'action' | 'actor_name'>
): string {
  if (entry.action === 'import') return 'Imported from config.yaml'
  return entry.actor_name ?? 'Deleted user'
}
