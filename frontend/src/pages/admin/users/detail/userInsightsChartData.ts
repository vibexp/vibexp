import type { CategoryDatum } from '@/components/CategoryBreakdownChart'
import type {
  ChartSeries,
  TimeSeriesDatum,
} from '@/components/TimeSeriesBarChart'
import { bucketKey } from '@/pages/admin/dashboard/buckets'
import type {
  AdminResourceCounts,
  AdminUserCreationPoint,
  AdminUserInsights,
} from '@/services/adminService'

/** The counted resource types, in display order (the insights/creation keys). */
export const RESOURCE_TYPE_KEYS = [
  'prompts',
  'memories',
  'artifacts',
  'blueprints',
  'agents',
  'feeds',
  'feed_items',
  'comments',
  'attachments',
] as const satisfies readonly (keyof AdminResourceCounts &
  keyof AdminUserCreationPoint)[]

export type ResourceTypeKey = (typeof RESOURCE_TYPE_KEYS)[number]

export const RESOURCE_TYPE_LABELS: Record<ResourceTypeKey, string> = {
  prompts: 'Prompts',
  memories: 'Memories',
  artifacts: 'Artifacts',
  blueprints: 'Blueprints',
  agents: 'Agents',
  feeds: 'Feeds',
  feed_items: 'Feed items',
  comments: 'Comments',
  attachments: 'Attachments',
}

/**
 * The design system has five chart tokens for nine types, so colours cycle. The
 * `strip` legend is toggleable, which is how two same-coloured series are told
 * apart (maintainer decision on #1137).
 */
export const CHART_FILLS = [
  'var(--chart-1)',
  'var(--chart-2)',
  'var(--chart-3)',
  'var(--chart-4)',
  'var(--chart-5)',
] as const

export const RESOURCE_TYPE_SERIES: readonly ChartSeries[] =
  RESOURCE_TYPE_KEYS.map((key, index) => ({
    key,
    label: RESOURCE_TYPE_LABELS[key],
    fill: CHART_FILLS[index % CHART_FILLS.length],
  }))

/**
 * Creation points as chart data, one key per resource type.
 *
 * The server gap-fills buckets, so points pass through without inventing any.
 */
export function creationToChartData(
  points: readonly AdminUserCreationPoint[]
): TimeSeriesDatum[] {
  return points.map(point => {
    const datum: TimeSeriesDatum = { date: bucketKey(point.bucket), total: 0 }
    for (const key of RESOURCE_TYPE_KEYS) {
      datum[key] = point[key]
      datum.total += point[key]
    }
    return datum
  })
}

/** Breakdowns of the user's resources by type, by team and by project. */
export function insightsToBreakdowns(insights: AdminUserInsights): {
  byType: CategoryDatum[]
  byTeam: CategoryDatum[]
  byProject: CategoryDatum[]
} {
  const byType = RESOURCE_TYPE_KEYS.map(key => ({
    key,
    label: RESOURCE_TYPE_LABELS[key],
    count: insights.totals[key],
  }))

  const byTeam = insights.teams.map(team => ({
    key: team.team_id,
    label: team.team_name,
    count: team.counts.total,
    // A team the user has left still owns what they created there.
    detail: team.is_member ? undefined : 'former member',
  }))

  // Only prompts, artifacts, memories and blueprints are project-scoped, so the
  // project figures sum those four rather than reading a total.
  const byProject = insights.teams.flatMap(team =>
    team.projects.map(project => ({
      key: project.project_id,
      label: project.project_name,
      count:
        project.counts.prompts +
        project.counts.artifacts +
        project.counts.memories +
        project.counts.blueprints,
      detail: team.team_name,
    }))
  )

  return { byType, byTeam, byProject }
}

/**
 * A singular wire resource type (`feed_item`) as a display label (`Feed item`).
 *
 * Deliberately generic over the string so a type added to the spec later still
 * renders sensibly rather than disappearing.
 */
export function formatResourceType(type: string): string {
  const spaced = type.replaceAll('_', ' ')
  return spaced.charAt(0).toUpperCase() + spaced.slice(1)
}

/** Headline total of a breakdown: the sum of its own rows. */
export function sumCounts(data: readonly CategoryDatum[]): number {
  return data.reduce((sum, datum) => sum + datum.count, 0)
}
