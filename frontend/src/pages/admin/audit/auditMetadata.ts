/**
 * How an audit event's free-form `metadata` reads in the detail drawer (#1517).
 *
 * The recorders use three shapes for "what changed", and the drawer shows all
 * of them as the same old → new rows:
 *
 * - a resource update (#1513): `changed_fields` plus `<field>: {old, new}`
 *   for the small scalar fields (a content field is named, without values);
 * - a move: `from_project_*` / `to_project_*`;
 * - a settings change (#1512): `before` / `after` snapshots, where `null` means
 *   the built-in defaults applied.
 *
 * Every other key is listed as a detail. Kept pure so each shape is tested
 * without rendering.
 */

export interface AuditChange {
  field: string
  /** Rendered old value; `undefined` when there was none. */
  old?: string
  new?: string
  /**
   * The field changed but its values were not recorded — resource content is
   * never put in the audit log (epic #1510, decision 4), so `changed_fields`
   * names it without an old/new pair.
   */
  valuesWithheld?: boolean
}

export interface AuditDetail {
  key: string
  value: string
}

export interface DescribedMetadata {
  changes: AuditChange[]
  details: AuditDetail[]
}

/** Shown for a settings snapshot side that was "nothing stored". */
export const DEFAULTS_LABEL = 'Built-in defaults'

type JsonObject = Record<string, unknown>

function isObject(value: unknown): value is JsonObject {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** A value as one line of text: scalars as-is, structures as compact JSON. */
export function formatAuditValue(value: unknown): string {
  if (value === null || value === undefined) return '—'
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value)
  }
  return JSON.stringify(value)
}

/** A `{old, new}` pair, as a resource update records one changed field. */
function isOldNewPair(
  value: unknown
): value is { old?: unknown; new?: unknown } {
  if (!isObject(value)) return false
  const keys = Object.keys(value)
  return keys.length > 0 && keys.every(key => key === 'old' || key === 'new')
}

/** The `before` → `after` rows of a settings change, one per setting. */
function snapshotChanges(before: unknown, after: unknown): AuditChange[] {
  const beforeObj = isObject(before) ? before : undefined
  const afterObj = isObject(after) ? after : undefined
  const keys = [
    ...new Set([
      ...Object.keys(beforeObj ?? {}),
      ...Object.keys(afterObj ?? {}),
    ]),
  ]
  return keys.map(field => ({
    field,
    old: beforeObj ? formatAuditValue(beforeObj[field]) : DEFAULTS_LABEL,
    new: afterObj ? formatAuditValue(afterObj[field]) : DEFAULTS_LABEL,
  }))
}

/** The project a move left and entered, by name where recorded. */
function moveChange(metadata: JsonObject): AuditChange | undefined {
  if (!('from_project_id' in metadata) && !('to_project_id' in metadata)) {
    return undefined
  }
  const side = (prefix: 'from' | 'to') =>
    formatAuditValue(
      metadata[`${prefix}_project_name`] ?? metadata[`${prefix}_project_id`]
    )
  return { field: 'project', old: side('from'), new: side('to') }
}

/** Keys consumed by a change row, so they are not listed again as details. */
const MOVE_KEYS = new Set([
  'from_project_id',
  'from_project_name',
  'to_project_id',
  'to_project_name',
])

export function describeAuditMetadata(
  metadata: Readonly<JsonObject> | null | undefined
): DescribedMetadata {
  const changes: AuditChange[] = []
  const details: AuditDetail[] = []
  if (!metadata) return { changes, details }

  const move = moveChange(metadata)
  if (move) changes.push(move)

  const hasSnapshots = 'before' in metadata || 'after' in metadata
  if (hasSnapshots) {
    changes.push(...snapshotChanges(metadata.before, metadata.after))
  }

  for (const [key, value] of Object.entries(metadata)) {
    if (MOVE_KEYS.has(key)) continue
    if (hasSnapshots && (key === 'before' || key === 'after')) continue
    // `changed_fields` names the fields; the pairs spell out the values.
    if (key === 'changed_fields') continue
    if (isOldNewPair(value)) {
      changes.push({
        field: key,
        old: 'old' in value ? formatAuditValue(value.old) : undefined,
        new: 'new' in value ? formatAuditValue(value.new) : undefined,
      })
      continue
    }
    details.push({ key, value: formatAuditValue(value) })
  }

  // A changed field the recorder named but gave no values for (content).
  const changedFields = Array.isArray(metadata.changed_fields)
    ? metadata.changed_fields.filter(
        (field): field is string => typeof field === 'string'
      )
    : []
  for (const field of changedFields) {
    if (changes.some(change => change.field === field)) continue
    changes.push({ field, valuesWithheld: true })
  }
  return { changes, details }
}
