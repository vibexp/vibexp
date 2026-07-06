import { useEffect, useState } from 'react'

import type { TeamCreationCountByDate } from '@/services/teamService'
import { teamService } from '@/services/teamService'
import type { TeamStats } from '@/types/team'

import {
  type ChartSeries,
  TimeSeriesBarChart,
  type TimeSeriesDatum,
} from './TimeSeriesBarChart'

// Order + colours mirror the design mock (Artifacts, Memories, Blueprints,
// Prompts → chart-1..4); Projects are omitted to match the four-type legend.
const CUMULATIVE_SERIES: readonly ChartSeries[] = [
  { key: 'artifacts', label: 'Artifacts', fill: 'var(--chart-1)' },
  { key: 'memories', label: 'Memories', fill: 'var(--chart-2)' },
  { key: 'blueprints', label: 'Blueprints', fill: 'var(--chart-3)' },
  { key: 'prompts', label: 'Prompts', fill: 'var(--chart-4)' },
]

const SERIES_KEYS = ['artifacts', 'memories', 'blueprints', 'prompts'] as const
type SeriesKey = (typeof SERIES_KEYS)[number]

/** Current per-type totals (all-time) pulled from team stats, the anchor for the
 * cumulative series so growth is measured against the real workspace size. */
export function totalsFromStats(stats: TeamStats): Record<SeriesKey, number> {
  return {
    artifacts: stats.total_artifacts,
    memories: stats.total_memories,
    blueprints: stats.total_blueprints,
    prompts: stats.total_prompts,
  }
}

/**
 * Walk the daily creation series backwards from today's known totals to derive
 * the cumulative count of each resource type on every day in the window:
 * `cum[last] = totalNow`, and `cum[d] = cum[d+1] − created on (d+1)`. This needs
 * no extra endpoint — it reconstructs history from the current totals plus the
 * per-day creations. Counts are clamped at zero to stay robust against any
 * stats/creation skew.
 */
export function buildCumulativeSeries(
  counts: TeamCreationCountByDate[],
  totalsNow: Record<SeriesKey, number>
): TimeSeriesDatum[] {
  const running: Record<SeriesKey, number> = { ...totalsNow }
  const reversed: TimeSeriesDatum[] = []
  for (let i = counts.length - 1; i >= 0; i--) {
    const datum: TimeSeriesDatum = { date: counts[i].date, total: 0 }
    let dayTotal = 0
    for (const key of SERIES_KEYS) {
      const value = Math.max(0, running[key])
      datum[key] = value
      dayTotal += value
    }
    datum.total = dayTotal
    reversed.push(datum)
    // Step to the previous day by removing this day's creations.
    for (const key of SERIES_KEYS) {
      running[key] -= counts[i][key]
    }
  }
  return reversed.reverse()
}

interface TeamResourceCumulativeChartProps {
  teamId: string
  /** Range is owned by the page-level filter; the chart hides its own selector. */
  range: string
}

/**
 * Team-wide cumulative "growth" chart: the total number of each resource type
 * that existed on each day across the selected window, stacked so the bar height
 * traces overall workspace growth. Derived entirely client-side from the
 * resource-creation series plus current team totals (no dedicated endpoint).
 * Re-fetches whenever the team or range changes; aborts in-flight work on cleanup.
 */
export function TeamResourceCumulativeChart({
  teamId,
  range,
}: TeamResourceCumulativeChartProps) {
  const [data, setData] = useState<TimeSeriesDatum[]>([])
  const [grandTotal, setGrandTotal] = useState<number>(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    const fetchData = async () => {
      setLoading(true)
      try {
        const [metrics, stats] = await Promise.all([
          teamService.getTeamResourceCreationMetrics(
            teamId,
            range,
            controller.signal
          ),
          teamService.getTeamStats(teamId),
        ])
        if (controller.signal.aborted) return
        const totalsNow = totalsFromStats(stats)
        setData(buildCumulativeSeries(metrics.data.counts, totalsNow))
        setGrandTotal(SERIES_KEYS.reduce((sum, key) => sum + totalsNow[key], 0))
        setError(false)
      } catch (err) {
        if (controller.signal.aborted) return
        console.error('Failed to fetch cumulative resource metrics:', err)
        setData([])
        setGrandTotal(0)
        setError(true)
      } finally {
        if (!controller.signal.aborted) setLoading(false)
      }
    }
    void fetchData()
    return () => {
      controller.abort()
    }
  }, [teamId, range])

  return (
    <TimeSeriesBarChart
      title="Total resources by type"
      totalLabel="Total resources"
      total={grandTotal}
      series={CUMULATIVE_SERIES}
      data={data}
      range={range}
      hideRangeControl
      loading={loading}
      error={error}
      errorMessage="Couldn't load resource totals"
      emptyMessage="No resources yet"
      stacked
      chartType="area"
      legend="strip"
    />
  )
}
