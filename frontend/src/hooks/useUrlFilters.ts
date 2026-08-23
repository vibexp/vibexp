import { useCallback, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router'

/** A filter set: flat string values, so it maps 1:1 onto the query string. */
export type UrlFilters = Record<string, string | undefined>

/**
 * Syncs a flat filter object to the query string.
 *
 * Added here, in the admin-shell foundation, rather than three times over in
 * #459/#460/#461: no list page in the SPA URL-syncs its filters today (they all
 * hold them in `useState`), so building it per page would have guaranteed three
 * divergent param encodings.
 *
 * Behaviour:
 * - **The URL is the state.** There is no `useState` mirror, so a shared link, a
 *   reload and the back button all reproduce the same view — the whole reason to
 *   URL-sync instead of keeping filters in component state.
 * - **Defaults and empty values stay out of the URL**, so a list page with
 *   nothing applied has a clean address bar, and two ways of reaching the same
 *   view produce the same string. A value equal to its default is *removed*
 *   rather than written.
 * - **Changing any filter resets `page` to its default** (when `page` is one of
 *   the filters), because page 7 of a narrowed result set is usually empty — the
 *   classic filter-pagination bug. Setting `page` alone is exempt, or paging
 *   could never advance.
 * - Params outside `defaults` are never touched, so a page can adopt this hook
 *   while keeping unrelated query params of its own.
 *
 * Updates `replace:` the history entry rather than pushing, so typing in a
 * search box does not bury the previous page under one entry per keystroke.
 *
 * **The first `defaults` wins.** Callers pass an inline object literal (often
 * `{ ...FILTER_DEFAULTS }`), so a fresh identity arrives on every render and the
 * hook cannot treat it as reactive without recomputing `filters` — and a new
 * `filters` identity on every render would re-fire every fetch effect that
 * depends on it. It is therefore captured once, on mount, and assumed constant
 * for the life of the component; a caller needing different defaults should
 * remount (a `key`) rather than mutate the object it passes.
 *
 * Note for the wrapper hooks (`useResourceListFilters`, `useAdminListFilters`):
 * they consult their own live `defaults` prop alongside this hook's frozen copy.
 * The two agree only because every caller passes a module-level constant, which
 * is the contract above — keep it that way.
 */
export function useUrlFilters<T extends UrlFilters>(defaults: T) {
  const [searchParams, setSearchParams] = useSearchParams()

  // Captured in state rather than a ref: `filters` is derived during render, so
  // the defaults it derives from have to be readable during render too, and a
  // ref's `current` is neither readable there nor trackable as a memo
  // dependency. State gives one stable object for the component's lifetime.
  const [initialDefaults] = useState(defaults)

  // Key the memo off the serialized query string rather than the
  // `URLSearchParams` instance, which react-router replaces on every render, so
  // `filters` is referentially stable and safe in a fetch effect's deps.
  const search = searchParams.toString()

  const filters = useMemo(() => {
    const params = new URLSearchParams(search)
    const resolved: UrlFilters = {}
    for (const [key, fallback] of Object.entries(initialDefaults)) {
      resolved[key] = params.get(key) ?? fallback
    }
    return resolved as T
  }, [search, initialDefaults])

  const setFilters = useCallback(
    (next: Partial<T>) => {
      const changesOtherThanPage = Object.keys(next).some(key => key !== 'page')
      const updates: UrlFilters = { ...next }
      if (changesOtherThanPage && 'page' in initialDefaults) {
        updates.page = initialDefaults.page
      }

      setSearchParams(
        current => {
          const params = new URLSearchParams(current)
          for (const [key, value] of Object.entries(updates)) {
            // Empty string, undefined, and "same as the default" all mean
            // "not applied", so none of them belong in the URL.
            if (!value || value === initialDefaults[key]) {
              params.delete(key)
            } else {
              params.set(key, value)
            }
          }
          return params
        },
        { replace: true }
      )
    },
    [setSearchParams, initialDefaults]
  )

  const resetFilters = useCallback(() => {
    setSearchParams(
      current => {
        const params = new URLSearchParams(current)
        // Clear only the keys this hook owns; unrelated params survive.
        for (const key of Object.keys(initialDefaults)) {
          params.delete(key)
        }
        return params
      },
      { replace: true }
    )
  }, [setSearchParams, initialDefaults])

  return { filters, setFilters, resetFilters }
}
