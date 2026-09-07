import { useLocation } from 'react-router'

/**
 * `NavLink`'s own `isActive`, computed outside the component.
 *
 * Why this exists: a Radix `asChild` trigger slots its props onto the child,
 * and `Slot` merges `className` by string-joining — which stringifies
 * `NavLink`'s *function* `className` into garbage classes. The rails therefore
 * pass a plain string and need `isActive` from somewhere else (#891).
 *
 * Why not `useMatch`: it is the primitive `NavLink` uses, but the two disagree
 * on a trailing slash. `matchPath` compiles `end: true` with a trailing `\/*$`,
 * so `/admin/` matches `{ path: '/admin', end: true }`, while `NavLink`'s own
 * check is strict equality and returns false. The row would then be styled
 * active while `NavLink` withheld `aria-current="page"` — a discrepancy that
 * could not exist before, when both came from one source. This mirrors
 * `NavLink`'s computation exactly instead (react-router
 * `dist/**\/lib/dom/lib.js`, `let isActive = …`), including the default
 * case-insensitive comparison, so the highlight and `aria-current` can never
 * drift apart.
 */
export function useNavLinkActive(href: string, end = false): boolean {
  const { pathname } = useLocation()
  const location = pathname.toLowerCase()
  const target = href.toLowerCase()
  // Where a descendant match's separator has to sit: one char earlier when the
  // target itself ends in a slash, exactly as NavLink computes it.
  const boundary =
    target !== '/' && target.endsWith('/') ? target.length - 1 : target.length

  return (
    location === target ||
    (!end && location.startsWith(target) && location.charAt(boundary) === '/')
  )
}
