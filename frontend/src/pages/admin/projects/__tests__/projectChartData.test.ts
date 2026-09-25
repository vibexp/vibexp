import type { AdminProjectDetail } from '@/services/adminService'

import {
  countsToBreakdown,
  PROJECT_RESOURCE_TYPE_SERIES,
  projectCreationToChartData,
  resourceCountLabel,
} from '../detail/projectChartData'

describe('projectCreationToChartData', () => {
  it('maps each bucket to a UTC date key with per-type values and a total', () => {
    const data = projectCreationToChartData([
      {
        bucket: '2026-09-20T00:00:00Z',
        prompts: 2,
        memories: 1,
        artifacts: 3,
        blueprints: 0,
        feed_items: 4,
      },
    ])

    expect(data).toEqual([
      {
        date: '2026-09-20',
        prompts: 2,
        memories: 1,
        artifacts: 3,
        blueprints: 0,
        feed_items: 4,
        total: 10,
      },
    ])
  })

  it('charts only the project-scoped types', () => {
    expect(PROJECT_RESOURCE_TYPE_SERIES.map(s => s.key)).toEqual([
      'prompts',
      'memories',
      'artifacts',
      'blueprints',
      'feed_items',
    ])
  })
})

describe('countsToBreakdown', () => {
  it('labels known types, keeps an unknown one and drops the total', () => {
    const counts = {
      prompts: 12,
      feed_items: 8,
      widgets: 9,
      total: 29,
    } as unknown as AdminProjectDetail['resource_counts']

    expect(countsToBreakdown(counts)).toEqual([
      { key: 'prompts', label: 'Prompts', count: 12 },
      { key: 'feed_items', label: 'Feed items', count: 8 },
      { key: 'widgets', label: 'Widgets', count: 9 },
    ])
  })
})

describe('resourceCountLabel', () => {
  it('labels the total and the known types, and nothing else', () => {
    expect(resourceCountLabel('total')).toBe('Total resources')
    expect(resourceCountLabel('memories')).toBe('Memories')
    expect(resourceCountLabel('widgets')).toBeUndefined()
  })
})
