import { useEffect, useState } from 'react'

import { teamService } from '@/services/teamService'
import type { TeamFeedCreationCountByDate } from '@/types'

import { type ChartSeries, TimeSeriesBarChart } from './TimeSeriesBarChart'

// The chart surfaces feed *updates* (feed items) — the AI activity users care
// about. New feed channels are rare and would flatten the scale, so they are
// omitted from the series and the header total.
const FEED_SERIES: readonly ChartSeries[] = [
  { key: 'feed_items', label: 'Feed updates', fill: 'var(--chart-3)' },
]

interface TeamFeedCreationChartProps {
  teamId: string
  /** Range is owned by the page-level filter; the chart hides its own selector. */
  range: string
}

/**
 * Team-wide bar chart of AI feed activity — how many feed updates (items) were
 * posted over time. The range is controlled by the page (a single filter drives
 * all charts), so the in-card selector is hidden; re-fetches whenever the team
 * or range changes and aborts in-flight requests on cleanup.
 */
export function TeamFeedCreationChart({
  teamId,
  range,
}: TeamFeedCreationChartProps) {
  const [data, setData] = useState<TeamFeedCreationCountByDate[]>([])
  const [totalUpdates, setTotalUpdates] = useState<number>(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    const fetchData = async () => {
      setLoading(true)
      try {
        const response = await teamService.getTeamFeedCreationMetrics(
          teamId,
          range,
          controller.signal
        )
        const counts = response.data.counts
        setData(counts)
        // Header total mirrors the rendered series (feed items only), not the
        // backend's total_created which also counts new feed channels.
        setTotalUpdates(counts.reduce((sum, c) => sum + c.feed_items, 0))
        setError(false)
      } catch (err) {
        // An abort is expected on cleanup (team/range changed or unmount); it
        // must not surface as an error or wipe the current data.
        if (controller.signal.aborted) return
        console.error('Failed to fetch team feed creation metrics:', err)
        setData([])
        setTotalUpdates(0)
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
      title="AI feeds created"
      totalLabel="Feed updates"
      total={totalUpdates}
      series={FEED_SERIES}
      data={data}
      range={range}
      hideRangeControl
      loading={loading}
      error={error}
      errorMessage="Couldn't load feed activity"
      emptyMessage="No feed activity yet"
      chartType="area"
      legend="none"
    />
  )
}
