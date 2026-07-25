import { useCallback, useEffect, useRef, useState } from 'react'

import type { AdminTeamListItem } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

const LIMIT = 25
const DEBOUNCE_MS = 300

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
 *
 * Paginates rather than loading every team: on an instance where personal
 * workspaces run one per user, "fetch all teams to fill a dropdown" is a request
 * that grows without bound. Each query change resets to page 1 and accumulated
 * pages are appended, so the picker can scroll past the first page.
 */
export function useAdminTeamSearch(): AdminTeamSearchResult {
  const [teams, setTeams] = useState<AdminTeamListItem[]>([])
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')
  const [page, setPage] = useState(1)
  const [totalPages, setTotalPages] = useState(0)
  // Sequence guard: a slow page-1 response for an old query must not overwrite a
  // newer query's results.
  const seqRef = useRef(0)

  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedQuery(query)
      setPage(1)
    }, DEBOUNCE_MS)
    return () => {
      clearTimeout(timer)
    }
  }, [query])

  useEffect(() => {
    const seq = ++seqRef.current
    const firstPage = page === 1
    if (firstPage) setLoading(true)
    else setLoadingMore(true)
    setError(null)

    adminService
      .listTeams({
        page,
        limit: LIMIT,
        search: debouncedQuery || undefined,
        sort_by: 'name',
        sort_order: 'asc',
      })
      .then(response => {
        if (seq !== seqRef.current) return
        setTeams(prev =>
          firstPage ? response.teams : [...prev, ...response.teams]
        )
        setTotalPages(response.total_pages)
      })
      .catch((err: unknown) => {
        if (seq !== seqRef.current) return
        setError(getErrorMessage(err, 'Failed to load teams'))
      })
      .finally(() => {
        if (seq !== seqRef.current) return
        setLoading(false)
        setLoadingMore(false)
      })
  }, [debouncedQuery, page])

  const hasMore = page < totalPages

  const loadMore = useCallback(() => {
    // Guarded internally so a scroll handler can call it freely.
    if (loading || loadingMore || !hasMore) return
    setPage(prev => prev + 1)
  }, [loading, loadingMore, hasMore])

  return {
    teams,
    loading,
    loadingMore,
    error,
    hasMore,
    loadMore,
    setQuery,
    query,
  }
}
