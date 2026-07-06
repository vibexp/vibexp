import { render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'

import type {
  TeamCreationCountByDate,
  TeamResourceCreationMetricsResponse,
} from '../../services/teamService'
import type { TeamStats } from '../../services/teamService'

const mockGetTeamResourceCreationMetrics = jest.fn()
const mockGetTeamStats = jest.fn()

jest.mock('../../services/teamService', () => ({
  teamService: {
    getTeamResourceCreationMetrics: (...args: unknown[]) =>
      mockGetTeamResourceCreationMetrics(...args),
    getTeamStats: (...args: unknown[]) => mockGetTeamStats(...args),
  },
}))

jest.mock('recharts', () => {
  const actual = jest.requireActual('recharts')
  return {
    ...actual,
    ResponsiveContainer: ({ children }: { children: ReactNode }) => (
      <div style={{ width: 400, height: 180 }}>{children}</div>
    ),
  }
})

import {
  buildCumulativeSeries,
  TeamResourceCumulativeChart,
  totalsFromStats,
} from '../TeamResourceCumulativeChart'

function makeStats(overrides: Partial<TeamStats> = {}): TeamStats {
  return {
    total_projects: 0,
    total_prompts: 10,
    total_artifacts: 5,
    total_blueprints: 0,
    total_memories: 0,
    total_feed_items: 0,
    ...overrides,
  }
}

function makeCount(
  date: string,
  overrides: Partial<TeamCreationCountByDate> = {}
): TeamCreationCountByDate {
  return {
    date,
    prompts: 0,
    artifacts: 0,
    blueprints: 0,
    memories: 0,
    projects: 0,
    total: 0,
    ...overrides,
  }
}

function buildMetrics(
  counts: TeamCreationCountByDate[]
): TeamResourceCreationMetricsResponse {
  return {
    status: 'success',
    message: 'ok',
    data: {
      total_created: counts.reduce((s, c) => s + c.total, 0),
      range: '30d',
      counts,
    },
  }
}

describe('buildCumulativeSeries', () => {
  it('walks current totals backwards to reconstruct per-day cumulative counts', () => {
    const counts = [
      makeCount('2026-05-01', { prompts: 2, artifacts: 1, total: 3 }),
      makeCount('2026-05-02', { prompts: 3, artifacts: 0, total: 3 }),
    ]
    const totalsNow = totalsFromStats(makeStats())

    const series = buildCumulativeSeries(counts, totalsNow)

    // Last day equals today's known totals; earlier days subtract later creations.
    expect(series).toHaveLength(2)
    expect(series[1]).toMatchObject({
      date: '2026-05-02',
      prompts: 10,
      artifacts: 5,
      total: 15,
    })
    expect(series[0]).toMatchObject({
      date: '2026-05-01',
      prompts: 7, // 10 − 3 created on 05-02
      artifacts: 5, // 5 − 0
      total: 12,
    })
  })

  it('clamps cumulative counts at zero when creations exceed known totals', () => {
    const counts = [
      makeCount('2026-05-01', { prompts: 0, total: 0 }),
      makeCount('2026-05-02', { prompts: 50, total: 50 }),
    ]
    const totalsNow = totalsFromStats(makeStats({ total_prompts: 10 }))

    const series = buildCumulativeSeries(counts, totalsNow)

    // 10 − 50 would be negative; clamp to 0 rather than render a negative bar.
    expect(series[0].prompts).toBe(0)
    expect(series[1].prompts).toBe(10)
  })
})

describe('TeamResourceCumulativeChart', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('fetches creation metrics + stats and renders the grand total', async () => {
    mockGetTeamResourceCreationMetrics.mockResolvedValue(
      buildMetrics([makeCount('2026-05-02', { prompts: 3, total: 3 })])
    )
    mockGetTeamStats.mockResolvedValue(makeStats())

    render(<TeamResourceCumulativeChart teamId="team-123" range="30d" />)

    await waitFor(() => {
      // grand total = 10 prompts + 5 artifacts
      expect(screen.getByText('15')).toBeInTheDocument()
    })
    expect(mockGetTeamResourceCreationMetrics).toHaveBeenCalledWith(
      'team-123',
      '30d',
      expect.any(AbortSignal)
    )
    expect(mockGetTeamStats).toHaveBeenCalledWith('team-123')
  })

  it('does not render its own range selector (page owns the filter)', async () => {
    mockGetTeamResourceCreationMetrics.mockResolvedValue(
      buildMetrics([makeCount('2026-05-02', { prompts: 1, total: 1 })])
    )
    mockGetTeamStats.mockResolvedValue(makeStats())

    render(<TeamResourceCumulativeChart teamId="team-123" range="30d" />)

    await waitFor(() => {
      expect(screen.getByText('15')).toBeInTheDocument()
    })
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
  })
})
