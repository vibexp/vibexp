import { render, waitFor } from '@testing-library/react'

import type { TimeSeriesBarChartProps } from '@/components/TimeSeriesBarChart'

// Capture the props handed to the shared chart so we can assert SessionsChart's
// data transformation (API newest-first → ascending, RFC3339 date → YYYY-MM-DD,
// total mirrored from count) without depending on recharts internals.
const captured: { props?: TimeSeriesBarChartProps } = {}
jest.mock('@/components/TimeSeriesBarChart', () => ({
  TimeSeriesBarChart: (props: TimeSeriesBarChartProps) => {
    captured.props = props
    return <div data-testid="ts-chart" />
  },
}))

const mockGetSessionCounts = jest.fn()
jest.mock('@/utils/api', () => ({
  apiClient: {
    getSessionCounts: (...args: unknown[]) => mockGetSessionCounts(...args),
  },
}))

import { SessionsChart } from '../SessionsChart'

describe('SessionsChart', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    captured.props = undefined
  })

  it('reverses to ascending dates, trims RFC3339 to YYYY-MM-DD, and mirrors total', async () => {
    mockGetSessionCounts.mockResolvedValue({
      data: {
        total_sessions: 5,
        // The sessions endpoint returns newest-first DATEs serialised as RFC3339.
        counts: [
          { date: '2026-05-02T00:00:00Z', count: 2 },
          { date: '2026-05-01T00:00:00Z', count: 3 },
        ],
      },
    })

    render(<SessionsChart range="30d" onRangeChange={jest.fn()} />)

    await waitFor(() => {
      expect(captured.props?.data.length).toBe(2)
    })

    expect(captured.props?.total).toBe(5)
    expect(captured.props?.legend).toBe('none')
    expect(captured.props?.data).toEqual([
      { date: '2026-05-01', count: 3, total: 3 },
      { date: '2026-05-02', count: 2, total: 2 },
    ])
  })

  it('surfaces a zero total and empty data when the fetch fails', async () => {
    const consoleError = jest
      .spyOn(console, 'error')
      .mockImplementation(() => {})
    mockGetSessionCounts.mockRejectedValue(new Error('boom'))

    render(<SessionsChart range="30d" onRangeChange={jest.fn()} />)

    await waitFor(() => {
      expect(captured.props?.error).toBe(true)
    })
    expect(captured.props?.total).toBe(0)
    expect(captured.props?.data).toEqual([])
    consoleError.mockRestore()
  })
})
