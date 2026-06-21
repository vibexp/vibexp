import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { useTeam } from '../contexts/TeamContext'
import { projectService } from '../services/projectService'
import type { Project } from '../types/project'

interface UseProjectSearchOptions {
  /** Page size for both the initial load and search/pagination requests. */
  limit?: number
  /** Project id to drop from results (e.g. the migration source project). */
  excludeProjectId?: string
  /** Debounce delay in milliseconds applied to the search query. */
  debounceMs?: number
}

interface ProjectSearchResult {
  projects: Project[]
  /** True while the first page (initial load or a new query) is loading. */
  loading: boolean
  /** True while a subsequent page is being appended via {@link loadMore}. */
  loadingMore: boolean
  error: string | null
  /** Whether more pages are available for the current query. */
  hasMore: boolean
  /**
   * Fetch and append the next page for the current query. A no-op while a load
   * is in flight or the current query is exhausted.
   */
  loadMore: () => void
  /** Update the search query. The backend fetch is debounced and resets paging. */
  setQuery: (query: string) => void
  query: string
}

const DEFAULT_LIMIT = 100
const DEFAULT_DEBOUNCE_MS = 300

/**
 * Server-driven project search for the project pickers. Owns a debounced fetch
 * against `projectService.getProjects` and accumulates pages so a picker can
 * scale past the first page (search + infinite scroll / "load more") instead of
 * filtering an in-memory slice. Each query change resets pagination to page 1.
 */
export function useProjectSearch(
  options: UseProjectSearchOptions = {}
): ProjectSearchResult {
  const { currentTeam } = useTeam()
  const {
    limit = DEFAULT_LIMIT,
    excludeProjectId,
    debounceMs = DEFAULT_DEBOUNCE_MS,
  } = options

  const [projects, setProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [hasMore, setHasMore] = useState(false)
  const [query, setQueryState] = useState('')

  // Monotonic request token: a slower earlier fetch must not overwrite the
  // results of a newer one (out-of-order responses while the user types or
  // paginates).
  const requestIdRef = useRef(0)
  // The last page successfully loaded for the current query (1-based).
  const pageRef = useRef(1)

  const fetchPage = useCallback(
    async (search: string, page: number) => {
      if (!currentTeam) {
        setError('No team selected')
        return
      }

      const requestId = ++requestIdRef.current
      const append = page > 1

      try {
        if (append) {
          setLoadingMore(true)
        } else {
          setLoading(true)
        }
        setError(null)

        const trimmed = search.trim()
        const response = await projectService.getProjects(currentTeam.id, {
          limit,
          page,
          ...(trimmed ? { search: trimmed } : {}),
        })

        // A newer request started while this one was in flight — discard.
        if (requestId !== requestIdRef.current) return

        const incoming = excludeProjectId
          ? response.projects.filter(p => p.id !== excludeProjectId)
          : response.projects

        setProjects(prev => (append ? [...prev, ...incoming] : incoming))
        pageRef.current = response.page
        setHasMore(response.page < response.total_pages)
      } catch {
        if (requestId !== requestIdRef.current) return
        setError(
          append ? 'Failed to load more projects' : 'Failed to load projects'
        )
        if (!append) {
          setProjects([])
          setHasMore(false)
        }
      } finally {
        if (requestId === requestIdRef.current) {
          setLoading(false)
          setLoadingMore(false)
        }
      }
    },
    [currentTeam, limit, excludeProjectId]
  )

  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const setQuery = useCallback((next: string) => {
    setQueryState(next)
  }, [])

  // Load the first page once a team is available, then re-fetch (debounced)
  // whenever the query changes. A query change always restarts at page 1.
  useEffect(() => {
    if (!currentTeam) return

    if (timerRef.current) {
      clearTimeout(timerRef.current)
    }

    timerRef.current = setTimeout(() => {
      void fetchPage(query, 1)
    }, debounceMs)

    return () => {
      if (timerRef.current) {
        clearTimeout(timerRef.current)
      }
    }
  }, [query, currentTeam, debounceMs, fetchPage])

  const loadMore = useCallback(() => {
    if (loading || loadingMore || !hasMore) return
    void fetchPage(query, pageRef.current + 1)
  }, [loading, loadingMore, hasMore, query, fetchPage])

  return useMemo(
    () => ({
      projects,
      loading,
      loadingMore,
      error,
      hasMore,
      loadMore,
      setQuery,
      query,
    }),
    [projects, loading, loadingMore, error, hasMore, loadMore, setQuery, query]
  )
}
