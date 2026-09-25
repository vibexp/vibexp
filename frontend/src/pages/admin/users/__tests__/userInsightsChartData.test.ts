import type {
  AdminUserCreationPoint,
  AdminUserInsights,
} from '@/services/adminService'

import {
  creationToChartData,
  formatResourceType,
  insightsToBreakdowns,
  RESOURCE_TYPE_KEYS,
  RESOURCE_TYPE_SERIES,
} from '../detail/userInsightsChartData'

function counts(overrides: Partial<AdminUserInsights['totals']> = {}) {
  return {
    prompts: 0,
    memories: 0,
    artifacts: 0,
    blueprints: 0,
    agents: 0,
    feeds: 0,
    feed_items: 0,
    comments: 0,
    attachments: 0,
    total: 0,
    ...overrides,
  }
}

function point(
  bucket: string,
  overrides: Partial<AdminUserCreationPoint> = {}
): AdminUserCreationPoint {
  return {
    bucket,
    prompts: 0,
    memories: 0,
    artifacts: 0,
    blueprints: 0,
    agents: 0,
    feeds: 0,
    feed_items: 0,
    comments: 0,
    attachments: 0,
    ...overrides,
  }
}

describe('RESOURCE_TYPE_SERIES', () => {
  it('has one series per counted type, cycling the five chart tokens', () => {
    expect(RESOURCE_TYPE_SERIES.map(s => s.key)).toEqual([
      ...RESOURCE_TYPE_KEYS,
    ])
    expect(RESOURCE_TYPE_SERIES[0].fill).toBe('var(--chart-1)')
    expect(RESOURCE_TYPE_SERIES[5].fill).toBe('var(--chart-1)')
    expect(RESOURCE_TYPE_SERIES[7].label).toBe('Comments')
  })
})

describe('creationToChartData', () => {
  it('keys buckets by UTC day and totals every type', () => {
    const data = creationToChartData([
      point('2026-09-01T00:00:00Z', { prompts: 2, comments: 3 }),
      point('2026-09-02T00:00:00Z', { attachments: 1 }),
    ])

    expect(data).toHaveLength(2)
    expect(data[0]).toMatchObject({
      date: '2026-09-01',
      total: 5,
      prompts: 2,
      comments: 3,
      memories: 0,
    })
    expect(data[1]).toMatchObject({
      date: '2026-09-02',
      total: 1,
      attachments: 1,
    })
  })

  it('returns no data for no points', () => {
    expect(creationToChartData([])).toEqual([])
  })
})

describe('insightsToBreakdowns', () => {
  const insights: AdminUserInsights = {
    user_id: 'u1',
    totals: counts({ prompts: 4, memories: 2, feed_items: 1, total: 7 }),
    teams: [
      {
        team_id: 't1',
        team_name: 'Engineering',
        is_member: true,
        counts: counts({ prompts: 4, memories: 2, total: 6 }),
        projects: [
          {
            project_id: 'p1',
            project_name: 'Core',
            counts: { prompts: 3, artifacts: 0, memories: 2, blueprints: 0 },
          },
        ],
      },
      {
        team_id: 't2',
        team_name: 'Old team',
        is_member: false,
        counts: counts({ feed_items: 1, total: 1 }),
        projects: [],
      },
    ],
  }

  it('breaks down by type from the totals', () => {
    const { byType } = insightsToBreakdowns(insights)
    expect(byType).toHaveLength(RESOURCE_TYPE_KEYS.length)
    expect(byType.find(d => d.key === 'prompts')).toMatchObject({
      label: 'Prompts',
      count: 4,
    })
    expect(byType.find(d => d.key === 'feed_items')).toMatchObject({
      label: 'Feed items',
      count: 1,
    })
  })

  it('breaks down by team, marking teams the user has left', () => {
    const { byTeam } = insightsToBreakdowns(insights)
    expect(byTeam).toEqual([
      { key: 't1', label: 'Engineering', count: 6, detail: undefined },
      { key: 't2', label: 'Old team', count: 1, detail: 'former member' },
    ])
  })

  it('breaks down by project, summing the project-scoped types', () => {
    const { byProject } = insightsToBreakdowns(insights)
    expect(byProject).toEqual([
      { key: 'p1', label: 'Core', count: 5, detail: 'Engineering' },
    ])
  })
})

describe('formatResourceType', () => {
  it('labels singular wire types', () => {
    expect(formatResourceType('feed_item')).toBe('Feed item')
    expect(formatResourceType('prompt')).toBe('Prompt')
  })
})
