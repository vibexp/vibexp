import { render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'

import type { TeamResourceCreationMetricsResponse } from '../../services/teamService'

const mockGetTeamResourceCreationMetrics = jest.fn()

jest.mock('../../services/teamService', () => ({
  teamService: {
    getTeamResourceCreationMetrics: (...args: unknown[]) =>
      mockGetTeamResourceCreationMetrics(...args),
  },
}))

// recharts' ResponsiveContainer measures its parent, which has no layout in
// jsdom. Render children with a fixed size so the chart mounts deterministically.
jest.mock('recharts', () => {
  const actual = jest.requireActual('recharts')
  return {
    ...actual,
    ResponsiveContainer: ({ children }: { children: ReactNode }) => (
      <div style={{ width: 400, height: 180 }}>{children}</div>
    ),
  }
})

import { TeamResourceCreationChart } from '../TeamResourceCreationChart'

function buildResponse(
  overrides: Partial<TeamResourceCreationMetricsResponse['data']> = {}
): TeamResourceCreationMetricsResponse {
  return {
    status: 'success',
    message: 'ok',
    data: {
      total_created: 7,
      range: '30d',
      counts: [
        {
          date: '2026-05-01',
          prompts: 3,
          artifacts: 1,
          blueprints: 0,
          memories: 2,
          projects: 1,
          total: 7,
        },
      ],
      ...overrides,
    },
  }
}

describe('TeamResourceCreationChart', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('fetches team-wide metrics with the controlled range and renders the total', async () => {
    mockGetTeamResourceCreationMetrics.mockResolvedValue(buildResponse())

    render(<TeamResourceCreationChart teamId="team-123" range="30d" />)

    await waitFor(() => {
      expect(screen.getByText('7')).toBeInTheDocument()
    })

    expect(mockGetTeamResourceCreationMetrics).toHaveBeenCalledWith(
      'team-123',
      '30d',
      expect.any(AbortSignal)
    )
  })

  it('refetches when the range prop changes (page-level filter)', async () => {
    mockGetTeamResourceCreationMetrics.mockResolvedValue(buildResponse())

    const { rerender } = render(
      <TeamResourceCreationChart teamId="team-123" range="30d" />
    )
    await waitFor(() => {
      expect(mockGetTeamResourceCreationMetrics).toHaveBeenCalledTimes(1)
    })

    rerender(<TeamResourceCreationChart teamId="team-123" range="7d" />)

    await waitFor(() => {
      expect(mockGetTeamResourceCreationMetrics).toHaveBeenLastCalledWith(
        'team-123',
        '7d',
        expect.any(AbortSignal)
      )
    })
  })

  it('does not render its own range selector (page owns the filter)', async () => {
    mockGetTeamResourceCreationMetrics.mockResolvedValue(buildResponse())

    render(<TeamResourceCreationChart teamId="team-123" range="30d" />)

    await waitFor(() => {
      expect(screen.getByText('7')).toBeInTheDocument()
    })
    // The shared shell renders the range dropdown as a combobox; hidden here.
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
  })

  it('renders a clickable legend for the four design resource types', async () => {
    mockGetTeamResourceCreationMetrics.mockResolvedValue(buildResponse())

    render(<TeamResourceCreationChart teamId="team-123" range="30d" />)

    for (const label of ['Artifacts', 'Memories', 'Blueprints', 'Prompts']) {
      expect(
        await screen.findByRole('button', { name: label })
      ).toBeInTheDocument()
    }
    // Projects is intentionally omitted to match the mock.
    expect(
      screen.queryByRole('button', { name: 'Projects' })
    ).not.toBeInTheDocument()
  })

  it('renders the empty state when nothing was created', async () => {
    mockGetTeamResourceCreationMetrics.mockResolvedValue(
      buildResponse({ total_created: 0, counts: [] })
    )

    render(<TeamResourceCreationChart teamId="team-123" range="30d" />)

    expect(
      await screen.findByText('No resources created yet')
    ).toBeInTheDocument()
  })
})
