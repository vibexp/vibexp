import type { DateRangeValue } from '@/components/ui/date-range'
import { DateRangePicker } from '@/components/ui/date-range-picker'

export interface DateTimeRangeFilterProps {
  label: string
  value: DateRangeValue
  onChange: (next: DateRangeValue) => void
  /** Injectable clock for the picker's presets (tests). */
  now?: Date
}

/**
 * A labelled `DateRangePicker` for the advanced panel, so the three admin list
 * pages label their date filters the same way.
 *
 * Day-granular despite the name: the URL carries local `YYYY-MM-DD` days and the
 * request carries start-of-day / end-of-day instants (`rangeToInstants`).
 */
export function DateTimeRangeFilter({
  label,
  value,
  onChange,
  now,
}: Readonly<DateTimeRangeFilterProps>) {
  return (
    <fieldset className="flex min-w-0 flex-col gap-1.5">
      <legend className="mb-1.5 text-sm font-medium leading-none">
        {label}
      </legend>
      <DateRangePicker
        value={value}
        onChange={onChange}
        placeholder="Any time"
        ariaLabel={label}
        now={now}
      />
    </fieldset>
  )
}
