/**
 * Secondary navigation for the `/teams/:id/**` scope (#539).
 *
 * Data lives here rather than in `TeamTabs.tsx` because a `.tsx` that exports
 * both a component and a const trips `react-refresh/only-export-components`
 * (an error in this repo's flat config). Keeping it separate also makes the
 * hrefs unit-testable without rendering.
 *
 * Mirrors `pages/admin/admin-nav.ts`: absolute hrefs so `NavLink` active
 * matching is unambiguous, and `end` on the index tab so it is not marked
 * active on every child path.
 */
export interface TeamTab {
  label: string
  /** Absolute path, built from the resolved team id. */
  href: string
  /** `end` for the index tab so child routes don't keep it active. */
  end?: boolean
}

/**
 * The tabs for one team.
 *
 * Projects joined in #542, together with the routes it points at - #539
 * deliberately shipped without it rather than link to a route that did not
 * exist yet.
 */
export function teamTabsFor(teamId: string): TeamTab[] {
  return [
    { label: 'Overview', href: `/teams/${teamId}`, end: true },
    { label: 'Projects', href: `/teams/${teamId}/projects` },
    { label: 'Analytics', href: `/teams/${teamId}/analytics` },
    { label: 'Settings', href: `/teams/${teamId}/settings` },
  ]
}
