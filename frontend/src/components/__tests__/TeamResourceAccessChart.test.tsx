import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'

import type { ResourceAccessMetricsResponse } from '../../services/resourceAccessService'

const mockGetTeamResourceAccessMetrics = jest.fn()

jest.mock('../../services/teamService', () => ({
  teamService: {
    getTeamResourceAccessMetrics: (...args: unknown[]) =>
      mockGetTeamResourceAccessMetrics(...args),
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

import { TeamResourceAccessChart } from '../TeamResourceAccessChart'

function buildResponse(
  overrides: Partial<ResourceAccessMetricsResponse['data']> = {}
): ResourceAccessMetricsResponse {
  return {
    status: 'success',
    message: 'ok',
    data: {
      total_accesses: 4,
      range: '30d',
      counts: [
        { date: '2026-05-01', web: 3, cli: 1, mcp: 0, api: 0, total: 4 },
      ],
      ...overrides,
    },
  }
}

describe('TeamResourceAccessChart', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('fetches team-wide access metrics with the controlled range and renders the total', async () => {
    mockGetTeamResourceAccessMetrics.mockResolvedValue(buildResponse())

    render(<TeamResourceAccessChart teamId="team-123" range="30d" />)

    await waitFor(() => {
      expect(screen.getByText('4')).toBeInTheDocument()
    })

    expect(mockGetTeamResourceAccessMetrics).toHaveBeenCalledWith(
      'team-123',
      '30d',
      expect.any(AbortSignal)
    )
  })

  it('does not render its own range selector (page owns the filter)', async () => {
    mockGetTeamResourceAccessMetrics.mockResolvedValue(buildResponse())

    render(<TeamResourceAccessChart teamId="team-123" range="30d" />)

    await waitFor(() => {
      expect(screen.getByText('4')).toBeInTheDocument()
    })
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
  })

  // Regression guard for #1797: the stacked access chart must surface a legend
  // so users can tell the Web/CLI/MCP/API segments apart (previously the colors
  // only appeared in the hover tooltip).
  it('renders a clickable legend for every access source', async () => {
    mockGetTeamResourceAccessMetrics.mockResolvedValue(buildResponse())

    render(<TeamResourceAccessChart teamId="team-123" range="30d" />)

    for (const label of ['Web', 'CLI', 'MCP', 'API']) {
      expect(
        await screen.findByRole('button', { name: label })
      ).toBeInTheDocument()
    }

    const webToggle = screen.getByRole('button', { name: 'Web' })
    expect(webToggle).toHaveAttribute('aria-pressed', 'true')
    fireEvent.click(webToggle)
    expect(webToggle).toHaveAttribute('aria-pressed', 'false')
  })

  it('renders the empty state when total_accesses is zero', async () => {
    mockGetTeamResourceAccessMetrics.mockResolvedValue(
      buildResponse({ total_accesses: 0, counts: [] })
    )

    render(<TeamResourceAccessChart teamId="team-123" range="30d" />)

    expect(await screen.findByText('No activity yet')).toBeInTheDocument()
  })
})
