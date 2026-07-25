import type { DateRangeValue } from '@/components/ui/date-range'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { AdminFilterBar } from '@/pages/admin/AdminFilterBar'

/**
 * The three states of the personal/shared filter.
 *
 * A tri-state select rather than a checkbox, because `is_personal` is an
 * **optional** boolean on the API: "All" must send no parameter at all, and
 * `is_personal=false` means "shared only" — a checkbox cannot express the
 * difference, and conflating them would silently hide every personal workspace.
 */
export type TeamKindFilter = 'all' | 'shared' | 'personal'

export interface TeamFiltersProps {
  /** Uncommitted search text; the page debounces it into the URL. */
  searchInput: string
  onSearchInputChange: (value: string) => void
  kind: TeamKindFilter
  onKindChange: (value: TeamKindFilter) => void
  created: DateRangeValue
  onCreatedChange: (value: DateRangeValue) => void
  /** Shown only while at least one filter is applied. */
  onClear?: () => void
  hasActiveFilters: boolean
}

export function TeamFilters({
  searchInput,
  onSearchInputChange,
  kind,
  onKindChange,
  created,
  onCreatedChange,
  onClear,
  hasActiveFilters,
}: Readonly<TeamFiltersProps>) {
  return (
    <AdminFilterBar
      searchInput={searchInput}
      onSearchInputChange={onSearchInputChange}
      searchPlaceholder="Search name, slug, or owner email…"
      searchLabel="Search teams"
      created={created}
      onCreatedChange={onCreatedChange}
      onClear={onClear}
      hasActiveFilters={hasActiveFilters}
    >
      <Select
        value={kind}
        onValueChange={value => {
          onKindChange(value as TeamKindFilter)
        }}
      >
        <SelectTrigger className="w-[160px]" aria-label="Team type">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All teams</SelectItem>
          <SelectItem value="shared">Shared only</SelectItem>
          <SelectItem value="personal">Personal only</SelectItem>
        </SelectContent>
      </Select>
    </AdminFilterBar>
  )
}
