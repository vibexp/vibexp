/**
 * AdminProjectDetail (#461): metadata, team, creator, resource counts, and the
 * not-found path; plus the Overview / Configuration tabs (#1146).
 */
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import type { Mocked } from 'vitest'

import type { AdminProjectDetail as AdminProjectDetailType } from '@/services/adminService'

vi.mock('@/services/adminService', () => ({
  adminService: { getProject: vi.fn() },
}))

// The self-fetching tabs are covered by their own suites; here they are stubbed
// so the shell's tab routing is observable without their requests.
const tabStubs = vi.hoisted(() => ({
  overview: function OverviewStub({ projectId }: { projectId: string }) {
    return <div data-testid="overview-tab">overview {projectId}</div>
  },
  config: function ConfigStub({
    projectId,
    projectName,
    teamId,
  }: {
    projectId: string
    projectName: string
    teamId: string
  }) {
    return (
      <div data-testid="config-tab">
        config {projectId} {projectName} {teamId}
      </div>
    )
  },
}))
vi.mock('@/pages/admin/projects/detail/ProjectOverviewTab', () => ({
  ProjectOverviewTab: tabStubs.overview,
}))
vi.mock('@/pages/admin/projects/detail/ProjectConfigTab', () => ({
  ProjectConfigTab: tabStubs.config,
}))

import { adminService } from '@/services/adminService'

import { AdminProjectDetail } from '../AdminProjectDetail'

const mockAdminService = adminService as Mocked<typeof adminService>

function detail(
  overrides: Partial<AdminProjectDetailType> = {}
): AdminProjectDetailType {
  return {
    id: 'p1',
    name: 'Platform',
    slug: 'platform',
    description: 'Core platform work',
    git_url: 'https://github.com/acme/platform',
    homepage: 'https://platform.acme.dev',
    team: { id: 't1', name: 'Engineering', slug: 'engineering' },
    owner: { id: 'u1', email: 'creator@example.com', name: 'Creator' },
    // Distinct values per type: a shared number would let a transposed mapping
    // pass unnoticed, since all four fields are same-typed.
    resource_counts: {
      prompts: 12,
      artifacts: 4,
      memories: 27,
      blueprints: 3,
      feed_items: 8,
      total: 54,
    },
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-02-03T00:00:00Z',
    ...overrides,
  }
}

function renderDetail(id = 'p1', search = '') {
  return render(
    <MemoryRouter initialEntries={[`/admin/projects/${id}${search}`]}>
      <Routes>
        <Route path="/admin/projects/:id" element={<AdminProjectDetail />} />
      </Routes>
    </MemoryRouter>
  )
}

beforeEach(() => {
  vi.clearAllMocks()
})

it('renders the project with its team, creator and metadata', async () => {
  mockAdminService.getProject.mockResolvedValue(detail())
  renderDetail()

  expect(
    await screen.findByRole('heading', { name: 'Platform' })
  ).toBeInTheDocument()
  expect(screen.getByText('platform')).toBeInTheDocument()
  expect(screen.getByText('Core platform work')).toBeInTheDocument()
  expect(
    screen.getByText('https://github.com/acme/platform')
  ).toBeInTheDocument()
  expect(screen.getByText('https://platform.acme.dev')).toBeInTheDocument()
  expect(mockAdminService.getProject).toHaveBeenCalledWith('p1')
})

it('links the team and the creator to their own admin pages', async () => {
  mockAdminService.getProject.mockResolvedValue(detail())
  renderDetail()

  expect(
    await screen.findByRole('link', { name: 'Engineering' })
  ).toHaveAttribute('href', '/admin/teams/t1')
  // The creator (projects.user_id), not the team's owner — separate links because
  // they are separate people in the data model.
  expect(
    screen.getByRole('link', { name: 'creator@example.com' })
  ).toHaveAttribute('href', '/admin/users/u1')
})

it('renders each resource count against its own label', async () => {
  mockAdminService.getProject.mockResolvedValue(detail())
  renderDetail()

  await screen.findByRole('heading', { name: 'Platform' })
  // Labelled, never the raw API key (#1143 added feed_items and total).
  expect(screen.queryByText('feed_items')).not.toBeInTheDocument()
  expect(screen.queryByText('total')).not.toBeInTheDocument()
  for (const [label, value] of [
    ['Prompts', '12'],
    ['Artifacts', '4'],
    ['Memories', '27'],
    ['Blueprints', '3'],
    ['Feed items', '8'],
    ['Total resources', '54'],
  ]) {
    const cell = screen.getByText(label).parentElement
    expect(cell).not.toBeNull()
    expect(cell).toHaveTextContent(value)
  }
})

it('reports no agent or feed counts, because those are not project-scoped', async () => {
  mockAdminService.getProject.mockResolvedValue(detail())
  renderDetail()

  await screen.findByRole('heading', { name: 'Platform' })
  // #453 omits them rather than reporting 0 — a zero would read as "no agents"
  // when the truth is "agents do not belong to projects". The panel is driven by
  // the response, so it must not invent them either.
  expect(screen.queryByText('Agents')).not.toBeInTheDocument()
  expect(screen.queryByText('Feeds')).not.toBeInTheDocument()
})

it('renders a resource type the API adds later, without a code change', async () => {
  mockAdminService.getProject.mockResolvedValue(
    detail({
      resource_counts: {
        ...detail().resource_counts,
        // Cast: the point is that the panel is driven by the response keys, so a
        // type the current schema does not know still surfaces.
        widgets: 9,
      } as AdminProjectDetailType['resource_counts'],
    })
  )
  renderDetail()

  await screen.findByRole('heading', { name: 'Platform' })
  expect(screen.getByText('widgets')).toBeInTheDocument()
  expect(screen.getByText('9')).toBeInTheDocument()
})

it('shows an em dash for the empty-string columns rather than a blank', async () => {
  mockAdminService.getProject.mockResolvedValue(
    detail({ description: '', git_url: '', homepage: '' })
  )
  renderDetail()

  await screen.findByRole('heading', { name: 'Platform' })
  // The API documents these as empty strings when unset, not null.
  expect(screen.getAllByText('—')).toHaveLength(3)
})

it('shows the error state for an unknown id', async () => {
  mockAdminService.getProject.mockRejectedValue(new Error('404'))
  renderDetail('missing')

  expect(await screen.findByText('Failed to load project')).toBeInTheDocument()
  // No half-rendered shell behind the error.
  expect(
    screen.queryByRole('heading', { name: 'Platform' })
  ).not.toBeInTheDocument()
})

it('shows a skeleton while loading', () => {
  mockAdminService.getProject.mockReturnValue(new Promise(() => undefined))
  renderDetail()

  expect(screen.getByTestId('detail-skeleton')).toBeInTheDocument()
})

it('offers a way back to the list', async () => {
  mockAdminService.getProject.mockResolvedValue(detail())
  renderDetail()

  expect(
    await screen.findByRole('link', { name: 'Back to projects' })
  ).toHaveAttribute('href', '/admin/projects')
})

describe('tabs (#1146)', () => {
  it('opens on Overview and does not mount Configuration', async () => {
    mockAdminService.getProject.mockResolvedValue(detail())
    renderDetail()

    expect(await screen.findByTestId('overview-tab')).toHaveTextContent(
      'overview p1'
    )
    expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute(
      'data-state',
      'active'
    )
    expect(screen.queryByTestId('config-tab')).not.toBeInTheDocument()
  })

  it('mounts Configuration only when it is opened, keeping the counts above', async () => {
    mockAdminService.getProject.mockResolvedValue(detail())
    renderDetail()
    await screen.findByTestId('overview-tab')

    await userEvent.click(screen.getByRole('tab', { name: 'Configuration' }))
    expect(screen.getByTestId('config-tab')).toHaveTextContent(
      'config p1 Platform t1'
    )
    expect(screen.queryByTestId('overview-tab')).not.toBeInTheDocument()
    expect(screen.getByText('Total resources')).toBeInTheDocument()
  })

  it('opens the tab named in ?tab=', async () => {
    mockAdminService.getProject.mockResolvedValue(detail())
    renderDetail('p1', '?tab=configuration')

    expect(await screen.findByTestId('config-tab')).toBeInTheDocument()
    expect(screen.queryByTestId('overview-tab')).not.toBeInTheDocument()
  })

  it('falls back to Overview for an unknown ?tab=', async () => {
    mockAdminService.getProject.mockResolvedValue(detail())
    renderDetail('p1', '?tab=bogus')

    expect(await screen.findByTestId('overview-tab')).toBeInTheDocument()
    expect(screen.queryByTestId('config-tab')).not.toBeInTheDocument()
  })
})
