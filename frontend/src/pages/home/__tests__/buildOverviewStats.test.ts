import type { TeamStats } from '@/types/team'

import { buildOverviewStats, type OverviewInputs } from '../buildOverviewStats'

function makeStats(overrides: Partial<TeamStats> = {}): TeamStats {
  return {
    total_projects: 1,
    total_prompts: 4,
    total_artifacts: 110,
    total_blueprints: 24,
    total_memories: 51,
    total_feed_items: 1862,
    ...overrides,
  }
}

function makeInputs(overrides: Partial<OverviewInputs> = {}): OverviewInputs {
  return {
    teamStats: makeStats(),
    totalSessions: 0,
    sessionsTrendPct: 0,
    totalAgents: 9,
    mcpToolsCount: 19,
    weekly: {
      prompts: 2,
      artifacts: 96,
      blueprints: 5,
      memories: 7,
      feed: 214,
    },
    isEmptyWorkspace: false,
    ...overrides,
  }
}

describe('buildOverviewStats', () => {
  it('produces the eight design cards in order with the right values', () => {
    const stats = buildOverviewStats(makeInputs())

    expect(stats.map(s => s.label)).toEqual([
      'AI Sessions',
      'Total Prompts',
      'Total Artifacts',
      'Total Memories',
      'Total Blueprints',
      'Total Agents',
      'AI Feed updates',
      'MCP Tools',
    ])
    expect(stats.map(s => s.value)).toEqual([0, 4, 110, 51, 24, 9, 1862, 19])
  })

  it('shows a muted "0%" sessions trend at zero and an up trend when positive', () => {
    const flat = buildOverviewStats(makeInputs({ sessionsTrendPct: 0 }))
    expect(flat[0].trend).toEqual({ label: '0%', tone: 'flat' })

    const up = buildOverviewStats(
      makeInputs({ totalSessions: 120, sessionsTrendPct: 12 })
    )
    expect(up[0].trend).toEqual({ label: '+12%', tone: 'up' })
  })

  it('derives up-trend badges and "this week" subtitles from weekly deltas', () => {
    const stats = buildOverviewStats(makeInputs())
    const artifacts = stats.find(s => s.label === 'Total Artifacts')
    expect(artifacts?.trend).toEqual({ label: '+96', tone: 'up' })
    expect(artifacts?.subtitle).toBe('+96 this week')

    const feed = stats.find(s => s.label === 'AI Feed updates')
    expect(feed?.trend).toEqual({ label: '+214', tone: 'up' })
    expect(feed?.subtitle).toBe('+214 this week')
  })

  it('omits trend badges for Agents and MCP Tools (no weekly source)', () => {
    const stats = buildOverviewStats(makeInputs())
    expect(stats.find(s => s.label === 'Total Agents')?.trend).toBeNull()
    expect(stats.find(s => s.label === 'MCP Tools')?.trend).toBeNull()
  })

  it('suppresses every trend badge and subtitle for an empty workspace', () => {
    const stats = buildOverviewStats(
      makeInputs({
        isEmptyWorkspace: true,
        teamStats: makeStats({
          total_prompts: 0,
          total_artifacts: 0,
          total_blueprints: 0,
          total_memories: 0,
          total_feed_items: 0,
        }),
      })
    )
    expect(stats.every(s => s.trend === null)).toBe(true)
    expect(stats.every(s => s.subtitle === undefined)).toBe(true)
  })

  it('falls back to zeros when stats / weekly data are missing', () => {
    const stats = buildOverviewStats(
      makeInputs({ teamStats: null, weekly: null })
    )
    expect(stats.find(s => s.label === 'Total Prompts')?.value).toBe(0)
    expect(stats.find(s => s.label === 'Total Prompts')?.trend).toBeNull()
    // MCP Tools comes from a static count, not team stats.
    expect(stats.find(s => s.label === 'MCP Tools')?.value).toBe(19)
  })
})
