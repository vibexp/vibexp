import { useCallback, useEffect, useRef, useState } from 'react'

import { getErrorMessage } from '@/utils/errorHandling'

const LIMIT = 25
const DEBOUNCE_MS = 300

/** One page of a server-side search. */
export interface AdminSearchPage<T> {
  items: T[]
  totalPages: number
}

/** Loads one page of results for a query (`undefined` = no search text). */
export type AdminSearchFetcher<T> = (
  query: string | undefined,
  page: number,
  limit: number
) => Promise<AdminSearchPage<T>>

export interface AdminPagedSearchResult<T> {
  items: T[]
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
 * Server-driven, paginated search for the admin pickers (teams, users,
 * projects).
 *
 * Paginates rather than loading everything: on an instance where personal
 * workspaces run one per user, "fetch all to fill a dropdown" is a request that
 * grows without bound. Each query change resets to page 1 and accumulated pages
 * are appended, so a picker can scroll past the first page.
 *
 * `fetchPage` must be stable (`useCallback`): a new identity re-runs the
 * search, which is how a fetcher that closes over an option (e.g. "shared teams
 * only") refreshes when that option changes.
 */
export function useAdminPagedSearch<T>(
  fetchPage: AdminSearchFetcher<T>,
  fallbackError: string
): AdminPagedSearchResult<T> {
  const [items, setItems] = useState<T[]>([])
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

  // Armed only when the typed query differs from the one searched. A timer armed
  // at mount (or after typing back to the searched text) would reset `page` to 1
  // for an unchanged query, throwing away the pages already scrolled in (#1448).
  useEffect(() => {
    if (query === debouncedQuery) return
    const timer = setTimeout(() => {
      setDebouncedQuery(query)
      setPage(1)
    }, DEBOUNCE_MS)
    return () => {
      clearTimeout(timer)
    }
  }, [query, debouncedQuery])

  useEffect(() => {
    const seq = ++seqRef.current
    const firstPage = page === 1
    if (firstPage) setLoading(true)
    else setLoadingMore(true)
    setError(null)

    fetchPage(debouncedQuery || undefined, page, LIMIT)
      .then(response => {
        if (seq !== seqRef.current) return
        setItems(prev =>
          firstPage ? response.items : [...prev, ...response.items]
        )
        setTotalPages(response.totalPages)
      })
      .catch((err: unknown) => {
        if (seq !== seqRef.current) return
        setError(getErrorMessage(err, fallbackError))
      })
      .finally(() => {
        if (seq !== seqRef.current) return
        setLoading(false)
        setLoadingMore(false)
      })
  }, [debouncedQuery, page, fetchPage, fallbackError])

  const hasMore = page < totalPages

  const loadMore = useCallback(() => {
    // Guarded internally so a scroll handler can call it freely.
    if (loading || loadingMore || !hasMore) return
    setPage(prev => prev + 1)
  }, [loading, loadingMore, hasMore])

  return {
    items,
    loading,
    loadingMore,
    error,
    hasMore,
    loadMore,
    setQuery,
    query,
  }
}
