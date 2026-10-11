import type {
  TeamSettingsAuditEntry,
  TeamSettingsAuditSurface,
} from '@/services/teamSettingsAuditService'

/**
 * Formatting for a settings audit entry, shared by the team's own Audit page
 * and the admin team detail's Settings audit tab (#1142).
 *
 * A sibling data module rather than exports from `SettingsAudit.tsx`, so that
 * file keeps exporting components only (`react-refresh/only-export-components`).
 * Both callers read the same allowlisted `detail` keys and nothing else — a row
 * never renders `detail` wholesale, so a key the server later adds cannot leak.
 */

const SURFACE_LABELS: Record<TeamSettingsAuditSurface, string> = {
  model_provider: 'Model provider',
  embedding_provider: 'Embedding provider',
  custom_types: 'Artifact types',
  custom_statuses: 'Statuses',
}

/**
 * Epic #827 grows the surface enum, and the frontend's copy of it is the one
 * link in the API change flow that is bumped by hand — so the server can emit a
 * surface this build has no label for. Falling back to the raw value keeps the
 * What column readable instead of blank, which matters more here than anywhere:
 * a silently empty cell in the epic's compensating control reads as "nothing
 * happened". Same guard as `FreshnessAudit`'s `typeLabel`.
 */
export function surfaceLabel(surface: TeamSettingsAuditSurface): string {
  return SURFACE_LABELS[surface] || surface
}

/** The two fields of an audit entry the formatters read. */
export type AuditEntryDescription = Pick<
  TeamSettingsAuditEntry,
  'surface' | 'detail'
>

type Detail = AuditEntryDescription['detail']

function detailString(detail: Detail, key: string): string | null {
  const value = detail[key]
  return typeof value === 'string' && value.trim() !== '' ? value : null
}

function detailCount(detail: Detail, key: string): number {
  const value = detail[key]
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function detailStrings(detail: Detail, key: string): string[] {
  const value = detail[key]
  if (!Array.isArray(value)) return []
  return value.filter((item): item is string => typeof item === 'string')
}

/**
 * Whether the entry records a copy from another team.
 *
 * Every surface but `custom_statuses` does. A status entry is a change made
 * inside the team — it has no source team by design, so "from an unknown team"
 * would be a false statement about it rather than a missing fact.
 */
export function isCopyEntry(
  entry: Pick<AuditEntryDescription, 'surface'>
): boolean {
  return entry.surface !== 'custom_statuses'
}

const STATUS_RESOURCE_WORDS = new Map([
  ['prompt', 'prompt'],
  ['artifact', 'artifact'],
  ['blueprint', 'blueprint'],
  ['memory', 'memory'],
])

/**
 * A `custom_statuses` entry: a status created, renamed or deleted (#1265).
 *
 * `detail` carries `action`, `resource_type`, `slug` and `name`, plus
 * `previous_name` on a rename and `reassigned_count` / `reassigned_to` on a
 * delete. Only those keys are read. The names are the snapshot taken when the
 * change was made, so a since-deleted status still reads correctly.
 */
function describeStatusChange(detail: Detail): string {
  const resource = STATUS_RESOURCE_WORDS.get(
    detailString(detail, 'resource_type') ?? ''
  )
  const what = resource ? `${resource} status` : 'status'
  const name =
    detailString(detail, 'name') ?? detailString(detail, 'slug') ?? 'Unnamed'
  switch (detailString(detail, 'action')) {
    case 'created':
      return `Created ${what} "${name}"`
    case 'renamed': {
      const previous = detailString(detail, 'previous_name')
      return previous
        ? `Renamed ${what} "${previous}" to "${name}"`
        : `Renamed ${what} to "${name}"`
    }
    case 'deleted': {
      const moved = detailCount(detail, 'reassigned_count')
      const destination = detailString(detail, 'reassigned_to')
      if (moved === 0 || !destination) return `Deleted ${what} "${name}"`
      return `Deleted ${what} "${name}" — ${String(moved)} moved to ${destination}`
    }
    default:
      return `Changed ${what} "${name}"`
  }
}

/**
 * What arrived, in the entry's own words.
 *
 * The entry is a SNAPSHOT: the copy services write the resource names into
 * `detail` at write time precisely because the rows they name are polymorphic
 * and may be deleted afterwards (#832). So the name is read from `detail`, never
 * resolved live — a live lookup is the thing that breaks for a deleted resource.
 *
 * `custom_types` is the one surface where a single action copies a whole set, so
 * it has no `source_resource_id`/`created_resource_id` and its names live in
 * `detail.added_slugs` instead.
 */
export function describeResource(entry: AuditEntryDescription): string {
  if (entry.surface === 'custom_statuses') {
    return describeStatusChange(entry.detail)
  }
  if (entry.surface === 'custom_types') {
    const slugs = detailStrings(entry.detail, 'added_slugs')
    if (slugs.length === 0) return 'No new types — every one already existed'
    return `${String(slugs.length)} type${slugs.length === 1 ? '' : 's'}: ${slugs.join(', ')}`
  }
  return (
    detailString(entry.detail, 'created_name') ??
    detailString(entry.detail, 'source_name') ??
    'Unnamed'
  )
}

/** Whether the copy carried an API key along (`detail.has_api_key`). */
export function carriedCredential(entry: AuditEntryDescription): boolean {
  return entry.detail.has_api_key === true
}
