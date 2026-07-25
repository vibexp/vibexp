import { useEffect, useMemo, useState } from 'react'

import type { ChartSeries } from '@/components/TimeSeriesBarChart'
import { TimeSeriesBarChart } from '@/components/TimeSeriesBarChart'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import type { DateRangeValue } from '@/components/ui/date-range'
import { rangeToInstants } from '@/components/ui/date-range'
import { BreakdownPanels } from '@/pages/admin/dashboard/BreakdownPanels'
import {
  accessToChartData,
  countsToChartData,
  GROWTH_KEYS,
  growthToChartData,
  sumTotals,
} from '@/pages/admin/dashboard/buckets'
import type { Granularity } from '@/pages/admin/dashboard/DashboardControls'
import { DashboardControls } from '@/pages/admin/dashboard/DashboardControls'
import { DataWindowNote } from '@/pages/admin/dashboard/DataWindowNote'
import { StatCards } from '@/pages/admin/dashboard/StatCards'
import { SystemHealthPanel } from '@/pages/admin/dashboard/SystemHealthPanel'
import type {
  AdminDashboardOverview,
  AdminTimeseriesResponse,
} from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

/** Chart colours from the design tokens, so both themes work without overrides. */
const GROWTH_SERIES: ChartSeries[] = [
  { key: 'users', label: 'Users', fill: 'var(--chart-1)' },
  { key: 'teams', label: 'Teams', fill: 'var(--chart-2)' },
  { key: 'projects', label: 'Projects', fill: 'var(--chart-3)' },
  { key: 'prompts', label: 'Prompts', fill: 'var(--chart-4)' },
  { key: 'artifacts', label: 'Artifacts', fill: 'var(--chart-5)' },
  { key: 'memories', label: 'Memories', fill: 'var(--chart-1)' },
]

const SIGN_IN_SERIES: ChartSeries[] = [
  { key: 'count', label: 'Sign-ins', fill: 'var(--chart-2)' },
]

const SOURCE_FILLS = [
  'var(--chart-1)',
  'var(--chart-2)',
  'var(--chart-3)',
  'var(--chart-4)',
  'var(--chart-5)',
]

function Section({
  title,
  description,
  children,
}: Readonly<{
  title: string
  description?: string
  children: React.ReactNode
}>) {
  return (
    <section className="space-y-3">
      <div>
        <h2 className="text-sm font-semibold">{title}</h2>
        {description && (
          <p className="text-muted-foreground text-xs">{description}</p>
        )}
      </div>
      {children}
    </section>
  )
}

/**
 * The instance dashboard — totals, breakdowns, growth, activity and system health
 * from #451's two endpoints.
 *
 * Two independent fetches: the overview fires once, the time series refires when
 * the range or granularity changes. They also fail independently, so a broken
 * time-series request still leaves an admin with the totals and health data rather
 * than a blank page.
 */
export function AdminDashboard() {
  const [range, setRange] = useState<DateRangeValue>({})
  const [granularity, setGranularity] = useState<Granularity>('day')

  const [overview, setOverview] = useState<AdminDashboardOverview | null>(null)
  const [overviewLoading, setOverviewLoading] = useState(true)
  const [overviewError, setOverviewError] = useState<string | null>(null)

  const [series, setSeries] = useState<AdminTimeseriesResponse | null>(null)
  const [seriesLoading, setSeriesLoading] = useState(true)
  const [seriesError, setSeriesError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    adminService
      .getDashboardOverview()
      .then(result => {
        if (!cancelled) setOverview(result)
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setOverviewError(
            getErrorMessage(err, 'Failed to load instance totals')
          )
        }
      })
      .finally(() => {
        if (!cancelled) setOverviewLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  // The range is empty until an admin picks one, in which case the API applies
  // its own default (last 30 days) rather than the page inventing one.
  const { from, to } = useMemo(() => rangeToInstants(range), [range])

  useEffect(() => {
    let cancelled = false
    setSeriesLoading(true)
    setSeriesError(null)
    adminService
      .getDashboardTimeseries({ from, to, granularity })
      .then(result => {
        if (!cancelled) setSeries(result)
      })
      .catch((err: unknown) => {
        if (cancelled) return
        // Surfaces the API's own 400 text ("to is not after from", "range exceeds
        // the maximum span") instead of a blank chart area.
        setSeriesError(getErrorMessage(err, 'Failed to load the time series'))
        setSeries(null)
      })
      .finally(() => {
        if (!cancelled) setSeriesLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [from, to, granularity])

  const growthData = useMemo(
    () => growthToChartData(series?.growth ?? []),
    [series]
  )
  const signInData = useMemo(
    () => countsToChartData(series?.sign_ins ?? []),
    [series]
  )
  const access = useMemo(
    () => accessToChartData(series?.access_by_source ?? []),
    [series]
  )
  const accessSeries = useMemo<ChartSeries[]>(
    () =>
      access.sources.map((source, index) => ({
        key: source,
        label: source,
        fill: SOURCE_FILLS[index % SOURCE_FILLS.length],
      })),
    [access.sources]
  )

  const bucketNoun =
    granularity === 'day' ? 'day' : granularity === 'week' ? 'week' : 'month'

  return (
    <div className="space-y-8">
      <DashboardControls
        range={range}
        onRangeChange={setRange}
        granularity={granularity}
        onGranularityChange={setGranularity}
      />

      {overviewError && (
        <Alert variant="destructive">
          <AlertTitle>Failed to load instance totals</AlertTitle>
          <AlertDescription>{overviewError}</AlertDescription>
        </Alert>
      )}

      <Section title="Totals">
        <StatCards
          counts={overview?.counts ?? null}
          loading={overviewLoading}
        />
      </Section>

      <Section
        title="Breakdowns"
        description="Every entity with a status or type column, grouped by it."
      >
        <BreakdownPanels
          breakdowns={overview?.breakdowns ?? null}
          loading={overviewLoading}
        />
      </Section>

      <Section title="Growth" description={`New entities per ${bucketNoun}.`}>
        <TimeSeriesBarChart
          title="New entities"
          totalLabel="Total created"
          total={sumTotals(growthData)}
          series={GROWTH_SERIES}
          data={growthData}
          range=""
          hideRangeControl
          loading={seriesLoading}
          error={seriesError !== null}
          errorMessage={seriesError ?? ''}
          emptyMessage="Nothing was created in this range."
          chartType="area"
          stacked
        />
      </Section>

      <Section title="Activity">
        <div className="space-y-4">
          <div className="space-y-2">
            <TimeSeriesBarChart
              title="Sign-ins"
              totalLabel="Total sign-ins"
              total={sumTotals(signInData)}
              series={SIGN_IN_SERIES}
              data={signInData}
              range=""
              hideRangeControl
              loading={seriesLoading}
              error={seriesError !== null}
              errorMessage={seriesError ?? ''}
              emptyMessage="No sign-ins in this range."
              legend="none"
            />
            {series && (
              <DataWindowNote
                earliestRetainedAt={
                  series.data_window.sign_ins_earliest_retained_at
                }
                label="Sign-ins"
              />
            )}
          </div>

          <div className="space-y-2">
            <TimeSeriesBarChart
              title="Resource access by source"
              totalLabel="Total accesses"
              total={sumTotals(access.data)}
              series={accessSeries}
              data={access.data}
              range=""
              hideRangeControl
              loading={seriesLoading}
              error={seriesError !== null}
              errorMessage={seriesError ?? ''}
              emptyMessage="No resource access recorded in this range."
              legend="breakdown"
              stacked
            />
            {series && (
              <DataWindowNote
                earliestRetainedAt={
                  series.data_window.access_by_source_earliest_retained_at
                }
                label="Access events"
              />
            )}
          </div>
        </div>
      </Section>

      <Section title="System health">
        <SystemHealthPanel
          health={overview?.system_health ?? null}
          version={overview?.version ?? 'unknown'}
          loading={overviewLoading}
        />
      </Section>
    </div>
  )
}

// Re-exported so the growth chart's series list and the mapper cannot drift apart.
export { GROWTH_KEYS }
