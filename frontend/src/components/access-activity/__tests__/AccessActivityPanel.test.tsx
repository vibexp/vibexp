import { render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'

import type { ResourceAccessMetricsResponse } from '@/types'

const mockGetResourceAccessMetrics = jest.fn()

jest.mock('../../../services/resourceAccessService', () => ({
  resourceAccessService: {
    getResourceAccessMetrics: (...args: unknown[]) =>
      mockGetResourceAccessMetrics(...args),
  },
}))

// recharts' ResponsiveContainer measures its parent, which has no layout in
// jsdom. Render children at a fixed size so the chart mounts deterministically.
jest.mock('recharts', () => {
  const actual = jest.requireActual('recharts')
  return {
    ...actual,
    ResponsiveContainer: ({ children }: { children: ReactNode }) => (
      <div style={{ width: 400, height: 110 }}>{children}</div>
    ),
  }
})

import { AccessActivityPanel } from '../AccessActivityPanel'

const props = {
  teamId: 'team-123',
  resourceType: 'artifact' as const,
  resourceId: '7c9e6679-7425-40de-944b-e07fc1f90ae7',
}

function buildResponse(
  overrides: Partial<ResourceAccessMetricsResponse['data']> = {}
): ResourceAccessMetricsResponse {
  return {
    status: 'success',
    message: 'ok',
    data: {
      total_accesses: 22,
      range: '30d',
      counts: [
        { date: '2026-05-01', web: 12, cli: 4, mcp: 4, api: 2, total: 22 },
      ],
      ...overrides,
    },
  }
}

describe('AccessActivityPanel', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('fetches with the resource props and renders the total once data returns', async () => {
    mockGetResourceAccessMetrics.mockResolvedValue(buildResponse())

    render(<AccessActivityPanel {...props} />)

    await waitFor(() => {
      expect(screen.getByText('22')).toBeInTheDocument()
    })
    expect(mockGetResourceAccessMetrics).toHaveBeenCalledWith(
      props.teamId,
      props.resourceType,
      props.resourceId,
      '30d',
      expect.any(AbortSignal)
    )
  })

  it('renders the per-channel breakdown from the fetched counts', async () => {
    mockGetResourceAccessMetrics.mockResolvedValue(buildResponse())

    render(<AccessActivityPanel {...props} />)

    for (const label of ['Web', 'CLI', 'MCP', 'API']) {
      expect(await screen.findByText(label)).toBeInTheDocument()
    }
    expect(await screen.findByText('12')).toBeInTheDocument()
  })

  it('renders the empty state when total_accesses is zero', async () => {
    mockGetResourceAccessMetrics.mockResolvedValue(
      buildResponse({ total_accesses: 0, counts: [] })
    )

    render(<AccessActivityPanel {...props} />)

    expect(await screen.findByText('No activity yet')).toBeInTheDocument()
  })

  it('renders the error state when the fetch fails', async () => {
    const consoleError = jest
      .spyOn(console, 'error')
      .mockImplementation(() => {})
    mockGetResourceAccessMetrics.mockRejectedValue(new Error('boom'))

    render(<AccessActivityPanel {...props} />)

    expect(
      await screen.findByText(/couldn't load access activity/i)
    ).toBeInTheDocument()
    expect(consoleError).toHaveBeenCalled()
    consoleError.mockRestore()
  })
})
