import { Search } from 'lucide-react'

import { Button } from '@/components/ui/button'
import type { DateRangeValue } from '@/components/ui/date-range'
import { DateRangePicker } from '@/components/ui/date-range-picker'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

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
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative min-w-[240px] max-w-[420px] flex-1">
        <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
        <Input
          value={searchInput}
          onChange={event => {
            onSearchInputChange(event.target.value)
          }}
          placeholder="Search name, slug, or owner email…"
          aria-label="Search teams"
          className="pl-8"
        />
      </div>

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
