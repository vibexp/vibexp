import { useCallback, useEffect, useMemo, useState } from 'react'

import type { DateRangeValue } from '@/components/ui/date-range'
import {
  fromDateParam,
  rangeToInstants,
  toDateParam,
} from '@/components/ui/date-range'
import { useUrlFilters } from '@/hooks/useUrlFilters'

/** Every admin list page carries these; a page adds its own domain filters. */
export interface AdminListBaseFilters {
  page: string
  search: string
  created_from: string
  created_to: string
  sort_by: string
  sort_order: string
  [key: string]: string
}

const SEARCH_DEBOUNCE_MS = 400

export interface UseAdminListFiltersOptions<TSort extends string> {
  /** Filter defaults, including the base keys above. Omitted from the URL when unchanged. */
  defaults: AdminListBaseFilters
  /** Columns the API accepts for `sort_by`. Anything else in the URL falls back. */
  sortableKeys: readonly TSort[]
  defaultSort: TSort
  /**
   * Domain filter keys that make the view "filtered", beyond `search` and the
   * date range. Drives which empty state is shown and whether Clear is offered.
   */
  filterKeys: readonly string[]
}

/**
 * The filter/sort/pagination plumbing shared by the admin list pages.
 *
 * Extracted once there were three near-identical copies (Teams #460, Projects
 * #461, Users #459): the URL sync, the debounced search box, the sort toggle, the
 * date-range conversion and the clear behaviour were the same eighty lines each
 * time, which is both a maintenance cost and a duplication the quality gate
 * rightly flags. Each page keeps only its own columns, query call and domain
 * filters.
 *
 * What it guarantees for every page at once:
 * - the URL is the state, so a filtered view is shareable and reload-proof
 * - typing does not fire a request per keystroke, or a history entry per keystroke
 * - a `sort_by` the API would reject with a 400 never leaves the browser
 * - the date range travels as readable local days in the URL and as instants in
 *   the request, with the upper bound at local end-of-day
 * - clearing resets the search box too, so the next debounce tick cannot restore it
 */
export function useAdminListFilters<TSort extends string>({
  defaults,
  sortableKeys,
  defaultSort,
  filterKeys,
}: UseAdminListFiltersOptions<TSort>) {
  const { filters, setFilters, resetFilters } = useUrlFilters({ ...defaults })
  // Uncommitted text in the search box, debounced into the URL below.
  const [searchInput, setSearchInput] = useState(filters.search)

  useEffect(() => {
    const timer = setTimeout(() => {
      if (searchInput !== filters.search) {
        setFilters({ search: searchInput })
      }
    }, SEARCH_DEBOUNCE_MS)
    return () => {
      clearTimeout(timer)
    }
  }, [searchInput, filters.search, setFilters])

  const created: DateRangeValue = useMemo(
    () => ({
      from: fromDateParam(filters.created_from),
      to: fromDateParam(filters.created_to),
    }),
    [filters.created_from, filters.created_to]
  )

  const { from: createdFrom, to: createdTo } = useMemo(
    () => rangeToInstants(created),
    [created]
  )

  const page = Number(filters.page) || 1

  const sortBy = (sortableKeys as readonly string[]).includes(filters.sort_by)
    ? (filters.sort_by as TSort)
    : defaultSort
  const sortOrder: 'asc' | 'desc' =
    filters.sort_order === 'asc' ? 'asc' : 'desc'

  const hasActiveFilters =
    filters.search !== '' ||
    filters.created_from !== '' ||
    filters.created_to !== '' ||
    filterKeys.some(key => filters[key] !== defaults[key])

  const handleSortChange = useCallback(
    (key: TSort) => {
      // Clicking the active column flips direction; a new column starts
      // descending, the useful default for both dates and counts.
      setFilters(
        key === sortBy
          ? { sort_order: sortOrder === 'asc' ? 'desc' : 'asc' }
          : { sort_by: key, sort_order: 'desc' }
      )
    },
    [setFilters, sortBy, sortOrder]
  )

  const setCreated = useCallback(
    (value: DateRangeValue) => {
      setFilters({
        created_from: value.from ? toDateParam(value.from) : '',
        created_to: value.to ? toDateParam(value.to) : '',
      })
    },
    [setFilters]
  )

  const setPage = useCallback(
    (next: number) => {
      setFilters({ page: String(next) })
    },
    [setFilters]
  )

  const handleClear = useCallback(() => {
    // Stale text left in the box would be re-committed on the next debounce tick
    // and undo the clear.
    setSearchInput('')
    resetFilters()
  }, [resetFilters])

  return {
    filters,
    setFilters,
    searchInput,
    setSearchInput,
    page,
    setPage,
    sortBy,
    sortOrder,
    created,
    setCreated,
    createdFrom,
    createdTo,
    hasActiveFilters,
    handleSortChange,
    handleClear,
  }
}
