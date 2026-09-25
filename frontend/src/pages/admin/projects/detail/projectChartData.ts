import type { CategoryDatum } from '@/components/CategoryBreakdownChart'
import type {
  ChartSeries,
  TimeSeriesDatum,
} from '@/components/TimeSeriesBarChart'
import type { ResourceTypeKey } from '@/pages/admin/users/detail/userInsightsChartData'
import {
  CHART_FILLS,
  formatResourceType,
  RESOURCE_TYPE_LABELS,
  seriesToChartData,
} from '@/pages/admin/users/detail/userInsightsChartData'
import type {
  AdminProjectCreationPoint,
  AdminProjectDetail,
} from '@/services/adminService'

/**
 * The project-scoped resource types, in display order: only the tables with a
 * `project_id` column (#1145). A key missing from the creation point fails
 * `tsc`, so the chart can never ask for a series the API does not send.
 */
export const PROJECT_RESOURCE_TYPE_KEYS = [
  'prompts',
  'memories',
  'artifacts',
  'blueprints',
  'feed_items',
] as const satisfies readonly (keyof AdminProjectCreationPoint &
  ResourceTypeKey)[]

export const PROJECT_RESOURCE_TYPE_SERIES: readonly ChartSeries[] =
  PROJECT_RESOURCE_TYPE_KEYS.map((key, index) => ({
    key,
    label: RESOURCE_TYPE_LABELS[key],
    fill: CHART_FILLS[index % CHART_FILLS.length],
  }))

/** The label of a resource-count key, or `undefined` for one we do not know. */
export function resourceCountLabel(key: string): string | undefined {
  if (key === 'total') return 'Total resources'
  return (RESOURCE_TYPE_LABELS as Partial<Record<string, string>>)[key]
}

/** A project's creation points as chart data, one key per project type. */
export function projectCreationToChartData(
  points: readonly AdminProjectCreationPoint[]
): TimeSeriesDatum[] {
  return seriesToChartData(points, PROJECT_RESOURCE_TYPE_KEYS)
}

/**
 * The project's resources by type, from the counts the detail already loaded.
 *
 * Driven by the response keys (minus the total), so a type the API adds later
 * still shows, labelled from its key.
 */
export function countsToBreakdown(
  counts: AdminProjectDetail['resource_counts']
): CategoryDatum[] {
  return Object.entries(counts)
    .filter(([key]) => key !== 'total')
    .map(([key, count]) => ({
      key,
      label: resourceCountLabel(key) ?? formatResourceType(key),
      count,
    }))
}
