import { useCallback, useEffect, useMemo, useState } from 'react'

import type { DateRangeValue } from '@/components/ui/date-range'
import {
  fromDateParam,
  rangeToInstants,
  toDateParam,
} from '@/components/ui/date-range'
import { useUrlFilters } from '@/hooks/useUrlFilters'

import type {
  AdvancedFilterSpec,
  NumberRangeValue,
} from './filters/advancedFilterParams'
import {
  advancedKeys,
  dateRangeKeys,
  parseDateRange,
  parseRange,
  parseTriState,
  rangeKeys,
  sanitizeAdvanced,
  serializeRange,
  serializeTriState,
} from './filters/advancedFilterParams'

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
  /**
   * Filters shown in the "Advanced filters" panel (#1132). Their URL keys are
   * owned by this hook: cleaned into `advancedParams`, counted in
   * `hasActiveFilters`, and reset by Clear.
   *
   * Must be a **module-level constant**, like `defaults`: `useUrlFilters` freezes
   * the keys it owns on the first render, so a declaration that changes later
   * would not be picked up.
   */
  advanced?: AdvancedFilterSpec
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
 * - the filter state round-trips as a flat query (`currentQuery` / `applyQuery`),
 *   which is what saved filter presets store and restore (#1148)
 */
export function useAdminListFilters<TSort extends string>({
  defaults,
  sortableKeys,
  defaultSort,
  filterKeys,
  advanced,
}: UseAdminListFiltersOptions<TSort>) {
  // Advanced keys join the defaults as `''`, which is what makes Clear
  // (`resetFilters`) remove them from the URL too.
  const urlDefaults: AdminListBaseFilters = {
    ...Object.fromEntries(advancedKeys(advanced).map(key => [key, ''])),
    ...defaults,
  }
  const { filters, setFilters, resetFilters } = useUrlFilters(urlDefaults)
  // Every URL key this page owns except `page`: what a saved preset captures
  // and what applying one overwrites (#1148). Frozen like `useUrlFilters`' own
  // copy, since `defaults` and `advanced` are module-level constants.
  const [presetKeys] = useState(() =>
    Object.keys(urlDefaults).filter(key => key !== 'page')
  )
  const [presetDefaults] = useState(urlDefaults)
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

  // Invalid URL values (non-integer, min > max, malformed day) never reach
  // `advancedParams`, so they are never sent to the API.
  const { params: advancedParams, activeCount: advancedActiveCount } = useMemo(
    () => sanitizeAdvanced(advanced, filters),
    [advanced, filters]
  )

  const hasActiveFilters =
    filters.search !== '' ||
    filters.created_from !== '' ||
    filters.created_to !== '' ||
    filterKeys.some(key => filters[key] !== defaults[key]) ||
    advancedActiveCount > 0

  const getRange = useCallback(
    (name: string): NumberRangeValue => {
      const [minKey, maxKey] = rangeKeys(name)
      return parseRange(filters[minKey], filters[maxKey])
    },
    [filters]
  )

  const setRange = useCallback(
    (name: string, value: NumberRangeValue) => {
      setFilters(serializeRange(name, value))
    },
    [setFilters]
  )

  const getDateRange = useCallback(
    (name: string): DateRangeValue => {
      const [fromKey, toKey] = dateRangeKeys(name)
      return parseDateRange(filters[fromKey], filters[toKey])
    },
    [filters]
  )

  const setDateRange = useCallback(
    (name: string, value: DateRangeValue) => {
      const [fromKey, toKey] = dateRangeKeys(name)
      setFilters({
        [fromKey]: value.from ? toDateParam(value.from) : '',
        [toKey]: value.to ? toDateParam(value.to) : '',
      })
    },
    [setFilters]
  )

  const getTriState = useCallback(
    (name: string): boolean | undefined => parseTriState(filters[name]),
    [filters]
  )

  const setTriState = useCallback(
    (name: string, value: boolean | undefined) => {
      setFilters({ [name]: serializeTriState(value) })
    },
    [setFilters]
  )

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

  // The owned filters that differ from their defaults — exactly what a saved
  // preset stores. Sort is part of the slice; the page number is not.
  const currentQuery = useMemo(
    () =>
      Object.fromEntries(
        presetKeys
          .filter(key => filters[key] && filters[key] !== presetDefaults[key])
          .map(key => [key, filters[key]])
      ),
    [filters, presetKeys, presetDefaults]
  )

  const applyQuery = useCallback(
    (query: Readonly<Record<string, string | undefined>>) => {
      // Same reason as Clear: the box must show the preset's search, or the next
      // debounce tick re-commits the old text over it.
      setSearchInput(query.search ?? '')
      // One update covering every owned key, so the filters the preset lacks are
      // removed rather than left in place. Keys this page does not own are
      // ignored; malformed values fall out through the usual URL guards.
      setFilters(
        Object.fromEntries(presetKeys.map(key => [key, query[key] ?? '']))
      )
    },
    [presetKeys, setFilters]
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
    currentQuery,
    applyQuery,
    advancedParams,
    advancedActiveCount,
    getRange,
    setRange,
    getDateRange,
    setDateRange,
    getTriState,
    setTriState,
  }
}
