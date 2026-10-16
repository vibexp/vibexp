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
 *   the built-in defaults applied;
 * - a team settings change (#1520): `surface` and `action`, then the same
 *   `changed_fields` plus `{old, new}` pairs as a resource update, where a
 *   secret is named without values.
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
  /** Why no values are shown, when `valuesWithheld`. */
  withheldNote?: string
}

export interface AuditDetail {
  key: string
  value: string
}

export interface DescribedMetadata {
  changes: AuditChange[]
  details: AuditDetail[]
}

/** A changed resource field whose content is never recorded (#1513). */
export const CONTENT_WITHHELD = 'changed (content is not recorded)'

/** A changed team setting whose value is never recorded: a secret (#1520). */
export const VALUE_WITHHELD = 'changed (value not recorded)'

/** The team settings surfaces (#1520), as `surface` names them. */
export const TEAM_SETTINGS_SURFACE_LABELS: Readonly<Record<string, string>> = {
  search: 'Search',
  ai_summary: 'AI summary',
  email_provider: 'Email provider',
  model_provider: 'Model provider',
  embedding_provider: 'Embedding provider',
  freshness: 'Freshness',
  github_app: 'GitHub App',
  custom_types: 'Custom types',
  custom_statuses: 'Custom statuses',
}

/**
 * The team settings actions (#1520). Custom statuses keep their own history's
 * past-tense names (created / renamed / deleted).
 */
export const TEAM_SETTINGS_ACTION_LABELS: Readonly<Record<string, string>> = {
  create: 'Created',
  update: 'Updated',
  delete: 'Deleted',
  reset: 'Reset to defaults',
  copy: 'Copied from another team',
  connect: 'Connected',
  disconnect: 'Disconnected',
  rotate: 'Rotated',
  created: 'Created',
  renamed: 'Renamed',
  deleted: 'Deleted',
}

/** Labels for the detail keys a team settings event carries. */
const TEAM_SETTINGS_DETAIL_LABELS: Readonly<Record<string, string>> = {
  surface: 'Settings',
  action: 'Action',
  source_team_id: 'Source team',
  source_provider_id: 'Source provider',
  provider_id: 'Provider',
  rule_id: 'Rule',
  type_id: 'Type',
  type_slug: 'Type slug',
  status_id: 'Status',
  app_config_id: 'App config',
  reassigned_count: 'Reassigned',
  reassigned_to: 'Reassigned to',
  added_slugs: 'Added',
  skipped_slugs: 'Skipped',
  becomes_active: 'Becomes active',
  reconnected: 'Reconnected',
}

/** Detail keys whose values are names with a label of their own. */
const TEAM_SETTINGS_VALUE_LABELS: Readonly<
  Record<string, Readonly<Record<string, string>> | undefined>
> = {
  surface: TEAM_SETTINGS_SURFACE_LABELS,
  action: TEAM_SETTINGS_ACTION_LABELS,
}

/** The details a team settings change lists first, in order. */
const TEAM_SETTINGS_LEADING_KEYS: readonly string[] = ['surface', 'action']

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
  known: readonly AuditChange[],
  note: string
): AuditChange[] {
  if (!Array.isArray(changedFields)) return []
  const shown = new Set(known.map(change => change.field))
  return changedFields
    .filter((field): field is string => typeof field === 'string')
    .filter(field => !shown.has(field))
    .map(field => ({ field, valuesWithheld: true, withheldNote: note }))
}

/** Whether the metadata is a team settings change (#1520). */
function isTeamSettingsChange(metadata: Readonly<JsonObject>): boolean {
  return (
    typeof metadata.surface === 'string' && typeof metadata.action === 'string'
  )
}

/**
 * A team settings change's details: the surface and action first, by their
 * human labels, then the rest under readable keys.
 */
function teamSettingsDetails(details: readonly AuditDetail[]): AuditDetail[] {
  const rank = (key: string) => {
    const index = TEAM_SETTINGS_LEADING_KEYS.indexOf(key)
    return index === -1 ? TEAM_SETTINGS_LEADING_KEYS.length : index
  }
  return [...details]
    .sort((a, b) => rank(a.key) - rank(b.key))
    .map(detail => ({
      key: TEAM_SETTINGS_DETAIL_LABELS[detail.key] ?? detail.key,
      value:
        TEAM_SETTINGS_VALUE_LABELS[detail.key]?.[detail.value] ?? detail.value,
    }))
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
  const teamSettings = isTeamSettingsChange(metadata)
  const note = teamSettings ? VALUE_WITHHELD : CONTENT_WITHHELD
  return {
    changes: [
      ...changes,
      ...withheldChanges(metadata.changed_fields, changes, note),
    ],
    details: teamSettings ? teamSettingsDetails(details) : details,
  }
}
