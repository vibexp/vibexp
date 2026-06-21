import { Search } from 'lucide-react'

import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

export type StatusFilter = 'all' | 'draft' | 'published'
export type SharedFilter = 'all' | 'shared' | 'not_shared'

interface Props {
  searchInput: string
  onSearchInputChange: (value: string) => void
  statusFilter: StatusFilter
  onStatusChange: (value: StatusFilter) => void
  sharedFilter: SharedFilter
  onSharedChange: (value: SharedFilter) => void
}

export function PromptFilters({
  searchInput,
  onSearchInputChange,
  statusFilter,
  onStatusChange,
  sharedFilter,
  onSharedChange,
}: Props) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative min-w-[240px] max-w-[480px] flex-1">
        <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
        <Input
          value={searchInput}
          onChange={e => {
            onSearchInputChange(e.target.value)
          }}
          placeholder="Search prompts…"
          className="pl-8"
        />
      </div>

      <Select
        value={statusFilter}
        onValueChange={value => {
          onStatusChange(value as StatusFilter)
        }}
      >
        <SelectTrigger className="w-[140px]">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All statuses</SelectItem>
          <SelectItem value="published">Published</SelectItem>
          <SelectItem value="draft">Draft</SelectItem>
        </SelectContent>
      </Select>

      <Select
        value={sharedFilter}
        onValueChange={value => {
          onSharedChange(value as SharedFilter)
        }}
      >
        <SelectTrigger className="w-[140px]">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All</SelectItem>
          <SelectItem value="shared">Shared</SelectItem>
          <SelectItem value="not_shared">Not shared</SelectItem>
        </SelectContent>
      </Select>
    </div>
  )
}
