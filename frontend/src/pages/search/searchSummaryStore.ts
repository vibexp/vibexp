import type { SummaryState } from '@/pages/search/aiSummary'
import { classifySummaryError } from '@/pages/search/aiSummary'
import type { SearchSummaryRequest } from '@/services/searchService'
import { searchService } from '@/services/searchService'

/**
 * Session-wide cache of AI Summaries, keyed by `summaryKey` (#1079).
 *
 * Module-level on purpose: the search page and the header search dialog both
 * read it, and the dialog's content unmounts on close — so the store (not a
 * component) owns each request's promise and records its outcome whether or
 * not anyone is still subscribed. Opening `/search` for a query the dialog
 * already summarized therefore reuses the answer instead of regenerating it.
 */

/** Settled summaries kept before the oldest are evicted. */
const MAX_ENTRIES = 50

const entries = new Map<string, SummaryState>()
const listeners = new Set<() => void>()
// Bumped by `resetSearchSummaries`, so a request started before a reset
// never writes into the fresh cache.
let generation = 0

function store(key: string, state: SummaryState): void {
  // Re-insert so Map order tracks recency for eviction.
  entries.delete(key)
  entries.set(key, state)
  for (const [oldKey, oldState] of entries) {
    if (entries.size <= MAX_ENTRIES) break
    // A loading entry is what dedupes its in-flight request; never drop it
    // (nor the entry just stored).
    if (oldKey !== key && oldState.status !== 'loading') entries.delete(oldKey)
  }
  for (const listener of listeners) listener()
}

/** The summary held for `key`, if one was ever requested. */
export function getSearchSummary(key: string): SummaryState | undefined {
  return entries.get(key)
}

/** Subscribe to cache changes (the `useSyncExternalStore` contract). */
export function subscribeSearchSummaries(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

/**
 * Request the summary for `key`. A no-op while that key is already loading,
 * so a fast double expand/retry — or both surfaces at once — sends one call.
 */
export function requestSearchSummary(
  key: string,
  teamId: string,
  request: SearchSummaryRequest
): void {
  if (entries.get(key)?.status === 'loading') return
  const requestGeneration = generation
  const settle = (state: SummaryState) => {
    if (requestGeneration === generation) store(key, state)
  }
  store(key, { status: 'loading' })
  searchService
    .summarize(teamId, request)
    .then(data => {
      settle({ status: 'ready', data })
    })
    .catch((error: unknown) => {
      settle({ status: 'error', ...classifySummaryError(error) })
    })
}

/** Drop every cached summary (tests; nothing in the app needs it). */
export function resetSearchSummaries(): void {
  generation += 1
  entries.clear()
  for (const listener of listeners) listener()
}
