import { Link, useLocation } from 'react-router'

import { NAV_ITEMS } from '@/components/layout/nav-items'
import { useShell } from '@/components/layout/ShellContext'

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
 * Topbar breadcrumb mirroring the DS docs ("VibeXP DS / Overview"): a muted
 * root crumb, a divider, and the current page in foreground weight. On the
 * smallest screens the root crumb yields to the page label, which truncates
 * rather than wrapping (#886).
 *
 * A page about one thing names it (`useBreadcrumbLeaf`) and gets a third
 * crumb: "VibeXP / Artifacts / <title>". The section crumb then links back to
 * its list — it is no longer where the reader is — and the title is the one
 * that truncates.
 */
export function HeaderBreadcrumb() {
  const { pathname } = useLocation()
  const { breadcrumbLeaf } = useShell()
  const crumb = currentPageCrumb(pathname)

  return (
    <nav
      aria-label="Breadcrumb"
      className="text-muted-foreground flex min-w-0 items-center gap-2 text-sm"
    >
      <span className="hidden sm:inline">VibeXP</span>
      {crumb && (
        <>
          {DIVIDER}
          {breadcrumbLeaf ? (
            <Link
              to={crumb.href}
              className="hover:text-foreground hidden shrink-0 hover:underline sm:inline"
            >
              {crumb.label}
            </Link>
          ) : (
            <span className="text-foreground truncate font-semibold">
              {crumb.label}
            </span>
          )}
        </>
      )}
      {breadcrumbLeaf && (
        <>
          {crumb && DIVIDER}
          <span
            className="text-foreground truncate font-semibold"
            aria-current="page"
          >
            {breadcrumbLeaf}
          </span>
        </>
      )}
    </nav>
  )
}
