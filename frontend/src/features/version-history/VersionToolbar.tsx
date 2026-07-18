import { ChevronDown, Filter, GitCompare, Search } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

export type DateRange = 'all' | '24h' | '7d' | '30d'

export const DATE_RANGES: { value: DateRange; label: string }[] = [
  { value: 'all', label: 'All time' },
  { value: '24h', label: 'Last 24 hours' },
  { value: '7d', label: 'Last 7 days' },
  { value: '30d', label: 'Last 30 days' },
]

interface VersionToolbarProps {
  search: string
  onSearchChange: (value: string) => void
  authors: { id: string; name: string }[]
  authorFilter: string | null
  onAuthorFilter: (id: string | null) => void
  dateRange: DateRange
  onDateRange: (range: DateRange) => void
  selectedCount: number
  onCompare: () => void
}

export function VersionToolbar({
  search,
  onSearchChange,
  authors,
  authorFilter,
  onAuthorFilter,
  dateRange,
  onDateRange,
  selectedCount,
  onCompare,
}: Readonly<VersionToolbarProps>) {
  const activeDateLabel =
    DATE_RANGES.find(r => r.value === dateRange)?.label ?? 'All time'
  const activeAuthorLabel = authorFilter
    ? (authors.find(a => a.id === authorFilter)?.name ?? 'Author')
    : 'Author'

  return (
    <div className="vhc-toolbar">
      <label className="vhc-search">
        <Search aria-hidden="true" />
        <input
          type="text"
          placeholder="Search messages, authors…"
          aria-label="Search versions"
          value={search}
          onChange={e => {
            onSearchChange(e.target.value)
          }}
        />
      </label>

      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            className={cn('vhc-filter', authorFilter && 'is-active')}
          >
            <Filter aria-hidden="true" />
            {activeAuthorLabel}
            <ChevronDown aria-hidden="true" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuLabel>Filter by author</DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuCheckboxItem
            checked={authorFilter === null}
            onCheckedChange={() => {
              onAuthorFilter(null)
            }}
          >
            All authors
          </DropdownMenuCheckboxItem>
          {authors.map(author => (
            <DropdownMenuCheckboxItem
              key={author.id}
              checked={authorFilter === author.id}
              onCheckedChange={() => {
                onAuthorFilter(author.id)
              }}
            >
              {author.name}
            </DropdownMenuCheckboxItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>

      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            className={cn('vhc-filter', dateRange !== 'all' && 'is-active')}
          >
            {activeDateLabel}
            <ChevronDown aria-hidden="true" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          {DATE_RANGES.map(range => (
            <DropdownMenuItem
              key={range.value}
              onSelect={() => {
                onDateRange(range.value)
              }}
            >
              {range.label}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>

      <div className="vhc-toolbar__sp" />

      <span className="vhc-selbar">
        <span>
          <b>{selectedCount}</b> selected
        </span>
        <Button
          size="sm"
          disabled={selectedCount !== 2}
          data-testid="compare-button"
          onClick={onCompare}
        >
          <GitCompare />
          Compare
        </Button>
      </span>
    </div>
  )
}
