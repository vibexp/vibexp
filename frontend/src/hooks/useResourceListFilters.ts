import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { useUrlFilters } from '@/hooks/useUrlFilters'
import type { MetadataFilterValue } from '@/services/metadataService'
import {
  parseMetadataFilter,
  serializeMetadataFilter,
} from '@/services/metadataService'

const SEARCH_DEBOUNCE_MS = 500

/** The filter keys every metadata-filterable resource list carries. */
export interface ResourceListBaseFilters {
  page: string
  search: string
  metadata: string
  sort_order: string
  [key: string]: string
}

export interface UseResourceListFiltersOptions {
  /** Filter defaults, including the base keys above plus the page's own. */
  defaults: ResourceListBaseFilters
  /**
   * Domain filter keys that make the view "filtered" beyond search and
   * metadata (e.g. `type`, `status`). Drives which empty state is shown and
   * whether Clear is offered.
   */
  filterKeys: readonly string[]
  /** The globally selected project, from the header selector. */
  projectId: string | undefined
  /** True while a persisted project selection is still restoring. */
  isProjectLoading: boolean
  debounceMs?: number
}

/**
 * The shared filter plumbing behind the Blueprints, Artifacts and Memories list
 * pages (epic #519): URL-synced filters, a debounced search box, and the
 * metadata filter's URL round-trip.
 *
 * It exists because the three pages were otherwise verbatim copies of each
 * other — the same mistake `useAdminListFilters` was extracted to fix on the
 * admin side. That hook is admin-shaped (it mandates a created-date range), so
 * this is its resource-list sibling rather than a reuse of it.
 *
 * Two behaviours are easy to get wrong and are handled here once:
 *
 * - **The metadata param is re-serialized into a canonical string** that is
 *   what callers should use as both the fetch dependency and the request
 *   value. A malformed URL param is therefore never forwarded, and the fetch
 *   effect does not re-run merely because the parsed object is referentially
 *   new.
 * - **A persisted project RESTORING is not a project change.** Resetting the
 *   page on it would clobber the `?page=3` of a shared link, so the guard is
 *   armed only once `isProjectLoading` goes false — seeding it on first render
 *   is not enough, because the project is still undefined at that point and the
 *   restore itself then looks like a change.
 */
export function useResourceListFilters({
  defaults,
  filterKeys,
  projectId,
  isProjectLoading,
  debounceMs = SEARCH_DEBOUNCE_MS,
}: Readonly<UseResourceListFiltersOptions>) {
  const { filters, setFilters, resetFilters } = useUrlFilters({ ...defaults })

  // Uncommitted text in the search box, debounced into the URL below.
  const [searchInput, setSearchInput] = useState(filters.search)

  useEffect(() => {
    const timer = setTimeout(() => {
      if (searchInput !== filters.search) {
        setFilters({ search: searchInput })
      }
    }, debounceMs)
    return () => {
      clearTimeout(timer)
    }
  }, [searchInput, filters.search, setFilters, debounceMs])

  const previousProjectRef = useRef<string | undefined>(undefined)
  const projectSyncArmedRef = useRef(false)
  useEffect(() => {
    if (isProjectLoading) return
    if (!projectSyncArmedRef.current) {
      projectSyncArmedRef.current = true
      previousProjectRef.current = projectId
      return
    }
    if (previousProjectRef.current === projectId) return
    previousProjectRef.current = projectId
    setFilters({ page: defaults.page })
  }, [projectId, isProjectLoading, setFilters, defaults.page])

  const metadata = useMemo(
    () => parseMetadataFilter(filters.metadata),
    [filters.metadata]
  )
  const metadataParam = useMemo(
    () => serializeMetadataFilter(metadata),
    [metadata]
  )

  const page = Number(filters.page) || 1
  const sortOrder: 'asc' | 'desc' =
    filters.sort_order === 'asc' ? 'asc' : 'desc'

  const hasActiveFilters =
    filters.search !== '' ||
    Object.keys(metadata).length > 0 ||
    filterKeys.some(key => filters[key] !== defaults[key])

  const setPage = useCallback(
    (next: number) => {
      setFilters({ page: String(next) })
    },
    [setFilters]
  )

  const setMetadata = useCallback(
    (next: MetadataFilterValue) => {
      setFilters({ metadata: serializeMetadataFilter(next) ?? '' })
    },
    [setFilters]
  )

  const handleClear = useCallback(() => {
    // Stale text left in the box would be re-committed on the next debounce
    // tick and undo the clear.
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
    sortOrder,
    metadata,
    metadataParam,
    setMetadata,
    hasActiveFilters,
    handleClear,
  }
}
