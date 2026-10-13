import type { ListPageStatus } from './types'

/**
 * Resolves the ListPage status from the three states every list page tracks.
 * Shared by the resource list pages (agents, artifacts, blueprints, memories,
 * prompts, projects) so the loading → error → empty → ready precedence stays
 * identical everywhere.
 */
export function listPageStatus(
  loading: boolean,
  error: unknown,
  isEmpty: boolean
): ListPageStatus {
  if (loading) return 'loading'
  if (error) return 'error'
  return isEmpty ? 'empty' : 'ready'
}

/**
 * True when a list has nothing in it and nothing is filtering it: the blank
 * slate. There is nothing to filter, so the page drops its filter row and its
 * header action, and the empty state carries the one primary action. A list
 * emptied BY its filters is not a blank slate — it keeps the filters and offers
 * to clear them.
 */
export function isBlankSlate(
  status: ListPageStatus,
  hasActiveFilters: boolean
): boolean {
  return status === 'empty' && !hasActiveFilters
}
