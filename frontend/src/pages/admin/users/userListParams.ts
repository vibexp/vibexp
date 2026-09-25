import type { AdvancedParamValue } from '@/pages/admin/filters/advancedFilterParams'
import type { UserSortKey } from '@/pages/admin/users/userColumns'
import type { AdminUserListParams } from '@/services/adminService'

/**
 * The URL → request mapping of the admin users list, shared by the list call
 * and the CSV export (#1150) so the two can never send different filters.
 *
 * Kept pure so every filter's "sent only when set" contract is unit-testable
 * without rendering the page.
 */

/** `all` must send nothing; the other two are the API's enum values. */
export function statusParam(value: string): 'active' | 'suspended' | undefined {
  return value === 'active' || value === 'suspended' ? value : undefined
}

export interface UserListContext {
  /**
   * The cleaned range and date params — `useAdminListFilters`'s
   * `advancedParams`, which already drops invalid URL values.
   */
  advanced: Readonly<Record<string, AdvancedParamValue>>
  page: number
  limit: number
  createdFrom?: string
  createdTo?: string
  sortBy: UserSortKey
  sortOrder: 'asc' | 'desc'
}

/**
 * The `listAdminUsers` query for the page's URL filters. Every optional param is
 * present only when set: "all" statuses, "any" provider and an empty search all
 * send nothing.
 */
export function buildUserListParams(
  filters: Readonly<Record<string, string | undefined>>,
  ctx: UserListContext
): AdminUserListParams {
  return {
    page: ctx.page,
    limit: ctx.limit,
    search: filters.search === '' ? undefined : filters.search,
    status: statusParam(filters.status ?? ''),
    idp_provider:
      filters.idp_provider === '' ? undefined : filters.idp_provider,
    created_from: ctx.createdFrom,
    created_to: ctx.createdTo,
    sort_by: ctx.sortBy,
    sort_order: ctx.sortOrder,
    ...ctx.advanced,
  }
}
