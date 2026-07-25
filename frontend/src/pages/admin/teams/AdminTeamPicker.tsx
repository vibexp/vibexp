import { Check, ChevronsUpDown, Loader2 } from 'lucide-react'
import type { UIEvent } from 'react'
import { useEffect, useMemo, useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import { useAdminTeamSearch } from '@/pages/admin/teams/useAdminTeamSearch'
import type { AdminTeamListItem } from '@/services/adminService'

/** Sentinel value — never a real team id (those are uuids). */
const ALL_TEAMS_VALUE = '__all_teams__'
/** Distance from the bottom of the list (px) that pulls the next page. */
const SCROLL_LOAD_THRESHOLD = 48

export interface AdminTeamPickerProps {
  /** Selected team id, or '' for "All teams". */
  value: string
  onChange: (teamId: string) => void
  className?: string
  'aria-label'?: string
}

/**
 * Searchable, paginated team picker for the admin filter bars.
 *
 * Mirrors `components/ProjectPicker` — `Popover` + `Command` over server-driven
 * search — but reads through `useAdminTeamSearch`, since the product picker's
 * hook needs team context the admin shell does not mount.
 *
 * Deliberately **not** a plain `Select`: filling one would mean fetching every
 * team on the instance, and personal workspaces alone run about one per user.
 */
export function AdminTeamPicker({
  value,
  onChange,
  className,
  'aria-label': ariaLabel = 'Filter by team',
}: Readonly<AdminTeamPickerProps>) {
  const [open, setOpen] = useState(false)
  const {
    teams,
    loading,
    loadingMore,
    error,
    hasMore,
    loadMore,
    query,
    setQuery,
  } = useAdminTeamSearch()

  // Remember the chosen team so the trigger keeps its name even when the current
  // search page no longer contains it.
  const [picked, setPicked] = useState<AdminTeamListItem | null>(null)
  useEffect(() => {
    if (!value) setPicked(null)
  }, [value])

  const selected = useMemo(
    () =>
      teams.find(t => t.id === value) ?? (picked?.id === value ? picked : null),
    [teams, value, picked]
  )

  const triggerLabel = value ? (selected?.name ?? 'Selected team') : 'All teams'

  const handleScroll = (event: UIEvent<HTMLDivElement>) => {
    const el = event.currentTarget
    if (
      el.scrollHeight - el.scrollTop - el.clientHeight <
      SCROLL_LOAD_THRESHOLD
    ) {
      loadMore()
    }
  }

  const select = (team: AdminTeamListItem | null) => {
    setPicked(team)
    onChange(team?.id ?? '')
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-label={ariaLabel}
          className={cn(
            'w-[200px] justify-between font-normal',
            !value && 'text-muted-foreground',
            className
          )}
        >
          <span className="truncate">{triggerLabel}</span>
          <ChevronsUpDown className="ml-2 size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[260px] p-0" align="start">
        {/* shouldFilter={false}: the server already filtered, and filtering the
            loaded page again would hide results the next page would supply. */}
        <Command shouldFilter={false}>
          <CommandInput
            placeholder="Search teams…"
            className="h-9"
            value={query}
            onValueChange={setQuery}
          />
          <CommandList className="max-h-64" onScroll={handleScroll}>
            {loading && (
              <div className="text-muted-foreground flex items-center justify-center gap-2 py-6 text-sm">
                <Loader2 className="size-4 animate-spin" />
                Searching…
              </div>
            )}
            {error && !loading && (
              <div className="text-destructive px-3 py-6 text-center text-sm">
                {error}
              </div>
            )}
            {!loading && !error && (
              <>
                <CommandItem
                  value={ALL_TEAMS_VALUE}
                  onSelect={() => {
                    select(null)
                  }}
                >
                  <Check
                    className={cn(
                      'mr-2 size-4',
                      value ? 'opacity-0' : 'opacity-100'
                    )}
                  />
                  All teams
                </CommandItem>
                {teams.length === 0 && (
                  <CommandEmpty>No teams found.</CommandEmpty>
                )}
                {teams.map(team => (
                  <CommandItem
                    key={team.id}
                    value={team.id}
                    onSelect={() => {
                      select(team)
                    }}
                  >
                    <Check
                      className={cn(
                        'mr-2 size-4',
                        value === team.id ? 'opacity-100' : 'opacity-0'
                      )}
                    />
                    <span className="truncate">{team.name}</span>
                  </CommandItem>
                ))}
                {loadingMore && (
                  <div className="text-muted-foreground flex items-center justify-center gap-2 py-3 text-xs">
                    <Loader2 className="size-3 animate-spin" />
                    Loading more…
                  </div>
                )}
                {hasMore && !loadingMore && (
                  <div className="text-muted-foreground px-3 py-2 text-center text-xs">
                    Scroll for more
                  </div>
                )}
              </>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
