import { Route, Routes } from 'react-router-dom'

import { TeamSettings } from '@/pages/teams/settings/TeamSettings'
import { TeamAnalyticsPage } from '@/pages/teams/TeamAnalyticsPage'
import { TeamDetailsPage } from '@/pages/teams/TeamDetailsPage'
import type { Team } from '@/services/teamService'

/**
 * Routes for the team scope, relative to the `teams/:id/*` mount point in
 * `routes.tsx` (#539).
 *
 * Mirrors `pages/admin/AdminRoutes.tsx`: a `path="x/*"` mount whose element
 * renders chrome plus this nested `<Routes>`. That is the established idiom for
 * a scoped subtree in this SPA — `<Outlet>` appears nowhere in the codebase, so
 * using it here would introduce a second routing pattern.
 *
 * `TeamDetailsPage` and `TeamAnalyticsPage` still resolve the team from `:id`
 * themselves, exactly as they did after #538, so this mount changes nothing
 * about how they behave. Only `TeamSettings` takes the team the layout already
 * resolved.
 */
export function TeamRoutes({ team }: Readonly<{ team: Team }>) {
  return (
    <Routes>
      <Route index element={<TeamDetailsPage />} />
      <Route path="analytics" element={<TeamAnalyticsPage />} />
      {/* `settings/*`, not `settings`: #540 and #541 nest pages underneath. */}
      <Route path="settings/*" element={<TeamSettings team={team} />} />
      <Route path="*" element={<TeamNotFound />} />
    </Routes>
  )
}

/**
 * In-shell 404 so an unknown `/teams/:id/**` path keeps the team chrome and tab
 * bar instead of bouncing the user out to the app-level NotFound. Same intent
 * as `AdminRoutes`' `AdminNotFound`.
 */
function TeamNotFound() {
  return (
    <div className="space-y-2">
      <h1 className="text-xl font-semibold">Page not found</h1>
      <p className="text-muted-foreground text-sm">
        That team page does not exist.
      </p>
    </div>
  )
}
