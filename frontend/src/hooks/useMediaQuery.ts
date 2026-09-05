import { useCallback, useSyncExternalStore } from 'react'

/**
 * Tailwind v4 default breakpoints, as media queries. Keep these in sync with
 * the `md:` / `lg:` prefixes used in the shell so JS-gated rendering and
 * CSS-gated styling agree on where a breakpoint sits.
 */
export const BREAKPOINT_QUERIES = {
  /** ≥ 768px — tablet and up. */
  md: '(min-width: 48rem)',
  /** ≥ 1024px — desktop. */
  lg: '(min-width: 64rem)',
} as const

function canMatchMedia(): boolean {
  return (
    typeof window !== 'undefined' && typeof window.matchMedia === 'function'
  )
}

/**
 * Subscribes to a CSS media query. Returns `fallback` wherever `matchMedia`
 * is unavailable (SSR, jsdom), so tests and non-browser renders behave like a
 * desktop viewport by default instead of throwing.
 */
export function useMediaQuery(query: string, fallback = true): boolean {
  const subscribe = useCallback(
    (onChange: () => void) => {
      if (!canMatchMedia()) return (): void => undefined
      const mql = window.matchMedia(query)
      mql.addEventListener('change', onChange)
      return () => {
        mql.removeEventListener('change', onChange)
      }
    },
    [query]
  )

  const getSnapshot = useCallback(
    () => (canMatchMedia() ? window.matchMedia(query).matches : fallback),
    [query, fallback]
  )

  const getServerSnapshot = useCallback(() => fallback, [fallback])

  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot)
}
