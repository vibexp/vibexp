import { useEffect, useState } from 'react'

import { getErrorMessage } from '@/utils/errorHandling'

/** One page of a resource list, normalised across the list endpoints. */
export interface ResourceListPage<T> {
  items: T[]
  totalPages: number
  total: number
}

export interface UseResourceListQueryOptions<T> {
  /**
   * False while a prerequisite is still resolving (no team yet, a persisted
   * project still restoring) — nothing is fetched, and the page stays in its
   * loading state rather than flashing unfiltered results.
   */
  ready: boolean
  /**
   * Loads the current page. MUST be memoized with `useCallback` over the
   * filters it reads: it is this hook's dependency, so an unmemoized callback
   * would refetch on every render.
   */
  load: () => Promise<ResourceListPage<T>>
  /** Bump to force a refetch without changing the filters (e.g. after a delete). */
  reloadToken?: number
  /** Message shown when the failure carries none of its own. */
  errorFallback: string
  /** Side-channel for the app's error reporting; must be stable. */
  onError?: (error: unknown) => void
}

export interface ResourceListQueryState<T> {
  items: T[]
  loading: boolean
  error: string | null
  totalPages: number
  total: number
}

/**
 * Runs a resource list's fetch, with the cancellation guard every list page
 * needs and none of them had.
 *
 * The guard is the point: without it a slow response for an earlier filter can
 * land after a newer one and overwrite it — the classic debounced-search race.
 * Extracted so the three metadata-filterable list pages share one correct
 * implementation instead of three copies (SonarCloud flagged exactly that
 * duplication on #523).
 */
export function useResourceListQuery<T>({
  ready,
  load,
  reloadToken = 0,
  errorFallback,
  onError,
}: UseResourceListQueryOptions<T>): ResourceListQueryState<T> {
  const [state, setState] = useState<ResourceListQueryState<T>>({
    items: [],
    loading: true,
    error: null,
    totalPages: 0,
    total: 0,
  })

  useEffect(() => {
    if (!ready) return

    let cancelled = false
    setState(prev => ({ ...prev, loading: true, error: null }))

    load()
      .then(page => {
        if (cancelled) return
        setState({
          items: page.items,
          loading: false,
          error: null,
          totalPages: page.totalPages,
          total: page.total,
        })
      })
      .catch((error: unknown) => {
        if (cancelled) return
        setState(prev => ({
          ...prev,
          loading: false,
          error: getErrorMessage(error, errorFallback),
        }))
        onError?.(error)
      })

    return () => {
      cancelled = true
    }
  }, [ready, load, reloadToken, errorFallback, onError])

  return state
}
