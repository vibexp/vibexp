import {
  FolderKanban,
  KeyRound,
  LayoutDashboard,
  type LucideIcon,
  Mail,
  Search,
  Sparkles,
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
  /**
   * The sidebar group the item is listed under: the instance's records
   * (`admin`, the default) or its configuration (`settings`, #1191).
   */
  group?: AdminNavGroup
}

export type AdminNavGroup = 'admin' | 'settings'

/** Sidebar group labels, in display order. */
export const ADMIN_NAV_GROUPS: readonly {
  id: AdminNavGroup
  label: string
}[] = [
  { id: 'admin', label: 'Administration' },
  { id: 'settings', label: 'Settings' },
]

/** The items listed under `group`, in `ADMIN_NAV_ITEMS` order. */
export function adminNavItemsIn(group: AdminNavGroup): AdminNavItem[] {
  return ADMIN_NAV_ITEMS.filter(item => (item.group ?? 'admin') === group)
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
  {
    label: 'Email',
    href: '/admin/settings/email',
    icon: Mail,
    group: 'settings',
    description:
      'How this instance sends mail: provider, delivery health and change history.',
  },
  {
    label: 'Search',
    href: '/admin/settings/search',
    icon: Search,
    group: 'settings',
    description:
      'The search ranking defaults every team without its own settings uses.',
  },
  {
    label: 'AI Summary',
    href: '/admin/settings/ai-summary',
    icon: Sparkles,
    group: 'settings',
    description:
      'The AI summary defaults for teams, and the server budgets every team shares.',
  },
  {
    label: 'Authentication',
    href: '/admin/settings/auth',
    icon: KeyRound,
    group: 'settings',
    description:
      'Who can sign in to this instance: identity providers, the access allowlist and instance admins.',
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
