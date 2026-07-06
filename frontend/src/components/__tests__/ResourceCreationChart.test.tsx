import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'

import type { ResourceCreationMetricsResponse } from '../../services/resourceCreationService'

const mockGetResourceCreationMetrics = jest.fn()

jest.mock('../../services/resourceCreationService', () => ({
  resourceCreationService: {
    getResourceCreationMetrics: (...args: unknown[]) =>
      mockGetResourceCreationMetrics(...args),
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

import { ResourceCreationChart } from '../ResourceCreationChart'

const props = {
  teamId: 'team-123',
  slug: 'my-project',
}

function buildResponse(
  overrides: Partial<ResourceCreationMetricsResponse['data']> = {}
): ResourceCreationMetricsResponse {
  return {
    status: 'success',
    message: 'ok',
    data: {
      total_created: 6,
      range: '30d',
      counts: [
        {
          date: '2026-05-01',
          prompts: 3,
          artifacts: 1,
          blueprints: 0,
          memories: 2,
          total: 6,
        },
      ],
      ...overrides,
    },
  }
}

describe('ResourceCreationChart', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('shows a loading skeleton before data resolves', () => {
    let resolve: (value: ResourceCreationMetricsResponse) => void = () => {}
    mockGetResourceCreationMetrics.mockReturnValue(
      new Promise<ResourceCreationMetricsResponse>(r => {
        resolve = r
      })
    )

    render(<ResourceCreationChart {...props} />)

    expect(screen.getByText('Resources created')).toBeInTheDocument()
    expect(
      screen.queryByText('No resources created yet')
    ).not.toBeInTheDocument()

    resolve(buildResponse())
  })

  it('fetches with the project props and renders the total once data returns', async () => {
    mockGetResourceCreationMetrics.mockResolvedValue(buildResponse())

    render(<ResourceCreationChart {...props} />)

    await waitFor(() => {
      expect(screen.getByText('6')).toBeInTheDocument()
    })

    expect(mockGetResourceCreationMetrics).toHaveBeenCalledWith(
      props.teamId,
      props.slug,
      '30d',
      expect.any(AbortSignal)
    )
    expect(
      screen.queryByText('No resources created yet')
    ).not.toBeInTheDocument()
  })

  it('renders the empty state when total_created is zero', async () => {
    mockGetResourceCreationMetrics.mockResolvedValue(
      buildResponse({ total_created: 0, counts: [] })
    )

    render(<ResourceCreationChart {...props} />)

    expect(
      await screen.findByText('No resources created yet')
    ).toBeInTheDocument()
  })

  it('renders a distinct error state when the fetch fails', async () => {
    const consoleError = jest
      .spyOn(console, 'error')
      .mockImplementation(() => {})
    mockGetResourceCreationMetrics.mockRejectedValue(new Error('boom'))

    render(<ResourceCreationChart {...props} />)

    expect(
      await screen.findByText(/couldn't load resource creation/i)
    ).toBeInTheDocument()
    expect(consoleError).toHaveBeenCalled()
    consoleError.mockRestore()
  })

  it('shows a clickable legend that toggles a series', async () => {
    mockGetResourceCreationMetrics.mockResolvedValue(buildResponse())

    render(<ResourceCreationChart {...props} />)

    const promptsToggle = await screen.findByRole('button', { name: 'Prompts' })
    expect(promptsToggle).toHaveAttribute('aria-pressed', 'true')

    fireEvent.click(promptsToggle)

    expect(promptsToggle).toHaveAttribute('aria-pressed', 'false')
  })
})
