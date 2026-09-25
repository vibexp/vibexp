/**
 * AdminTeamDetail (#316, #1142): renders the team, owner and member list, the
 * error state (e.g. a 404 for an unknown id), and the `?tab=` shell around the
 * read-only configuration tabs.
 */
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, RouterProvider } from 'react-router'
import type { Mocked } from 'vitest'

import type { AdminTeamDetail as AdminTeamDetailType } from '@/services/adminService'

vi.mock('@/services/adminService', () => ({
  adminService: { getTeam: vi.fn() },
}))

// The self-fetching configuration tabs are covered by their own suites; here
// they are stubbed so the shell's tab routing is observable without requests.
const stubTab = vi.hoisted(
  () => (testId: string) =>
    function StubTab({ teamId }: { teamId: string }) {
      return (
        <div data-testid={testId}>
          {testId} {teamId}
        </div>
      )
    }
)
vi.mock('@/pages/admin/teams/detail/TeamSearchConfigTab', () => ({
  TeamSearchConfigTab: stubTab('search-tab'),
}))
vi.mock('@/pages/admin/teams/detail/TeamAISummaryConfigTab', () => ({
  TeamAISummaryConfigTab: stubTab('ai-summary-tab'),
}))
vi.mock('@/pages/admin/teams/detail/TeamFreshnessConfigTab', () => ({
  TeamFreshnessConfigTab: stubTab('freshness-tab'),
}))
vi.mock('@/pages/admin/teams/detail/TeamModelProvidersTab', () => ({
  TeamModelProvidersTab: stubTab('model-providers-tab'),
}))
vi.mock('@/pages/admin/teams/detail/TeamEmbeddingProvidersTab', () => ({
  TeamEmbeddingProvidersTab: stubTab('embedding-providers-tab'),
}))
vi.mock('@/pages/admin/teams/detail/TeamEmailConfigTab', () => ({
  TeamEmailConfigTab: stubTab('email-tab'),
}))
vi.mock('@/pages/admin/teams/detail/TeamGitHubConfigTab', () => ({
  TeamGitHubConfigTab: stubTab('github-tab'),
}))
vi.mock('@/pages/admin/teams/detail/TeamArtifactTypesTab', () => ({
  TeamArtifactTypesTab: stubTab('artifact-types-tab'),
}))
vi.mock('@/pages/admin/teams/detail/TeamSettingsAuditTab', () => ({
  TeamSettingsAuditTab: stubTab('settings-audit-tab'),
}))

import { adminService } from '@/services/adminService'

import { AdminTeamDetail } from '../AdminTeamDetail'

const mockAdminService = adminService as Mocked<typeof adminService>

const team: AdminTeamDetailType = {
  id: 't1',
  name: 'Engineering',
  slug: 'engineering',
  is_personal: false,
  owner: { id: 'o1', email: 'owner@example.com', name: 'Owner' },
  created_at: '2026-01-01T00:00:00Z',
  members: [
    {
      user_id: 'u1',
      email: 'alice@example.com',
      name: 'Alice',
      role: 'owner',
      joined_at: '2026-01-02T00:00:00Z',
    },
  ],
}

const CONFIG_TAB_IDS = [
  'search-tab',
  'ai-summary-tab',
  'freshness-tab',
  'model-providers-tab',
  'embedding-providers-tab',
  'email-tab',
  'github-tab',
  'artifact-types-tab',
  'settings-audit-tab',
]

function renderDetail(search = '') {
  const router = createMemoryRouter(
    [{ path: '/admin/teams/:id', element: <AdminTeamDetail /> }],
    { initialEntries: [`/admin/teams/t1${search}`] }
  )
  render(<RouterProvider router={router} />)
  return router
}

beforeEach(() => {
  mockAdminService.getTeam.mockResolvedValue(team)
})

afterEach(() => {
  vi.clearAllMocks()
})

it('renders the team, owner, and members', async () => {
  renderDetail()

  expect(
    await screen.findByRole('heading', { name: 'Engineering' })
  ).toBeInTheDocument()
  expect(screen.getByText('owner@example.com')).toBeInTheDocument()
  expect(screen.getByText('alice@example.com')).toBeInTheDocument()
  expect(mockAdminService.getTeam).toHaveBeenCalledWith('t1')
})

it('says so when the team has no members', async () => {
  mockAdminService.getTeam.mockResolvedValue({ ...team, members: [] })
  renderDetail()

  expect(
    await screen.findByText('This team has no members.')
  ).toBeInTheDocument()
})

it('shows an error state when the team is not found', async () => {
  mockAdminService.getTeam.mockRejectedValue(new Error('404'))
  renderDetail()

  expect(await screen.findByText('Failed to load team')).toBeInTheDocument()
})

describe('tabs (#1142)', () => {
  it('opens on Members and mounts no configuration tab', async () => {
    renderDetail()

    expect(
      await screen.findByRole('heading', { name: 'Members' })
    ).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Members' })).toHaveAttribute(
      'data-state',
      'active'
    )
    expect(screen.getAllByRole('tab')).toHaveLength(10)
    for (const testId of CONFIG_TAB_IDS) {
      expect(screen.queryByTestId(testId)).not.toBeInTheDocument()
    }
  })

  it('opens the tab named in ?tab=', async () => {
    renderDetail('?tab=email')

    expect(await screen.findByTestId('email-tab')).toHaveTextContent(
      'email-tab t1'
    )
    expect(screen.getByRole('tab', { name: 'Email' })).toHaveAttribute(
      'data-state',
      'active'
    )
    expect(
      screen.queryByRole('heading', { name: 'Members' })
    ).not.toBeInTheDocument()
  })

  it('falls back to Members for an unknown ?tab=', async () => {
    renderDetail('?tab=bogus')

    expect(
      await screen.findByRole('heading', { name: 'Members' })
    ).toBeInTheDocument()
  })

  it('mounts only the opened tab, keeps the header and replaces the URL', async () => {
    const router = renderDetail()
    await screen.findByRole('heading', { name: 'Members' })

    await userEvent.click(screen.getByRole('tab', { name: 'GitHub' }))
    expect(screen.getByTestId('github-tab')).toBeInTheDocument()
    expect(
      screen.queryByRole('heading', { name: 'Members' })
    ).not.toBeInTheDocument()
    for (const testId of CONFIG_TAB_IDS.filter(t => t !== 'github-tab')) {
      expect(screen.queryByTestId(testId)).not.toBeInTheDocument()
    }
    expect(router.state.location.search).toBe('?tab=github')
    expect(router.state.historyAction).toBe('REPLACE')
    expect(
      screen.getByRole('heading', { name: 'Engineering' })
    ).toBeInTheDocument()
    expect(screen.getByText('owner@example.com')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('tab', { name: 'Settings audit' }))
    expect(screen.getByTestId('settings-audit-tab')).toBeInTheDocument()
    expect(screen.queryByTestId('github-tab')).not.toBeInTheDocument()
  })
})
