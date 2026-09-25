import type { AdvancedFilterSpec } from '@/pages/admin/filters/advancedFilterParams'
import type { AdminUserListParams } from '@/services/adminService'

/**
 * The advanced filters of the admin users list (#1134), wired to the count and
 * last-activity params of `listAdminUsers` (#1133).
 */

/** A count range: its URL/API base name (`${name}_min` / `_max`) and label. */
export interface UserRangeFilter {
  name: string
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

export const LAST_RESOURCE_CREATED = 'last_resource_created'

/**
 * Module-level on purpose: `useAdminListFilters` freezes the URL keys it owns on
 * the first render.
 */
export const USER_ADVANCED_FILTERS: AdvancedFilterSpec = {
  ranges: USER_RANGE_FILTERS.map(filter => filter.name),
  dateRanges: [LAST_RESOURCE_CREATED],
}

type UserListQueryKey = keyof NonNullable<AdminUserListParams>

/**
 * Every URL key the spec above owns, checked against the published client: a
 * key the API does not accept would be sent and silently ignored, so a renamed
 * param fails `tsc -b` here instead.
 */
export const USER_ADVANCED_QUERY_KEYS = [
  'team_count_min',
  'team_count_max',
  'project_count_min',
  'project_count_max',
  'total_resource_count_min',
  'total_resource_count_max',
  'prompt_count_min',
  'prompt_count_max',
  'memory_count_min',
  'memory_count_max',
  'artifact_count_min',
  'artifact_count_max',
  'blueprint_count_min',
  'blueprint_count_max',
  'agent_count_min',
  'agent_count_max',
  'feed_count_min',
  'feed_count_max',
  'feed_item_count_min',
  'feed_item_count_max',
  'comment_count_min',
  'comment_count_max',
  'attachment_count_min',
  'attachment_count_max',
  'last_resource_created_from',
  'last_resource_created_to',
] as const satisfies readonly UserListQueryKey[]
