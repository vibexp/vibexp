import { useCallback } from 'react'

import { useAdminPagedSearch } from '@/pages/admin/filters/useAdminPagedSearch'
import type { AdminTeamListItem } from '@/services/adminService'
import { adminService } from '@/services/adminService'

export interface AdminTeamSearchResult {
  teams: AdminTeamListItem[]
  /** First page for the current query. */
  loading: boolean
  /** A subsequent page being appended. */
  loadingMore: boolean
  error: string | null
  hasMore: boolean
  loadMore: () => void
  setQuery: (query: string) => void
  query: string
}

/**
 * Server-driven team search for the admin team picker.
 *
 * The admin equivalent of `useProjectSearch`, which cannot be reused here: it
 * depends on `useTeam`, and the admin shell mounts no `TeamProvider` (#456).
 * The paging, debouncing and stale-response guard live in
 * `useAdminPagedSearch`, shared with the audit log's user and project pickers
 * (#1517).
 */
export function useAdminTeamSearch(
  options: Readonly<{ sharedOnly?: boolean }> = {}
): AdminTeamSearchResult {
  // A primitive, so the search does not re-run on every render.
  const sharedOnly = options.sharedOnly === true
  const fetchPage = useCallback(
    async (search: string | undefined, page: number, limit: number) => {
      const response = await adminService.listTeams({
        page,
        limit,
        search,
        // Personal workspaces run one per user and are all named alike; a
        // picker that cannot use them asks the server to leave them out.
        ...(sharedOnly ? { is_personal: false } : {}),
        sort_by: 'name',
        sort_order: 'asc',
      })
      return { items: response.teams, totalPages: response.total_pages }
    },
    [sharedOnly]
  )
  const { items, ...rest } = useAdminPagedSearch(
    fetchPage,
    'Failed to load teams'
  )
  return { teams: items, ...rest }
}
