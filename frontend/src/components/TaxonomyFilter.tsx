import { ChevronsUpDown, Loader2, Tags } from 'lucide-react'
import { useCallback, useState } from 'react'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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

/**
 * A controlled multi-select over a resource's taxonomy catalog — today the
 * team's prompt labels (#908).
 *
 * A `Select` cannot express this: the list endpoints take a comma-separated
 * list, so `?labels=a,b` has to be reachable from the UI as well as from a
 * shared link. The idiom is Popover + cmdk `Command`, the same one
 * `MetadataFilter` uses — deliberately not a Radix `Select`, which loops in
 * jsdom, and not a bespoke dropdown.
 *
 * Domain-free and fetch-free: the catalog arrives via props, so this stays
 * promotable into the design system unchanged. Toggling applies immediately —
 * there is no draft/Apply step, because unlike a metadata chip a label carries
 * no second dimension to fill in first.
 */
export interface TaxonomyFilterProps {
  /** The selected values, in the order they will be serialized. */
  value: readonly string[]
  onChange: (value: string[]) => void
  /** The catalog to choose from. */
  options: readonly string[]
  loading?: boolean
  error?: string | null
  /** Called when the popover opens, so the host can load the catalog lazily. */
  onOpen?: () => void
  /** Accessible name for the trigger, e.g. "Filter by labels". */
  label: string
  /** Noun used in the placeholder and empty state ("labels"). */
  noun?: string
  testId?: string
  disabled?: boolean
  /** Lets a filter bar give the trigger the width its other controls use. */
  className?: string
}

/** The trigger reads as the filter, not as a count, until there are too many. */
function triggerText(value: readonly string[], noun: string): string {
  if (value.length === 0) return `All ${noun}`
  if (value.length === 1) return value[0]
  return `${String(value.length)} ${noun}`
}

export function TaxonomyFilter({
  value,
  onChange,
  options,
  loading = false,
  error = null,
  onOpen,
  label,
  noun = 'labels',
  testId = 'taxonomy-filter',
  disabled = false,
  className,
}: Readonly<TaxonomyFilterProps>) {
  const [open, setOpen] = useState(false)

  const handleOpenChange = useCallback(
    (next: boolean) => {
      setOpen(next)
      if (next) onOpen?.()
    },
    [onOpen]
  )

  const toggle = useCallback(
    (candidate: string) => {
      onChange(
        value.includes(candidate)
          ? value.filter(entry => entry !== candidate)
          : [...value, candidate]
      )
    },
    [onChange, value]
  )

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-label={label}
          disabled={disabled}
          data-testid={testId}
          className={cn('justify-between font-normal', className)}
        >
          <span className="flex min-w-0 items-center gap-2">
            <Tags className="size-4 shrink-0 opacity-60" />
            <span className="truncate">{triggerText(value, noun)}</span>
          </span>
          <ChevronsUpDown className="ml-2 size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>

      <PopoverContent className="w-64 p-0" align="start">
        <Command shouldFilter>
          <CommandInput
            placeholder={`Search ${noun}…`}
            className="h-9"
            aria-label={`Search ${noun}`}
          />
          <CommandList className="max-h-64">
            {loading && (
              <div className="flex items-center justify-center py-6">
                <Loader2 className="size-4 animate-spin" />
              </div>
            )}
            {!loading && error !== null && (
              <div className="text-destructive px-3 py-6 text-center text-sm">
                {error}
              </div>
            )}
            {!loading && error === null && (
              <>
                <CommandEmpty>No {noun} found.</CommandEmpty>
                {options.map(option => (
                  // onSelect toggles rather than commits, so the list stays a
                  // multi-select and the popover stays open between picks.
                  <CommandItem
                    key={option}
                    value={option}
                    onSelect={() => {
                      toggle(option)
                    }}
                  >
                    <Checkbox
                      checked={value.includes(option)}
                      aria-label={option}
                      className="mr-2"
                      tabIndex={-1}
                    />
                    {option}
                  </CommandItem>
                ))}
              </>
            )}
          </CommandList>

          {value.length > 0 && (
            <div className="flex items-center justify-end border-t p-2">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                aria-label={`Clear ${noun} filter`}
                onClick={() => {
                  onChange([])
                }}
              >
                Clear
              </Button>
            </div>
          )}
        </Command>
      </PopoverContent>
    </Popover>
  )
}
