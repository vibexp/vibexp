import { Search } from 'lucide-react'

import { FreshnessFilterSelect } from '@/components/FreshnessFilterSelect'
import { MetadataFilterField } from '@/components/metadata/MetadataFilterField'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { MEMORY_STATUS_OPTIONS } from '@/pages/memories/memoryStatus'
import type { MemoryStatus } from '@/services/memoryService'
import type { MetadataFilterValue } from '@/services/metadataService'

export interface MemoryFiltersProps {
  /** Uncommitted search text; the page debounces it into the URL. */
  searchInput: string
  onSearchInputChange: (value: string) => void
  status?: MemoryStatus
  onStatusChange?: (status: MemoryStatus | undefined) => void
  /**
   * Committed metadata filter; one chip per key. Tags live at `metadata.tags`,
   * so the old dedicated tag Select is gone — filtering on `tags` is just a
   * metadata filter, and a server-side one at that (#518).
   */
  /** `stale` shows only resources the freshness rules currently flag (#738). */
  freshness: 'stale' | undefined
  onFreshnessChange: (value: 'stale' | undefined) => void
  metadata: MetadataFilterValue
  onMetadataChange: (value: MetadataFilterValue) => void
  /** Narrows the metadata catalog to the globally selected project. */
  projectId?: string
  /** Shown only while at least one filter is applied. */
  onClear?: () => void
  hasActiveFilters: boolean
}

// Project filtering moved to the global header project selector (useProject).
export function MemoryFilters({
  searchInput,
  onSearchInputChange,
  status,
  onStatusChange,
  freshness,
  onFreshnessChange,
  metadata,
  onMetadataChange,
  projectId,
  onClear,
  hasActiveFilters,
}: Readonly<MemoryFiltersProps>) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative min-w-[240px] max-w-[480px] flex-1">
        <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
        <Input
          value={searchInput}
          onChange={e => {
            onSearchInputChange(e.target.value)
          }}
          placeholder="Search memories…"
          aria-label="Search memories"
          className="pl-8"
        />
      </div>
      {onStatusChange && (
        <Select
          value={status ?? 'all'}
          onValueChange={value => {
            onStatusChange(
              value === 'all' ? undefined : (value as MemoryStatus)
            )
          }}
        >
          <SelectTrigger
            className="w-[150px]"
            aria-label="Filter by status"
            data-testid="memory-status-filter"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All statuses</SelectItem>
            {MEMORY_STATUS_OPTIONS.map(option => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}

      <FreshnessFilterSelect
        value={freshness}
        onChange={onFreshnessChange}
        ariaLabel="Filter memories by freshness"
        testId="memory-freshness-filter"
      />

      <MetadataFilterField
        resourceType="memories"
        projectId={projectId}
        value={metadata}
        onChange={onMetadataChange}
        ariaLabel="Filter memories by metadata"
      />

      {hasActiveFilters && onClear && (
        <Button variant="outline" onClick={onClear}>
          Clear filters
        </Button>
      )}
    </div>
  )
}
