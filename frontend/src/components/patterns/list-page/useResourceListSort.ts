import { useCallback, useMemo } from 'react'

import type { ResourceDescriptor } from '@/components/patterns/resource'

import type { SortDir } from './types'

/**
 * Descriptor-driven sorting for a resource list (#908).
 *
 * Which columns a list may be sorted by used to be a per-page constant, and the
 * four pages had drifted to three keys, three keys, one and one for no product
 * reason — with three different click behaviours to match. `descriptor.list`
 * now carries the keys, and this carries the behaviour:
 *
 * - clicking the active column flips the direction;
 * - a new column starts in the direction that reads naturally for it — A–Z for
 *   the name column, newest-first for everything else;
 * - a `sort_by` the URL happens to contain but the descriptor does not declare
 *   falls back, because the list endpoints answer an unknown `sort_by` with a
 *   400 rather than ignoring it.
 */
export interface UseResourceListSortOptions {
  descriptor: ResourceDescriptor
  /** The raw `sort_by` from the URL. */
  sortBy: string
  sortOrder: SortDir
  setFilters: (next: Record<string, string>) => void
  /** Used when the URL names a key the descriptor does not declare. */
  fallback?: string
}

export interface UseResourceListSortResult {
  sortableKeys: readonly string[]
  sortKey: string
  onSortChange: (key: string) => void
}

export function useResourceListSort({
  descriptor,
  sortBy,
  sortOrder,
  setFilters,
  fallback = 'updated_at',
}: Readonly<UseResourceListSortOptions>): UseResourceListSortResult {
  // Memoised on the descriptor rather than on the list: a kind with no `list`
  // section falls back to a fresh `[]` on every render, which would make both
  // `sortableKeys` and the Set below new identities each time.
  const sortableKeys = useMemo(
    () => descriptor.list?.sortable ?? [],
    [descriptor]
  )
  const nameKey = descriptor.fields.find(field => field.role === 'name')?.key

  // Membership only, so a Set rather than an array scan (Sonar S7776); the
  // array itself still goes to `ListTable`, which cares about the order.
  const allowed = useMemo(() => new Set<string>(sortableKeys), [sortableKeys])

  const sortKey = allowed.has(sortBy) ? sortBy : fallback

  const onSortChange = useCallback(
    (key: string) => {
      if (key === sortKey) {
        setFilters({ sort_order: sortOrder === 'asc' ? 'desc' : 'asc' })
        return
      }
      setFilters({ sort_by: key, sort_order: key === nameKey ? 'asc' : 'desc' })
    },
    [nameKey, setFilters, sortKey, sortOrder]
  )

  return { sortableKeys, sortKey, onSortChange }
}
