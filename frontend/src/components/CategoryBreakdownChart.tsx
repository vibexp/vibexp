import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { PanelTitle } from '@/components/ui/panel-title'
import { Skeleton } from '@/components/ui/skeleton'

/** One category row: its label, its count, and an optional muted suffix. */
export interface CategoryDatum {
  /** Stable key for React and for tests. */
  key: string
  label: string
  count: number
  /** Rendered after the label in muted text, e.g. a rule's threshold. */
  detail?: string
  /** Dims the row and its swatch — used for rules that are disabled. */
  muted?: boolean
}

interface CategoryBreakdownChartProps {
  title: string
  /** Label preceding the headline total, e.g. "Stale resources". */
  totalLabel: string
  total: number
  data: CategoryDatum[]
  loading: boolean
  error: boolean
  errorMessage: string
  emptyMessage: string
  /**
   * Render at most this many rows, then a "+N more" line. The by-project and
   * by-rule endpoints return an entry for EVERY project/rule including zeros,
   * so an unbounded list would grow with the workspace rather than with the
   * problem.
   */
  maxRows?: number
}

/**
 * Point-in-time breakdown of a single count across categories, as a sorted list
 * of proportional bars.
 *
 * This exists because `TimeSeriesBarChart` cannot render categorical data: it
 * maps every datum through `parseLocalDate(item.date)`, and its `breakdown`
 * legend derives its rows by reducing a *time series* over series keys. The
 * three freshness breakdowns (by type, by project, by rule) are
 * `{total_stale, counts}` with no date axis (#737).
 *
 * The row treatment — swatch, label, proportional bar, count — is deliberately
 * the same as that `breakdown` legend, on the same `--chart-N` variables, so the
 * two read as one chart family in both themes.
 */
export function CategoryBreakdownChart({
  title,
  totalLabel,
  total,
  data,
  loading,
  error,
  errorMessage,
  emptyMessage,
  maxRows = 8,
}: Readonly<CategoryBreakdownChartProps>) {
  // Sorted high→low, each bar scaled against the largest row rather than the
  // total: scaling against the total makes every bar invisible as soon as the
  // categories are many and even.
  const sorted = [...data].sort((a, b) => b.count - a.count)
  const shown = sorted.slice(0, maxRows)
  const hiddenCount = sorted.length - shown.length
  const max = Math.max(1, ...sorted.map(d => d.count))

  // Empty means "nothing is stale", which is a real and good answer — distinct
  // from having no categories at all.
  const isEmpty = sorted.length === 0 || total === 0

  const body = () => {
    if (loading) return <Skeleton className="h-[180px] w-full" />
    if (error || isEmpty) {
      return (
        <div className="text-muted-foreground flex h-16 items-center justify-center text-center text-sm">
          {error ? errorMessage : emptyMessage}
        </div>
      )
    }
    return (
      <ul className="border-border border-t">
        {shown.map((d, index) => (
          <li
            key={d.key}
            className="border-border flex items-center gap-2.5 border-b py-2 last:border-b-0"
            data-testid="category-row"
          >
            <span
              aria-hidden
              className="size-[9px] shrink-0 rounded-[2px]"
              style={{
                background: `var(--chart-${String((index % 5) + 1)})`,
                opacity: d.muted ? 0.4 : 1,
              }}
            />
            <span className={`flex-1 text-sm ${d.muted ? 'opacity-60' : ''}`}>
              {d.label}
              {d.detail && (
                <span className="text-muted-foreground ml-1.5 text-xs">
                  {d.detail}
                </span>
              )}
            </span>
            <span
              aria-hidden
              className="bg-secondary h-[5px] w-[84px] overflow-hidden rounded-full"
            >
              <span
                className="block h-full rounded-full"
                style={{
                  width: `${((d.count / max) * 100).toFixed(1)}%`,
                  background: `var(--chart-${String((index % 5) + 1)})`,
                }}
              />
            </span>
            <span className="w-7 text-right text-sm font-semibold tabular-nums">
              {d.count}
            </span>
          </li>
        ))}
        {hiddenCount > 0 && (
          <li className="text-muted-foreground py-2 text-xs">
            +{hiddenCount} more
          </li>
        )}
      </ul>
    )
  }

  return (
    <Card data-testid="category-breakdown-chart">
      <CardHeader>
        <PanelTitle>{title}</PanelTitle>
        <div className="text-muted-foreground mt-1 flex items-center gap-1.5 text-sm">
          <span>{totalLabel}:</span>
          {loading ? (
            <Skeleton className="h-4 w-10" />
          ) : (
            <span className="font-medium">{total}</span>
          )}
        </div>
      </CardHeader>
      <CardContent>{body()}</CardContent>
    </Card>
  )
}
