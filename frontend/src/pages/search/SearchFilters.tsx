import { Info, Search as SearchIcon } from 'lucide-react'
import { type KeyboardEvent } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { Project } from '@/services/projectService'
import type { SearchFilterType } from '@/services/searchService'

const TYPE_OPTIONS: { value: SearchFilterType; label: string }[] = [
  { value: 'prompts', label: 'Prompts' },
  { value: 'artifacts', label: 'Artifacts' },
  { value: 'blueprints', label: 'Blueprints' },
  { value: 'memories', label: 'Memories' },
]

interface Props {
  /** Current text in the query box (local, uncommitted until submit). */
  queryInput: string
  onQueryInputChange: (value: string) => void
  /** Commit the current query box value (Enter or the Search button). */
  onSubmit: () => void
  type?: SearchFilterType
  onTypeChange: (type: SearchFilterType | undefined) => void
  projects?: Project[]
  selectedProjectId?: string
  onProjectChange: (projectId: string | undefined) => void
}

/**
 * Filter bar for the platform-wide search page: a query box (Enter submits),
 * a resource-type filter and a project filter. All three feed deep-linkable
 * URL params owned by the parent page.
 */
export function SearchFilters({
  queryInput,
  onQueryInputChange,
  onSubmit,
  type,
  onTypeChange,
  projects = [],
  selectedProjectId,
  onProjectChange,
}: Props) {
  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      event.preventDefault()
      onSubmit()
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative min-w-[240px] max-w-[480px] flex-1">
        <SearchIcon className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
        <Input
          value={queryInput}
          onChange={e => {
            onQueryInputChange(e.target.value)
          }}
          onKeyDown={handleKeyDown}
          placeholder="Search prompts, artifacts, blueprints, memories…"
          aria-label="Search query"
          className="pl-8 pr-9"
        />
        <Popover>
          <PopoverTrigger asChild>
            <button
              type="button"
              aria-label="Search tips"
              className="text-muted-foreground hover:text-foreground focus-visible:ring-ring absolute right-2.5 top-1/2 flex size-5 -translate-y-1/2 items-center justify-center rounded-sm focus:outline-none focus-visible:ring-2"
            >
              <Info className="size-4" />
            </button>
          </PopoverTrigger>
          <PopoverContent align="end" className="w-80 text-sm">
            <p className="mb-2 font-medium">Search tips</p>
            <ul className="text-muted-foreground space-y-1.5">
              <li>
                <code className="text-foreground">
                  &quot;exact phrase&quot;
                </code>{' '}
                — match an exact phrase
              </li>
              <li>
                <code className="text-foreground">cat OR dog</code> — match
                either term
              </li>
              <li>
                <code className="text-foreground">cats -dogs</code> — exclude a
                term
              </li>
            </ul>
            <p className="text-muted-foreground mt-2 text-xs">
              Operators apply to keyword search (used when no embedding provider
              is configured).
            </p>
          </PopoverContent>
        </Popover>
      </div>

      <Select
        value={type ?? 'all'}
        onValueChange={value => {
          onTypeChange(
            value === 'all' ? undefined : (value as SearchFilterType)
          )
        }}
      >
        <SelectTrigger className="w-[150px]" aria-label="Filter by type">
          <SelectValue placeholder="All types" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All types</SelectItem>
          {TYPE_OPTIONS.map(option => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {projects.length > 0 && (
        <Select
          value={selectedProjectId ?? 'all'}
          onValueChange={value => {
            onProjectChange(value === 'all' ? undefined : value)
          }}
        >
          <SelectTrigger className="w-[180px]" aria-label="Filter by project">
            <SelectValue placeholder="All projects" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All projects</SelectItem>
            {projects.map(p => (
              <SelectItem key={p.id} value={p.id}>
                {p.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}

      <Button onClick={onSubmit} disabled={!queryInput.trim()}>
        <SearchIcon className="mr-2 size-4" />
        Search
      </Button>
    </div>
  )
}
