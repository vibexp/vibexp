import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Team, TeamMember } from '@/services/teamService'

jest.mock('@/services/teamService', () => ({
  teamService: {
    getTeamDetails: jest.fn(),
    getTeamMembers: jest.fn(),
    getTeamInvitations: jest.fn(),
    updateMemberRole: jest.fn(),
    removeMember: jest.fn(),
  },
}))

const mockRefreshTeams = jest.fn()
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({ refreshTeams: mockRefreshTeams }),
}))

jest.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'admin-1' } }),
}))

const mockToastError = jest.fn()
jest.mock('@/lib/toast', () => ({
  toast: {
    success: jest.fn(),
    error: (...args: unknown[]) => mockToastError(...args),
  },
}))

import { teamService } from '@/services/teamService'

import { TeamDetailsPage } from '../TeamDetailsPage'

// Radix Select needs these in jsdom.
beforeAll(() => {
  Element.prototype.scrollIntoView = jest.fn()
  Element.prototype.hasPointerCapture = jest.fn()
  Element.prototype.releasePointerCapture = jest.fn()
})

// The caller is an admin: they may update roles, but not delete or transfer.
const adminTeam: Team = {
  id: 'team-1',
  owner_id: 'owner-1',
  name: 'Engineering',
  slug: 'engineering',
  description: '',
  is_personal: false,
  permissions: ['member.role.update', 'member.remove', 'member.invite'],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

const members: TeamMember[] = [
  {
    user_id: 'admin-1',
    email: 'admin@example.com',
    name: 'Adam',
    role: 'admin',
    joined_at: '2024-01-01T00:00:00Z',
  },
  {
    user_id: 'user-2',
    email: 'bob@example.com',
    name: 'Bob',
    role: 'member',
    joined_at: '2024-01-01T00:00:00Z',
  },
]

const mocked = teamService as jest.Mocked<typeof teamService>

beforeEach(() => {
  jest.clearAllMocks()
  mocked.getTeamDetails.mockResolvedValue(adminTeam)
  mocked.getTeamMembers.mockResolvedValue(members)
  mocked.getTeamInvitations.mockResolvedValue([])
  mocked.updateMemberRole.mockResolvedValue(undefined)
})

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/settings/teams/team-1']}>
      <Routes>
        <Route path="/settings/teams/:id" element={<TeamDetailsPage />} />
      </Routes>
    </MemoryRouter>
  )
}

async function changeRole(memberName: string, to: string) {
  const user = userEvent.setup()
  await user.click(
    screen.getByRole('combobox', {
      name: new RegExp(`change role for ${memberName}`, 'i'),
    })
  )
  await user.click(screen.getByRole('option', { name: to }))
}

describe('TeamDetailsPage — role management (#225)', () => {
  it('sends the role change to the API', async () => {
    renderPage()
    await screen.findByText('Bob')

    await changeRole('bob', 'Admin')

    await waitFor(() => {
      expect(mocked.updateMemberRole).toHaveBeenCalledWith(
        'team-1',
        'user-2',
        'admin'
      )
    })
  })

  it("does not refetch after changing someone else's role", async () => {
    // The optimistic row is the whole update, and loadTeamDetails swaps the page
    // for a skeleton — refetching here would flash it away for nothing.
    renderPage()
    await screen.findByText('Bob')
    const detailCallsAfterLoad = mocked.getTeamDetails.mock.calls.length

    await changeRole('bob', 'Admin')

    await waitFor(() => {
      expect(mocked.updateMemberRole).toHaveBeenCalled()
    })
    expect(mocked.getTeamDetails).toHaveBeenCalledTimes(detailCallsAfterLoad)
    expect(mockRefreshTeams).not.toHaveBeenCalled()
  })

  it('resyncs this page and the team list when the caller demotes themselves', async () => {
    // The backend guards only the owner's role, so an admin CAN demote
    // themselves. Their permissions just changed: without a resync the SPA goes
    // on offering admin actions that now 403.
    renderPage()
    await screen.findByText('Adam')
    const detailCallsAfterLoad = mocked.getTeamDetails.mock.calls.length

    await changeRole('adam', 'Member')

    await waitFor(() => {
      expect(mocked.updateMemberRole).toHaveBeenCalledWith(
        'team-1',
        'admin-1',
        'member'
      )
    })
    await waitFor(() => {
      expect(mocked.getTeamDetails.mock.calls.length).toBeGreaterThan(
        detailCallsAfterLoad
      )
    })
    expect(mockRefreshTeams).toHaveBeenCalled()
  })

  it('reverts the row when the API rejects the change', async () => {
    mocked.updateMemberRole.mockRejectedValue(new Error('nope'))
    renderPage()
    await screen.findByText('Bob')

    await changeRole('bob', 'Admin')

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith('nope')
    })
    // Back to what the server still believes.
    await waitFor(() => {
      expect(
        screen.getByRole('combobox', { name: /change role for bob/i })
      ).toHaveTextContent('Member')
    })
  })

  it('hides role dropdowns entirely without member.role.update', async () => {
    mocked.getTeamDetails.mockResolvedValue({ ...adminTeam, permissions: [] })
    renderPage()
    await screen.findByText('Bob')

    expect(
      screen.queryByRole('combobox', { name: /change role for bob/i })
    ).not.toBeInTheDocument()
  })
})
