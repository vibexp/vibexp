import { endOfDay, format, isSameDay, startOfDay, subDays } from 'date-fns'

/**
 * Pure date-range helpers behind `DateRangePicker`.
 *
 * Split out of the component file so consumers (#458/#459/#460/#461) can import
 * the presets and the label formatter without pulling in a React component —
 * which also keeps `date-range-picker.tsx` a components-only module for Fast
 * Refresh.
 *
 * Everything here works in **local** time and never parses a date string:
 * `new Date('2026-05-01')` is parsed as UTC and lands on the previous day for
 * anyone west of it, the bug already documented on `parseLocalDate` in
 * `TimeSeriesBarChart.tsx`. Serializing for an API is the caller's job.
 */

/**
 * A selected range. Both ends optional so `{}` is a valid "nothing selected"
 * value.
 *
 * Deliberately **not** `react-day-picker`'s own `DateRange`, whose `from` is
 * required-but-possibly-undefined (`from: Date | undefined`) — that makes an
 * empty `{}` a type error for every consumer and puts a same-named type in two
 * places. This one is adapted to the library's shape at the boundary instead.
 */
export interface DateRangeValue {
  from?: Date
  to?: Date
}

/** A one-click relative window. `days` counts today, so 7 = today plus 6 back. */
export interface DateRangePreset {
  label: string
  days: number
}

export const DEFAULT_RANGE_PRESETS: DateRangePreset[] = [
  { label: 'Last 7 days', days: 7 },
  { label: 'Last 30 days', days: 30 },
  { label: 'Last 90 days', days: 90 },
]

/**
 * The range a preset denotes, relative to `now` (injectable so tests and the
 * picker can pin the clock).
 */
export function rangeForPreset(
  preset: DateRangePreset,
  now: Date = new Date()
): DateRangeValue {
  return {
    from: startOfDay(subDays(now, preset.days - 1)),
    to: endOfDay(now),
  }
}

/**
 * Orders the ends so `from` is never after `to`.
 *
 * The calendar cannot produce an inverted range, but a controlled `value` can —
 * a consumer restoring two dates from a URL has no ordering guarantee. Swapping
 * beats rendering a range the calendar shows as empty.
 */
export function normalizeRange(value: DateRangeValue): DateRangeValue {
  const { from, to } = value
  if (from && to && from > to) {
    return { from: to, to: from }
  }
  return value
}

/** Which preset, if any, the current value matches — used for the label. */
export function matchingPreset(
  value: DateRangeValue,
  presets: DateRangePreset[],
  now: Date = new Date()
): DateRangePreset | undefined {
  const { from, to } = value
  if (!from || !to) return undefined
  return presets.find(preset => {
    const candidate = rangeForPreset(preset, now)
    return (
      candidate.from !== undefined &&
      candidate.to !== undefined &&
      isSameDay(candidate.from, from) &&
      isSameDay(candidate.to, to)
    )
  })
}

/**
 * Human-readable summary of a range: the preset name when it matches one, a
 * single date when both ends fall on the same day, otherwise "from – to".
 * `undefined` when nothing is selected, so a caller can fall back to a
 * placeholder.
 */
export function formatRangeLabel(
  value: DateRangeValue,
  presets: DateRangePreset[] = DEFAULT_RANGE_PRESETS,
  now: Date = new Date()
): string | undefined {
  const { from, to } = normalizeRange(value)
  if (!from) return undefined

  const preset = matchingPreset({ from, to }, presets, now)
  if (preset) return preset.label
  if (!to || isSameDay(from, to)) return format(from, 'd MMM yyyy')
  return `${format(from, 'd MMM')} – ${format(to, 'd MMM yyyy')}`
}
