import type {
  AdvancedFilterSpec,
  AdvancedParamValue,
} from '@/pages/admin/filters/advancedFilterParams'
import { ownerEmailParam } from '@/pages/admin/filters/advancedFilterParams'
import type {
  AdminProjectListParams,
  AdminProjectResourceCounts,
} from '@/services/adminService'

/**
 * The URL → request mapping of the admin projects list (#1144), wired to the
 * filters `listAdminProjects` gained in #1143.
 *
 * Kept pure so every filter's "sent only when set" contract is unit-testable
 * without rendering the page.
 */

type ProjectListQueryKey = keyof AdminProjectListParams

export type ProjectSortKey = NonNullable<AdminProjectListParams['sort_by']>

/**
 * Base names whose `_min` / `_from` param the published client accepts. Typing
 * the declarations with these makes a renamed API param fail `tsc -b` instead
 * of being sent and silently ignored. `created` stays on the main-row picker.
 */
type RangeBase = {
  [K in ProjectListQueryKey]: K extends `${infer B}_min` ? B : never
}[ProjectListQueryKey]
type DateRangeBase = Exclude<
  {
    [K in ProjectListQueryKey]: K extends `${infer B}_from` ? B : never
  }[ProjectListQueryKey],
  'created'
>

export interface ProjectRangeFilter {
  name: RangeBase
  label: string
}

/**
 * The total, then the five project-scoped types, in the users list's order.
 * Agents, feeds, comments and attachments have no `project_id`, so the API
 * publishes no project filter for them.
 */
export const PROJECT_RESOURCE_RANGES = [
  { name: 'total_resource_count', label: 'Total resources' },
  { name: 'prompt_count', label: 'Prompts' },
  { name: 'memory_count', label: 'Memories' },
  { name: 'artifact_count', label: 'Artifacts' },
  { name: 'blueprint_count', label: 'Blueprints' },
  { name: 'feed_item_count', label: 'Feed items' },
] as const satisfies readonly ProjectRangeFilter[]

/**
 * The per-type breakdown of the Resources cell, in the same order as the
 * ranges above — kept next to them so the filters and the breakdown cannot
 * drift apart.
 */
export const PROJECT_RESOURCE_BREAKDOWN = [
  { key: 'prompts', singular: 'prompt', plural: 'prompts', label: 'Prompts' },
  {
    key: 'memories',
    singular: 'memory',
    plural: 'memories',
    label: 'Memories',
  },
  {
    key: 'artifacts',
    singular: 'artifact',
    plural: 'artifacts',
    label: 'Artifacts',
  },
  {
    key: 'blueprints',
    singular: 'blueprint',
    plural: 'blueprints',
    label: 'Blueprints',
  },
  {
    key: 'feed_items',
    singular: 'feed item',
    plural: 'feed items',
    label: 'Feed items',
  },
] as const satisfies readonly {
  key: Exclude<keyof AdminProjectResourceCounts, 'total'>
  singular: string
  plural: string
  label: string
}[]

export const LAST_RESOURCE_CREATED =
  'last_resource_created' satisfies DateRangeBase

/**
 * Compile-time exhaustiveness: every range and date-range filter the published
 * `listAdminProjects` accepts must have a control, and every per-type count the
 * row carries a breakdown entry. A param or count added to the spec and missing
 * here fails `tsc -b` instead of being unreachable from the UI.
 */
type MissingRanges = Exclude<
  RangeBase,
  (typeof PROJECT_RESOURCE_RANGES)[number]['name']
>
type MissingDateRanges = Exclude<DateRangeBase, typeof LAST_RESOURCE_CREATED>
type MissingBreakdown = Exclude<
  keyof AdminProjectResourceCounts,
  'total' | (typeof PROJECT_RESOURCE_BREAKDOWN)[number]['key']
>
export const PROJECT_FILTERS_EXHAUSTIVE: [
  MissingRanges,
  MissingDateRanges,
  MissingBreakdown,
] extends [never, never, never]
  ? true
  : never = true

/**
 * Module-level on purpose: `useAdminListFilters` freezes the URL keys it owns on
 * the first render.
 */
export const PROJECT_ADVANCED_FILTERS: AdvancedFilterSpec = {
  ranges: PROJECT_RESOURCE_RANGES.map(filter => filter.name),
  dateRanges: [LAST_RESOURCE_CREATED],
}

export interface ProjectListContext {
  /**
   * The cleaned range and date-range params — `useAdminListFilters`'s
   * `advancedParams`, which already drops invalid URL values.
   */
  advanced: Readonly<Record<string, AdvancedParamValue>>
  page: number
  limit: number
  createdFrom?: string
  createdTo?: string
  sortBy: ProjectSortKey
  sortOrder: 'asc' | 'desc'
}

/**
 * The `listAdminProjects` query for the page's URL filters. Every optional param
 * is present only when set: an empty or invalid range bound, an empty team and
 * a blank or malformed creator email all send nothing.
 */
export function buildProjectListParams(
  filters: Readonly<Record<string, string | undefined>>,
  ctx: ProjectListContext
): AdminProjectListParams {
  return {
    page: ctx.page,
    limit: ctx.limit,
    search: filters.search === '' ? undefined : filters.search,
    team_id: filters.team_id === '' ? undefined : filters.team_id,
    owner_email: ownerEmailParam(filters.owner_email),
    created_from: ctx.createdFrom,
    created_to: ctx.createdTo,
    sort_by: ctx.sortBy,
    sort_order: ctx.sortOrder,
    ...ctx.advanced,
  }
}
