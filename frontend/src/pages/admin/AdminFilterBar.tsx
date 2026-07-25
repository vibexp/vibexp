import { Search } from 'lucide-react'
import type { ReactNode } from 'react'

import { Button } from '@/components/ui/button'
import type { DateRangeValue } from '@/components/ui/date-range'
import { DateRangePicker } from '@/components/ui/date-range-picker'
import { Input } from '@/components/ui/input'

export interface AdminFilterBarProps {
  /** Uncommitted search text; the page debounces it into the URL. */
  searchInput: string
  onSearchInputChange: (value: string) => void
  searchPlaceholder: string
  /** Accessible name for the search box, e.g. "Search users". */
  searchLabel: string
  created: DateRangeValue
  onCreatedChange: (value: DateRangeValue) => void
  createdLabel?: string
  onClear?: () => void
  hasActiveFilters: boolean
  /** The page's own domain filters, rendered between search and the date range. */
  children?: ReactNode
}

/**
 * The filter controls every admin list page shares: a search box, a created-date
 * range, and a Clear action that appears only when something is applied.
 *
 * Extracted alongside `useAdminListFilters` for the same reason — three pages
 * (Teams, Projects, Users) had the same three controls, and the copies would
 * drift. Each page passes its own domain filters as children, so the ordering and
 * spacing stay consistent without every page restating the parts that are not
 * specific to it.
 */
export function AdminFilterBar({
  searchInput,
  onSearchInputChange,
  searchPlaceholder,
  searchLabel,
  created,
  onCreatedChange,
  createdLabel = 'Filter by creation date',
  onClear,
  hasActiveFilters,
  children,
}: Readonly<AdminFilterBarProps>) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative min-w-[220px] max-w-[420px] flex-1">
        <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
        <Input
          value={searchInput}
          onChange={event => {
            onSearchInputChange(event.target.value)
          }}
          placeholder={searchPlaceholder}
          aria-label={searchLabel}
          className="pl-8"
        />
      </div>

      {children}

      <DateRangePicker
        value={created}
        onChange={onCreatedChange}
        placeholder="Created any time"
        ariaLabel={createdLabel}
      />

      {hasActiveFilters && (
        <Button variant="ghost" size="sm" onClick={onClear}>
          Clear filters
        </Button>
      )}
    </div>
  )
}
