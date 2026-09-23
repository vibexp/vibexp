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
import { cn } from '@/lib/utils'
import type { ProviderModel } from '@/services/modelProviderService'

/**
 * Plain case-insensitive substring match instead of cmdk's default fuzzy
 * scoring: model ids share long common prefixes (`vendor/model-…`), so a
 * subsequence match turns a precise query like `model-299` into dozens of hits.
 */
const matchesModelId = (value: string, search: string) =>
  value.toLowerCase().includes(search.trim().toLowerCase()) ? 1 : 0

interface ModelComboboxProps {
  value: string
  onChange: (model: string) => void
  /** Every model the provider listed — filtered client-side, never truncated. */
  models: ProviderModel[]
  disabled?: boolean
  // Forwarded onto the trigger by `FormControl`, so the field's label and
  // error message stay wired to a real, focusable element.
  id?: string
  'aria-describedby'?: string
  'aria-invalid'?: boolean
}

/**
 * Searchable model picker over a provider's own model list (#1076).
 *
 * Built on `Popover` + `Command` like `SourceTeamPicker` rather than a
 * `Select`: it lives inside the provider `Dialog`, where a Radix `Select`
 * sends jsdom into an infinite focus-scope loop. The trigger is a real
 * `Button` — an `asChild` trigger needs an element with a box (#891).
 *
 * A typed id that is not in the list can still be used ("Use …"), because a
 * provider's listing is not always exhaustive (aliases, fine-tunes).
 */
export function ModelCombobox({
  value,
  onChange,
  models,
  disabled = false,
  id,
  'aria-describedby': ariaDescribedBy,
  'aria-invalid': ariaInvalid,
}: Readonly<ModelComboboxProps>) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')

  const typed = search.trim()
  const offerCustom = typed !== '' && !models.some(m => m.id === typed)

  const select = (model: string) => {
    onChange(model)
    setOpen(false)
    setSearch('')
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-describedby={ariaDescribedBy}
          aria-invalid={ariaInvalid}
          disabled={disabled}
          data-testid="model-combobox"
          className={cn(
            'w-full justify-between font-normal',
            !value && 'text-muted-foreground'
          )}
        >
          <span className="truncate">{value || 'Select a model…'}</span>
          <ChevronsUpDown className="ml-2 size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-[--radix-popover-trigger-width] p-0"
        align="start"
      >
        <Command filter={matchesModelId}>
          <CommandInput
            placeholder="Search models…"
            className="h-9"
            value={search}
            onValueChange={setSearch}
          />
          <CommandList className="max-h-72">
            <CommandEmpty>No models found.</CommandEmpty>
            <CommandGroup>
              {offerCustom && (
                <CommandItem
                  forceMount
                  value={`__custom__:${typed}`}
                  data-testid="model-option-custom"
                  onSelect={() => {
                    select(typed)
                  }}
                >
                  Use &quot;{typed}&quot;
                </CommandItem>
              )}
              {models.map(model => (
                <CommandItem
                  key={model.id}
                  value={model.id}
                  data-testid="model-option"
                  onSelect={() => {
                    select(model.id)
                  }}
                >
                  <Check
                    className={cn(
                      'mr-2 size-4 shrink-0',
                      value === model.id ? 'opacity-100' : 'opacity-0'
                    )}
                  />
                  <span className="truncate">{model.id}</span>
                  {model.owned_by && (
                    <span className="text-muted-foreground ml-auto pl-2 text-xs">
                      {model.owned_by}
                    </span>
                  )}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
