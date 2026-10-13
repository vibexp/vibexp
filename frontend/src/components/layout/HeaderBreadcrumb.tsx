import { Link, useLocation } from 'react-router'

import { NAV_ITEMS, STATIC_PAGE_LEAVES } from '@/components/layout/nav-items'
import { useShell } from '@/components/layout/ShellContext'
import { trimEndChar } from '@/utils/linearText'

export interface Crumb {
  href: string
  label: string
}

/**
 * Resolves the current route to its nav entry by picking the longest matching
 * href across nav items and their children (so `/prompts/123` → "My Prompts",
 * `/` → "Dashboard"). Returns null when nothing matches.
 */
export function currentPageCrumb(pathname: string): Crumb | null {
  const entries: Crumb[] = []
  for (const item of NAV_ITEMS) {
    entries.push({ href: item.href, label: item.label })
    for (const child of item.children ?? []) {
      entries.push({ href: child.href, label: child.label })
    }
  }

  let best: Crumb | null = null
  for (const entry of entries) {
    const matches =
      entry.href === '/'
        ? pathname === '/'
        : pathname === entry.href || pathname.startsWith(entry.href + '/')
    if (matches && (!best || entry.href.length > best.href.length)) {
      best = entry
    }
  }
  return best
}

const DIVIDER = (
  <span aria-hidden className="hidden opacity-50 sm:inline">
    /
  </span>
)

/**
 * What a page's breadcrumb (and its tab title) is made of: the navigation
 * section the route belongs to, then the crumbs after it — those the page
 * named itself, or the fixed name of a static sub-page.
 */
export function resolveBreadcrumb(
  pathname: string,
  namedTrail: readonly string[]
): { section: Crumb | null; trail: readonly string[] } {
  const section = currentPageCrumb(pathname)
  if (namedTrail.length > 0) return { section, trail: namedTrail }
  const staticLeaf = STATIC_PAGE_LEAVES.get(trimEndChar(pathname, '/'))
  // A sub-page is listed by its own address; the section's own page is not a
  // crumb after itself.
  if (staticLeaf && staticLeaf !== section?.label) {
    return { section, trail: [staticLeaf] }
  }
  return { section, trail: [] }
}

/**
 * Topbar breadcrumb mirroring the DS docs ("VibeXP DS / Overview"): a muted
 * root crumb, a divider, and the current page in foreground weight. On the
 * smallest screens the root crumb yields to the page label, which truncates
 * rather than wrapping (#886).
 *
 * A page under a section gets further crumbs: "VibeXP / Artifacts / <title>",
 * "VibeXP / Settings / API keys", "VibeXP / Teams / <team> / Projects". The
 * section crumb then links back to its page — it is no longer where the
 * reader is — and the last crumb is the one that truncates.
 */
export function HeaderBreadcrumb() {
  const { pathname } = useLocation()
  const { breadcrumbTrail } = useShell()
  const { section, trail } = resolveBreadcrumb(pathname, breadcrumbTrail)
  const last = trail.at(-1)

  return (
    <nav
      aria-label="Breadcrumb"
      className="text-muted-foreground flex min-w-0 items-center gap-2 text-sm"
    >
      <span className="hidden sm:inline">VibeXP</span>
      {section && (
        <>
          {DIVIDER}
          {last === undefined ? (
            <span className="text-foreground truncate font-semibold">
              {section.label}
            </span>
          ) : (
            <Link
              to={section.href}
              className="hover:text-foreground hidden shrink-0 hover:underline sm:inline"
            >
              {section.label}
            </Link>
          )}
        </>
      )}
      {trail.slice(0, -1).map(crumb => (
        <span
          key={crumb}
          className="hidden shrink-0 items-center gap-2 lg:flex"
        >
          {DIVIDER}
          <span className="max-w-48 truncate">{crumb}</span>
        </span>
      ))}
      {last !== undefined && (
        <>
          {DIVIDER}
          <span
            className="text-foreground truncate font-semibold"
            aria-current="page"
          >
            {last}
          </span>
        </>
      )}
    </nav>
  )
}
