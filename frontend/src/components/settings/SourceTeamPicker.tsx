import { Check, ChevronsUpDown } from 'lucide-react'
import { useMemo } from 'react'

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
import type { Team } from '@/services/teamService'

interface SourceTeamPickerProps {
  /**
   * The DESTINATION team, passed down as a prop rather than read from
   * `useTeam()`: on a cold deep-link the ambient team is still the previously
   * persisted one when a settings page's first effect runs (#584). It is
   * excluded from the options because source == destination is a 400.
   */
  destinationTeam: Team
  /** Currently selected source team id, or null when nothing is chosen. */
  value: string | null
  onChange: (team: Team) => void
  /**
   * Narrows which of the caller's teams may act as a source. The copy
   * endpoints authorize the SOURCE team on the same bar as the destination,
   * so a page whose copy needs a permission passes it here (e.g.
   * `t => t.permissions.includes('team.update')` for the provider copies).
   * Omitted means every other team the user belongs to qualifies, which is
   * the custom-types bar (membership alone).
   */
  canCopyFrom?: (team: Team) => boolean
  disabled?: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
  id?: string
}

/**
 * Searchable picker over the teams the user belongs to, minus the destination.
 *
 * Built on `Popover` + `Command` following {@link ProjectPicker} /
 * `TeamSwitcher` rather than a `Select`: a Radix `Select` mounted inside a
 * `Dialog` sends jsdom into an infinite focus-scope loop and takes the whole
 * Vitest suite down with it, and this picker's only home is inside a dialog.
 *
 * `useTeam().teams` is the right source here even though the ambient
 * *current* team is not: it is the membership-filtered list of the user's
 * teams, which is exactly the candidate set, and it carries no cold-deep-link
 * staleness because it is not scoped to the URL.
 */
export function SourceTeamPicker({
  destinationTeam,
  value,
  onChange,
  canCopyFrom,
  disabled = false,
  open,
  onOpenChange,
  id,
}: Readonly<SourceTeamPickerProps>) {
  const { teams } = useTeam()

  const options = useMemo(
    () =>
      teams.filter(
        team => team.id !== destinationTeam.id && (canCopyFrom?.(team) ?? true)
      ),
    [teams, destinationTeam.id, canCopyFrom]
  )

  const selected = useMemo(
    () => options.find(team => team.id === value) ?? null,
    [options, value]
  )

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          data-testid="source-team-picker"
          className={cn(
            'w-full justify-between font-normal',
            !selected && 'text-muted-foreground'
          )}
        >
          <span className="truncate">{selected?.name ?? 'Select a team…'}</span>
          <ChevronsUpDown className="ml-2 size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-[--radix-popover-trigger-width] p-0"
        align="start"
      >
        <Command>
          <CommandInput placeholder="Search teams…" className="h-9" />
          <CommandList className="max-h-72">
            <CommandEmpty>No other teams found.</CommandEmpty>
            <CommandGroup>
              {options.map(team => (
                <CommandItem
                  key={team.id}
                  value={team.name}
                  data-testid="source-team-option"
                  onSelect={() => {
                    onChange(team)
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
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
