import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router'

import type {
  AdminTopAccessedResource,
  AdminUserInsights,
} from '@/services/adminService'

const svc = vi.hoisted(() => ({
  getUserInsights: vi.fn(),
  getUserResourceCreationMetrics: vi.fn(),
  getUserResourceAccessMetrics: vi.fn(),
  getUserTopAccessedResources: vi.fn(),
}))
vi.mock('@/services/adminService', () => ({ adminService: svc }))

// recharts' ResponsiveContainer measures its parent, which has no layout in
// jsdom. Render children with a fixed size so the chart mounts deterministically.
vi.mock('recharts', async () => {
  const actual = await vi.importActual('recharts')
  return {
    ...actual,
    ResponsiveContainer: ({ children }: { children: ReactNode }) => (
      <div style={{ width: 400, height: 180 }}>{children}</div>
    ),
  }
})

import { UserOverviewTab } from '../detail/UserOverviewTab'

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

function counts(overrides: Partial<AdminUserInsights['totals']> = {}) {
  return {
    prompts: 0,
    memories: 0,
    artifacts: 0,
    blueprints: 0,
    agents: 0,
    feeds: 0,
    feed_items: 0,
    comments: 0,
    attachments: 0,
    total: 0,
    ...overrides,
  }
}

const INSIGHTS: AdminUserInsights = {
  user_id: 'u1',
  totals: counts({ prompts: 12, memories: 3, total: 15 }),
  teams: [
    {
      team_id: 't1',
      team_name: 'Engineering',
      is_member: true,
      counts: counts({ prompts: 12, memories: 3, total: 15 }),
      projects: [
        {
          project_id: 'p1',
          project_name: 'Core',
          counts: {
            prompts: 12,
            artifacts: 0,
            memories: 3,
            blueprints: 0,
            feed_items: 0,
            total: 15,
          },
        },
      ],
    },
  ],
}

const TOP: AdminTopAccessedResource = {
  resource_type: 'prompt',
  resource_short_id: '3f2a9c1e',
  team_id: 't1',
  team_name: 'Engineering',
  project_id: 'p1',
  project_name: 'Core',
  resource_deleted: false,
  access_count: 42,
}

function range(extra: object = {}) {
  return {
    from: '2026-08-26T00:00:00Z',
    to: '2026-09-25T00:00:00Z',
    granularity: 'day',
    ...extra,
  }
}

function renderTab() {
  return render(
    <MemoryRouter>
      <UserOverviewTab userId="u1" />
    </MemoryRouter>
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  svc.getUserInsights.mockResolvedValue(INSIGHTS)
  svc.getUserResourceCreationMetrics.mockResolvedValue(
    range({
      series: [
        {
          bucket: '2026-09-20T00:00:00Z',
          prompts: 2,
          memories: 0,
          artifacts: 0,
          blueprints: 0,
          agents: 0,
          feeds: 0,
          feed_items: 0,
          comments: 0,
          attachments: 0,
        },
      ],
    })
  )
  svc.getUserResourceAccessMetrics.mockResolvedValue(
    range({
      access_by_source: [
        { bucket: '2026-09-20T00:00:00Z', source: 'mcp', count: 7 },
      ],
      earliest_retained_at: '2026-06-01T00:00:00Z',
    })
  )
  svc.getUserTopAccessedResources.mockResolvedValue(
    range({
      granularity: undefined,
      items: [TOP],
      earliest_retained_at: '2026-06-01T00:00:00Z',
    })
  )
})

it('renders count cards, breakdowns and the top-accessed table', async () => {
  renderTab()

  expect(await screen.findByTestId('count-total')).toHaveTextContent('15')
  expect(screen.getByTestId('count-prompts')).toHaveTextContent('12')
  expect(screen.getByTestId('count-memories')).toHaveTextContent('3')
  expect(screen.getByTestId('count-feed_items')).toHaveTextContent('0')
  expect(svc.getUserInsights).toHaveBeenCalledWith('u1')

  expect(screen.getByText('By type')).toBeInTheDocument()
  expect(screen.getByText('By team')).toBeInTheDocument()
  expect(screen.getByText('By project')).toBeInTheDocument()

  expect(await screen.findByText('3f2a9c1e')).toBeInTheDocument()
  const row = screen.getByText('3f2a9c1e').closest('tr')
  expect(row).not.toBeNull()
  expect(within(row as HTMLElement).getByText('42')).toBeInTheDocument()
  expect(
    within(row as HTMLElement).getByRole('link', { name: 'Engineering' })
  ).toHaveAttribute('href', '/admin/teams/t1')
  expect(
    screen.getByText(/Access events are retained from/)
  ).toBeInTheDocument()
})

it('sends the same range to all three range-bound requests', async () => {
  renderTab()
  await screen.findByText('3f2a9c1e')

  // No range picked yet: no bounds, so the API applies its default.
  const initial = { from: undefined, to: undefined, granularity: 'day' }
  expect(svc.getUserResourceCreationMetrics).toHaveBeenCalledWith('u1', initial)
  expect(svc.getUserResourceAccessMetrics).toHaveBeenCalledWith('u1', initial)
  expect(svc.getUserTopAccessedResources).toHaveBeenCalledWith('u1', {
    from: undefined,
    to: undefined,
  })
})

it('refetches every range-bound panel when the granularity changes', async () => {
  renderTab()
  await screen.findByText('3f2a9c1e')

  await userEvent.click(screen.getByRole('combobox', { name: 'Bucket size' }))
  await userEvent.click(await screen.findByRole('option', { name: 'Weekly' }))

  await waitFor(() => {
    expect(svc.getUserResourceCreationMetrics).toHaveBeenLastCalledWith('u1', {
      from: undefined,
      to: undefined,
      granularity: 'week',
    })
  })
  expect(svc.getUserResourceAccessMetrics).toHaveBeenLastCalledWith('u1', {
    from: undefined,
    to: undefined,
    granularity: 'week',
  })
  expect(svc.getUserResourceCreationMetrics).toHaveBeenCalledTimes(2)
  expect(svc.getUserResourceAccessMetrics).toHaveBeenCalledTimes(2)
  expect(svc.getUserTopAccessedResources).toHaveBeenCalledTimes(2)
  // Insights are range-independent and are not refetched.
  expect(svc.getUserInsights).toHaveBeenCalledTimes(1)
})

it('refetches every range-bound panel with identical bounds when a preset is picked', async () => {
  renderTab()
  await screen.findByText('3f2a9c1e')

  await userEvent.click(
    screen.getByRole('button', { name: /Activity date range/ })
  )
  await userEvent.click(await screen.findByRole('button', { name: /7 days/ }))

  await waitFor(() => {
    expect(svc.getUserResourceCreationMetrics).toHaveBeenCalledTimes(2)
  })
  const [, creationParams] = svc.getUserResourceCreationMetrics.mock
    .lastCall as [string, { from?: string; to?: string }]
  const [, accessParams] = svc.getUserResourceAccessMetrics.mock.lastCall as [
    string,
    { from?: string; to?: string },
  ]
  const [, topParams] = svc.getUserTopAccessedResources.mock.lastCall as [
    string,
    { from?: string; to?: string },
  ]
  expect(creationParams.from).toEqual(expect.any(String))
  expect(creationParams.to).toEqual(expect.any(String))
  expect(accessParams).toEqual(creationParams)
  expect(topParams).toEqual({
    from: creationParams.from,
    to: creationParams.to,
  })
})

it('keeps the other panels when one series fails', async () => {
  svc.getUserResourceAccessMetrics.mockRejectedValue(
    new Error('access series down')
  )
  renderTab()

  expect(await screen.findByText('access series down')).toBeInTheDocument()
  expect(await screen.findByText('3f2a9c1e')).toBeInTheDocument()
  expect(screen.getByTestId('count-total')).toHaveTextContent('15')
  expect(screen.getByText('Resources created')).toBeInTheDocument()
})

it('shows the insights error and the top-accessed error independently', async () => {
  svc.getUserInsights.mockRejectedValue(new Error('counts down'))
  svc.getUserTopAccessedResources.mockRejectedValue(new Error('top down'))
  renderTab()

  expect(
    await screen.findByText('Failed to load resource counts')
  ).toBeInTheDocument()
  expect(
    await screen.findByText('Failed to load the most accessed resources')
  ).toBeInTheDocument()
  expect(screen.getByText('top down')).toBeInTheDocument()
})

it('shows the empty state for top-accessed', async () => {
  svc.getUserTopAccessedResources.mockResolvedValue(
    range({ items: [], earliest_retained_at: '2026-06-01T00:00:00Z' })
  )
  renderTab()

  expect(
    await screen.findAllByText('No resource access recorded in this range.')
  ).not.toHaveLength(0)
})

it('never renders a resource title from a top-accessed row', async () => {
  svc.getUserTopAccessedResources.mockResolvedValue(
    range({
      items: [{ ...TOP, title: 'Secret roadmap prompt' }],
      earliest_retained_at: '2026-06-01T00:00:00Z',
    })
  )
  renderTab()

  await screen.findByText('3f2a9c1e')
  expect(screen.queryByText(/Secret roadmap prompt/)).not.toBeInTheDocument()
})

it('flags a deleted resource without naming it', async () => {
  svc.getUserTopAccessedResources.mockResolvedValue(
    range({
      items: [{ ...TOP, resource_deleted: true }],
      earliest_retained_at: '2026-06-01T00:00:00Z',
    })
  )
  renderTab()

  expect(await screen.findByText('(deleted)')).toBeInTheDocument()
})
