import { AdminSearchPicker } from '@/pages/admin/filters/AdminSearchPicker'
import { useAdminTeamSearch } from '@/pages/admin/teams/useAdminTeamSearch'
import type { AdminTeamListItem } from '@/services/adminService'
import { adminService } from '@/services/adminService'

export interface AdminTeamPickerProps {
  /** Selected team id, or '' for "All teams". */
  value: string
  onChange: (teamId: string) => void
  className?: string
  'aria-label'?: string
  /**
   * Look up the name of a selected team no search page contains, so a shared
   * link shows the team rather than "Selected team" (#1517).
   */
  resolveName?: boolean
}

const teamId = (team: AdminTeamListItem) => team.id
const teamName = (team: AdminTeamListItem) => team.name
const resolveTeamName = async (id: string) =>
  (await adminService.getTeam(id)).name

/**
 * Searchable, paginated team picker for the admin filter bars.
 *
 * Reads through `useAdminTeamSearch`, since the product picker's hook needs
 * team context the admin shell does not mount. Deliberately **not** a plain
 * `Select`: filling one would mean fetching every team on the instance, and
 * personal workspaces alone run about one per user.
 */
export function AdminTeamPicker({
  value,
  onChange,
  className,
  'aria-label': ariaLabel = 'Filter by team',
  resolveName = false,
}: Readonly<AdminTeamPickerProps>) {
  const { teams, ...search } = useAdminTeamSearch()
  return (
    <AdminSearchPicker
      value={value}
      onChange={onChange}
      search={{ items: teams, ...search }}
      getId={teamId}
      getLabel={teamName}
      resolveLabel={resolveName ? resolveTeamName : undefined}
      allLabel="All teams"
      selectedFallback="Selected team"
      searchPlaceholder="Search teams…"
      emptyText="No teams found."
      aria-label={ariaLabel}
      className={className}
    />
  )
}
