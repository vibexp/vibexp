import { Check, ChevronsUpDown } from 'lucide-react'
import { useState } from 'react'

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

export function TeamSwitcher() {
  const { teams, currentTeam, setCurrentTeam, isLoading } = useTeam()
  const [open, setOpen] = useState(false)

  if (isLoading) {
    return (
      <Button variant="outline" size="sm" disabled className="h-8">
        Loading…
      </Button>
    )
  }

  if (teams.length === 0) {
    return null
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          role="combobox"
          aria-expanded={open}
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
                      currentTeam?.id === team.id ? 'opacity-100' : 'opacity-0'
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
