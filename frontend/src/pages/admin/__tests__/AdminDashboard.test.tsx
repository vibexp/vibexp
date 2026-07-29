/**
 * AdminDashboard (#458): the four metric groups, the shared range/granularity
 * control, and — the point of the two-fetch design — per-panel failure isolation.
 */
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Mocked } from 'vitest'

import type {
  AdminDashboardOverview,
  AdminTimeseriesResponse,
} from '@/services/adminService'

vi.mock('@/services/adminService', () => ({
  adminService: {
    getDashboardOverview: vi.fn(),
    getDashboardTimeseries: vi.fn(),
  },
}))

import { adminService } from '@/services/adminService'

import { AdminDashboard } from '../AdminDashboard'

const mockAdminService = adminService as Mocked<typeof adminService>

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

function overview(
  o: Partial<AdminDashboardOverview> = {}
): AdminDashboardOverview {
  return {
    counts: {
      users: 42,
      teams: 7,
      projects: 30,
      prompts: 340,
      artifacts: 128,
      memories: 512,
      blueprints: 64,
      agents: 9,
      feeds: 4,
      api_keys: 17,
    },
    breakdowns: [
      {
        entity: 'prompts',
        field: 'status',
        buckets: [
          { value: 'published', count: 300 },
          { value: 'draft', count: 40 },
        ],
      },
      {
        entity: 'users',
        field: 'status',
        // The API reports NULL as an empty string.
        buckets: [{ value: '', count: 2 }],
      },
    ],
    system_health: {
      database_size_bytes: 184549376,
      tables: [
        { table: 'prompts', estimated_rows: 34012 },
        { table: 'memories', estimated_rows: 512 },
      ],
    },
    version: '1.2.3',
    ...o,
  }
}

function timeseries(
  o: Partial<AdminTimeseriesResponse> = {}
): AdminTimeseriesResponse {
  return {
    from: '2026-06-25T00:00:00Z',
    to: '2026-07-25T00:00:00Z',
    granularity: 'day',
    growth: [
      {
        bucket: '2026-07-01T00:00:00Z',
        users: 2,
        teams: 1,
        projects: 0,
        prompts: 5,
        artifacts: 0,
        memories: 3,
      },
      {
        bucket: '2026-07-02T00:00:00Z',
        users: 0,
        teams: 0,
        projects: 0,
        prompts: 0,
        artifacts: 0,
        memories: 0,
      },
    ],
    sign_ins: [
      { bucket: '2026-07-01T00:00:00Z', count: 12 },
      { bucket: '2026-07-02T00:00:00Z', count: 0 },
    ],
    access_by_source: [
      { bucket: '2026-07-01T00:00:00Z', source: 'web', count: 30 },
      { bucket: '2026-07-01T00:00:00Z', source: 'mcp', count: 12 },
      { bucket: '2026-07-02T00:00:00Z', source: 'web', count: 4 },
      { bucket: '2026-07-02T00:00:00Z', source: 'mcp', count: 0 },
    ],
    data_window: {
      sign_ins_earliest_retained_at: '2026-04-26T00:00:00Z',
      access_by_source_earliest_retained_at: '2026-04-26T00:00:00Z',
    },
    ...o,
  }
}

const lastSeriesQuery = () => {
  const { calls } = mockAdminService.getDashboardTimeseries.mock
  return calls[calls.length - 1][0]
}

beforeEach(() => {
  vi.clearAllMocks()
  mockAdminService.getDashboardOverview.mockResolvedValue(overview())
  mockAdminService.getDashboardTimeseries.mockResolvedValue(timeseries())
})

it('renders all four metric groups', async () => {
  render(<AdminDashboard />)

  // Totals
  expect(await screen.findByText('42')).toBeInTheDocument()
  expect(screen.getByText('API keys')).toBeInTheDocument()
  // Breakdowns
  expect(screen.getByText('Prompts by status')).toBeInTheDocument()
  expect(screen.getByText('published')).toBeInTheDocument()
  // Growth + activity
  expect(screen.getByText('New entities')).toBeInTheDocument()
  expect(screen.getByText('Sign-ins')).toBeInTheDocument()
  expect(screen.getByText('Resource access by source')).toBeInTheDocument()
  // System health
  expect(screen.getByText('Largest tables')).toBeInTheDocument()
  expect(screen.getByText('176 MiB')).toBeInTheDocument()
  expect(screen.getByText(/Backend version 1\.2\.3/)).toBeInTheDocument()
})

it('names a NULL breakdown value rather than rendering a blank row', async () => {
  render(<AdminDashboard />)

  await screen.findByText('Users by status')
  // The API reports NULL as '', which would otherwise be an unlabelled bar.
  expect(screen.getByText('Not set')).toBeInTheDocument()
})

it('labels estimated row counts as estimates', async () => {
  render(<AdminDashboard />)

  await screen.findByText('Largest tables')
  // The figure comes from pg_stat_user_tables, not COUNT(*) — presenting it as
  // exact would be a number an admin might act on.
  expect(
    screen.getByText(/Estimated row counts from table statistics/)
  ).toBeInTheDocument()
  expect(screen.getByText('~34,012')).toBeInTheDocument()
})

it('sends no range on first load, letting the API apply its own default', async () => {
  render(<AdminDashboard />)

  await waitFor(() => {
    expect(mockAdminService.getDashboardTimeseries).toHaveBeenCalled()
  })
  expect(lastSeriesQuery()).toEqual({
    from: undefined,
    to: undefined,
    granularity: 'day',
  })
})

it('refetches the series when the granularity changes, and only the series', async () => {
  render(<AdminDashboard />)
  await screen.findByText('42')
  expect(mockAdminService.getDashboardOverview).toHaveBeenCalledTimes(1)

  await userEvent.click(screen.getByRole('combobox', { name: 'Bucket size' }))
  await userEvent.click(await screen.findByRole('option', { name: 'Weekly' }))

  await waitFor(() => {
    expect(lastSeriesQuery().granularity).toBe('week')
  })
  // The totals do not depend on the range, so they are not refetched.
  expect(mockAdminService.getDashboardOverview).toHaveBeenCalledTimes(1)
})

it('refetches the series when the range changes', async () => {
  render(<AdminDashboard />)
  await screen.findByText('42')

  await userEvent.click(
    screen.getByRole('button', { name: /Dashboard date range/ })
  )
  await userEvent.click(
    await screen.findByRole('button', { name: 'Last 7 days' })
  )

  await waitFor(() => {
    expect(lastSeriesQuery().from).toBeDefined()
  })
  const query = lastSeriesQuery()
  // Local midnight to local end-of-day, serialized once at the service boundary.
  expect(new Date(query.from!).getHours()).toBe(0)
  expect(new Date(query.to!).getHours()).toBe(23)
})

it('has exactly one range control on the page', async () => {
  render(<AdminDashboard />)
  await screen.findByText('New entities')

  // TimeSeriesBarChart ships its own range select; with three charts on the page
  // that would be four competing controls, so they run in hidden-control mode.
  expect(
    screen.getAllByRole('button', { name: /Dashboard date range/ })
  ).toHaveLength(1)
  expect(
    screen.queryByRole('combobox', { name: /Last 7 days|range/i })
  ).not.toBeInTheDocument()
})

describe('failure isolation', () => {
  it('keeps the totals and health panels when the series request fails', async () => {
    mockAdminService.getDashboardTimeseries.mockRejectedValue(
      new Error('range exceeds the maximum span')
    )
    render(<AdminDashboard />)

    // The whole point of two independent fetches: a broken chart must not blank
    // the page an admin came to read.
    expect(await screen.findByText('42')).toBeInTheDocument()
    expect(screen.getByText('Largest tables')).toBeInTheDocument()
    // And the API's own explanation is shown, not a generic failure.
    expect(
      await screen.findAllByText('range exceeds the maximum span')
    ).not.toHaveLength(0)
  })

  it('keeps the charts when the overview request fails', async () => {
    mockAdminService.getDashboardOverview.mockRejectedValue(
      new Error('overview exploded')
    )
    render(<AdminDashboard />)

    expect(
      await screen.findByText('Failed to load instance totals')
    ).toBeInTheDocument()
    expect(screen.getByText('overview exploded')).toBeInTheDocument()
    // Charts unaffected.
    expect(screen.getByText('New entities')).toBeInTheDocument()
  })

  it('clears a previous error once a later request succeeds', async () => {
    mockAdminService.getDashboardTimeseries
      .mockRejectedValueOnce(new Error('transient'))
      .mockResolvedValueOnce(timeseries())
    render(<AdminDashboard />)
    await screen.findAllByText('transient')

    await userEvent.click(screen.getByRole('combobox', { name: 'Bucket size' }))
    await userEvent.click(
      await screen.findByRole('option', { name: 'Monthly' })
    )

    await waitFor(() => {
      expect(screen.queryByText('transient')).not.toBeInTheDocument()
    })
  })
})

it('renders empty states rather than broken charts for an empty range', async () => {
  mockAdminService.getDashboardTimeseries.mockResolvedValue(
    timeseries({ growth: [], sign_ins: [], access_by_source: [] })
  )
  render(<AdminDashboard />)

  expect(
    await screen.findByText('Nothing was created in this range.')
  ).toBeInTheDocument()
  expect(screen.getByText('No sign-ins in this range.')).toBeInTheDocument()
  expect(
    screen.getByText('No resource access recorded in this range.')
  ).toBeInTheDocument()
})

it('explains the retention window on both activity panels', async () => {
  render(<AdminDashboard />)

  // Awaited on the notes themselves: the chart titles render while the series is
  // still in flight, so waiting for "Sign-ins" would race the data.
  const notes = await screen.findAllByText(/are retained from/)
  expect(notes).toHaveLength(2)
  expect(notes[0].textContent).toContain('Sign-ins are retained from')
  expect(notes[1].textContent).toContain('Access events are retained from')
})

it('reports no breakdowns rather than an empty grid', async () => {
  mockAdminService.getDashboardOverview.mockResolvedValue(
    overview({ breakdowns: [] })
  )
  render(<AdminDashboard />)

  expect(
    await screen.findByText(
      'No status or type columns to break down on this instance.'
    )
  ).toBeInTheDocument()
})

it('handles a breakdown whose buckets are all empty', async () => {
  mockAdminService.getDashboardOverview.mockResolvedValue(
    overview({
      breakdowns: [{ entity: 'agents', field: 'status', buckets: [] }],
      // A zero total would make the proportional bars divide by zero.
    })
  )
  render(<AdminDashboard />)

  expect(await screen.findByText('Agents by status')).toBeInTheDocument()
  expect(screen.getByText('No rows yet.')).toBeInTheDocument()
})

it('reports no table statistics rather than an empty list', async () => {
  mockAdminService.getDashboardOverview.mockResolvedValue(
    overview({
      system_health: { database_size_bytes: 0, tables: [] },
    })
  )
  render(<AdminDashboard />)

  expect(
    await screen.findByText('No table statistics available yet.')
  ).toBeInTheDocument()
  expect(screen.getByText('0 B')).toBeInTheDocument()
})

it('shows skeletons while the overview is in flight', () => {
  mockAdminService.getDashboardOverview.mockReturnValue(
    new Promise(() => undefined)
  )
  render(<AdminDashboard />)

  expect(screen.getAllByTestId('stat-skeleton')).toHaveLength(10)
})
