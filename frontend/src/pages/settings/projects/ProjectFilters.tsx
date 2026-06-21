import { Search } from 'lucide-react'

import { Input } from '@/components/ui/input'

interface Props {
  searchInput: string
  onSearchInputChange: (value: string) => void
}

export function ProjectFilters({ searchInput, onSearchInputChange }: Props) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative min-w-[240px] max-w-[480px] flex-1">
        <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
        <Input
          value={searchInput}
          onChange={e => {
            onSearchInputChange(e.target.value)
          }}
          placeholder="Search projects…"
          className="pl-8"
        />
      </div>
    </div>
  )
}
