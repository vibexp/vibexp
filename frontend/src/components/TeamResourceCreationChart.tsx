import { useEffect, useState } from 'react'

import type { TeamCreationCountByDate } from '@/services/teamService'
import { teamService } from '@/services/teamService'

import { type ChartSeries, TimeSeriesBarChart } from './TimeSeriesBarChart'

// Order + colours mirror the design mock: Artifacts, Memories, Blueprints,
// Prompts mapped to chart-1..4. Projects are intentionally omitted to match.
const CREATION_SERIES: readonly ChartSeries[] = [
  { key: 'artifacts', label: 'Artifacts', fill: 'var(--chart-1)' },
  { key: 'memories', label: 'Memories', fill: 'var(--chart-2)' },
  { key: 'blueprints', label: 'Blueprints', fill: 'var(--chart-3)' },
  { key: 'prompts', label: 'Prompts', fill: 'var(--chart-4)' },
]

interface TeamResourceCreationChartProps {
  teamId: string
  /** Range is owned by the page-level filter; the chart hides its own selector. */
  range: string
}

/**
 * Team-wide multi-series bar chart of how many prompts, artifacts, blueprints,
 * memories, and projects were created over time. Grouped (not stacked) with a
 * clickable legend so each series can be isolated. The range is controlled by the
 * page (a single filter drives all charts), so the in-card selector is hidden;
 * re-fetches whenever the team or range changes.
 */
export function TeamResourceCreationChart({
  teamId,
  range,
}: Readonly<TeamResourceCreationChartProps>) {
  const [data, setData] = useState<TeamCreationCountByDate[]>([])
  const [totalCreated, setTotalCreated] = useState<number>(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    const fetchData = async () => {
      setLoading(true)
      try {
        const response = await teamService.getTeamResourceCreationMetrics(
          teamId,
          range,
          controller.signal
        )
        setData(response.data.counts)
        setTotalCreated(response.data.total_created || 0)
        setError(false)
      } catch (err) {
        // An abort is expected on cleanup (team/range changed or unmount); it
        // must not surface as an error or wipe the current data.
        if (controller.signal.aborted) return
        console.error('Failed to fetch team resource creation metrics:', err)
        setData([])
        setTotalCreated(0)
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
      title="Resources created"
      totalLabel="Total created"
      total={totalCreated}
      series={CREATION_SERIES}
      data={data}
      range={range}
      hideRangeControl
      loading={loading}
      error={error}
      errorMessage="Couldn't load resource creation"
      emptyMessage="No resources created yet"
      stacked
      legend="strip"
    />
  )
}
