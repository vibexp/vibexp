import { Search } from 'lucide-react'

import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

export type StatusFilter = 'all' | 'active' | 'paused' | 'error'

interface AgentFiltersProps {
  searchInput: string
  onSearchInputChange: (value: string) => void
  currentStatusFilter: StatusFilter
  onStatusFilterChange: (status: StatusFilter) => void
}

export function AgentFilters({
  searchInput,
  onSearchInputChange,
  currentStatusFilter,
  onStatusFilterChange,
}: Readonly<AgentFiltersProps>) {
  return (
    <div className="flex flex-col gap-3 lg:flex-row">
      <div className="relative flex-1">
        <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
        <Input
          type="text"
          placeholder="Search agents by name or description…"
          className="pl-8"
          value={searchInput}
          onChange={e => {
            onSearchInputChange(e.target.value)
          }}
        />
      </div>

      <div className="lg:w-48">
        <Select
          value={currentStatusFilter}
          onValueChange={v => {
            onStatusFilterChange(v as StatusFilter)
          }}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All statuses</SelectItem>
            <SelectItem value="active">Active</SelectItem>
            <SelectItem value="paused">Paused</SelectItem>
            <SelectItem value="error">Error</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>
  )
}
