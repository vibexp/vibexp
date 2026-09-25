import type { LucideIcon } from 'lucide-react'
import {
  BookOpen,
  Bot,
  FileText,
  HardDrive,
  Layers,
  MessageSquare,
  Package,
  Paperclip,
  Radio,
  Rss,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'

import { CategoryBreakdownChart } from '@/components/CategoryBreakdownChart'
import type { ChartSeries } from '@/components/TimeSeriesBarChart'
import { TimeSeriesBarChart } from '@/components/TimeSeriesBarChart'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { DateRangeValue } from '@/components/ui/date-range'
import { rangeToInstants } from '@/components/ui/date-range'
import { Skeleton } from '@/components/ui/skeleton'
import { accessToChartData, sumTotals } from '@/pages/admin/dashboard/buckets'
import type { Granularity } from '@/pages/admin/dashboard/DashboardControls'
import { DashboardControls } from '@/pages/admin/dashboard/DashboardControls'
import { DataWindowNote } from '@/pages/admin/dashboard/DataWindowNote'
import { Section } from '@/pages/admin/dashboard/Section'
import type { Slot } from '@/pages/admin/detail/slot'
import { LOADING, settle } from '@/pages/admin/detail/slot'
import { TopAccessedTable } from '@/pages/admin/detail/TopAccessedTable'
import type { ResourceTypeKey } from '@/pages/admin/users/detail/userInsightsChartData'
import {
  CHART_FILLS,
  creationToChartData,
  insightsToBreakdowns,
  RESOURCE_TYPE_KEYS,
  RESOURCE_TYPE_LABELS,
  RESOURCE_TYPE_SERIES,
  sumCounts,
} from '@/pages/admin/users/detail/userInsightsChartData'
import type {
  AdminResourceCounts,
  AdminTopAccessedResource,
  AdminUserAccessMetrics,
  AdminUserInsights,
  AdminUserResourceCreationMetrics,
} from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

const TYPE_ICONS: Record<ResourceTypeKey, LucideIcon> = {
  prompts: FileText,
  memories: HardDrive,
  artifacts: Package,
  blueprints: BookOpen,
  agents: Bot,
  feeds: Radio,
  feed_items: Rss,
  comments: MessageSquare,
  attachments: Paperclip,
}

function CountCards({
  insights,
  loading,
}: Readonly<{ insights: AdminUserInsights | null; loading: boolean }>) {
  const cards: {
    key: keyof AdminResourceCounts
    label: string
    icon: LucideIcon
  }[] = [
    { key: 'total', label: 'Total', icon: Layers },
    ...RESOURCE_TYPE_KEYS.map(key => ({
      key,
      label: RESOURCE_TYPE_LABELS[key],
      icon: TYPE_ICONS[key],
    })),
  ]
  const grid = 'grid grid-cols-2 gap-4 sm:grid-cols-3 xl:grid-cols-5'

  if (loading) {
    return (
      <div className={grid}>
        {cards.map(card => (
          <Skeleton
            key={card.key}
            data-testid="count-skeleton"
            className="h-24 w-full"
          />
        ))}
      </div>
    )
  }
  if (!insights) return null

  return (
    <div className={grid}>
      {cards.map(({ key, label, icon: Icon }) => (
        <Card key={key} data-testid={`count-${key}`}>
          <CardHeader className="pb-2">
            <CardTitle className="text-muted-foreground flex items-center gap-2 text-xs font-medium">
              <Icon className="size-4" aria-hidden />
              {label}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-semibold tabular-nums">
              {insights.totals[key].toLocaleString()}
            </p>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

/**
 * The user's footprint: counts and breakdowns (range-independent), then the
 * creation and access series plus the top-accessed list, all three driven by
 * one range + bucket-size control so they always describe the same window.
 */
export function UserOverviewTab({ userId }: Readonly<{ userId: string }>) {
  const [range, setRange] = useState<DateRangeValue>({})
  const [granularity, setGranularity] = useState<Granularity>('day')

  const [insights, setInsights] = useState<Slot<AdminUserInsights>>(LOADING)
  const [creation, setCreation] =
    useState<Slot<AdminUserResourceCreationMetrics>>(LOADING)
  const [access, setAccess] = useState<Slot<AdminUserAccessMetrics>>(LOADING)
  const [top, setTop] = useState<Slot<AdminTopAccessedResource[]>>(LOADING)

  useEffect(() => {
    let cancelled = false
    setInsights(LOADING)
    adminService
      .getUserInsights(userId)
      .then(data => {
        if (!cancelled) setInsights({ data, loading: false, error: null })
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setInsights({
            data: null,
            loading: false,
            error: getErrorMessage(err, 'Failed to load resource counts'),
          })
        }
      })
    return () => {
      cancelled = true
    }
  }, [userId])

  // An empty range sends no bounds and lets the API apply its own default.
  const { from, to } = useMemo(() => rangeToInstants(range), [range])

  useEffect(() => {
    let cancelled = false
    setCreation(LOADING)
    setAccess(LOADING)
    setTop(LOADING)

    // Each request settles into its own slot, so one failing panel leaves the
    // other two rendered.
    const isCancelled = () => cancelled
    settle(
      adminService.getUserResourceCreationMetrics(userId, {
        from,
        to,
        granularity,
      }),
      setCreation,
      'Failed to load the creation series',
      isCancelled
    )
    settle(
      adminService.getUserResourceAccessMetrics(userId, {
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
        .getUserTopAccessedResources(userId, { from, to })
        .then(response => response.items),
      setTop,
      'Failed to load the most accessed resources',
      isCancelled
    )

    return () => {
      cancelled = true
    }
  }, [userId, from, to, granularity])

  const breakdowns = useMemo(
    () => (insights.data ? insightsToBreakdowns(insights.data) : null),
    [insights.data]
  )
  const creationData = useMemo(
    () => creationToChartData(creation.data?.series ?? []),
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

  const breakdownProps = {
    loading: insights.loading,
    error: insights.error !== null,
    errorMessage: insights.error ?? '',
  }

  return (
    <div className="space-y-8">
      {insights.error && (
        <Alert variant="destructive">
          <AlertTitle>Failed to load resource counts</AlertTitle>
          <AlertDescription>{insights.error}</AlertDescription>
        </Alert>
      )}

      <Section title="Resource counts">
        <CountCards insights={insights.data} loading={insights.loading} />
      </Section>

      <Section title="Breakdowns">
        <div className="grid gap-4 lg:grid-cols-3">
          <CategoryBreakdownChart
            title="By type"
            totalLabel="Resources"
            data={breakdowns?.byType ?? []}
            total={sumCounts(breakdowns?.byType ?? [])}
            emptyMessage="This user has created nothing yet."
            maxRows={9}
            {...breakdownProps}
          />
          <CategoryBreakdownChart
            title="By team"
            totalLabel="Resources"
            data={breakdowns?.byTeam ?? []}
            total={sumCounts(breakdowns?.byTeam ?? [])}
            emptyMessage="No resources in any team."
            {...breakdownProps}
          />
          <CategoryBreakdownChart
            title="By project"
            totalLabel="Resources"
            data={breakdowns?.byProject ?? []}
            total={sumCounts(breakdowns?.byProject ?? [])}
            emptyMessage="No project-scoped resources."
            {...breakdownProps}
          />
        </div>
      </Section>

      <div className="space-y-4">
        <DashboardControls
          range={range}
          onRangeChange={setRange}
          granularity={granularity}
          onGranularityChange={setGranularity}
          ariaLabel="Activity date range"
        />

        <Section
          title="Created over time"
          description={`Resources created per ${granularity}.`}
        >
          <TimeSeriesBarChart
            title="Resources created"
            totalLabel="Total created"
            total={sumTotals(creationData)}
            series={RESOURCE_TYPE_SERIES}
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
          description={`Resource accesses per ${granularity}, by source.`}
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
            emptyMessage="No resource access recorded in this range."
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
          <TopAccessedTable slot={top} />
        </Section>
      </div>
    </div>
  )
}
