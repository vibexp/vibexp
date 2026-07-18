import { useEffect, useState } from 'react'

import {
  type ChartSeries,
  TimeSeriesBarChart,
} from '@/components/TimeSeriesBarChart'
import type {
  AccessCountByDate,
  ResourceAccessType,
} from '@/services/resourceAccessService'
import { resourceAccessService } from '@/services/resourceAccessService'

interface AccessActivityPanelProps {
  teamId: string
  resourceType: ResourceAccessType
  resourceId: string
}

/** Access channels in stack order (web at the bottom), each mapped to a token. */
const ACCESS_CHANNELS: readonly ChartSeries[] = [
  { key: 'web', label: 'Web', fill: 'var(--chart-1)' },
  { key: 'cli', label: 'CLI', fill: 'var(--chart-2)' },
  { key: 'mcp', label: 'MCP', fill: 'var(--chart-3)' },
  { key: 'api', label: 'API', fill: 'var(--chart-4)' },
]

/**
 * Self-fetching Access activity widget. Loads per-resource access metrics
 * (web/cli/mcp/api over a time range) and renders the shared TimeSeriesBarChart
 * in its compact density with a per-channel breakdown legend. Re-fetches
 * whenever the target resource or selected range changes; aborts in-flight
 * requests on cleanup. Drop-in across resource detail pages — the
 * access-activity counterpart to MetadataPanel.
 */
export function AccessActivityPanel({
  teamId,
  resourceType,
  resourceId,
}: Readonly<AccessActivityPanelProps>) {
  const [data, setData] = useState<AccessCountByDate[]>([])
  const [totalAccesses, setTotalAccesses] = useState(0)
  const [range, setRange] = useState('30d')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    const fetchData = async () => {
      setLoading(true)
      try {
        const response = await resourceAccessService.getResourceAccessMetrics(
          teamId,
          resourceType,
          resourceId,
          range,
          controller.signal
        )
        setData(response.data.counts)
        setTotalAccesses(response.data.total_accesses || 0)
        setError(false)
      } catch (err) {
        // An abort is expected on cleanup (props/range changed or unmount);
        // it must not surface as an error or wipe the current data.
        if (controller.signal.aborted) return
        console.error('Failed to fetch resource access metrics:', err)
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
  }, [teamId, resourceType, resourceId, range])

  return (
    <TimeSeriesBarChart
      title="Access activity"
      totalLabel="Total accesses"
      total={totalAccesses}
      series={ACCESS_CHANNELS}
      data={data}
      range={range}
      onRangeChange={setRange}
      loading={loading}
      error={error}
      errorMessage="Couldn't load access activity"
      emptyMessage="No activity yet"
      stacked
      legend="breakdown"
      size="compact"
    />
  )
}
