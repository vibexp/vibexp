import { Search } from 'lucide-react'

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

interface Props {
  searchInput: string
  onSearchInputChange: (value: string) => void
  tags?: string[]
  selectedTag?: string
  onTagChange?: (tag: string | undefined) => void
  status?: MemoryStatus
  onStatusChange?: (status: MemoryStatus | undefined) => void
}

// Project filtering moved to the global header project selector (useProject).
export function MemoryFilters({
  searchInput,
  onSearchInputChange,
  tags = [],
  selectedTag,
  onTagChange,
  status,
  onStatusChange,
}: Readonly<Props>) {
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
          className="pl-8"
        />
      </div>
      {tags.length > 0 && onTagChange && (
        <Select
          value={selectedTag ?? 'all'}
          onValueChange={value => {
            onTagChange(value === 'all' ? undefined : value)
          }}
        >
          <SelectTrigger className="w-[140px]">
            <SelectValue placeholder="All tags" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All tags</SelectItem>
            {tags.map(tag => (
              <SelectItem key={tag} value={tag}>
                {tag}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
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
    </div>
  )
}
