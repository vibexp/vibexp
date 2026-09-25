import type { AdvancedFilterSpec } from '@/pages/admin/filters/advancedFilterParams'
import { sanitizeAdvanced } from '@/pages/admin/filters/advancedFilterParams'
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
export const TEAM_MEMBERSHIP_RANGES: readonly TeamRangeFilter[] = [
  { name: 'member_count', label: 'Members' },
  { name: 'owner_count', label: 'Owners' },
  { name: 'admin_count', label: 'Admins' },
  { name: 'project_count', label: 'Projects' },
]

/** The total, then the nine resource types, in the users list's order. */
export const TEAM_RESOURCE_RANGES: readonly TeamRangeFilter[] = [
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
]

/** Setup state; the order matches the list's "Setup" indicator column. */
export const TEAM_SETUP_TRISTATES: readonly TeamTriStateFilter[] = [
  { name: 'embedding_configured', label: 'Embedding configured' },
  { name: 'llm_configured', label: 'LLM configured' },
  { name: 'ai_summary_enabled', label: 'AI summary enabled' },
  { name: 'email_configured', label: 'Email configured' },
  { name: 'github_configured', label: 'GitHub configured' },
  { name: 'search_settings_customized', label: 'Custom search settings' },
  { name: 'freshness_enabled', label: 'Freshness enabled' },
]

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

/** The trimmed owner email, or `undefined` when blank. */
export function ownerEmailParam(value: string | undefined): string | undefined {
  const trimmed = value?.trim() ?? ''
  return trimmed === '' ? undefined : trimmed
}

export interface TeamListContext {
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
  // Invalid URL values (non-integer, min > max) never reach `params`.
  const { params: advanced } = sanitizeAdvanced(TEAM_ADVANCED_FILTERS, filters)
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
    ...advanced,
  }
}
