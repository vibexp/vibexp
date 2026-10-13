import { Check, ChevronsUpDown } from 'lucide-react'
import { useState } from 'react'
import { useLocation } from 'react-router'

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
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
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

interface TeamSwitcherProps {
  /**
   * - `full`: the team's name on a full-width trigger — the expanded sidebar
   *   and the mobile drawer.
   * - `rail`: the team's initial on an icon-sized trigger, named by a tooltip
   *   — the collapsed sidebar, where there is no room for a name.
   */
  variant?: 'full' | 'rail'
}

/**
 * The workspace (team) selector, at the top of the sidebar under the brand.
 *
 * It lives in the navigation, not in the header beside the project filter,
 * because it is not a filter: the team is the global scope everything on the
 * page belongs to.
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
export function TeamSwitcher({
  variant = 'full',
}: Readonly<TeamSwitcherProps>) {
  const { teams, currentTeam, setCurrentTeam, isLoading } = useTeam()
  const { pathname } = useLocation()
  const [open, setOpen] = useState(false)
  const rail = variant === 'rail'

  const pinned = pinnedTeamId(pathname)

  // `TeamScopeLayout` (#539) syncs the ambient team to `:id`, but its effect
  // runs after this component has already rendered. Showing a loading state
  // until the two agree is what stops the previous team flashing under a URL
  // that names a different one.
  const awaitingScopeSync = pinned !== null && currentTeam?.id !== pinned

  if (isLoading || awaitingScopeSync) {
    return (
      <Button
        variant="outline"
        size="sm"
        disabled
        className={rail ? 'size-9 p-0' : 'h-9 w-full justify-start'}
        aria-label="Loading workspace"
      >
        {rail ? '…' : 'Loading…'}
      </Button>
    )
  }

  if (teams.length === 0) {
    return null
  }

  const personal = isPersonalPath(pathname)
  const disabled = personal || pinned !== null
  const name = currentTeam?.name ?? 'Select team'

  let note: string | undefined
  if (personal) {
    note = 'Your team does not apply to this page'
  } else if (pinned !== null) {
    note = `This page is scoped to ${currentTeam?.name ?? 'this team'}`
  }

  const trigger = (
    <PopoverTrigger asChild>
      {/* A real button in both forms: the popover (and, on the rail, the
          tooltip) anchors on its box (#891). */}
      <Button
        variant="outline"
        size="sm"
        role="combobox"
        aria-expanded={open}
        aria-label={rail ? `Workspace: ${name}` : undefined}
        disabled={disabled}
        className={
          rail
            ? 'size-9 p-0 font-semibold'
            : 'h-9 w-full min-w-0 justify-between'
        }
        data-testid="team-switcher"
      >
        {rail ? (
          <>
            <span aria-hidden="true">
              {Array.from(name.trim())[0]?.toUpperCase() ?? '?'}
            </span>
            <span className="sr-only" data-testid="current-team-name">
              {name}
            </span>
          </>
        ) : (
          <>
            <span className="truncate" data-testid="current-team-name">
              {name}
            </span>
            <ChevronsUpDown className="ml-2 size-4 shrink-0 opacity-50" />
          </>
        )}
      </Button>
    </PopoverTrigger>
  )

  return (
    // The wrapper carries the "why is this disabled" note: a disabled button
    // fires no pointer events, so it cannot show a tooltip of its own.
    <span title={note} className={rail ? 'inline-flex' : 'flex w-full min-w-0'}>
      <Popover open={open} onOpenChange={setOpen}>
        {rail ? (
          <Tooltip>
            <TooltipTrigger asChild>{trigger}</TooltipTrigger>
            <TooltipContent side="right">{name}</TooltipContent>
          </Tooltip>
        ) : (
          trigger
        )}
        <PopoverContent
          className="w-60 p-0"
          align="start"
          side={rail ? 'right' : 'bottom'}
        >
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
