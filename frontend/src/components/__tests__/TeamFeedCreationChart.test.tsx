import { render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'

import type { TeamFeedCreationMetricsResponse } from '../../types'

const mockGetTeamFeedCreationMetrics = jest.fn()

jest.mock('../../services/teamService', () => ({
  teamService: {
    getTeamFeedCreationMetrics: (...args: unknown[]) =>
      mockGetTeamFeedCreationMetrics(...args),
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

import { TeamFeedCreationChart } from '../TeamFeedCreationChart'

function buildResponse(
  overrides: Partial<TeamFeedCreationMetricsResponse['data']> = {}
): TeamFeedCreationMetricsResponse {
  return {
    status: 'success',
    message: 'ok',
    data: {
      total_created: 9,
      range: '30d',
      counts: [
        { date: '2026-05-01', feeds: 1, feed_items: 4, total: 5 },
        { date: '2026-05-02', feeds: 0, feed_items: 3, total: 3 },
      ],
      ...overrides,
    },
  }
}

describe('TeamFeedCreationChart', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('fetches with the controlled range and totals feed items only (not channels)', async () => {
    mockGetTeamFeedCreationMetrics.mockResolvedValue(buildResponse())

    render(<TeamFeedCreationChart teamId="team-123" range="30d" />)

    await waitFor(() => {
      // 4 + 3 feed items; the new feed channel is excluded from the header total.
      expect(screen.getByText('7')).toBeInTheDocument()
    })
    expect(mockGetTeamFeedCreationMetrics).toHaveBeenCalledWith(
      'team-123',
      '30d',
      expect.any(AbortSignal)
    )
  })

  it('renders the empty state when there is no feed activity', async () => {
    mockGetTeamFeedCreationMetrics.mockResolvedValue(
      buildResponse({ total_created: 0, counts: [] })
    )

    render(<TeamFeedCreationChart teamId="team-123" range="30d" />)

    expect(await screen.findByText('No feed activity yet')).toBeInTheDocument()
  })
})
