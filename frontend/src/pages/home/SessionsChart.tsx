import { useEffect, useState } from 'react'

import {
  type ChartSeries,
  TimeSeriesBarChart,
  type TimeSeriesDatum,
} from '@/components/TimeSeriesBarChart'
import type { SessionCountByDate } from '@/types'
import { apiClient } from '@/utils/api'

/** Single-series sessions-per-day. Uses the primary token (no categorical split). */
const SESSIONS_SERIES: readonly ChartSeries[] = [
  { key: 'count', label: 'Sessions', fill: 'var(--primary)' },
]

interface SessionsChartProps {
  range: string
  onRangeChange: (range: string) => void
}

/**
 * Self-fetching daily session-count chart. A thin wrapper over the shared
 * TimeSeriesBarChart: maps the API's newest-first counts into ascending
 * TimeSeriesDatum rows and renders a single-series bar chart with no legend.
 * Re-fetches whenever the selected range changes.
 */
export function SessionsChart({ range, onRangeChange }: SessionsChartProps) {
  const [data, setData] = useState<SessionCountByDate[]>([])
  const [totalSessions, setTotalSessions] = useState<number>(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  useEffect(() => {
    const fetchData = async () => {
      setLoading(true)
      try {
        const response = await apiClient.getSessionCounts(range)
        setData(response.data.counts)
        setTotalSessions(response.data.total_sessions || 0)
        setError(false)
      } catch (err) {
        console.error('Failed to fetch session counts:', err)
        setData([])
        setTotalSessions(0)
        setError(true)
      } finally {
        setLoading(false)
      }
    }
    void fetchData()
  }, [range])

  // The API returns newest-first; the chart expects ascending dates. The
  // sessions endpoint serialises its DATE as an RFC3339 timestamp (unlike the
  // YYYY-MM-DD the access/creation endpoints emit), so trim to the date part
  // for parseLocalDate. `total` mirrors `count` so the shared shell's per-day
  // total reads correctly.
  const chartData: TimeSeriesDatum[] = data
    .slice()
    .reverse()
    .map(item => ({
      date: item.date.slice(0, 10),
      count: item.count,
      total: item.count,
    }))

  return (
    <TimeSeriesBarChart
      title="Session activity"
      totalLabel="Total sessions"
      total={totalSessions}
      series={SESSIONS_SERIES}
      data={chartData}
      range={range}
      onRangeChange={onRangeChange}
      loading={loading}
      error={error}
      errorMessage="Couldn't load session activity"
      emptyMessage="No activity yet"
      legend="none"
    />
  )
}
