import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { Team } from '@/services/teamService'

// `usePermissions` is deliberately NOT mocked — the gate is exercised through
// the real hook against the `permissions` array on these fixtures, which is
// what the server publishes (#224). It reads `useAuth` too, hence that mock.
const mockUseTeam = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

vi.mock('@/services/teamSettingsAuditService', () => ({
  teamSettingsAuditService: { getAudit: vi.fn() },
}))

import type { TeamSettingsAuditEntry } from '@/services/teamSettingsAuditService'
import { teamSettingsAuditService } from '@/services/teamSettingsAuditService'

import { SettingsAudit } from '../SettingsAudit'

const mocked = vi.mocked(teamSettingsAuditService)

const ownerTeam = {
  id: 'team-1',
  name: 'Test Team',
  permissions: ['team.settings.update'],
} as unknown as Team

const memberTeam = {
  id: 'team-1',
  name: 'Test Team',
  permissions: ['resource.create'],
} as unknown as Team

const entry = (
  overrides: Partial<TeamSettingsAuditEntry> = {}
): TeamSettingsAuditEntry => ({
  id: 'a1',
  surface: 'model_provider',
  actor_user_id: '11111111-2222-3333-4444-555555555555',
  actor_name: 'Ada Lovelace',
  source_team_id: '99999999-8888-7777-6666-555555555555',
  source_team_name: 'Platform Team',
  source_resource_id: '22222222-3333-4444-5555-666666666666',
  created_resource_id: '33333333-4444-5555-6666-777777777777',
  detail: {
    source_name: 'OpenAI (Platform)',
    created_name: 'OpenAI (Platform)',
    has_api_key: true,
  },
  created_at: '2026-08-10T12:00:00Z',
  ...overrides,
})

const page = (
  entries: TeamSettingsAuditEntry[],
  totalPages = 1,
  total = entries.length
) => ({
  entries,
  total_count: total,
  page: 1,
  per_page: 20,
  total_pages: totalPages,
})

beforeEach(() => {
  vi.clearAllMocks()
  mockUseTeam.mockReturnValue({
    currentTeam: ownerTeam,
    teams: [ownerTeam],
    isLoading: false,
    setCurrentTeam: vi.fn(),
    refreshTeams: vi.fn(),
  })
  mocked.getAudit.mockResolvedValue(page([entry()]))
})

describe('SettingsAudit permission gate', () => {
  it('renders the log for a team granting team.settings.update', async () => {
    render(<SettingsAudit team={ownerTeam} />)

    expect(await screen.findByTestId('settings-audit-row')).toBeInTheDocument()
  })

  it('refuses to render or fetch without team.settings.update', async () => {
    render(<SettingsAudit team={memberTeam} />)

    expect(
      await screen.findByTestId('settings-audit-forbidden')
    ).toBeInTheDocument()
    expect(screen.queryByTestId('settings-audit-row')).not.toBeInTheDocument()
    expect(mocked.getAudit).not.toHaveBeenCalled()
  })

  // #584 / the issue's load-bearing instruction: the gate must key on the team
  // the URL resolved, not the ambient one. Driving the two apart is the only
  // way to catch a `RequirePermission`-style ambient read.
  it('gates on the URL team, not the ambient one', async () => {
    mockUseTeam.mockReturnValue({
      currentTeam: ownerTeam,
      teams: [ownerTeam],
      isLoading: false,
      setCurrentTeam: vi.fn(),
      refreshTeams: vi.fn(),
    })
    const urlTeamB = {
      id: 'team-b',
      name: 'Team B',
      permissions: [],
    } as unknown as Team

    render(<SettingsAudit team={urlTeamB} />)

    expect(
      await screen.findByTestId('settings-audit-forbidden')
    ).toBeInTheDocument()
    expect(mocked.getAudit).not.toHaveBeenCalled()
  })

  it('reads the URL team when it is the permitted one', async () => {
    mockUseTeam.mockReturnValue({
      currentTeam: { id: 'team-a', name: 'Team A', permissions: [] },
      teams: [],
      isLoading: false,
      setCurrentTeam: vi.fn(),
      refreshTeams: vi.fn(),
    })
    const urlTeamB = {
      id: 'team-b',
      name: 'Team B',
      permissions: ['team.settings.update'],
    } as unknown as Team

    render(<SettingsAudit team={urlTeamB} />)

    await waitFor(() => {
      expect(mocked.getAudit).toHaveBeenCalledWith(
        'team-b',
        1,
        20,
        expect.anything()
      )
    })
  })
})

describe('SettingsAudit content', () => {
  it('shows the empty state when nothing has been copied in', async () => {
    mocked.getAudit.mockResolvedValue(page([]))

    render(<SettingsAudit team={ownerTeam} />)

    expect(
      await screen.findByTestId('settings-audit-empty')
    ).toBeInTheDocument()
    expect(screen.queryByTestId('settings-audit-row')).not.toBeInTheDocument()
  })

  it('names the surface, the resource and the source team on a copy row', async () => {
    render(<SettingsAudit team={ownerTeam} />)

    const row = await screen.findByTestId('settings-audit-row')
    expect(within(row).getByText('Model provider')).toBeInTheDocument()
    expect(within(row).getByText('OpenAI (Platform)')).toBeInTheDocument()
    expect(within(row).getByText(/from Platform Team/)).toBeInTheDocument()
    expect(within(row).getByText('Ada Lovelace')).toBeInTheDocument()
    // Whether a credential moved is the fact worth auditing.
    expect(within(row).getByTestId('carried-credential')).toBeInTheDocument()
  })

  it('names the copied types for a custom_types copy, which has no resource id', async () => {
    mocked.getAudit.mockResolvedValue(
      page([
        entry({
          surface: 'custom_types',
          source_resource_id: null,
          created_resource_id: null,
          detail: {
            added_ids: ['t1', 't2'],
            added_slugs: ['design-doc', 'runbook'],
            skipped_slugs: ['bug-report'],
          },
        }),
      ])
    )

    render(<SettingsAudit team={ownerTeam} />)

    const row = await screen.findByTestId('settings-audit-row')
    expect(within(row).getByText('Artifact types')).toBeInTheDocument()
    expect(
      within(row).getByText('2 types: design-doc, runbook')
    ).toBeInTheDocument()
    expect(
      within(row).queryByTestId('carried-credential')
    ).not.toBeInTheDocument()
  })

  // Epic #827 grows the surface enum server-side, and the frontend's copy of it
  // is bumped by hand — so a build can meet a surface it has no label for. A
  // blank What cell in the compensating control would read as "nothing
  // happened", which is the one thing this page must never say.
  it('falls back to the raw surface when the server sends an unmapped one', async () => {
    mocked.getAudit.mockResolvedValue(
      page([
        entry({
          surface: 'something_new' as TeamSettingsAuditEntry['surface'],
        }),
      ])
    )

    render(<SettingsAudit team={ownerTeam} />)

    const row = await screen.findByTestId('settings-audit-row')
    expect(within(row).getByText('something_new')).toBeInTheDocument()
    expect(within(row).getByText('OpenAI (Platform)')).toBeInTheDocument()
  })

  // The source team carries no foreign key by design, so it can be deleted out
  // from under an entry. The row must still read as a record of what happened.
  it('still renders a row whose source team has been deleted', async () => {
    mocked.getAudit.mockResolvedValue(page([entry({ source_team_name: null })]))

    render(<SettingsAudit team={ownerTeam} />)

    const row = await screen.findByTestId('settings-audit-row')
    expect(within(row).getByText(/from a deleted team/)).toBeInTheDocument()
    expect(within(row).getByText('OpenAI (Platform)')).toBeInTheDocument()
  })

  it('still renders a row whose actor has deleted their account', async () => {
    mocked.getAudit.mockResolvedValue(
      page([entry({ actor_user_id: null, actor_name: null })])
    )

    render(<SettingsAudit team={ownerTeam} />)

    const row = await screen.findByTestId('settings-audit-row')
    expect(within(row).getByText('Deleted user')).toBeInTheDocument()
  })
})

describe('SettingsAudit paging', () => {
  it('requests the first page and hides the controls on a single page', async () => {
    render(<SettingsAudit team={ownerTeam} />)

    await screen.findByTestId('settings-audit-row')
    expect(mocked.getAudit).toHaveBeenCalledWith(
      'team-1',
      1,
      20,
      expect.anything()
    )
    expect(
      screen.queryByRole('button', { name: 'Next' })
    ).not.toBeInTheDocument()
  })

  it('asks the endpoint for the next page', async () => {
    mocked.getAudit.mockResolvedValue(page([entry()], 3, 45))
    const user = userEvent.setup()

    render(<SettingsAudit team={ownerTeam} />)

    await screen.findByTestId('settings-audit-row')
    expect(screen.getByText(/Page 1 of 3/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()

    await user.click(screen.getByRole('button', { name: 'Next' }))

    await waitFor(() => {
      expect(mocked.getAudit).toHaveBeenCalledWith(
        'team-1',
        2,
        20,
        expect.anything()
      )
    })
  })

  it('keeps the table and controls when a later page fails', async () => {
    mocked.getAudit.mockResolvedValue(page([entry()], 3, 45))
    const user = userEvent.setup()

    render(<SettingsAudit team={ownerTeam} />)
    await screen.findByTestId('settings-audit-row')

    mocked.getAudit.mockRejectedValueOnce(new Error('page out of range'))
    await user.click(screen.getByRole('button', { name: 'Next' }))

    expect(await screen.findByText('page out of range')).toBeInTheDocument()
    expect(screen.getByTestId('settings-audit-row')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Previous' })).toBeEnabled()
  })

  it('replaces the view when the very first load fails', async () => {
    mocked.getAudit.mockRejectedValue(new Error('boom'))

    render(<SettingsAudit team={ownerTeam} />)

    expect(await screen.findByText('boom')).toBeInTheDocument()
    expect(screen.queryByTestId('settings-audit-row')).not.toBeInTheDocument()
  })
})
