import { Search } from 'lucide-react'

import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { Feed } from '@/services/feedService'
import type { Project } from '@/services/projectService'

interface FeedFiltersProps {
  searchInput: string
  onSearchInputChange: (value: string) => void
  feedId: string | undefined
  onFeedChange: (value: string | undefined) => void
  projectId: string | undefined
  onProjectChange: (value: string | undefined) => void
  assistantName: string | undefined
  onAssistantChange: (value: string | undefined) => void
  feeds: Feed[]
  projects: Project[]
  assistants: string[]
  hideFeedFilter?: boolean
}

export function FeedFilters({
  searchInput,
  onSearchInputChange,
  feedId,
  onFeedChange,
  projectId,
  onProjectChange,
  assistantName,
  onAssistantChange,
  feeds,
  projects,
  assistants,
  hideFeedFilter = false,
}: Readonly<FeedFiltersProps>) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative min-w-[240px] flex-1">
        <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
        <Input
          value={searchInput}
          onChange={e => {
            onSearchInputChange(e.target.value)
          }}
          placeholder="Search feed items…"
          className="pl-8"
        />
      </div>

      {!hideFeedFilter && (
        <Select
          value={feedId ?? 'all'}
          onValueChange={value => {
            onFeedChange(value === 'all' ? undefined : value)
          }}
        >
          <SelectTrigger className="w-[180px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All feeds</SelectItem>
            {feeds.map(f => (
              <SelectItem key={f.id} value={f.id}>
                {f.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}

      <Select
        value={projectId ?? 'all'}
        onValueChange={value => {
          onProjectChange(value === 'all' ? undefined : value)
        }}
      >
        <SelectTrigger className="w-[180px]">
          <SelectValue />
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

      <Select
        value={assistantName ?? 'all'}
        onValueChange={value => {
          onAssistantChange(value === 'all' ? undefined : value)
        }}
      >
        <SelectTrigger className="w-[180px]">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All assistants</SelectItem>
          {assistants.map(a => (
            <SelectItem key={a} value={a}>
              {a}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
