import { useLocation } from 'react-router-dom'

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
 * root crumb, a divider, and the current page in foreground weight. Hidden on
 * the smallest screens where the mobile hamburger already provides context.
 */
export function HeaderBreadcrumb() {
  const { pathname } = useLocation()
  const label = currentPageLabel(pathname)

  return (
    <nav
      aria-label="Breadcrumb"
      className="text-muted-foreground hidden items-center gap-2 text-sm sm:flex"
    >
      <span>VibeXP</span>
      {label && (
        <>
          <span aria-hidden className="opacity-50">
            /
          </span>
          <span className="text-foreground font-semibold">{label}</span>
        </>
      )}
    </nav>
  )
}
