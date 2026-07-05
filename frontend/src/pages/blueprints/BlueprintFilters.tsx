import { Search } from 'lucide-react'

import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { Blueprint } from '@/types'

interface Props {
  searchInput: string
  onSearchInputChange: (value: string) => void
  type: Blueprint['type'] | undefined
  onTypeChange: (value: Blueprint['type'] | undefined) => void
}

// Project filtering moved to the global header project selector (useProject).
export function BlueprintFilters({
  searchInput,
  onSearchInputChange,
  type,
  onTypeChange,
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
          placeholder="Search blueprints…"
          className="pl-8"
        />
      </div>
      <Select
        value={type ?? 'all'}
        onValueChange={value => {
          onTypeChange(
            value === 'all' ? undefined : (value as Blueprint['type'])
          )
        }}
      >
        <SelectTrigger className="w-[150px]">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All types</SelectItem>
          <SelectItem value="general">General</SelectItem>
          <SelectItem value="claude-code">Claude Code</SelectItem>
          <SelectItem value="claude">Claude</SelectItem>
          <SelectItem value="cursor">Cursor</SelectItem>
          <SelectItem value="codex">Codex</SelectItem>
        </SelectContent>
      </Select>
    </div>
  )
}
