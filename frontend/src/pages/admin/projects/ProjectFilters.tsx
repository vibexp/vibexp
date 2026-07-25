import type { DateRangeValue } from '@/components/ui/date-range'
import { AdminFilterBar } from '@/pages/admin/AdminFilterBar'
import { AdminTeamPicker } from '@/pages/admin/teams/AdminTeamPicker'

export interface ProjectFiltersProps {
  /** Uncommitted search text; the page debounces it into the URL. */
  searchInput: string
  onSearchInputChange: (value: string) => void
  teamId: string
  onTeamIdChange: (teamId: string) => void
  created: DateRangeValue
  onCreatedChange: (value: DateRangeValue) => void
  onClear?: () => void
  hasActiveFilters: boolean
}

export function ProjectFilters({
  searchInput,
  onSearchInputChange,
  teamId,
  onTeamIdChange,
  created,
  onCreatedChange,
  onClear,
  hasActiveFilters,
}: Readonly<ProjectFiltersProps>) {
  return (
    <AdminFilterBar
      searchInput={searchInput}
      onSearchInputChange={onSearchInputChange}
      searchPlaceholder="Search name or slug…"
      searchLabel="Search projects"
      created={created}
      onCreatedChange={onCreatedChange}
      onClear={onClear}
      hasActiveFilters={hasActiveFilters}
    >
      <AdminTeamPicker value={teamId} onChange={onTeamIdChange} />
    </AdminFilterBar>
  )
}
