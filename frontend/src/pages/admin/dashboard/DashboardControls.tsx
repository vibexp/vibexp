import type { DateRangeValue } from '@/components/ui/date-range'
import { DateRangePicker } from '@/components/ui/date-range-picker'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

export type Granularity = 'day' | 'week' | 'month'

/**
 * The one range control on the page.
 *
 * Every time-series panel is driven from here, so changing the range is a single
 * request rather than one per chart — and `TimeSeriesBarChart`'s own in-card range
 * select is suppressed (`hideRangeControl`) so there are never two controls
 * disagreeing about what the charts show.
 */
export function DashboardControls({
  range,
  onRangeChange,
  granularity,
  onGranularityChange,
}: Readonly<{
  range: DateRangeValue
  onRangeChange: (value: DateRangeValue) => void
  granularity: Granularity
  onGranularityChange: (value: Granularity) => void
}>) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <DateRangePicker
        value={range}
        onChange={onRangeChange}
        placeholder="Last 30 days"
        ariaLabel="Dashboard date range"
      />
      <Select
        value={granularity}
        onValueChange={value => {
          onGranularityChange(value as Granularity)
        }}
      >
        <SelectTrigger className="w-[130px]" aria-label="Bucket size">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="day">Daily</SelectItem>
          <SelectItem value="week">Weekly</SelectItem>
          <SelectItem value="month">Monthly</SelectItem>
        </SelectContent>
      </Select>
    </div>
  )
}
