import { Check, ChevronsUpDown } from 'lucide-react'
import { useState } from 'react'
import { useLocation } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { useTeam } from '@/contexts/TeamContext'
import { cn } from '@/lib/utils'

/**
 * Routes where the selected team cannot apply.
 *
 * A **denylist**, deliberately the opposite polarity to `ProjectSwitcher`'s
 * allowlist. Only six sections are project-filtered, but nearly everything is
 * team-scoped - prompts, artifacts, blueprints, memories, feeds, agents,
 * search, the dashboard, MCP servers, AI tools - so an allowlist would silently
 * mis-scope every route added later. A denylist fails safe: a new route keeps
 * the switcher enabled, which is right far more often than not.
 *
 * `/admin` never actually reaches here - the instance-admin portal renders its
 * own `AdminHeader` with no `TeamProvider` (#456), so this component does not
 * mount there at all. The entry stays as documentation of the intent.
 */
const PERSONAL_PREFIXES = ['/settings', '/notifications', '/admin']

function isPersonalPath(pathname: string): boolean {
  // `/teams` (the list) is EXACT-match only. As a prefix it would swallow
  // `/teams/:id/**`, which is team-scoped and needs the other explanation.
  if (pathname === '/teams') return true
  return PERSONAL_PREFIXES.some(
    prefix => pathname === prefix || pathname.startsWith(`${prefix}/`)
  )
}

/** The team id pinned by a `/teams/:id/**` URL, if the path is one. */
function pinnedTeamId(pathname: string): string | null {
  return /^\/teams\/([^/]+)/.exec(pathname)?.[1] ?? null
}

/**
 * Header team selector.
 *
 * Three states (#544):
 *  - **Interactive** - anywhere team-scoped, which is most of the app.
 *  - **Disabled, personal** - the page does not act on a team at all.
 *  - **Disabled, URL-pinned** - inside `/teams/:id/**` the address bar decides
 *    the team, so offering a switch here would contradict the URL. Making the
 *    switcher navigate between teams was rejected (epic decision 4).
 *
 * The two disabled states share a look but never a tooltip: conflating "does
 * not apply" with "the URL already chose" makes the second read like a bug.
 */
export function TeamSwitcher() {
  const { teams, currentTeam, setCurrentTeam, isLoading } = useTeam()
  const { pathname } = useLocation()
  const [open, setOpen] = useState(false)

  const pinned = pinnedTeamId(pathname)

  // `TeamScopeLayout` (#539) syncs the ambient team to `:id`, but its effect
  // runs after this header has already rendered. Showing "Loading..." until the
  // two agree is what stops the previous team flashing in the header under a
  // URL that names a different one.
  const awaitingScopeSync = pinned !== null && currentTeam?.id !== pinned

  if (isLoading || awaitingScopeSync) {
    return (
      <Button variant="outline" size="sm" disabled className="h-8">
        Loading…
      </Button>
    )
  }

  if (teams.length === 0) {
    return null
  }

  const personal = isPersonalPath(pathname)
  const disabled = personal || pinned !== null

  const tooltip = personal
    ? 'Your team does not apply to this page'
    : pinned !== null
      ? `This page is scoped to ${currentTeam?.name ?? 'this team'}`
      : undefined

  return (
    <span title={tooltip}>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            size="sm"
            role="combobox"
            aria-expanded={open}
            disabled={disabled}
            className="h-8 max-w-[220px] justify-between"
            data-testid="team-switcher"
          >
            <span className="truncate" data-testid="current-team-name">
              {currentTeam?.name ?? 'Select team'}
            </span>
            <ChevronsUpDown className="ml-2 size-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-[240px] p-0" align="start">
          <Command>
            <CommandInput placeholder="Search team…" className="h-9" />
            <CommandList>
              <CommandEmpty>No teams found.</CommandEmpty>
              <CommandGroup>
                {teams.map(team => (
                  <CommandItem
                    key={team.id}
                    value={team.name}
                    onSelect={() => {
                      setCurrentTeam(team)
                      setOpen(false)
                    }}
                  >
                    <Check
                      className={cn(
                        'mr-2 size-4',
                        currentTeam?.id === team.id
                          ? 'opacity-100'
                          : 'opacity-0'
                      )}
                    />
                    <span className="truncate">{team.name}</span>
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </span>
  )
}
