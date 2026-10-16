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

/** Whether the metadata is a settings change: `before`/`after` snapshots. */
function hasSnapshots(metadata: Readonly<JsonObject>): boolean {
  return 'before' in metadata || 'after' in metadata
}

/** Whether `key` is consumed by a change row rather than listed as a detail. */
function isChangeKey(key: string, snapshots: boolean): boolean {
  if (MOVE_KEYS.has(key)) return true
  if (snapshots && (key === 'before' || key === 'after')) return true
  // `changed_fields` names the fields; the pairs spell out the values.
  return key === 'changed_fields'
}

/** The `{old, new}` pairs as change rows and every other key as a detail. */
function pairsAndDetails(
  metadata: Readonly<JsonObject>,
  snapshots: boolean
): DescribedMetadata {
  const changes: AuditChange[] = []
  const details: AuditDetail[] = []
  for (const [key, value] of Object.entries(metadata)) {
    if (isChangeKey(key, snapshots)) continue
    if (isOldNewPair(value)) {
      changes.push({
        field: key,
        old: 'old' in value ? formatAuditValue(value.old) : undefined,
        new: 'new' in value ? formatAuditValue(value.new) : undefined,
      })
    } else {
      details.push({ key, value: formatAuditValue(value) })
    }
  }
  return { changes, details }
}

/**
 * Changed fields the recorder named without values (content), as rows marked
 * `valuesWithheld`. `known` are the fields already shown with values.
 */
function withheldChanges(
  changedFields: unknown,
  known: readonly AuditChange[]
): AuditChange[] {
  if (!Array.isArray(changedFields)) return []
  const shown = new Set(known.map(change => change.field))
  return changedFields
    .filter((field): field is string => typeof field === 'string')
    .filter(field => !shown.has(field))
    .map(field => ({ field, valuesWithheld: true }))
}

export function describeAuditMetadata(
  metadata: Readonly<JsonObject> | null | undefined
): DescribedMetadata {
  if (!metadata) return { changes: [], details: [] }

  const snapshots = hasSnapshots(metadata)
  const move = moveChange(metadata)
  const fromSnapshots = snapshots
    ? snapshotChanges(metadata.before, metadata.after)
    : []
  const { changes: pairs, details } = pairsAndDetails(metadata, snapshots)
  const changes = [...(move ? [move] : []), ...fromSnapshots, ...pairs]
  return {
    changes: [...changes, ...withheldChanges(metadata.changed_fields, changes)],
    details,
  }
}
