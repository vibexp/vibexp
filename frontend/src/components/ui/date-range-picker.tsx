import { CalendarIcon } from 'lucide-react'
import * as React from 'react'

import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import type {
  DateRangePreset,
  DateRangeValue,
} from '@/components/ui/date-range'
import {
  DEFAULT_RANGE_PRESETS,
  formatRangeLabel,
  matchingPreset,
  normalizeRange,
  rangeForPreset,
} from '@/components/ui/date-range'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'

export interface DateRangePickerProps {
  value: DateRangeValue
  onChange: (value: DateRangeValue) => void
  /** Override the presets; pass `[]` for a calendar-only picker. */
  presets?: DateRangePreset[]
  placeholder?: string
  align?: 'start' | 'center' | 'end'
  disabled?: boolean
  className?: string
  /** Accessible name for the trigger, when "Date range: …" is not specific enough. */
  ariaLabel?: string
  /** Injectable clock, so a preset's "today" is deterministic in tests. */
  now?: Date
}

/**
 * Date-range picker: preset shortcuts beside a two-month calendar, in a Popover.
 *
 * ```tsx
 * const [range, setRange] = useState<DateRangeValue>({})
 * <DateRangePicker value={range} onChange={setRange} placeholder="Any date" />
 * ```
 *
 * Fully controlled — it holds no range state, only whether the popover is open —
 * so the parent can keep the value anywhere, including the URL. Domain-free by
 * design (no data fetching, no router, no admin imports), on the same terms as
 * `ListPage`, so it can move into `@vibexp/design-system` unchanged.
 *
 * Keyboard and screen-reader behaviour comes from the primitives underneath:
 * Radix's Popover handles Enter/Space to open, Escape to close and focus return,
 * and `react-day-picker` renders labelled month grids with arrow-key day
 * navigation.
 *
 * Date arithmetic and formatting live in `date-range.ts`, in local time
 * throughout.
 */
export function DateRangePicker({
  value,
  onChange,
  presets = DEFAULT_RANGE_PRESETS,
  placeholder = 'Pick a date range',
  align = 'start',
  disabled = false,
  className,
  ariaLabel,
  now,
}: Readonly<DateRangePickerProps>) {
  const [open, setOpen] = React.useState(false)
  // One clock for the whole render, so a preset's label cannot be computed
  // against a different "now" than the range it produced.
  const clock = now ?? new Date()
  const normalized = normalizeRange(value)
  const label = formatRangeLabel(normalized, presets, clock)
  const selectedPreset = matchingPreset(normalized, presets, clock)

  const handlePreset = (preset: DateRangePreset) => {
    onChange(rangeForPreset(preset, clock))
    // A preset is a complete choice, so close. The calendar stays open instead,
    // because a range needs two clicks.
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          disabled={disabled}
          aria-label={
            ariaLabel ?? (label ? `Date range: ${label}` : placeholder)
          }
          className={cn(
            'justify-start gap-2 font-normal',
            !label && 'text-muted-foreground',
            className
          )}
        >
          <CalendarIcon className="size-4" aria-hidden />
          {label ?? placeholder}
        </Button>
      </PopoverTrigger>
      <PopoverContent align={align} className="w-auto p-0">
        <div className="flex flex-col sm:flex-row">
          {presets.length > 0 && (
            <div className="flex flex-col gap-1 border-b p-2 sm:border-b-0 sm:border-r">
              {presets.map(preset => (
                <Button
                  key={preset.label}
                  variant={
                    selectedPreset?.label === preset.label
                      ? 'secondary'
                      : 'ghost'
                  }
                  size="sm"
                  className="justify-start font-normal"
                  onClick={() => {
                    handlePreset(preset)
                  }}
                >
                  {preset.label}
                </Button>
              ))}
            </div>
          )}
          <div>
            <Calendar
              mode="range"
              numberOfMonths={2}
              // Open on the month the range starts in, not on today, so a
              // restored range is visible without paging back to it. Falling
              // back to `clock` rather than letting the library reach for its
              // own `new Date()` keeps an injected `now` authoritative for
              // everything the picker shows.
              defaultMonth={normalized.from ?? clock}
              today={clock}
              selected={
                normalized.from
                  ? { from: normalized.from, to: normalized.to }
                  : undefined
              }
              onSelect={range => {
                onChange({ from: range?.from, to: range?.to })
              }}
            />
            <div className="flex justify-end border-t p-2">
              <Button
                variant="ghost"
                size="sm"
                disabled={!normalized.from && !normalized.to}
                onClick={() => {
                  onChange({})
                  setOpen(false)
                }}
              >
                Clear
              </Button>
            </div>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}
