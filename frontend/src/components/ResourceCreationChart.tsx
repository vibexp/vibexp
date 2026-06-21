import { useEffect, useState } from 'react'

import { resourceCreationService } from '@/services/resourceCreationService'
import type { CreationCountByDate } from '@/types'

import { type ChartSeries, TimeSeriesBarChart } from './TimeSeriesBarChart'

const CREATION_SERIES: readonly ChartSeries[] = [
  { key: 'prompts', label: 'Prompts', fill: 'var(--chart-1)' },
  { key: 'artifacts', label: 'Artifacts', fill: 'var(--chart-2)' },
  { key: 'blueprints', label: 'Blueprints', fill: 'var(--chart-3)' },
  { key: 'memories', label: 'Memories', fill: 'var(--chart-4)' },
]

interface ResourceCreationChartProps {
  teamId: string
  slug: string
}

/**
 * Self-fetching, compact multi-series bar chart of how many prompts, artifacts,
 * blueprints, and memories were created in a project over time. Grouped (not
 * stacked) with a clickable legend so each series can be isolated — the resource
 * types have heterogeneous scales. Renders via the shared TimeSeriesBarChart
 * shell; re-fetches whenever the project or selected range changes.
 */
export function ResourceCreationChart({
  teamId,
  slug,
}: ResourceCreationChartProps) {
  const [data, setData] = useState<CreationCountByDate[]>([])
  const [totalCreated, setTotalCreated] = useState<number>(0)
  const [range, setRange] = useState('30d')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    const fetchData = async () => {
      setLoading(true)
      try {
        const response =
          await resourceCreationService.getResourceCreationMetrics(
            teamId,
            slug,
            range,
            controller.signal
          )
        setData(response.data.counts)
        setTotalCreated(response.data.total_created || 0)
        setError(false)
      } catch (err) {
        // An abort is expected on cleanup (props/range changed or unmount);
        // it must not surface as an error or wipe the current data.
        if (controller.signal.aborted) return
        console.error('Failed to fetch resource creation metrics:', err)
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
  }, [teamId, slug, range])

  return (
    <TimeSeriesBarChart
      title="Resources created"
      totalLabel="Total created"
      total={totalCreated}
      series={CREATION_SERIES}
      data={data}
      range={range}
      onRangeChange={setRange}
      loading={loading}
      error={error}
      errorMessage="Couldn't load resource creation"
      emptyMessage="No resources created yet"
      legend="strip"
      size="compact"
    />
  )
}
