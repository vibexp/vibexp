import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router'

import type {
  AdminProjectDetail,
  AdminTopAccessedResource,
} from '@/services/adminService'

const svc = vi.hoisted(() => ({
  getProjectResourceCreationMetrics: vi.fn(),
  getProjectResourceAccessMetrics: vi.fn(),
  getProjectTopAccessedResources: vi.fn(),
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

import { ProjectOverviewTab } from '../detail/ProjectOverviewTab'

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

const COUNTS: AdminProjectDetail['resource_counts'] = {
  prompts: 12,
  artifacts: 4,
  memories: 27,
  blueprints: 3,
  feed_items: 8,
  total: 54,
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

function renderTab(counts = COUNTS) {
  return render(
    <MemoryRouter>
      <ProjectOverviewTab projectId="p1" resourceCounts={counts} />
    </MemoryRouter>
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  svc.getProjectResourceCreationMetrics.mockResolvedValue(
    range({
      series: [
        {
          bucket: '2026-09-20T00:00:00Z',
          prompts: 2,
          memories: 1,
          artifacts: 0,
          blueprints: 0,
          feed_items: 0,
        },
      ],
    })
  )
  svc.getProjectResourceAccessMetrics.mockResolvedValue(
    range({
      access_by_source: [
        { bucket: '2026-09-20T00:00:00Z', source: 'mcp', count: 7 },
      ],
      earliest_retained_at: '2026-06-01T00:00:00Z',
    })
  )
  svc.getProjectTopAccessedResources.mockResolvedValue(
    range({
      granularity: undefined,
      items: [TOP],
      earliest_retained_at: '2026-06-01T00:00:00Z',
    })
  )
})

it('renders the breakdown from the counts, the charts and the top table', async () => {
  renderTab()

  expect(screen.getByText('By type')).toBeInTheDocument()
  // Every project-scoped type, labelled; the total is the headline, not a row.
  for (const label of [
    'Prompts',
    'Memories',
    'Artifacts',
    'Blueprints',
    'Feed items',
  ]) {
    expect(screen.getAllByText(label).length).toBeGreaterThan(0)
  }
  expect(screen.getByText('Resources created')).toBeInTheDocument()
  expect(screen.getByText('Resource access by source')).toBeInTheDocument()
  expect(await screen.findByText('3f2a9c1e')).toBeInTheDocument()
  expect(screen.getByText('42')).toBeInTheDocument()
  expect(
    screen.getByText(/Access events are retained from/)
  ).toBeInTheDocument()
})

it('sends the same range to all three range-bound requests', async () => {
  renderTab()
  await screen.findByText('3f2a9c1e')

  // No range picked yet: no bounds, so the API applies its default.
  const initial = { from: undefined, to: undefined, granularity: 'day' }
  expect(svc.getProjectResourceCreationMetrics).toHaveBeenCalledWith(
    'p1',
    initial
  )
  expect(svc.getProjectResourceAccessMetrics).toHaveBeenCalledWith(
    'p1',
    initial
  )
  expect(svc.getProjectTopAccessedResources).toHaveBeenCalledWith('p1', {
    from: undefined,
    to: undefined,
  })
})

it('refetches every range-bound panel when the granularity changes', async () => {
  renderTab()
  await screen.findByText('3f2a9c1e')

  await userEvent.click(screen.getByRole('combobox', { name: 'Bucket size' }))
  await userEvent.click(await screen.findByRole('option', { name: 'Weekly' }))

  const week = { from: undefined, to: undefined, granularity: 'week' }
  await waitFor(() => {
    expect(svc.getProjectResourceCreationMetrics).toHaveBeenLastCalledWith(
      'p1',
      week
    )
  })
  expect(svc.getProjectResourceAccessMetrics).toHaveBeenLastCalledWith(
    'p1',
    week
  )
  expect(svc.getProjectTopAccessedResources).toHaveBeenLastCalledWith('p1', {
    from: undefined,
    to: undefined,
  })
  expect(svc.getProjectResourceCreationMetrics).toHaveBeenCalledTimes(2)
  expect(svc.getProjectResourceAccessMetrics).toHaveBeenCalledTimes(2)
  expect(svc.getProjectTopAccessedResources).toHaveBeenCalledTimes(2)
})

it('refetches every range-bound panel with identical bounds when a preset is picked', async () => {
  renderTab()
  await screen.findByText('3f2a9c1e')

  await userEvent.click(
    screen.getByRole('button', { name: /Project activity date range/ })
  )
  await userEvent.click(await screen.findByRole('button', { name: /7 days/ }))

  await waitFor(() => {
    expect(svc.getProjectResourceCreationMetrics).toHaveBeenCalledTimes(2)
  })
  const [, creationParams] = svc.getProjectResourceCreationMetrics.mock
    .lastCall as [string, { from?: string; to?: string; granularity: string }]
  const [, accessParams] = svc.getProjectResourceAccessMetrics.mock
    .lastCall as [string, { from?: string; to?: string }]
  const [, topParams] = svc.getProjectTopAccessedResources.mock.lastCall as [
    string,
    { from?: string; to?: string },
  ]
  expect(creationParams.from).toEqual(expect.any(String))
  expect(creationParams.to).toEqual(expect.any(String))
  expect(creationParams.granularity).toBe('day')
  expect(accessParams).toEqual(creationParams)
  expect(topParams).toEqual({
    from: creationParams.from,
    to: creationParams.to,
  })
})

it('keeps the other panels when one series fails', async () => {
  svc.getProjectResourceAccessMetrics.mockRejectedValue(
    new Error('access series down')
  )
  renderTab()

  expect(await screen.findByText('access series down')).toBeInTheDocument()
  expect(await screen.findByText('3f2a9c1e')).toBeInTheDocument()
  expect(screen.getByText('Resources created')).toBeInTheDocument()
  expect(screen.getByText('By type')).toBeInTheDocument()
})

it('shows the top-accessed error on its own', async () => {
  svc.getProjectTopAccessedResources.mockRejectedValue(new Error('top down'))
  renderTab()

  expect(
    await screen.findByText('Failed to load the most accessed resources')
  ).toBeInTheDocument()
  expect(screen.getByText('top down')).toBeInTheDocument()
})

it('shows the empty states', async () => {
  svc.getProjectTopAccessedResources.mockResolvedValue(
    range({ items: [], earliest_retained_at: '2026-06-01T00:00:00Z' })
  )
  renderTab({
    prompts: 0,
    artifacts: 0,
    memories: 0,
    blueprints: 0,
    feed_items: 0,
    total: 0,
  })

  expect(
    await screen.findByText('No resource access recorded in this range.')
  ).toBeInTheDocument()
  expect(
    screen.getByText('This project has no resources yet.')
  ).toBeInTheDocument()
})

it('never renders a resource title, and drops the constant location columns', async () => {
  svc.getProjectTopAccessedResources.mockResolvedValue(
    range({
      items: [{ ...TOP, title: 'Secret roadmap prompt' }],
      earliest_retained_at: '2026-06-01T00:00:00Z',
    })
  )
  renderTab()

  await screen.findByText('3f2a9c1e')
  expect(screen.queryByText(/Secret roadmap prompt/)).not.toBeInTheDocument()
  expect(
    screen.queryByRole('columnheader', { name: 'Team' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('columnheader', { name: 'Project' })
  ).not.toBeInTheDocument()
  expect(screen.queryByText('Engineering')).not.toBeInTheDocument()
  expect(screen.queryByText('Core')).not.toBeInTheDocument()
})
