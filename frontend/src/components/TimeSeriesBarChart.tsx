import { useMemo, useState } from 'react'
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { PanelTitle } from '@/components/ui/panel-title'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

/** A single bar series: the data key it reads, its legend label, and its fill. */
export interface ChartSeries {
  key: string
  label: string
  fill: string
}

/** One day's data point: a YYYY-MM-DD date, a per-day total, and one numeric
 * value per series key. */
export interface TimeSeriesDatum {
  date: string
  total: number
  [key: string]: number | string
}

/** The shared selectable reporting windows, identical across timeseries charts. */
export const TIME_SERIES_RANGE_OPTIONS = [
  { value: '7d', label: 'Last 7 days' },
  { value: '14d', label: 'Last 14 days' },
  { value: '30d', label: 'Last 30 days' },
  { value: '60d', label: 'Last 2 months' },
  { value: '90d', label: 'Last 3 months' },
  { value: '180d', label: 'Last 6 months' },
]

/**
 * Parse a date-only string (YYYY-MM-DD) as local midnight so axis labels match
 * the user's timezone. `new Date('2026-05-01')` parses as UTC and shifts the
 * label a day west of UTC.
 */
export function parseLocalDate(dateOnly: string): Date {
  const [year, month, day] = dateOnly.split('-').map(Number)
  return new Date(year, month - 1, day)
}

/**
 * Legend treatment — always rendered **below** the chart when shown:
 * - `strip`: a one-line, clickable swatch+label row that toggles each series'
 *   visibility. The default for multi-series charts.
 * - `breakdown`: one row per series (swatch + label + proportional bar + count),
 *   sorted high→low — the richer "how was it split?" view used by the compact
 *   Access activity widget. Static (no toggling).
 * - `none`: no legend, e.g. single-series charts like "Session activity".
 */
export type ChartLegend = 'none' | 'strip' | 'breakdown'

/**
 * Visual density:
 * - `compact`: small title, ~110px chart, no axes, small range selector, and
 *   first/last date ticks under the bars. For narrow detail/sidebar cards.
 * - `comfortable`: `PanelTitle`, ~180px chart with X/Y axes, full range selector.
 *   For full-width analytics pages.
 */
export type ChartSize = 'compact' | 'comfortable'

interface ChartTooltipProps {
  active?: boolean
  payload?: {
    payload: TimeSeriesDatum & { label: string }
  }[]
  series?: readonly ChartSeries[]
}

function ChartTooltip({ active, payload, series = [] }: ChartTooltipProps) {
  if (!active || !payload?.length) return null
  const entry = payload[0].payload
  return (
    <div className="bg-popover text-popover-foreground rounded-md border px-3 py-2 text-xs shadow-md">
      <p className="font-medium">{entry.label}</p>
      {series.map(s => (
        <p key={s.key} className="text-muted-foreground">
          {s.label}: {entry[s.key]}
        </p>
      ))}
      <p className="mt-1 font-medium">Total: {entry.total}</p>
    </div>
  )
}

export interface TimeSeriesBarChartProps {
  /** Card heading, e.g. "Access activity" or "Resources created". */
  title: string
  /** Label preceding the running total, e.g. "Total accesses". */
  totalLabel: string
  /** The grand total across the whole window (drives the header + empty state). */
  total: number
  /** The bar series to render, in display order. */
  series: readonly ChartSeries[]
  /** Per-day data points (sparse-free / zero-filled by the caller). */
  data: TimeSeriesDatum[]
  /** Currently selected range value (one of TIME_SERIES_RANGE_OPTIONS). */
  range: string
  /** Required unless hideRangeControl is set (the in-card range selector calls it). */
  onRangeChange?: (range: string) => void
  /**
   * Hide the in-card range selector. Use when an external control (e.g. a single
   * page-level filter driving several charts) owns the range instead. Defaults to
   * false so existing self-contained charts keep their own selector.
   */
  hideRangeControl?: boolean
  loading: boolean
  error: boolean
  /** Message shown when the fetch failed. */
  errorMessage: string
  /** Message shown when there is no data / a zero total. */
  emptyMessage: string
  /** Stack the bars/areas (single cumulative series per day) vs. grouped. */
  stacked?: boolean
  /** Legend treatment, rendered below the chart. Defaults to `strip`. */
  legend?: ChartLegend
  /** Visual density. Defaults to `comfortable`. */
  size?: ChartSize
  /**
   * Chart geometry. `bar` (default) renders grouped/stacked bars; `area` renders
   * stacked filled areas (resource growth); `line` renders thin lines (feed
   * activity). All three share the same axes, tooltip, legend, and range control.
   */
  chartType?: 'bar' | 'area' | 'line'
}

interface ChartCanvasProps {
  chartType: 'bar' | 'area' | 'line'
  chartData: (TimeSeriesDatum & { label: string })[]
  visibleSeries: readonly ChartSeries[]
  compact: boolean
  stacked: boolean
}

/**
 * The recharts canvas: bar, stacked area, or line geometry, all sharing the same
 * grid/axes/tooltip. Extracted so the three geometries live in one place and the
 * shell component stays small.
 */
function ChartCanvas({
  chartType,
  chartData,
  visibleSeries,
  compact,
  stacked,
}: ChartCanvasProps) {
  const axes = (
    <>
      <CartesianGrid
        strokeDasharray="3 3"
        stroke="var(--border)"
        vertical={false}
      />
      {!compact && (
        <XAxis
          dataKey="label"
          fontSize={12}
          stroke="var(--muted-foreground)"
          tickLine={false}
          axisLine={false}
        />
      )}
      {!compact && (
        <YAxis
          fontSize={12}
          stroke="var(--muted-foreground)"
          tickLine={false}
          axisLine={false}
          allowDecimals={false}
        />
      )}
      <Tooltip
        cursor={{ fill: 'var(--muted)' }}
        content={<ChartTooltip series={visibleSeries} />}
      />
    </>
  )
  const margin = { top: 8, right: 8, left: 0, bottom: 0 }

  if (chartType === 'area') {
    return (
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={chartData} margin={margin}>
          {axes}
          {visibleSeries.map(s => (
            <Area
              key={s.key}
              type="monotone"
              dataKey={s.key}
              stackId={stacked ? 'a' : undefined}
              stroke={s.fill}
              fill={s.fill}
              fillOpacity={0.2}
              strokeWidth={2}
            />
          ))}
        </AreaChart>
      </ResponsiveContainer>
    )
  }
  if (chartType === 'line') {
    return (
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={chartData} margin={margin}>
          {axes}
          {visibleSeries.map(s => (
            <Line
              key={s.key}
              type="monotone"
              dataKey={s.key}
              stroke={s.fill}
              strokeWidth={2}
              dot={false}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    )
  }
  return (
    <ResponsiveContainer width="100%" height="100%">
      <BarChart data={chartData} margin={margin}>
        {axes}
        {visibleSeries.map((s, index) => (
          <Bar
            key={s.key}
            dataKey={s.key}
            stackId={stacked ? 'a' : undefined}
            fill={s.fill}
            radius={
              stacked && index !== visibleSeries.length - 1
                ? [0, 0, 0, 0]
                : [4, 4, 0, 0]
            }
          />
        ))}
      </BarChart>
    </ResponsiveContainer>
  )
}

/**
 * Presentational timeseries chart: a card with a range selector, a recharts
 * chart, and an optional legend rendered below it. It holds no data-fetching
 * logic — the caller supplies `data`, `total`, `loading`, and `error`, and
 * configures the series, labels, stacking, legend, and density. This is the
 * single shared shell behind every bar chart in the app (Access activity via
 * AccessActivityPanel, Resources created, the team charts, Session activity).
 */
export function TimeSeriesBarChart({
  title,
  totalLabel,
  total,
  series,
  data,
  range,
  onRangeChange,
  hideRangeControl = false,
  loading,
  error,
  errorMessage,
  emptyMessage,
  stacked = false,
  legend = 'strip',
  size = 'comfortable',
  chartType = 'bar',
}: TimeSeriesBarChartProps) {
  const [hiddenKeys, setHiddenKeys] = useState<Set<string>>(new Set())

  const toggleSeries = (key: string) => {
    setHiddenKeys(prev => {
      const next = new Set(prev)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }

  const compact = size === 'compact'

  // Only the interactive strip legend can hide series; the breakdown and none
  // variants always render every series.
  const visibleSeries =
    legend === 'strip' ? series.filter(s => !hiddenKeys.has(s.key)) : series

  const chartData = useMemo(
    () =>
      data.map(item => ({
        ...item,
        label: parseLocalDate(item.date).toLocaleDateString('en-US', {
          month: 'short',
          day: 'numeric',
        }),
      })),
    [data]
  )

  // Per-series totals for the breakdown legend, sorted high→low with each bar
  // scaled relative to the largest series.
  const breakdown = useMemo(() => {
    const totals = series.map(s => ({
      ...s,
      count: data.reduce((sum, d) => sum + (Number(d[s.key]) || 0), 0),
    }))
    const max = Math.max(1, ...totals.map(t => t.count))
    return totals
      .slice()
      .sort((a, b) => b.count - a.count)
      .map(t => ({ ...t, pct: (t.count / max) * 100 }))
  }, [series, data])

  const isEmpty = chartData.length === 0 || total === 0
  const stateBoxHeight = compact ? 'h-24' : 'h-16'

  return (
    <Card
      className={compact ? 'overflow-hidden' : undefined}
      data-testid="timeseries-bar-chart"
    >
      <CardHeader
        className={cn(
          'flex flex-row items-start justify-between gap-4',
          compact && 'p-5 pb-0'
        )}
      >
        <div>
          <PanelTitle>{title}</PanelTitle>
          <div
            className={cn(
              'text-muted-foreground flex items-center gap-1.5',
              compact ? 'mt-[3px] text-xs' : 'mt-1 text-sm'
            )}
          >
            <span>{totalLabel}:</span>
            {loading ? (
              <Skeleton className="h-4 w-10" />
            ) : (
              <span
                className={
                  compact
                    ? 'text-foreground font-semibold tabular-nums'
                    : 'font-medium'
                }
              >
                {total}
              </span>
            )}
          </div>
        </div>
        {!hideRangeControl && (
          <Select value={range} onValueChange={onRangeChange}>
            <SelectTrigger
              className={
                compact ? 'h-[30px] w-auto gap-1.5 px-2.5 text-xs' : 'w-[160px]'
              }
              aria-label="Select time range"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TIME_SERIES_RANGE_OPTIONS.map(option => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </CardHeader>
      <CardContent className={compact ? 'p-5 pt-4' : undefined}>
        {loading ? (
          <Skeleton
            className={compact ? 'h-[110px] w-full' : 'h-[180px] w-full'}
          />
        ) : error ? (
          <div
            className={cn(
              'text-muted-foreground flex items-center justify-center text-sm',
              stateBoxHeight
            )}
          >
            {errorMessage}
          </div>
        ) : isEmpty ? (
          <div
            className={cn(
              'text-muted-foreground flex items-center justify-center text-sm',
              stateBoxHeight
            )}
          >
            {emptyMessage}
          </div>
        ) : (
          <>
            <div className={compact ? 'h-[110px] w-full' : 'h-[180px] w-full'}>
              <ChartCanvas
                chartType={chartType}
                chartData={chartData}
                visibleSeries={visibleSeries}
                compact={compact}
                stacked={stacked}
              />
            </div>

            {/* Compact charts hide the axes, so surface first/last date ticks. */}
            {compact && (
              <div className="text-muted-foreground mt-2 flex justify-between text-xs tabular-nums">
                <span>{chartData[0].label}</span>
                <span>{chartData[chartData.length - 1].label}</span>
              </div>
            )}

            {legend === 'strip' && (
              <div className="mt-3 flex flex-wrap gap-3">
                {series.map(s => {
                  const isHidden = hiddenKeys.has(s.key)
                  return (
                    <button
                      key={s.key}
                      type="button"
                      onClick={() => {
                        toggleSeries(s.key)
                      }}
                      aria-pressed={!isHidden}
                      className={cn(
                        'flex items-center gap-1.5 text-xs transition-opacity',
                        isHidden && 'opacity-40'
                      )}
                    >
                      <span
                        className="h-2.5 w-2.5 rounded-[2px]"
                        style={{ backgroundColor: s.fill }}
                      />
                      <span>{s.label}</span>
                    </button>
                  )
                })}
              </div>
            )}

            {legend === 'breakdown' && (
              <ul className="border-border mt-4 border-t">
                {breakdown.map(s => (
                  <li
                    key={s.key}
                    className="border-border flex items-center gap-2.5 border-b py-2 last:border-b-0"
                  >
                    <span
                      aria-hidden
                      className="size-[9px] shrink-0 rounded-[2px]"
                      style={{ background: s.fill }}
                    />
                    <span className="flex-1 text-sm">{s.label}</span>
                    <span
                      aria-hidden
                      className="bg-secondary h-[5px] w-[84px] overflow-hidden rounded-full"
                    >
                      <span
                        className="block h-full rounded-full"
                        style={{
                          width: `${s.pct.toFixed(1)}%`,
                          background: s.fill,
                        }}
                      />
                    </span>
                    <span className="w-7 text-right text-sm font-semibold tabular-nums">
                      {s.count}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </>
        )}
      </CardContent>
    </Card>
  )
}
