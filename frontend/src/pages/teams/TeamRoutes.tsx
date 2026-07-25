import { Route, Routes } from 'react-router-dom'

import { Customization } from '@/pages/teams/settings/customization/Customization'
import { EmbeddingProviders } from '@/pages/teams/settings/embedding-providers/EmbeddingProviders'
import { GitHubIntegration } from '@/pages/teams/settings/integrations/github/GitHubIntegration'
import { ModelProviders } from '@/pages/teams/settings/model-providers/ModelProviders'
import { SearchSettings } from '@/pages/teams/settings/search/SearchSettings'
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
      <Route path="settings" element={<TeamSettings team={team} />} />
      {/* The four team-scoped configuration pages, relocated in #540. They read
          the team from `useTeam()`, which TeamScopeLayout has already pointed at
          the URL's team — except SearchSettings, whose permission gating takes
          the resolved team explicitly so it cannot depend on that sync. */}
      <Route path="settings/search" element={<SearchSettings team={team} />} />
      <Route path="settings/model-providers" element={<ModelProviders />} />
      <Route
        path="settings/embedding-providers"
        element={<EmbeddingProviders />}
      />
      <Route path="settings/customization" element={<Customization />} />
      <Route
        path="settings/integrations/github"
        element={<GitHubIntegration />}
      />
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
