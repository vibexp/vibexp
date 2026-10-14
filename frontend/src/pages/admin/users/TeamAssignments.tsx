import { Loader2, Plus, X } from 'lucide-react'
import { type UIEvent, useState } from 'react'

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
import { useAdminTeamSearch } from '@/pages/admin/teams/useAdminTeamSearch'

/** The API's limit on teams per new user. */
export const MAX_TEAM_ASSIGNMENTS = 50

export type TeamRole = 'member' | 'admin'

export interface TeamAssignment {
  team_id: string
  team_name: string
  role: TeamRole
}

const SCROLL_LOAD_THRESHOLD = 48

/**
 * The teams a new user joins, each with a role (#1443). Teams come from the
 * admin team search in a Popover + Command, not a Radix `Select` (a `Select`
 * inside the dialog exhausts memory under jsdom, and filling one would fetch
 * every team). Personal workspaces are not offered: the API refuses them.
 * The role is a pair of radio buttons for the same reason.
 *
 * `errors` holds the server's message per team id: the caller maps the
 * server's `teams[i]` back to the team it named, so removing another row does
 * not move a message onto the wrong team.
 */
export function TeamAssignments({
  value,
  onChange,
  errors = {},
  disabled,
}: Readonly<{
  value: readonly TeamAssignment[]
  onChange: (next: TeamAssignment[]) => void
  errors?: Readonly<Record<string, string>>
  disabled?: boolean
}>) {
  const [open, setOpen] = useState(false)
  const chosen = new Set(value.map(row => row.team_id))
  const full = value.length >= MAX_TEAM_ASSIGNMENTS

  return (
    <fieldset className="space-y-2" disabled={disabled}>
      <legend className="text-sm font-medium">Add to teams (optional)</legend>
      {value.length > 0 && (
        <ul className="space-y-2" data-testid="team-assignments">
          {value.map(row => (
            <li key={row.team_id} className="rounded-md border p-2">
              <div className="flex flex-wrap items-center gap-2">
                <span className="min-w-0 flex-1 truncate text-sm font-medium">
                  {row.team_name}
                </span>
                <div
                  role="radiogroup"
                  aria-label={`Role in ${row.team_name}`}
                  className="flex items-center gap-3 text-sm"
                >
                  {(['member', 'admin'] as const).map(role => (
                    <label key={role} className="flex items-center gap-1">
                      <input
                        type="radio"
                        name={`team-role-${row.team_id}`}
                        value={role}
                        checked={row.role === role}
                        onChange={() => {
                          onChange(
                            value.map(other =>
                              other.team_id === row.team_id
                                ? { ...other, role }
                                : other
                            )
                          )
                        }}
                      />
                      {role === 'member' ? 'Member' : 'Admin'}
                    </label>
                  ))}
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-7"
                  aria-label={`Remove ${row.team_name}`}
                  onClick={() => {
                    onChange(
                      value.filter(other => other.team_id !== row.team_id)
                    )
                  }}
                >
                  <X className="size-4" />
                </Button>
              </div>
              {errors[row.team_id] && (
                <p className="text-destructive mt-1 text-xs" role="alert">
                  {errors[row.team_id]}
                </p>
              )}
            </li>
          ))}
        </ul>
      )}
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button type="button" variant="outline" size="sm" disabled={full}>
            <Plus className="mr-2 size-4" />
            Add a team
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-[280px] p-0" align="start">
          {/* Mounted only while open, so the team search runs only when the
              admin is adding a team. */}
          <TeamSearchList
            exclude={chosen}
            onPick={team => {
              onChange([
                ...value,
                { team_id: team.id, team_name: team.name, role: 'member' },
              ])
              setOpen(false)
            }}
          />
        </PopoverContent>
      </Popover>
      {full && (
        <p className="text-muted-foreground text-xs">
          A new user can be added to at most {MAX_TEAM_ASSIGNMENTS} teams here.
        </p>
      )}
    </fieldset>
  )
}

/** The searchable list of teams that can be added: shared teams not chosen yet. */
function TeamSearchList({
  exclude,
  onPick,
}: Readonly<{
  exclude: ReadonlySet<string>
  onPick: (team: { id: string; name: string }) => void
}>) {
  const {
    teams,
    loading,
    loadingMore,
    error,
    hasMore,
    loadMore,
    query,
    setQuery,
  } = useAdminTeamSearch({ sharedOnly: true })
  // The server already leaves personal workspaces out; this keeps a stray one
  // (and every chosen team) off the list.
  const offered = teams.filter(
    team => !team.is_personal && !exclude.has(team.id)
  )

  const handleScroll = (event: UIEvent<HTMLDivElement>) => {
    const el = event.currentTarget
    if (
      el.scrollHeight - el.scrollTop - el.clientHeight <
      SCROLL_LOAD_THRESHOLD
    ) {
      loadMore()
    }
  }

  return (
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
            {offered.length === 0 && (
              <CommandEmpty>No teams to add.</CommandEmpty>
            )}
            {offered.map(team => (
              <CommandItem
                key={team.id}
                value={team.id}
                onSelect={() => {
                  onPick(team)
                }}
              >
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
  )
}
