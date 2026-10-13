import type { TeamStats } from '@/services/teamService'

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
    totalAgents: 9,
    weekly: {
      prompts: 2,
      artifacts: 96,
      blueprints: 5,
      memories: 7,
      feed: 214,
    },
    ...overrides,
  }
}

describe('buildOverviewStats', () => {
  it('produces the six workspace totals in order with the right values', () => {
    const stats = buildOverviewStats(makeInputs())

    expect(stats.map(s => [s.label, s.value])).toEqual([
      ['Prompts', 4],
      ['Artifacts', 110],
      ['Memories', 51],
      ['Blueprints', 24],
      ['Agents', 9],
      ['Feed updates', 1862],
    ])
  })

  it('has no MCP tools entry: a product constant is not a workspace total', () => {
    const labels = buildOverviewStats(makeInputs()).map(s => s.label)

    expect(labels.some(label => /mcp/i.test(label))).toBe(false)
  })

  it('carries the weekly delta of each resource type', () => {
    const byLabel = Object.fromEntries(
      buildOverviewStats(makeInputs()).map(s => [s.label, s.weeklyDelta])
    )

    expect(byLabel).toEqual({
      Prompts: 2,
      Artifacts: 96,
      Memories: 7,
      Blueprints: 5,
      Agents: undefined,
      'Feed updates': 214,
    })
  })

  it('falls back to zero values and no deltas before anything has loaded', () => {
    const stats = buildOverviewStats(
      makeInputs({ teamStats: null, totalAgents: 0, weekly: null })
    )

    expect(stats.every(s => s.value === 0)).toBe(true)
    expect(stats.every(s => s.weeklyDelta === undefined)).toBe(true)
  })
})
