import type { ReactNode } from 'react'
import { useLocation } from 'react-router-dom'

import { PageHeader } from '@/components/PageHeader'
import { adminSectionFor } from '@/pages/admin/admin-nav'
import { AdminHeader } from '@/pages/admin/AdminHeader'
import { AdminSidebar } from '@/pages/admin/AdminSidebar'

/**
 * Shell for the instance-admin portal.
 *
 * Replaces the epic-#309 `AdminLayout`, which was a page header plus a
 * horizontal three-item sub-nav — a shape that does not extend to four sections
 * with sub-pages.
 *
 * It mirrors the main app shell's LAYOUT CONTRACT (sidebar + sticky header +
 * centred, width-capped main) and its design tokens, but shares no code with it,
 * because the two answer to different scopes: this one mounts under no
 * TeamProvider or ProjectProvider and has no team/project concepts at all. That
 * separation is the point of #456 — the admin frontend can now evolve without
 * any change having to prove it did not disturb the product app.
 *
 * Content follows the same container contract as `Layout` — centred, capped at
 * `max-w-screen-xl`, pages keep their own vertical rhythm and add no `mx-auto
 * max-w-*` of their own — so the five migrated pages needed no changes.
 */
export function AdminShell({ children }: Readonly<{ children: ReactNode }>) {
  const { pathname } = useLocation()
  // Section heading for the landing/list pages only. `AdminLayout` rendered one
  // unconditionally ("Admin Portal"); without this, `/admin` and `/admin/users`
  // would ship with no <h1> at all, since neither page titles itself.
  const section = adminSectionFor(pathname)

  return (
    <div className="flex min-h-screen">
      <AdminSidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <AdminHeader />
        <main className="flex-1 overflow-auto">
          <div className="mx-auto w-full max-w-screen-xl px-4 py-6 md:px-6 lg:px-8">
            {section && (
              <PageHeader
                title={section.label}
                description={section.description}
              />
            )}
            {children}
          </div>
        </main>
      </div>
    </div>
  )
}
