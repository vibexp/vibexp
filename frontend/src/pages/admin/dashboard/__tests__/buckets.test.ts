/**
 * The dashboard's bucket → chart-data mappers (#458).
 *
 * Tested directly because the pivot and the timezone choice are the two places
 * this page can silently misrepresent the server's data.
 */
import {
  accessToChartData,
  bucketKey,
  countsToChartData,
  growthToChartData,
  sumTotals,
} from '@/pages/admin/dashboard/buckets'

describe('bucketKey', () => {
  it('names the UTC day of the bucket', () => {
    // The API documents buckets as UTC-aligned, so the axis has to agree with the
    // server's bucketing. Reading these in local time would let a bucket appear
    // under the previous day for any negative offset.
    expect(bucketKey('2026-07-01T00:00:00Z')).toBe('2026-07-01')
    expect(bucketKey('2026-01-01T00:00:00Z')).toBe('2026-01-01')
    // Late-UTC-evening instant: still its own UTC day, whatever the runner's zone.
    expect(bucketKey('2026-03-15T23:30:00Z')).toBe('2026-03-15')
  })

  it('pads single-digit months and days', () => {
    expect(bucketKey('2026-02-03T00:00:00Z')).toBe('2026-02-03')
  })

  it('passes an unparseable value through rather than rendering NaN', () => {
    expect(bucketKey('not-a-date')).toBe('not-a-date')
  })
})

describe('growthToChartData', () => {
  const point = {
    bucket: '2026-07-01T00:00:00Z',
    users: 2,
    teams: 1,
    projects: 3,
    prompts: 5,
    artifacts: 7,
    memories: 11,
  }

  it('maps each entity to its own series key and totals the bucket', () => {
    const [datum] = growthToChartData([point])

    expect(datum.date).toBe('2026-07-01')
    // Distinct values per entity, so a transposed mapping cannot pass.
    expect(datum.users).toBe(2)
    expect(datum.teams).toBe(1)
    expect(datum.projects).toBe(3)
    expect(datum.prompts).toBe(5)
    expect(datum.artifacts).toBe(7)
    expect(datum.memories).toBe(11)
    expect(datum.total).toBe(29)
  })

  it('keeps an all-zero bucket instead of dropping it', () => {
    const empty = {
      bucket: '2026-07-02T00:00:00Z',
      users: 0,
      teams: 0,
      projects: 0,
      prompts: 0,
      artifacts: 0,
      memories: 0,
    }

    const data = growthToChartData([point, empty])

    // #451 gap-fills server-side; dropping a zero bucket here would compress the
    // axis and make a quiet period look like it never happened.
    expect(data).toHaveLength(2)
    expect(data[1].total).toBe(0)
  })

  it('returns nothing for no points', () => {
    expect(growthToChartData([])).toEqual([])
  })
})

describe('countsToChartData', () => {
  it('maps a count into both the series key and the total', () => {
    const data = countsToChartData([
      { bucket: '2026-07-01T00:00:00Z', count: 12 },
      { bucket: '2026-07-02T00:00:00Z', count: 0 },
    ])

    expect(data).toEqual([
      { date: '2026-07-01', total: 12, count: 12 },
      { date: '2026-07-02', total: 0, count: 0 },
    ])
  })
})

describe('accessToChartData', () => {
  const points = [
    { bucket: '2026-07-01T00:00:00Z', source: 'web', count: 30 },
    { bucket: '2026-07-01T00:00:00Z', source: 'mcp', count: 12 },
    { bucket: '2026-07-02T00:00:00Z', source: 'web', count: 4 },
  ]

  it('pivots (bucket, source, count) rows into one datum per bucket', () => {
    const { data } = accessToChartData(points)

    expect(data).toHaveLength(2)
    expect(data[0]).toMatchObject({ date: '2026-07-01', web: 30, mcp: 12 })
    expect(data[0].total).toBe(42)
  })

  it('zero-fills a source missing from a bucket', () => {
    const { data } = accessToChartData(points)

    // 'mcp' has no row in the second bucket. Left undefined, the chart would drop
    // the point and the stacked area would jump.
    expect(data[1]).toMatchObject({ date: '2026-07-02', web: 4, mcp: 0 })
    expect(data[1].total).toBe(4)
  })

  it('derives the source list from the data, sorted', () => {
    const { sources } = accessToChartData(points)

    // Only sources actually observed appear — a deployment with no CLI traffic
    // should show no CLI series rather than an empty one.
    expect(sources).toEqual(['mcp', 'web'])
  })

  it('handles no access data', () => {
    expect(accessToChartData([])).toEqual({ data: [], sources: [] })
  })
})

describe('sumTotals', () => {
  it('adds the bucket totals', () => {
    expect(
      sumTotals([
        { date: '2026-07-01', total: 3 },
        { date: '2026-07-02', total: 4 },
      ])
    ).toBe(7)
  })

  it('is zero for no data', () => {
    expect(sumTotals([])).toBe(0)
  })
})
