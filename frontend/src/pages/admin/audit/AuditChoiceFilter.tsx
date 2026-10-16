import { Check, ChevronsUpDown } from 'lucide-react'
import { Fragment, useState } from 'react'

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

export interface AuditChoice {
  value: string
  label: string
  /** Group id, matched against `groups`. */
  group?: string
}

export interface AuditChoiceFilterProps {
  /** The selected values (one at most when `multiple` is false). */
  value: readonly string[]
  onChange: (value: string[]) => void
  options: readonly AuditChoice[]
  /** Section headings, in display order; options are listed under theirs. */
  groups?: readonly { id: string; label: string }[]
  /** Allow several values (default). A single-choice filter toggles one. */
  multiple?: boolean
  /** Accessible name of the trigger, e.g. "Filter by operation". */
  label: string
  /** Trigger text with nothing selected, e.g. "All operations". */
  allLabel: string
  /** Plural noun for "3 operations". */
  noun: string
  /** Show a search box over the options (long lists). */
  searchable?: boolean
  className?: string
}

function triggerText(
  value: readonly string[],
  options: readonly AuditChoice[],
  allLabel: string,
  noun: string
): string {
  if (value.length === 0) return allLabel
  if (value.length === 1) {
    return options.find(option => option.value === value[0])?.label ?? value[0]
  }
  return `${String(value.length)} ${noun}`
}

/**
 * The audit log's choice filters (#1517): operation (grouped by domain),
 * source, outcome and resource type.
 *
 * The same Popover + cmdk `Command` idiom as `TaxonomyFilter` — not a Radix
 * `Select`, which loops in jsdom and cannot hold several values. Toggling
 * applies immediately; the page writes the selection to the URL.
 */
export function AuditChoiceFilter({
  value,
  onChange,
  options,
  groups,
  multiple = true,
  label,
  allLabel,
  noun,
  searchable = false,
  className,
}: Readonly<AuditChoiceFilterProps>) {
  const [open, setOpen] = useState(false)

  const toggle = (choice: string) => {
    const selected = value.includes(choice)
    if (!multiple) {
      onChange(selected ? [] : [choice])
      setOpen(false)
      return
    }
    // Kept in option order, so the URL is the same however it was clicked.
    const next = new Set(value)
    if (selected) next.delete(choice)
    else next.add(choice)
    onChange(options.map(option => option.value).filter(item => next.has(item)))
  }

  const renderItem = (option: AuditChoice) => (
    <CommandItem
      key={option.value}
      value={`${option.label} ${option.value}`}
      onSelect={() => {
        toggle(option.value)
      }}
    >
      <Check
        className={cn(
          'mr-2 size-4 shrink-0',
          value.includes(option.value) ? 'opacity-100' : 'opacity-0'
        )}
      />
      <span className="truncate">{option.label}</span>
    </CommandItem>
  )

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-label={label}
          className={cn(
            'w-[180px] justify-between font-normal',
            value.length === 0 && 'text-muted-foreground',
            className
          )}
        >
          <span className="truncate">
            {triggerText(value, options, allLabel, noun)}
          </span>
          <ChevronsUpDown className="ml-2 size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[260px] p-0" align="start">
        <Command>
          {searchable && (
            <CommandInput placeholder={`Search ${noun}…`} className="h-9" />
          )}
          <CommandList className="max-h-72">
            <CommandEmpty>No {noun} found.</CommandEmpty>
            {groups === undefined
              ? options.map(renderItem)
              : groups.map(group => {
                  const inGroup = options.filter(o => o.group === group.id)
                  if (inGroup.length === 0) return <Fragment key={group.id} />
                  return (
                    <CommandGroup key={group.id} heading={group.label}>
                      {inGroup.map(renderItem)}
                    </CommandGroup>
                  )
                })}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
