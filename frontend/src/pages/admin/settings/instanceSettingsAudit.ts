import type { AdminInstanceSettingsAuditEntry } from '@/services/adminService'

/**
 * Presentation rules shared by every instance settings change history
 * (email #1191, search and AI summary #1202).
 *
 * Audit snapshots also carry bookkeeping (timestamps, `updated_by`, version)
 * that changes without anyone changing a setting, so each section diffs an
 * ALLOWLIST of the fields an admin actually sets.
 */

export type AuditSnapshot = Record<string, unknown> | null

/** An allowlisted field: its snapshot key and the label the history shows. */
export type AuditField = readonly [key: string, label: string]

export interface AuditFieldChange {
  key: string
  label: string
  before: string
  after: string
}

export function formatAuditValue(value: unknown): string {
  if (value === undefined || value === null || value === '') return '—'
  if (typeof value === 'boolean') return value ? 'Yes' : 'No'
  if (typeof value === 'string') return value
  if (typeof value === 'number') return String(value)
  return JSON.stringify(value)
}

const readTopLevel = (snapshot: AuditSnapshot, key: string): unknown =>
  snapshot?.[key]

/**
 * The allowlisted fields whose displayed value differs between the two
 * snapshots. An import or first save (no `before`) lists every set field; a
 * delete (no `after`) lists every field that was set. `read` resolves a key in
 * a snapshot, for sections whose snapshot is not flat.
 */
export function diffAuditFields(
  entry: Pick<AdminInstanceSettingsAuditEntry, 'before' | 'after'>,
  fields: readonly AuditField[],
  read: (snapshot: AuditSnapshot, key: string) => unknown = readTopLevel
): AuditFieldChange[] {
  const changes: AuditFieldChange[] = []
  for (const [key, label] of fields) {
    const before = formatAuditValue(read(entry.before, key))
    const after = formatAuditValue(read(entry.after, key))
    if (before !== after) changes.push({ key, label, before, after })
  }
  return changes
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

/**
 * The label of a settings section that has defaults to fall back on: removing
 * its stored row is what "Reset to defaults" does.
 */
export function resettableAuditActionLabel(
  action: AdminInstanceSettingsAuditEntry['action']
): string {
  return action === 'delete' ? 'Reset to defaults' : auditActionLabel(action)
}

/** Who made the change: the boot-time import has no actor by design. */
export function auditActorLabel(
  entry: Pick<AdminInstanceSettingsAuditEntry, 'action' | 'actor_name'>
): string {
  if (entry.action === 'import') return 'Imported from config.yaml'
  return entry.actor_name ?? 'Deleted user'
}
