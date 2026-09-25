import type { AdvancedFilterSpec } from '@/pages/admin/filters/advancedFilterParams'
import type { AdminUserListParams } from '@/services/adminService'

/**
 * The advanced filters of the admin users list (#1134), wired to the count and
 * last-activity params of `listAdminUsers` (#1133).
 */

type UserListQueryKey = keyof NonNullable<AdminUserListParams>

/**
 * Base names whose `_min` / `_from` param the published client accepts. Typing
 * the declarations with these makes a renamed API param fail `tsc -b` instead
 * of being sent and silently ignored.
 */
type RangeBase = {
  [K in UserListQueryKey]: K extends `${infer B}_min` ? B : never
}[UserListQueryKey]
type DateRangeBase = {
  [K in UserListQueryKey]: K extends `${infer B}_from` ? B : never
}[UserListQueryKey]

/** A count range: its URL/API base name (`${name}_min` / `_max`) and label. */
export interface UserRangeFilter {
  name: RangeBase
  label: string
}

/** Panel order: membership counts, the total, then the nine resource types. */
export const USER_RANGE_FILTERS: readonly UserRangeFilter[] = [
  { name: 'team_count', label: 'Teams' },
  { name: 'project_count', label: 'Projects' },
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

export const LAST_RESOURCE_CREATED: DateRangeBase = 'last_resource_created'

/**
 * Module-level on purpose: `useAdminListFilters` freezes the URL keys it owns on
 * the first render.
 */
export const USER_ADVANCED_FILTERS: AdvancedFilterSpec = {
  ranges: USER_RANGE_FILTERS.map(filter => filter.name),
  dateRanges: [LAST_RESOURCE_CREATED],
}
