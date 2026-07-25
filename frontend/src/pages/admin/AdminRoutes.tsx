import { Route, Routes } from 'react-router-dom'

import { AdminStats } from '@/pages/admin/AdminStats'
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
 * `/admin/projects` has a nav entry already (admin-nav.ts) but no route until
 * #461 adds the pages; until then it falls through to the catch-all below rather
 * than rendering a blank shell.
 */
export function AdminRoutes() {
  return (
    <Routes>
      <Route index element={<AdminStats />} />
      <Route path="users" element={<AdminUsers />} />
      <Route path="users/:id" element={<AdminUserDetail />} />
      <Route path="teams" element={<AdminTeams />} />
      <Route path="teams/:id" element={<AdminTeamDetail />} />
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
