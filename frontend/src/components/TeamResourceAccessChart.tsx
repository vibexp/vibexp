import { useEffect, useState } from 'react'

import type { AccessCountByDate } from '@/services/resourceAccessService'
import { teamService } from '@/services/teamService'

import { CHANNEL_FILL, CHANNEL_LABEL } from './channelSeries'
import { type ChartSeries, TimeSeriesBarChart } from './TimeSeriesBarChart'

const SOURCE_SERIES: readonly ChartSeries[] = (
  ['web', 'cli', 'mcp', 'api'] as const
).map(key => ({ key, label: CHANNEL_LABEL[key], fill: CHANNEL_FILL[key] }))

interface TeamResourceAccessChartProps {
  teamId: string
  /** Range is owned by the page-level filter; the chart hides its own selector. */
  range: string
}

/**
 * Team-wide stacked bar chart of access activity broken down by source
 * (web/cli/mcp/api), aggregated across every resource in the team. The range is
 * controlled by the page (a single filter drives all charts), so the in-card
 * selector is hidden; re-fetches whenever the team or range changes.
 */
export function TeamResourceAccessChart({
  teamId,
  range,
}: Readonly<TeamResourceAccessChartProps>) {
  const [data, setData] = useState<AccessCountByDate[]>([])
  const [totalAccesses, setTotalAccesses] = useState<number>(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    const fetchData = async () => {
      setLoading(true)
      try {
        const response = await teamService.getTeamResourceAccessMetrics(
          teamId,
          range,
          controller.signal
        )
        setData(response.data.counts)
        setTotalAccesses(response.data.total_accesses || 0)
        setError(false)
      } catch (err) {
        // An abort is expected on cleanup (team/range changed or unmount); it
        // must not surface as an error or wipe the current data.
        if (controller.signal.aborted) return
        console.error('Failed to fetch team resource access metrics:', err)
        setData([])
        setTotalAccesses(0)
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
      title="Access activity"
      totalLabel="Total accesses"
      total={totalAccesses}
      series={SOURCE_SERIES}
      data={data}
      range={range}
      hideRangeControl
      loading={loading}
      error={error}
      errorMessage="Couldn't load access activity"
      emptyMessage="No activity yet"
      stacked
      legend="strip"
    />
  )
}
