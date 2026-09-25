import { useEffect, useMemo, useState } from 'react'

import { CategoryBreakdownChart } from '@/components/CategoryBreakdownChart'
import type { ChartSeries } from '@/components/TimeSeriesBarChart'
import { TimeSeriesBarChart } from '@/components/TimeSeriesBarChart'
import type { DateRangeValue } from '@/components/ui/date-range'
import { rangeToInstants } from '@/components/ui/date-range'
import { accessToChartData, sumTotals } from '@/pages/admin/dashboard/buckets'
import type { Granularity } from '@/pages/admin/dashboard/DashboardControls'
import { DashboardControls } from '@/pages/admin/dashboard/DashboardControls'
import { DataWindowNote } from '@/pages/admin/dashboard/DataWindowNote'
import { Section } from '@/pages/admin/dashboard/Section'
import type { Slot } from '@/pages/admin/detail/slot'
import { LOADING, settle } from '@/pages/admin/detail/slot'
import { TopAccessedTable } from '@/pages/admin/detail/TopAccessedTable'
import {
  countsToBreakdown,
  PROJECT_RESOURCE_TYPE_SERIES,
  projectCreationToChartData,
} from '@/pages/admin/projects/detail/projectChartData'
import {
  CHART_FILLS,
  sumCounts,
} from '@/pages/admin/users/detail/userInsightsChartData'
import type {
  AdminProjectAccessMetrics,
  AdminProjectDetail,
  AdminProjectResourceCreationMetrics,
  AdminTopAccessedResource,
} from '@/services/adminService'
import { adminService } from '@/services/adminService'

/**
 * The project's activity (#1146): a breakdown by type from the counts the page
 * already loaded, then the creation series, the access series and the
 * most-accessed list, all three driven by one range + bucket-size control so
 * they always describe the same window.
 */
export function ProjectOverviewTab({
  projectId,
  resourceCounts,
}: Readonly<{
  projectId: string
  resourceCounts: AdminProjectDetail['resource_counts']
}>) {
  const [range, setRange] = useState<DateRangeValue>({})
  const [granularity, setGranularity] = useState<Granularity>('day')

  const [creation, setCreation] =
    useState<Slot<AdminProjectResourceCreationMetrics>>(LOADING)
  const [access, setAccess] = useState<Slot<AdminProjectAccessMetrics>>(LOADING)
  const [top, setTop] = useState<Slot<AdminTopAccessedResource[]>>(LOADING)

  // An empty range sends no bounds and lets the API apply its own default.
  const { from, to } = useMemo(() => rangeToInstants(range), [range])

  useEffect(() => {
    let cancelled = false
    const isCancelled = () => cancelled
    setCreation(LOADING)
    setAccess(LOADING)
    setTop(LOADING)

    settle(
      adminService.getProjectResourceCreationMetrics(projectId, {
        from,
        to,
        granularity,
      }),
      setCreation,
      'Failed to load the creation series',
      isCancelled
    )
    settle(
      adminService.getProjectResourceAccessMetrics(projectId, {
        from,
        to,
        granularity,
      }),
      setAccess,
      'Failed to load the access series',
      isCancelled
    )
    settle(
      adminService
        .getProjectTopAccessedResources(projectId, { from, to })
        .then(response => response.items),
      setTop,
      'Failed to load the most accessed resources',
      isCancelled
    )

    return () => {
      cancelled = true
    }
  }, [projectId, from, to, granularity])

  const byType = useMemo(
    () => countsToBreakdown(resourceCounts),
    [resourceCounts]
  )
  const creationData = useMemo(
    () => projectCreationToChartData(creation.data?.series ?? []),
    [creation.data]
  )
  const accessChart = useMemo(
    () => accessToChartData(access.data?.access_by_source ?? []),
    [access.data]
  )
  const accessSeries = useMemo<ChartSeries[]>(
    () =>
      accessChart.sources.map((source, index) => ({
        key: source,
        label: source,
        fill: CHART_FILLS[index % CHART_FILLS.length],
      })),
    [accessChart.sources]
  )

  return (
    <div className="space-y-8">
      <Section title="Breakdown">
        <div className="grid gap-4 lg:grid-cols-3">
          <CategoryBreakdownChart
            title="By type"
            totalLabel="Resources"
            data={byType}
            total={sumCounts(byType)}
            emptyMessage="This project has no resources yet."
            // Fed from the page's already-loaded counts: nothing to wait for.
            loading={false}
            error={false}
            errorMessage=""
          />
        </div>
      </Section>

      <div className="space-y-4">
        <DashboardControls
          range={range}
          onRangeChange={setRange}
          granularity={granularity}
          onGranularityChange={setGranularity}
          ariaLabel="Project activity date range"
        />

        <Section
          title="Created over time"
          description={`Resources created in this project per ${granularity}.`}
        >
          <TimeSeriesBarChart
            title="Resources created"
            totalLabel="Total created"
            total={sumTotals(creationData)}
            series={PROJECT_RESOURCE_TYPE_SERIES}
            data={creationData}
            range=""
            hideRangeControl
            loading={creation.loading}
            error={creation.error !== null}
            errorMessage={creation.error ?? ''}
            emptyMessage="Nothing was created in this range."
            chartType="bar"
            legend="strip"
            stacked
          />
        </Section>

        <Section
          title="Access over time"
          description={`Accesses to this project and its resources per ${granularity}, by source.`}
        >
          <TimeSeriesBarChart
            title="Resource access by source"
            totalLabel="Total accesses"
            total={sumTotals(accessChart.data)}
            series={accessSeries}
            data={accessChart.data}
            range=""
            hideRangeControl
            loading={access.loading}
            error={access.error !== null}
            errorMessage={access.error ?? ''}
            emptyMessage="No access recorded in this range."
            legend="breakdown"
            stacked
          />
          {access.data && (
            <DataWindowNote
              earliestRetainedAt={access.data.earliest_retained_at}
              label="Access events"
            />
          )}
        </Section>

        <Section title="Most accessed resources">
          <TopAccessedTable slot={top} showLocation={false} />
        </Section>
      </div>
    </div>
  )
}
