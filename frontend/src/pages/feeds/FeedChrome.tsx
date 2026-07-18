import { Search } from 'lucide-react'

import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import type { Feed } from '@/services/feedService'

/**
 * Pill-style tab group used by both /feeds and /feeds/:id.
 * Mirrors the prototype's `.tabs` + `.tab` + `.tab .count` styling.
 */
interface FeedTabsProps {
  tab: 'active' | 'archived'
  onChange: (tab: 'active' | 'archived') => void
  activeCount?: number
  archivedCount?: number
  className?: string
}

export function FeedTabs({
  tab,
  onChange,
  activeCount,
  archivedCount,
  className,
}: Readonly<FeedTabsProps>) {
  const tabs: {
    value: 'active' | 'archived'
    label: string
    count?: number
  }[] = [
    { value: 'active', label: 'Active', count: activeCount },
    { value: 'archived', label: 'Archived', count: archivedCount },
  ]
  return (
    <div
      className={cn('inline-flex gap-1 rounded-lg bg-muted p-[3px]', className)}
    >
      {tabs.map(({ value, label, count }) => {
        const isActive = tab === value
        return (
          <button
            key={value}
            type="button"
            onClick={() => {
              onChange(value)
            }}
            className={cn(
              'inline-flex items-center gap-1.5 rounded-md px-3.5 py-2 text-sm font-medium transition-colors',
              isActive
                ? 'bg-background text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'
            )}
          >
            <span>{label}</span>
            {count !== undefined && (
              <span
                className={cn(
                  'inline-flex min-w-[1.25rem] justify-center rounded-full px-1.5 text-xs font-medium tabular-nums',
                  isActive
                    ? 'bg-muted text-foreground'
                    : 'bg-secondary text-muted-foreground'
                )}
              >
                {count}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}

/**
 * Search + filter row used by both /feeds and /feeds/:id.
 * Feed selector is optional — used only on the global /feeds index.
 * Project filtering moved to the global header project selector (useProject).
 */
interface FeedToolbarProps {
  searchInput: string
  onSearchChange: (value: string) => void
  assistants: string[]
  assistantName: string | undefined
  onAssistantChange: (value: string | undefined) => void
  feeds?: Feed[]
  feedId?: string
  onFeedChange?: (value: string | undefined) => void
}

export function FeedToolbar({
  searchInput,
  onSearchChange,
  assistants,
  assistantName,
  onAssistantChange,
  feeds,
  feedId,
  onFeedChange,
}: Readonly<FeedToolbarProps>) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative min-w-[240px] flex-1">
        <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
        <Input
          value={searchInput}
          onChange={e => {
            onSearchChange(e.target.value)
          }}
          placeholder="Search feed items…"
          className="pl-8"
        />
      </div>

      {feeds && onFeedChange && (
        <Select
          value={feedId ?? '__all__'}
          onValueChange={v => {
            onFeedChange(v === '__all__' ? undefined : v)
          }}
        >
          <SelectTrigger className="w-[180px]">
            <SelectValue placeholder="All feeds" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">All feeds</SelectItem>
            {feeds.map(f => (
              <SelectItem key={f.id} value={f.id}>
                {f.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}

      <Select
        value={assistantName ?? '__all__'}
        onValueChange={v => {
          onAssistantChange(v === '__all__' ? undefined : v)
        }}
      >
        <SelectTrigger className="w-[180px]">
          <SelectValue placeholder="All assistants" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__all__">All assistants</SelectItem>
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
