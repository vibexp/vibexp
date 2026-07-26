import { Route, Routes } from 'react-router-dom'

import { ProjectCreate } from '@/pages/teams/projects/ProjectCreate'
import { ProjectDetails } from '@/pages/teams/projects/ProjectDetails'
import { ProjectEdit } from '@/pages/teams/projects/ProjectEdit'
import { ProjectMigrate } from '@/pages/teams/projects/ProjectMigrate'
import { Projects } from '@/pages/teams/projects/Projects'
import { Customization } from '@/pages/teams/settings/customization/Customization'
import { EmailProvider } from '@/pages/teams/settings/email-provider/EmailProvider'
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
 *
 * `reloadToken` is bumped by the layout when a header action changed the team
 * or its roster (#666). Only the Overview tab renders roster data, so it is the
 * only route that takes it.
 */
export function TeamRoutes({
  team,
  reloadToken = 0,
}: Readonly<{ team: Team; reloadToken?: number }>) {
  return (
    <Routes>
      <Route index element={<TeamDetailsPage reloadToken={reloadToken} />} />
      <Route path="analytics" element={<TeamAnalyticsPage />} />
      {/* Projects sit beside analytics, NOT under settings/ (#542). `create`
          before `:slug` for legibility; `:slug/edit` matches every other
          resource in the app rather than the old `edit/:slug`. */}
      <Route path="projects" element={<Projects />} />
      <Route path="projects/create" element={<ProjectCreate />} />
      <Route path="projects/:slug" element={<ProjectDetails />} />
      <Route path="projects/:slug/edit" element={<ProjectEdit />} />
      <Route path="projects/:slug/migrate" element={<ProjectMigrate />} />
      <Route path="settings" element={<TeamSettings team={team} />} />
      {/* The team-scoped configuration pages, relocated in #540 and extended
          with the email provider in #506. Each one takes the team the layout
          resolved from the URL — none of them may read `useTeam()` (#584).
          React fires child effects before parent effects, so on a cold
          deep-link a page's load effect runs BEFORE TeamScopeLayout's
          `setCurrentTeam` sync, and the ambient team is still whichever one was
          last persisted. */}
      <Route path="settings/search" element={<SearchSettings team={team} />} />
      <Route
        path="settings/email-provider"
        element={<EmailProvider team={team} />}
      />
      <Route
        path="settings/model-providers"
        element={<ModelProviders team={team} />}
      />
      <Route
        path="settings/embedding-providers"
        element={<EmbeddingProviders team={team} />}
      />
      <Route
        path="settings/customization"
        element={<Customization team={team} />}
      />
      <Route
        path="settings/integrations/github"
        element={<GitHubIntegration team={team} />}
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
