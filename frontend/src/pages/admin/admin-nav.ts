import {
  FolderKanban,
  LayoutDashboard,
  type LucideIcon,
  Users,
  UsersRound,
} from 'lucide-react'

export interface AdminNavItem {
  label: string
  /** Absolute path so NavLink active-matching is unambiguous. */
  href: string
  icon: LucideIcon
  /** `end` for the index route so it isn't marked active on child paths. */
  end?: boolean
  /** Subtitle for the shell's section heading on this exact path. */
  description: string
}

/**
 * Scoped navigation for the instance-admin portal. Kept separate from the main
 * `nav-items.ts` on purpose: admin nav must never leak into the app sidebar for
 * non-admins — it lives only inside the guarded `/admin` shell.
 *
 * Also the source of the shell's section heading (`AdminShell`), so a section's
 * nav label and its page title can never disagree.
 */
export const ADMIN_NAV_ITEMS: AdminNavItem[] = [
  {
    label: 'Dashboard',
    href: '/admin',
    icon: LayoutDashboard,
    end: true,
    description: 'Instance health, growth, and activity at a glance.',
  },
  {
    label: 'Users',
    href: '/admin/users',
    icon: Users,
    description: 'Every account on this instance.',
  },
  {
    label: 'Teams',
    href: '/admin/teams',
    icon: UsersRound,
    description: 'Every team on this instance.',
  },
  {
    label: 'Projects',
    href: '/admin/projects',
    icon: FolderKanban,
    description: 'Every project on this instance.',
  },
]

/**
 * The nav item whose path is *exactly* the current one, if any.
 *
 * Exact-match only: a detail page such as `/admin/users/:id` renders its own
 * `PageHeader` for the record it shows, so the shell must not also title it
 * "Users". Sidebar highlighting still uses `NavLink`'s prefix matching, so the
 * section stays marked active there.
 */
export function adminSectionFor(pathname: string): AdminNavItem | undefined {
  const normalized =
    pathname.length > 1 && pathname.endsWith('/')
      ? pathname.slice(0, -1)
      : pathname
  return ADMIN_NAV_ITEMS.find(item => item.href === normalized)
}
