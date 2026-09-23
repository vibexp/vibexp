import { useCallback, useSyncExternalStore } from 'react'

import type { SummaryState } from '@/pages/search/aiSummary'
import { summaryKey } from '@/pages/search/aiSummary'
import {
  getSearchSummary,
  requestSearchSummary,
  subscribeSearchSummaries,
} from '@/pages/search/searchSummaryStore'
import type {
  SearchFilterType,
  SearchSummaryRequest,
} from '@/services/searchService'

export interface UseSearchSummaryResult {
  /** Identity of the current search's summary; undefined without a team. */
  key: string | undefined
  /** The current search's summary, if one was requested. */
  state: SummaryState | undefined
  /** Request the summary unless this search already has one (or is loading). */
  generate: () => void
  /** Request it again regardless (the error state's Retry). */
  retry: () => void
}

/**
 * AI Summaries for a search, cached by search identity (#1078, #1079).
 *
 * The cache is the session-wide `searchSummaryStore`, never component state:
 * Radix unmounts a closed panel's children (and the header search dialog's
 * whole content), so state kept there would be lost and the request would
 * re-fire on every expand. The store is shared by the search page and the
 * header search dialog, so the same search is summarized once for both. The
 * key excludes the page, so paging reuses the summary; a new query or filter
 * is a new key and generates anew.
 */
export function useSearchSummary(
  teamId: string | undefined,
  query: string,
  type: SearchFilterType | undefined,
  projectId: string | undefined
): UseSearchSummaryResult {
  const key = teamId ? summaryKey(teamId, query, type, projectId) : undefined

  const getSnapshot = useCallback(
    () => (key ? getSearchSummary(key) : undefined),
    [key]
  )
  const state = useSyncExternalStore(subscribeSearchSummaries, getSnapshot)

  const request = useCallback(
    (requestKey: string) => {
      if (!teamId) return
      // No paging fields: the summary is grounded in the global top-N.
      const req: SearchSummaryRequest = { query }
      if (type) req.types = [type]
      if (projectId) req.project_id = projectId
      requestSearchSummary(requestKey, teamId, req)
    },
    [teamId, query, type, projectId]
  )

  const generate = useCallback(() => {
    if (key && getSearchSummary(key) === undefined) request(key)
  }, [key, request])

  const retry = useCallback(() => {
    if (key) request(key)
  }, [key, request])

  return { key, state, generate, retry }
}
