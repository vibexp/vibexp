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
import { Link } from 'react-router'

import { CategoryBreakdownChart } from '@/components/CategoryBreakdownChart'
import type { ChartSeries } from '@/components/TimeSeriesBarChart'
import { TimeSeriesBarChart } from '@/components/TimeSeriesBarChart'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { DateRangeValue } from '@/components/ui/date-range'
import { rangeToInstants } from '@/components/ui/date-range'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { accessToChartData, sumTotals } from '@/pages/admin/dashboard/buckets'
import type { Granularity } from '@/pages/admin/dashboard/DashboardControls'
import { DashboardControls } from '@/pages/admin/dashboard/DashboardControls'
import { DataWindowNote } from '@/pages/admin/dashboard/DataWindowNote'
import type { ResourceTypeKey } from '@/pages/admin/users/detail/userInsightsChartData'
import {
  CHART_FILLS,
  creationToChartData,
  formatResourceType,
  insightsToBreakdowns,
  RESOURCE_TYPE_KEYS,
  RESOURCE_TYPE_LABELS,
  RESOURCE_TYPE_SERIES,
} from '@/pages/admin/users/detail/userInsightsChartData'
import type {
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

/** One async slot: its value, whether it is in flight, and its error. */
interface Slot<T> {
  data: T | null
  loading: boolean
  error: string | null
}

const LOADING: Slot<never> = { data: null, loading: true, error: null }

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

function CountCards({
  insights,
  loading,
}: Readonly<{ insights: AdminUserInsights | null; loading: boolean }>) {
  const cards: { key: string; label: string; icon: LucideIcon }[] = [
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
              {insights.totals[
                key as keyof AdminUserInsights['totals']
              ].toLocaleString()}
            </p>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

/**
 * The resources a user accessed most, as opaque references.
 *
 * Each cell reads one allowlisted field — never a spread of the row — so a
 * title can never leak into this table even if the wire type grows one.
 */
function TopAccessedTable({
  slot,
}: Readonly<{ slot: Slot<AdminTopAccessedResource[]> }>) {
  if (slot.loading) return <Skeleton className="h-32 w-full" />
  if (slot.error) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Failed to load the most accessed resources</AlertTitle>
        <AlertDescription>{slot.error}</AlertDescription>
      </Alert>
    )
  }
  const rows = slot.data ?? []
  if (rows.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        No resource access recorded in this range.
      </p>
    )
  }

  return (
    <Card className="overflow-hidden">
      <Table>
        <TableHeader>
          <TableRow className="bg-muted/40 hover:bg-muted/40">
            <TableHead className="h-9 text-xs font-medium">Type</TableHead>
            <TableHead className="h-9 text-xs font-medium">Team</TableHead>
            <TableHead className="h-9 text-xs font-medium">Project</TableHead>
            <TableHead className="h-9 text-xs font-medium">Id</TableHead>
            <TableHead className="h-9 text-right text-xs font-medium">
              Accesses
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map(row => (
            <TableRow key={`${row.resource_type}-${row.resource_short_id}`}>
              <TableCell className="py-3">
                <Badge variant="outline">
                  {formatResourceType(row.resource_type)}
                </Badge>
              </TableCell>
              <TableCell className="py-3 text-sm">
                <Link
                  to={`/admin/teams/${row.team_id}`}
                  className="hover:underline"
                >
                  {row.team_name}
                </Link>
              </TableCell>
              <TableCell className="py-3 text-sm">
                {row.project_name ?? '—'}
              </TableCell>
              <TableCell className="py-3 font-mono text-xs">
                {row.resource_short_id}
                {row.resource_deleted && (
                  <span className="text-muted-foreground ml-2 font-sans">
                    (deleted)
                  </span>
                )}
              </TableCell>
              <TableCell className="py-3 text-right text-sm tabular-nums">
                {row.access_count.toLocaleString()}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
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
    function settle<T>(
      request: Promise<T>,
      set: (slot: Slot<T>) => void,
      fallback: string
    ) {
      request
        .then(data => {
          if (!cancelled) set({ data, loading: false, error: null })
        })
        .catch((err: unknown) => {
          if (!cancelled) {
            set({
              data: null,
              loading: false,
              error: getErrorMessage(err, fallback),
            })
          }
        })
    }

    settle(
      adminService.getUserResourceCreationMetrics(userId, {
        from,
        to,
        granularity,
      }),
      setCreation,
      'Failed to load the creation series'
    )
    settle(
      adminService.getUserResourceAccessMetrics(userId, {
        from,
        to,
        granularity,
      }),
      setAccess,
      'Failed to load the access series'
    )
    settle(
      adminService
        .getUserTopAccessedResources(userId, { from, to })
        .then(response => response.items),
      setTop,
      'Failed to load the most accessed resources'
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
    total: insights.data?.totals.total ?? 0,
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
            emptyMessage="This user has created nothing yet."
            maxRows={9}
            {...breakdownProps}
          />
          <CategoryBreakdownChart
            title="By team"
            totalLabel="Resources"
            data={breakdowns?.byTeam ?? []}
            emptyMessage="No resources in any team."
            {...breakdownProps}
          />
          <CategoryBreakdownChart
            title="By project"
            totalLabel="Resources"
            data={breakdowns?.byProject ?? []}
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
