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
