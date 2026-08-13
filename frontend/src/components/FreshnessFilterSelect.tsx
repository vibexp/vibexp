import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

/** The only freshness filter the API accepts; anything else is a 400. */
export const FRESHNESS_FILTER_STALE = 'stale'

/**
 * The "stale only" filter shared by all four resource list bars (#738).
 *
 * One component rather than four near-identical `Select`s, for the same reason
 * `FreshnessBadge` is shared: four copies of a two-option filter is exactly how
 * the four list surfaces drift apart.
 *
 * `undefined` means no freshness filtering — the API treats an absent param as
 * "everything" and rejects any value other than `stale` with a 400, so the
 * "all" option must clear the param rather than send a second value.
 */
export function FreshnessFilterSelect({
  value,
  onChange,
  ariaLabel = 'Filter by freshness',
  testId = 'freshness-filter',
}: Readonly<{
  value: 'stale' | undefined
  onChange: (value: 'stale' | undefined) => void
  ariaLabel?: string
  testId?: string
}>) {
  return (
    <Select
      value={value ?? 'all'}
      onValueChange={next => {
        onChange(next === FRESHNESS_FILTER_STALE ? 'stale' : undefined)
      }}
    >
      <SelectTrigger
        className="w-[150px]"
        aria-label={ariaLabel}
        data-testid={testId}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">All freshness</SelectItem>
        <SelectItem value={FRESHNESS_FILTER_STALE}>Stale only</SelectItem>
      </SelectContent>
    </Select>
  )
}
