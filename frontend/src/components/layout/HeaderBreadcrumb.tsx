import { useLocation } from 'react-router'

import { NAV_ITEMS } from '@/components/layout/nav-items'

/**
 * Resolves the current route to its nav label by picking the longest matching
 * href across nav items and their children (so `/prompts/123` → "My Prompts",
 * `/` → "Dashboard"). Returns null when nothing matches.
 */
function currentPageLabel(pathname: string): string | null {
  const entries: { href: string; label: string }[] = []
  for (const item of NAV_ITEMS) {
    entries.push({ href: item.href, label: item.label })
    for (const child of item.children ?? []) {
      entries.push({ href: child.href, label: child.label })
    }
  }

  let best: { href: string; label: string } | null = null
  for (const entry of entries) {
    const matches =
      entry.href === '/'
        ? pathname === '/'
        : pathname === entry.href || pathname.startsWith(entry.href + '/')
    if (matches && (!best || entry.href.length > best.href.length)) {
      best = entry
    }
  }
  return best?.label ?? null
}

/**
 * Topbar breadcrumb mirroring the DS docs ("VibeXP DS / Overview"): a muted
 * root crumb, a divider, and the current page in foreground weight. On the
 * smallest screens the root crumb yields to the page label, which truncates
 * rather than wrapping (#886).
 */
export function HeaderBreadcrumb() {
  const { pathname } = useLocation()
  const label = currentPageLabel(pathname)

  return (
    <nav
      aria-label="Breadcrumb"
      className="text-muted-foreground flex min-w-0 items-center gap-2 text-sm"
    >
      <span className="hidden sm:inline">VibeXP</span>
      {label && (
        <>
          <span aria-hidden className="hidden opacity-50 sm:inline">
            /
          </span>
          <span className="text-foreground truncate font-semibold">
            {label}
          </span>
        </>
      )}
    </nav>
  )
}
