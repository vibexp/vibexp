import { BarChart3, type LucideIcon, Users, UsersRound } from 'lucide-react'

export interface AdminNavItem {
  label: string
  /** Absolute path so NavLink active-matching is unambiguous. */
  href: string
  icon: LucideIcon
  /** `end` for the index route so it isn't marked active on child paths. */
  end?: boolean
}

/**
 * Scoped navigation for the instance-admin portal. Kept separate from the main
 * `nav-items.ts` on purpose: admin nav must never leak into the app sidebar for
 * non-admins — it lives only inside the guarded `/admin` shell.
 */
export const ADMIN_NAV_ITEMS: AdminNavItem[] = [
  { label: 'Stats', href: '/admin', icon: BarChart3, end: true },
  { label: 'Users', href: '/admin/users', icon: Users },
  { label: 'Teams', href: '/admin/teams', icon: UsersRound },
]
