import type { TimeSeriesDatum } from '@/components/TimeSeriesBarChart'
import type {
  AdminCountPoint,
  AdminGrowthPoint,
  AdminSourcePoint,
} from '@/services/adminService'

/**
 * The bucket instant as a `YYYY-MM-DD` key, read in **UTC**.
 *
 * The API documents every bucket as "start of the bucket, in UTC", so the axis
 * has to name the UTC day. Formatting it in local time would let a bucket appear
 * under the previous day's label for any negative offset, putting the chart's
 * x-axis a day out of step with the server's own bucketing — the mirror image of
 * the `parseLocalDate` problem in `TimeSeriesBarChart`, where a date-only string
 * had to be read as local because that is what it meant.
 */
export function bucketKey(bucketIso: string): string {
  const date = new Date(bucketIso)
  if (Number.isNaN(date.getTime())) return bucketIso
  const month = String(date.getUTCMonth() + 1).padStart(2, '0')
  const day = String(date.getUTCDate()).padStart(2, '0')
  return `${String(date.getUTCFullYear())}-${month}-${day}`
}

/** The growth entities charted, in display order. */
export const GROWTH_KEYS = [
  'users',
  'teams',
  'projects',
  'prompts',
  'artifacts',
  'memories',
] as const

/**
 * Growth points as chart data.
 *
 * Passed through unchanged apart from the key mapping: #451 gap-fills server-side,
 * so a bucket with no activity already arrives as an explicit zero. Filling again
 * here would risk inventing buckets the server deliberately excluded.
 */
export function growthToChartData(
  points: readonly AdminGrowthPoint[]
): TimeSeriesDatum[] {
  return points.map(point => {
    const datum: TimeSeriesDatum = {
      date: bucketKey(point.bucket),
      total: GROWTH_KEYS.reduce((sum, key) => sum + point[key], 0),
    }
    for (const key of GROWTH_KEYS) {
      datum[key] = point[key]
    }
    return datum
  })
}

/** Sign-in counts as single-series chart data. */
export function countsToChartData(
  points: readonly AdminCountPoint[]
): TimeSeriesDatum[] {
  return points.map(point => ({
    date: bucketKey(point.bucket),
    total: point.count,
    count: point.count,
  }))
}

/**
 * Access points pivoted from (bucket, source, count) rows into one datum per
 * bucket with a key per source.
 *
 * The API only reports sources actually observed in the range, so the series list
 * is derived from the data rather than hardcoded — a deployment with no CLI
 * traffic should show no CLI series at all, not an empty one.
 */
export function accessToChartData(points: readonly AdminSourcePoint[]): {
  data: TimeSeriesDatum[]
  sources: string[]
} {
  const sources = [...new Set(points.map(point => point.source))].sort((a, b) =>
    a.localeCompare(b)
  )
  const byBucket = new Map<string, TimeSeriesDatum>()

  for (const point of points) {
    const key = bucketKey(point.bucket)
    let datum = byBucket.get(key)
    if (!datum) {
      datum = { date: key, total: 0 }
      // Zero every observed source so a bucket missing one renders a gap rather
      // than an undefined the chart would drop.
      for (const source of sources) datum[source] = 0
      byBucket.set(key, datum)
    }
    datum[point.source] = point.count
    datum.total += point.count
  }

  return { data: [...byBucket.values()], sources }
}

/** Total across every bucket and series, for the chart header. */
export function sumTotals(data: readonly TimeSeriesDatum[]): number {
  return data.reduce((sum, datum) => sum + datum.total, 0)
}
