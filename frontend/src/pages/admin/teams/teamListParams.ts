import type {
  AdvancedFilterSpec,
  AdvancedParamValue,
} from '@/pages/admin/filters/advancedFilterParams'
import type { AdminTeamListParams } from '@/services/adminService'

/**
 * The URL → request mapping of the admin teams list (#1139), wired to the
 * filters `listAdminTeams` gained in #1138.
 *
 * Kept pure so every filter's "sent only when set" contract is unit-testable
 * without rendering the page.
 */

type TeamListQueryKey = keyof AdminTeamListParams

export type TeamSortKey = NonNullable<AdminTeamListParams['sort_by']>

/**
 * Base names whose `_min` param the published client accepts, and its boolean
 * params. Typing the declarations with these makes a renamed API param fail
 * `tsc -b` instead of being sent and silently ignored.
 */
type RangeBase = {
  [K in TeamListQueryKey]: K extends `${infer B}_min` ? B : never
}[TeamListQueryKey]
type TriStateKey = Exclude<
  {
    [K in TeamListQueryKey]-?: NonNullable<
      AdminTeamListParams[K]
    > extends boolean
      ? K
      : never
  }[TeamListQueryKey],
  'is_personal'
>

export interface TeamRangeFilter {
  name: RangeBase
  label: string
}

export interface TeamTriStateFilter {
  name: TriStateKey
  label: string
}

/** Role and project counts, shown in the panel's "Membership" group. */
export const TEAM_MEMBERSHIP_RANGES = [
  { name: 'member_count', label: 'Members' },
  { name: 'owner_count', label: 'Owners' },
  { name: 'admin_count', label: 'Admins' },
  { name: 'project_count', label: 'Projects' },
] as const satisfies readonly TeamRangeFilter[]

/** The total, then the nine resource types, in the users list's order. */
export const TEAM_RESOURCE_RANGES = [
  { name: 'total_resource_count', label: 'Total resources' },
  { name: 'prompt_count', label: 'Prompts' },
  { name: 'memory_count', label: 'Memories' },
  { name: 'artifact_count', label: 'Artifacts' },
  { name: 'blueprint_count', label: 'Blueprints' },
  { name: 'agent_count', label: 'Agents' },
  { name: 'feed_count', label: 'Feeds' },
  { name: 'feed_item_count', label: 'Feed items' },
  { name: 'comment_count', label: 'Comments' },
  { name: 'attachment_count', label: 'Attachments' },
] as const satisfies readonly TeamRangeFilter[]

/** Setup state; the order matches the list's "Setup" indicator column. */
export const TEAM_SETUP_TRISTATES = [
  { name: 'embedding_configured', label: 'Embedding configured' },
  { name: 'llm_configured', label: 'LLM configured' },
  { name: 'ai_summary_enabled', label: 'AI summary enabled' },
  { name: 'email_configured', label: 'Email configured' },
  { name: 'github_configured', label: 'GitHub configured' },
  { name: 'search_settings_customized', label: 'Custom search settings' },
  { name: 'freshness_enabled', label: 'Freshness enabled' },
] as const satisfies readonly TeamTriStateFilter[]

/**
 * Compile-time exhaustiveness: every range and boolean filter the published
 * `listAdminTeams` accepts must have a control above. A param added to the spec
 * and missing here fails `tsc -b` instead of being unreachable from the UI.
 */
type MissingRanges = Exclude<
  RangeBase,
  | (typeof TEAM_MEMBERSHIP_RANGES)[number]['name']
  | (typeof TEAM_RESOURCE_RANGES)[number]['name']
>
type MissingTriStates = Exclude<
  TriStateKey,
  (typeof TEAM_SETUP_TRISTATES)[number]['name']
>
export const TEAM_FILTERS_EXHAUSTIVE: [
  MissingRanges,
  MissingTriStates,
] extends [never, never]
  ? true
  : never = true

/**
 * Module-level on purpose: `useAdminListFilters` freezes the URL keys it owns on
 * the first render.
 */
export const TEAM_ADVANCED_FILTERS: AdvancedFilterSpec = {
  ranges: [...TEAM_MEMBERSHIP_RANGES, ...TEAM_RESOURCE_RANGES].map(
    filter => filter.name
  ),
  triStates: TEAM_SETUP_TRISTATES.map(filter => filter.name),
}

/**
 * Maps the tri-state UI filter onto the optional `is_personal` boolean.
 *
 * `undefined` for "all" is the whole point: sending `is_personal=false` would
 * mean "shared only" and silently hide every personal workspace.
 */
export function isPersonalParam(kind: string): boolean | undefined {
  if (kind === 'personal') return true
  if (kind === 'shared') return false
  return undefined
}

/**
 * A bare RFC 5322 dot-atom address: the shape the server's check
 * (`mail.ParseAddress` round-tripping to the same string) accepts. Anything else
 * — `boss`, `john..doe@corp.com`, `<a@b.co>`, `"a"@b.co` — is answered with a
 * 400, so it must never leave the browser. Non-ASCII is allowed, as Go's parser
 * allows UTF-8 atoms (RFC 6532); quoted local parts and domain literals are
 * refused, being valid but vanishingly rare for an owner lookup.
 */
const ATOM = "[\\w!#$%&'*+/=?^`{|}~\\u0080-\\uffff-]+"
const DOT_ATOM = `${ATOM}(?:\\.${ATOM})*`
const EMAIL = new RegExp(`^${DOT_ATOM}@${DOT_ATOM}$`, 'u')

/**
 * The trimmed owner email, or `undefined` when blank or not an address — a
 * hand-edited `?owner_email=boss` must not turn every reload into an error.
 */
export function ownerEmailParam(value: string | undefined): string | undefined {
  const trimmed = value?.trim() ?? ''
  return EMAIL.test(trimmed) ? trimmed : undefined
}

export interface TeamListContext {
  /**
   * The cleaned range and tri-state params — `useAdminListFilters`'s
   * `advancedParams`, which already drops invalid URL values.
   */
  advanced: Readonly<Record<string, AdvancedParamValue>>
  page: number
  limit: number
  createdFrom?: string
  createdTo?: string
  sortBy: TeamSortKey
  sortOrder: 'asc' | 'desc'
}

/**
 * The `listAdminTeams` query for the page's URL filters. Every optional param is
 * present only when set: an "any" tri-state, an empty or invalid range bound and
 * a blank owner email all send nothing.
 */
export function buildTeamListParams(
  filters: Readonly<Record<string, string | undefined>>,
  ctx: TeamListContext
): AdminTeamListParams {
  return {
    page: ctx.page,
    limit: ctx.limit,
    search: filters.search === '' ? undefined : filters.search,
    is_personal: isPersonalParam(filters.kind ?? ''),
    owner_email: ownerEmailParam(filters.owner_email),
    created_from: ctx.createdFrom,
    created_to: ctx.createdTo,
    sort_by: ctx.sortBy,
    sort_order: ctx.sortOrder,
    ...ctx.advanced,
  }
}
