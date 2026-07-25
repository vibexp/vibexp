import { Search } from 'lucide-react'

import { Button } from '@/components/ui/button'
import type { DateRangeValue } from '@/components/ui/date-range'
import { DateRangePicker } from '@/components/ui/date-range-picker'
import { Input } from '@/components/ui/input'
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

/**
 * Filter bar for the admin projects list. Same shape as `TeamFilters` (#460) —
 * search, one domain filter, a created-date range — so the two admin list pages
 * read identically.
 */
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
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative min-w-[240px] max-w-[420px] flex-1">
        <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
        <Input
          value={searchInput}
          onChange={event => {
            onSearchInputChange(event.target.value)
          }}
          placeholder="Search name or slug…"
          aria-label="Search projects"
          className="pl-8"
        />
      </div>

      <AdminTeamPicker value={teamId} onChange={onTeamIdChange} />

      <DateRangePicker
        value={created}
        onChange={onCreatedChange}
        placeholder="Created any time"
        ariaLabel="Filter by creation date"
      />

      {hasActiveFilters && (
        <Button variant="ghost" size="sm" onClick={onClear}>
          Clear filters
        </Button>
      )}
    </div>
  )
}
