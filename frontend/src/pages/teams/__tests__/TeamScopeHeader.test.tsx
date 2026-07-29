import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import type { Mocked } from 'vitest'

import type { Team } from '@/services/teamService'

vi.mock('@/services/teamService', () => ({
  teamService: {
    getTeamMembers: vi.fn(),
    inviteMembers: vi.fn(),
  },
}))

vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({ currentTeam: null }),
}))

vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

// The modals have their own suites; stub them to a marker so this file asserts
// what the header decides to open, not what each dialog renders.
vi.mock('../EditTeamModal', () => ({
  EditTeamModal: () => <div data-testid="edit-team-modal" />,
}))
vi.mock('../DeleteTeamModal', () => ({
  DeleteTeamModal: () => <div data-testid="delete-team-modal" />,
}))
vi.mock('../InviteTeamMembersModal', () => ({
  InviteTeamMembersModal: ({ isOpen }: { isOpen: boolean }) =>
    isOpen ? <div data-testid="invite-members-modal" /> : null,
}))
vi.mock('../TransferOwnershipModal', () => ({
  TransferOwnershipModal: ({ isOpen }: { isOpen: boolean }) =>
    isOpen ? <div data-testid="transfer-ownership-modal" /> : null,
}))

import { teamService } from '@/services/teamService'

import { TeamScopeHeader } from '../TeamScopeHeader'

const mocked = teamService as Mocked<typeof teamService>

const ownerPermissions: Team['permissions'] = [
  'team.update',
  'team.delete',
  'team.transfer',
  'member.invite',
]

const makeTeam = (overrides: Partial<Team> = {}): Team => ({
  id: 'team-1',
  owner_id: 'user-1',
  name: 'Engineering',
  slug: 'engineering',
  description: '',
  is_personal: false,
  role: 'owner',
  permissions: ownerPermissions,
  member_count: 2,
  created_at: '2026-07-26T16:48:00Z',
  updated_at: '2026-07-26T16:48:00Z',
  ...overrides,
})

function renderHeader(team: Team = makeTeam(), onTeamChanged = vi.fn()) {
  render(
    <MemoryRouter>
      <TeamScopeHeader team={team} onTeamChanged={onTeamChanged} />
    </MemoryRouter>
  )
  return { onTeamChanged }
}

beforeEach(() => {
  vi.clearAllMocks()
  mocked.getTeamMembers.mockResolvedValue([])
})

describe('TeamScopeHeader — identity (#666)', () => {
  it('renders the team name as the page title, with the role and meta line', () => {
    renderHeader()

    expect(
      screen.getByRole('heading', { level: 1, name: 'Engineering' })
    ).toBeInTheDocument()
    expect(screen.getByText('Owner')).toBeInTheDocument()
    expect(
      screen.getByText(/2 members · created Jul 26, 2026/)
    ).toBeInTheDocument()
  })

  it('links back to the team list', () => {
    renderHeader()

    expect(screen.getByRole('link', { name: /all teams/i })).toHaveAttribute(
      'href',
      '/teams'
    )
  })

  it('omits the member clause when the count is unknown', () => {
    // `member_count` is only populated on list responses, so a team resolved by
    // the layout's deep-link fallback has none — "0 members" would be a lie.
    renderHeader(makeTeam({ member_count: undefined }))

    expect(screen.getByText('created Jul 26, 2026')).toBeInTheDocument()
    expect(screen.queryByText(/\d+ members?/)).not.toBeInTheDocument()
  })

  it('renders no role badge when the server omits the role', () => {
    renderHeader(makeTeam({ role: undefined }))

    expect(screen.queryByText('Owner')).not.toBeInTheDocument()
  })
})

describe('TeamScopeHeader — action gating (#666)', () => {
  it('offers every action to an owner', async () => {
    const user = userEvent.setup()
    renderHeader()

    expect(
      screen.getByRole('button', { name: 'Edit team' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /invite members/i })
    ).toBeInTheDocument()

    await user.click(screen.getByTestId('team-actions-menu'))
    expect(await screen.findByTestId('transfer-ownership-button')).toBeVisible()
    expect(screen.getByTestId('delete-team-button')).toBeVisible()
  })

  it('shows only what the permissions allow', () => {
    renderHeader(makeTeam({ role: 'member', permissions: [] }))

    expect(
      screen.queryByRole('button', { name: 'Edit team' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /invite members/i })
    ).not.toBeInTheDocument()
    // With neither delete nor transfer there is nothing to overflow into.
    expect(screen.queryByTestId('team-actions-menu')).not.toBeInTheDocument()
  })

  it('hides invite, transfer and delete in a personal workspace', () => {
    // A private workspace has nobody to invite or hand it to, and cannot be
    // deleted — regardless of the (owner) permissions it reports.
    renderHeader(makeTeam({ is_personal: true }))

    expect(
      screen.getByRole('button', { name: 'Edit team' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /invite members/i })
    ).not.toBeInTheDocument()
    expect(screen.queryByTestId('team-actions-menu')).not.toBeInTheDocument()
  })
})

describe('TeamScopeHeader — actions (#666)', () => {
  it('opens the invite dialog from the header', async () => {
    const user = userEvent.setup()
    renderHeader()

    expect(screen.queryByTestId('invite-members-modal')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /invite members/i }))

    expect(screen.getByTestId('invite-members-modal')).toBeInTheDocument()
  })

  it('loads the roster only when ownership transfer is opened', async () => {
    const user = userEvent.setup()
    renderHeader()

    // Rendering the header on every team route must not cost a roster request.
    expect(mocked.getTeamMembers).not.toHaveBeenCalled()

    await user.click(screen.getByTestId('team-actions-menu'))
    await user.click(await screen.findByTestId('transfer-ownership-button'))

    await waitFor(() => {
      expect(mocked.getTeamMembers).toHaveBeenCalledWith('team-1')
    })
    expect(
      await screen.findByTestId('transfer-ownership-modal')
    ).toBeInTheDocument()
  })
})
