import { useCallback, useRef, useState } from 'react'

import type { SummaryState } from '@/pages/search/aiSummary'
import { classifySummaryError, summaryKey } from '@/pages/search/aiSummary'
import type {
  SearchFilterType,
  SearchSummaryRequest,
} from '@/services/searchService'
import { searchService } from '@/services/searchService'

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
 * AI Summaries for the search page, cached by search identity (#1078).
 *
 * Owned by the page rather than the collapsible: Radix unmounts a closed
 * panel's children, so state kept there would be lost on collapse and the
 * request would re-fire on every expand. The key excludes the page, so paging
 * reuses the summary; a new query or filter is a new key and generates anew.
 */
export function useSearchSummary(
  teamId: string | undefined,
  query: string,
  type: SearchFilterType | undefined,
  projectId: string | undefined
): UseSearchSummaryResult {
  const [summaries, setSummaries] = useState<Map<string, SummaryState>>(
    () => new Map()
  )
  // Keys with a request in flight, so a fast double expand/retry never sends
  // a second request for the same search.
  const inFlight = useRef(new Set<string>())

  const key = teamId ? summaryKey(teamId, query, type, projectId) : undefined

  const request = useCallback(
    (requestKey: string) => {
      if (!teamId || inFlight.current.has(requestKey)) return
      inFlight.current.add(requestKey)
      const store = (next: SummaryState) => {
        setSummaries(prev => new Map(prev).set(requestKey, next))
      }
      store({ status: 'loading' })
      // No paging fields: the summary is grounded in the global top-N.
      const req: SearchSummaryRequest = { query }
      if (type) req.types = [type]
      if (projectId) req.project_id = projectId
      searchService
        .summarize(teamId, req)
        .then(data => {
          store({ status: 'ready', data })
        })
        .catch((error: unknown) => {
          store({ status: 'error', ...classifySummaryError(error) })
        })
        .finally(() => {
          inFlight.current.delete(requestKey)
        })
    },
    [teamId, query, type, projectId]
  )

  const generate = useCallback(() => {
    if (key && !summaries.has(key)) request(key)
  }, [key, summaries, request])

  const retry = useCallback(() => {
    if (key) request(key)
  }, [key, request])

  return {
    key,
    state: key ? summaries.get(key) : undefined,
    generate,
    retry,
  }
}
