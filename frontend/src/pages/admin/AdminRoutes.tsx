import { Route, Routes } from 'react-router-dom'

import { AdminDashboard } from '@/pages/admin/AdminDashboard'
import { AdminProjectDetail } from '@/pages/admin/AdminProjectDetail'
import { AdminProjects } from '@/pages/admin/AdminProjects'
import { AdminTeamDetail } from '@/pages/admin/AdminTeamDetail'
import { AdminTeams } from '@/pages/admin/AdminTeams'
import { AdminUserDetail } from '@/pages/admin/AdminUserDetail'
import { AdminUsers } from '@/pages/admin/AdminUsers'

/**
 * Routes for the instance-admin portal, relative to the `/admin/*` mount point
 * in `App.tsx`.
 *
 * Lifted verbatim from the nested block that used to live in `routes.tsx`, so
 * the migrated pages keep their exact paths — `/admin`, `/admin/users`,
 * `/admin/users/:id`, `/admin/teams`, `/admin/teams/:id`.
 *
 * `/admin/projects` and `/admin/projects/:id` were added in #461, completing the
 * four sections the sidebar advertises.
 */
export function AdminRoutes() {
  return (
    <Routes>
      <Route index element={<AdminDashboard />} />
      <Route path="users" element={<AdminUsers />} />
      <Route path="users/:id" element={<AdminUserDetail />} />
      <Route path="teams" element={<AdminTeams />} />
      <Route path="teams/:id" element={<AdminTeamDetail />} />
      <Route path="projects" element={<AdminProjects />} />
      <Route path="projects/:id" element={<AdminProjectDetail />} />
      <Route path="*" element={<AdminNotFound />} />
    </Routes>
  )
}

/**
 * In-shell 404 so an unknown `/admin/**` path keeps the admin chrome instead of
 * bouncing the admin out to the product app's NotFound.
 */
function AdminNotFound() {
  return (
    <div className="space-y-2">
      <h1 className="text-xl font-semibold">Page not found</h1>
      <p className="text-muted-foreground text-sm">
        That admin page does not exist.
      </p>
    </div>
  )
}
