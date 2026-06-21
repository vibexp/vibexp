import { Search } from 'lucide-react'

import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

interface ExecutionFiltersProps {
  searchInput: string
  onSearchInputChange: (value: string) => void
  currentStatusFilter: string
  onStatusFilterChange: (status: string) => void
}

export function ExecutionFilters({
  searchInput,
  onSearchInputChange,
  currentStatusFilter,
  onStatusFilterChange,
}: ExecutionFiltersProps) {
  return (
    <div className="flex flex-col gap-3 lg:flex-row">
      <div className="relative flex-1">
        <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
        <Input
          type="text"
          placeholder="Search executions…"
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
          onValueChange={onStatusFilterChange}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All statuses</SelectItem>
            <SelectItem value="success">Success</SelectItem>
            <SelectItem value="error">Error</SelectItem>
            <SelectItem value="running">Running</SelectItem>
            <SelectItem value="pending">Pending</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>
  )
}
