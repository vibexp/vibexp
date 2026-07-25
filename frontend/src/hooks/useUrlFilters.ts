import { useCallback, useEffect, useMemo, useRef } from 'react'
import { useSearchParams } from 'react-router-dom'

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
 * `defaults` is read through a ref, so passing an inline object literal is fine;
 * it is assumed constant for the life of the component.
 */
export function useUrlFilters<T extends UrlFilters>(defaults: T) {
  const [searchParams, setSearchParams] = useSearchParams()

  const defaultsRef = useRef(defaults)
  useEffect(() => {
    defaultsRef.current = defaults
  }, [defaults])

  // Key the memo off the serialized query string rather than the
  // `URLSearchParams` instance, which react-router replaces on every render, so
  // `filters` is referentially stable and safe in a fetch effect's deps.
  const search = searchParams.toString()

  const filters = useMemo(() => {
    const params = new URLSearchParams(search)
    const resolved: UrlFilters = {}
    for (const [key, fallback] of Object.entries(defaultsRef.current)) {
      resolved[key] = params.get(key) ?? fallback
    }
    return resolved as T
  }, [search])

  const setFilters = useCallback(
    (next: Partial<T>) => {
      const currentDefaults = defaultsRef.current
      const changesOtherThanPage = Object.keys(next).some(key => key !== 'page')
      const updates: UrlFilters = { ...next }
      if (changesOtherThanPage && 'page' in currentDefaults) {
        updates.page = currentDefaults.page
      }

      setSearchParams(
        current => {
          const params = new URLSearchParams(current)
          for (const [key, value] of Object.entries(updates)) {
            // Empty string, undefined, and "same as the default" all mean
            // "not applied", so none of them belong in the URL.
            if (!value || value === currentDefaults[key]) {
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
    [setSearchParams]
  )

  const resetFilters = useCallback(() => {
    setSearchParams(
      current => {
        const params = new URLSearchParams(current)
        // Clear only the keys this hook owns; unrelated params survive.
        for (const key of Object.keys(defaultsRef.current)) {
          params.delete(key)
        }
        return params
      },
      { replace: true }
    )
  }, [setSearchParams])

  return { filters, setFilters, resetFilters }
}
