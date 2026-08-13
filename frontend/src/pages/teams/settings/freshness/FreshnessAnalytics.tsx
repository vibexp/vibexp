import { useCallback, useEffect, useState } from 'react'

import {
  CategoryBreakdownChart,
  type CategoryDatum,
} from '@/components/CategoryBreakdownChart'
import {
  type ChartSeries,
  TIME_SERIES_RANGE_OPTIONS,
  TimeSeriesBarChart,
} from '@/components/TimeSeriesBarChart'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type {
  FreshnessByProjectMetricsData,
  FreshnessByRuleMetricsData,
  FreshnessByTypeMetricsData,
  FreshnessMetricsRange,
} from '@/services/freshnessService'
import { freshnessService } from '@/services/freshnessService'

import { RESOURCE_TYPE_OPTIONS } from './freshnessOptions'

const DEFAULT_RANGE: FreshnessMetricsRange = '30d'

/**
 * `marked` and `cleared` are the two flows; `stale_total` is a LEVEL and is
 * deliberately NOT charted beside them — stacking a level on flows would draw a
 * quantity that means nothing. The payload's `total` is the flow sum, which is
 * what the shared chart's tooltip reports.
 */
const OVER_TIME_SERIES: readonly ChartSeries[] = [
  { key: 'marked', label: 'Marked stale', fill: 'var(--chart-1)' },
  { key: 'cleared', label: 'Cleared', fill: 'var(--chart-2)' },
]

/**
 * Copy for a workspace that simply has no history yet. Freshness starts clean
 * and accrues after deploy — with no backfill — so an empty chart here is the
 * expected state on a new install, not a symptom. Saying so is an explicit
 * acceptance criterion of #737.
 */
const NO_HISTORY =
  'No freshness activity in this window yet. Freshness starts clean and builds up as rules run, so a new workspace has nothing to show here at first.'

const NOTHING_STALE = 'Nothing is stale right now.'

/** One panel's async state. A failure is per-panel: one dead endpoint must not blank the tab. */
interface PanelState<T> {
  data: T | null
  loading: boolean
  error: boolean
}

const initial = <T,>(): PanelState<T> => ({
  data: null,
  loading: true,
  error: false,
})

/**
 * Fetches one metrics panel, ignoring aborts.
 *
 * An abort is expected on unmount and on every range change; treating it as an
 * error would flash a failure message over data that is about to arrive.
 *
 * `load` MUST be memoized with `useCallback` by the caller — its identity is
 * what decides when this refetches, exactly as `useResourceListQuery` requires.
 * A fresh closure per render would refetch forever.
 */
function usePanel<T>(load: (signal: AbortSignal) => Promise<T>): PanelState<T> {
  const [state, setState] = useState<PanelState<T>>(initial<T>)
  const run = load

  useEffect(() => {
    const controller = new AbortController()
    setState(prev => ({ ...prev, loading: true }))
    run(controller.signal)
      .then(data => {
        if (!controller.signal.aborted) {
          setState({ data, loading: false, error: false })
        }
      })
      .catch((err: unknown) => {
        if (controller.signal.aborted) return
        console.error('Failed to load freshness metrics', err)
        setState({ data: null, loading: false, error: true })
      })
    return () => {
      controller.abort()
    }
  }, [run])

  return state
}

function typeRows(data: FreshnessByTypeMetricsData | null): CategoryDatum[] {
  return (data?.counts ?? []).map(c => ({
    key: c.resource_type,
    label:
      RESOURCE_TYPE_OPTIONS.find(o => o.value === c.resource_type)?.label ??
      c.resource_type,
    count: c.count,
  }))
}

function projectRows(
  data: FreshnessByProjectMetricsData | null
): CategoryDatum[] {
  return (data?.counts ?? []).map(c => ({
    key: c.project_id,
    label: c.name,
    count: c.count,
  }))
}

function ruleRows(data: FreshnessByRuleMetricsData | null): CategoryDatum[] {
  return (data?.counts ?? []).map(c => ({
    key: c.rule_id,
    // A rule has no name, so it is identified by what it does. The types are
    // what distinguish two rules on the same project in practice.
    label: c.resource_types
      .map(t => RESOURCE_TYPE_OPTIONS.find(o => o.value === t)?.label ?? t)
      .join(', '),
    detail: `${String(c.threshold_days)}d${c.enabled ? '' : ' · disabled'}`,
    count: c.count,
    muted: !c.enabled,
  }))
}

/**
 * The analytics tab: one time series plus three point-in-time breakdowns.
 *
 * Visible to every team member, including those without `team.settings.update` —
 * the engine writes to everyone's resources, so everyone may see what it did.
 * The range selector drives only the over-time chart; the other three are
 * "right now" counts with no window.
 */
export function FreshnessAnalytics({ teamId }: Readonly<{ teamId: string }>) {
  const [range, setRange] = useState<FreshnessMetricsRange>(DEFAULT_RANGE)

  const overTime = usePanel(
    useCallback(
      (signal: AbortSignal) =>
        freshnessService.getOverTimeMetrics(teamId, range, signal),
      [teamId, range]
    )
  )
  const byType = usePanel(
    useCallback(
      (signal: AbortSignal) =>
        freshnessService.getByTypeMetrics(teamId, signal),
      [teamId]
    )
  )
  const byProject = usePanel(
    useCallback(
      (signal: AbortSignal) =>
        freshnessService.getByProjectMetrics(teamId, signal),
      [teamId]
    )
  )
  const byRule = usePanel(
    useCallback(
      (signal: AbortSignal) =>
        freshnessService.getByRuleMetrics(teamId, signal),
      [teamId]
    )
  )

  const overTimeTotal =
    (overTime.data?.total_marked ?? 0) + (overTime.data?.total_cleared ?? 0)

  return (
    <div className="space-y-6">
      <div className="flex justify-end">
        <Select
          value={range}
          onValueChange={value => {
            setRange(value as FreshnessMetricsRange)
          }}
        >
          <SelectTrigger className="w-[160px]" aria-label="Select time range">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TIME_SERIES_RANGE_OPTIONS.map(option => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <TimeSeriesBarChart
        title="Staleness over time"
        totalLabel="Transitions"
        total={overTimeTotal}
        series={OVER_TIME_SERIES}
        data={overTime.data?.counts ?? []}
        range={range}
        hideRangeControl
        loading={overTime.loading}
        error={overTime.error}
        errorMessage="Couldn't load staleness over time"
        emptyMessage={NO_HISTORY}
      />

      <div className="grid gap-6 lg:grid-cols-2">
        <CategoryBreakdownChart
          title="Stale by resource type"
          totalLabel="Stale resources"
          total={byType.data?.total_stale ?? 0}
          data={typeRows(byType.data)}
          loading={byType.loading}
          error={byType.error}
          errorMessage="Couldn't load the type breakdown"
          emptyMessage={NOTHING_STALE}
        />

        <CategoryBreakdownChart
          title="Stale by project"
          totalLabel="Stale resources"
          total={byProject.data?.total_stale ?? 0}
          data={projectRows(byProject.data)}
          loading={byProject.loading}
          error={byProject.error}
          errorMessage="Couldn't load the project breakdown"
          emptyMessage={NOTHING_STALE}
        />
      </div>

      <CategoryBreakdownChart
        title="Impact per rule"
        totalLabel="Stale resources"
        total={byRule.data?.total_stale ?? 0}
        data={ruleRows(byRule.data)}
        loading={byRule.loading}
        error={byRule.error}
        errorMessage="Couldn't load the per-rule breakdown"
        emptyMessage="No rules have flagged anything yet."
      />
      <p className="text-muted-foreground text-xs">
        A resource matched by several rules counts once in the total and once
        under each rule, so the per-rule figures can sum to more than the total.
      </p>
    </div>
  )
}
