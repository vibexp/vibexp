import { NavLink, Outlet } from 'react-router-dom'

import { PageHeader } from '@/components/PageHeader'
import { cn } from '@/lib/utils'
import { ADMIN_NAV_ITEMS } from '@/pages/admin/admin-nav'

/**
 * Minimal shell for the instance-admin portal (epic #309): a page header plus a
 * scoped sub-nav (Stats / Users / Teams) above the routed page body rendered
 * through `<Outlet />`. Assembled from existing primitives — the design team
 * restyles later. It renders inside the guarded `/admin` route subtree, so it is
 * only ever mounted for an instance admin.
 */
export function AdminLayout() {
  return (
    <div className="space-y-6">
      <PageHeader
        title="Admin Portal"
        description="Instance administration — statistics, users, and teams."
      />
      <nav
        aria-label="Admin sections"
        className="flex flex-wrap gap-1 border-b pb-2"
      >
        {ADMIN_NAV_ITEMS.map(item => {
          const Icon = item.icon
          return (
            <NavLink
              key={item.href}
              to={item.href}
              end={item.end}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
                  isActive
                    ? 'bg-sidebar-accent text-sidebar-accent-foreground'
                    : 'text-muted-foreground hover:bg-sidebar-accent/50 hover:text-foreground'
                )
              }
            >
              <Icon className="size-4 shrink-0 opacity-85" aria-hidden />
              {item.label}
            </NavLink>
          )
        })}
      </nav>
      <Outlet />
    </div>
  )
}
