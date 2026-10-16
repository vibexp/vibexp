import type { components } from '@vibexp/api-client'

import type { DateRangeValue } from '@/components/ui/date-range'
import { rangeToInstants } from '@/components/ui/date-range'
import type {
  AdminAuditExportParams,
  AdminAuditListParams,
} from '@/services/adminAuditService'

import { type AuditOperation, isAuditOperation } from './operationLabels'

/**
 * The URL → request mapping of the admin audit log (#1517), wired to
 * `listAdminAuditEvents` / `exportAdminAuditEvents` (#1516).
 *
 * Kept pure, like the other admin list params, so the default range and every
 * filter's "sent only when set" contract are unit-testable without rendering
 * the page.
 */

export type AuditSource = components['schemas']['AdminAuditSource']
export type AuditOutcome = components['schemas']['AdminAuditOutcome']

/** Events per page; the API allows up to 200. */
export const AUDIT_PAGE_SIZE = 50

/** The default window, in days including today, when the URL carries no range. */
export const AUDIT_DEFAULT_RANGE_DAYS = 7

/**
 * URL keys and their defaults. `search` carries `q` and the `created_*` pair the
 * date range: they are `useAdminListFilters`' base keys, so the search box, the
 * date picker, Clear and saved presets work exactly as on the other admin lists.
 * The list has no sort or page in the URL — it is newest first, paged by cursor.
 *
 * Module-level on purpose: `useAdminListFilters` freezes the keys it owns on the
 * first render.
 */
export const AUDIT_FILTER_DEFAULTS = {
  page: '1',
  search: '',
  created_from: '',
  created_to: '',
  sort_by: 'created_at',
  sort_order: 'desc',
  actor_user_id: '',
  team_id: '',
  project_id: '',
  operation: '',
  resource_type: '',
  source: '',
  outcome: '',
}

/** The domain filters that make the view "filtered" (Clear, empty state). */
export const AUDIT_FILTER_KEYS = [
  'actor_user_id',
  'team_id',
  'project_id',
  'operation',
  'resource_type',
  'source',
  'outcome',
] as const

export const AUDIT_SOURCES = [
  'web',
  'cli',
  'mcp',
  'api',
  'system',
] as const satisfies readonly AuditSource[]

export const AUDIT_OUTCOMES = [
  'success',
  'denied',
  'failure',
] as const satisfies readonly AuditOutcome[]

/**
 * Compile-time exhaustiveness: a source or outcome added to the spec and
 * missing above fails `tsc -b` instead of being unreachable from the filter.
 */
type MissingSources = Exclude<AuditSource, (typeof AUDIT_SOURCES)[number]>
type MissingOutcomes = Exclude<AuditOutcome, (typeof AUDIT_OUTCOMES)[number]>
export const AUDIT_ENUMS_EXHAUSTIVE: [MissingSources, MissingOutcomes] extends [
  never,
  never,
]
  ? true
  : never = true

/**
 * The resource types the audit log records today: the resource writes use the
 * resource's own type; the auth, security and team events use `user`,
 * `api_key`, `oauth_client`, `team`, `team_member` and `team_invitation`
 * (`models.AuditResourceType*`). `resource_type` is a free string in the API,
 * so this is the filter's menu, not a constraint.
 */
export const AUDIT_RESOURCE_TYPES = [
  { value: 'prompt', label: 'Prompt' },
  { value: 'blueprint', label: 'Blueprint' },
  { value: 'artifact', label: 'Artifact' },
  { value: 'memory', label: 'Memory' },
  { value: 'feed', label: 'Feed' },
  { value: 'feed_item', label: 'Feed item' },
  { value: 'project', label: 'Project' },
  { value: 'user', label: 'User' },
  { value: 'api_key', label: 'API key' },
  { value: 'oauth_client', label: 'OAuth client' },
  { value: 'team', label: 'Team' },
  { value: 'team_member', label: 'Team member' },
  { value: 'team_invitation', label: 'Team invitation' },
  { value: 'team_settings', label: 'Team settings' },
] as const

/** A comma-separated URL value as a list, dropping blanks. */
export function splitList(value: string | undefined): string[] {
  if (!value) return []
  return value
    .split(',')
    .map(item => item.trim())
    .filter(item => item !== '')
}

/** A list as the comma-separated URL value (`''` removes the key). */
export function joinList(values: readonly string[]): string {
  return values.join(',')
}

/** The URL's values that belong to `allowed`, in URL order and deduplicated. */
function knownValues<T extends string>(
  value: string | undefined,
  isKnown: (item: string) => item is T
): T[] {
  return [...new Set(splitList(value))].filter(isKnown)
}

const isSource = (item: string): item is AuditSource =>
  (AUDIT_SOURCES as readonly string[]).includes(item)
const isOutcome = (item: string): item is AuditOutcome =>
  (AUDIT_OUTCOMES as readonly string[]).includes(item)

/** The selected operations, ignoring any a hand-edited URL made up. */
export function selectedOperations(
  value: string | undefined
): AuditOperation[] {
  return knownValues(value, isAuditOperation)
}

export function selectedSources(value: string | undefined): AuditSource[] {
  return knownValues(value, isSource)
}

export function selectedOutcomes(value: string | undefined): AuditOutcome[] {
  return knownValues(value, isOutcome)
}

/**
 * The last {@link AUDIT_DEFAULT_RANGE_DAYS} local days, today included: what
 * the page shows while the URL carries no range, so the free-text search is
 * always bounded by a date range (#1516's lead decision).
 */
export function defaultAuditRange(now: Date): DateRangeValue {
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const from = new Date(today)
  from.setDate(from.getDate() - (AUDIT_DEFAULT_RANGE_DAYS - 1))
  return { from, to: today }
}

/**
 * The range in effect: the URL's when it carries either bound, otherwise the
 * default. A one-sided URL range stays one-sided — the admin asked for it.
 */
export function effectiveAuditRange(
  urlRange: DateRangeValue,
  now: Date
): DateRangeValue {
  if (urlRange.from !== undefined || urlRange.to !== undefined) {
    return urlRange
  }
  return defaultAuditRange(now)
}

export interface AuditListContext {
  /** The range in effect (see `effectiveAuditRange`). */
  range: DateRangeValue
  limit: number
}

/**
 * The `listAdminAuditEvents` query for the page's URL filters. Every optional
 * param is present only when set; values outside the enums are dropped rather
 * than sent for a 400.
 */
export function buildAuditListParams(
  filters: Readonly<Record<string, string | undefined>>,
  ctx: AuditListContext
): AdminAuditListParams {
  const { from, to } = rangeToInstants(ctx.range)
  const operations = selectedOperations(filters.operation)
  const sources = selectedSources(filters.source)
  const outcomes = selectedOutcomes(filters.outcome)
  const search = filters.search?.trim() ?? ''
  const params: AdminAuditListParams = { limit: ctx.limit }
  if (filters.actor_user_id) params.actor_user_id = filters.actor_user_id
  if (filters.team_id) params.team_id = filters.team_id
  if (filters.project_id) params.project_id = filters.project_id
  if (operations.length > 0) params.operation = operations
  if (filters.resource_type) params.resource_type = filters.resource_type
  if (from) params.from = from
  if (to) params.to = to
  if (sources.length > 0) params.source = sources
  if (outcomes.length > 0) params.outcome = outcomes
  if (search !== '') params.q = search
  return params
}

/**
 * The CSV export query for a list query: the same filters, without the cursor
 * and page size, so the export and the table on screen cannot disagree.
 */
export function auditExportParams(
  params: AdminAuditListParams
): AdminAuditExportParams {
  const { limit: _limit, cursor: _cursor, ...rest } = params
  return rest
}
