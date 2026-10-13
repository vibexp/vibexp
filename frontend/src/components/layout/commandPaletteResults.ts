import type { SearchResultItem } from '@/services/searchService'

/** The order result groups are listed in, and their headings. */
export const RESULT_GROUPS: {
  type: SearchResultItem['type']
  heading: string
}[] = [
  { type: 'prompt', heading: 'Prompts' },
  { type: 'artifact', heading: 'Artifacts' },
  { type: 'blueprint', heading: 'Blueprints' },
  { type: 'memory', heading: 'Memories' },
]

export interface ResultGroup {
  type: SearchResultItem['type']
  heading: string
  items: SearchResultItem[]
}

/**
 * Search results as the palette lists them: one row per RESOURCE, grouped by
 * type in a fixed order, each group keeping the endpoint's relevance order.
 *
 * The endpoint returns matched chunks, so a long resource can come back
 * several times; the palette is for jumping to a resource, and shows each
 * once — its best-ranked hit. Empty groups are left out.
 */
export function groupResults(
  results: readonly SearchResultItem[]
): ResultGroup[] {
  const seen = new Set<string>()
  const unique = results.filter(item => {
    const key = `${item.type}:${item.id}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
  return RESULT_GROUPS.map(group => ({
    ...group,
    items: unique.filter(item => item.type === group.type),
  })).filter(group => group.items.length > 0)
}

/** The full results page for a query. */
export const searchPagePath = (query: string): string =>
  `/search?q=${encodeURIComponent(query)}`
