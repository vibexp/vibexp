import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { PendingTeamInvitation, Team } from '@/services/teamService'

jest.mock('@/services/teamService', () => ({
  teamService: {
    getTeams: jest.fn(),
    getPendingInvitations: jest.fn(),
    rejectInvitation: jest.fn(),
  },
}))

// The accept flow (accept → refresh teams → switch → navigate → toast) lives
// in useAcceptAndEnterTeam and is not this page's logic — stub the hook and
// assert only the page's reaction to its typed result.
const mockAcceptAndEnterTeam = jest.fn()
jest.mock('@/hooks/useAcceptAndEnterTeam', () => ({
  useAcceptAndEnterTeam: () => mockAcceptAndEnterTeam,
}))

const mockHandleError = jest.fn()
jest.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))

jest.mock('@/lib/toast', () => ({
  toast: {
    info: jest.fn(),
    success: jest.fn(),
    error: jest.fn(),
  },
}))

jest.mock('@/components/invitations/invitationEvents', () => ({
  emitInvitationsChanged: jest.fn(),
}))

// CreateTeamModal has its own service wiring (create + refresh); probe it so
// this suite only asserts the open/close/success plumbing from the page.
jest.mock('../CreateTeamModal', () => ({
  CreateTeamModal: ({
    isOpen,
    onClose,
    onSuccess,
  }: {
    isOpen: boolean
    onClose: () => void
    onSuccess: () => void
  }) =>
    isOpen ? (
      <div data-testid="create-team-modal">
        <button type="button" onClick={onClose}>
          probe-modal-close
        </button>
        <button type="button" onClick={onSuccess}>
          probe-modal-success
        </button>
      </div>
    ) : null,
}))

import { emitInvitationsChanged } from '@/components/invitations/invitationEvents'
import { toast } from '@/lib/toast'
import { teamService } from '@/services/teamService'

import { Teams } from '../Teams'

function buildTeam(overrides: Partial<Team> = {}): Team {
  return {
    id: 'team-1',
    owner_id: 'user-1',
    name: 'Engineering',
    slug: 'engineering',
    description: 'Core engineering group',
    is_personal: false,
    role: 'owner',
    permissions: [],
    member_count: 5,
    created_at: '2026-01-15T10:30:00Z',
    updated_at: '2026-01-20T14:45:00Z',
    ...overrides,
  }
}

function buildInvitation(
  overrides: Partial<PendingTeamInvitation> = {}
): PendingTeamInvitation {
  return {
    id: 'inv-1',
    token: 'token-abc',
    team_id: 'team-9',
    team_name: 'Acme Corp',
    status: 'pending',
    invited_by: { name: 'Alice', email: 'alice@example.com' },
    created_at: '2026-02-01T00:00:00Z',
    expires_at: '2026-03-01T00:00:00Z',
    ...overrides,
  }
}

function renderTeams() {
  return render(
    <MemoryRouter initialEntries={['/settings/teams']}>
      <Routes>
        <Route path="/settings/teams" element={<Teams />} />
        <Route
          path="/settings/teams/:id"
          element={<div data-testid="team-details-probe">Team details</div>}
        />
      </Routes>
    </MemoryRouter>
  )
}

describe('Teams page', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    ;(teamService.getTeams as jest.Mock).mockResolvedValue([])
    ;(teamService.getPendingInvitations as jest.Mock).mockResolvedValue([])
  })

  describe('data states', () => {
    it('shows loading skeletons while the fetches are in flight', () => {
      ;(teamService.getTeams as jest.Mock).mockImplementation(
        () => new Promise(() => undefined)
      )
      ;(teamService.getPendingInvitations as jest.Mock).mockImplementation(
        () => new Promise(() => undefined)
      )

      const { container } = renderTeams()

      expect(
        container.querySelectorAll('.animate-pulse').length
      ).toBeGreaterThan(0)
      expect(screen.queryByText('Your Teams')).not.toBeInTheDocument()
    })

    it('renders the team list with role badges — role is display-only, no action gating on this page', async () => {
      ;(teamService.getTeams as jest.Mock).mockResolvedValue([
        buildTeam(),
        buildTeam({
          id: 'team-2',
          name: 'Design',
          description: '',
          role: 'admin',
          member_count: 3,
        }),
        buildTeam({
          id: 'team-3',
          name: 'Docs',
          description: '',
          role: 'member',
          member_count: 2,
        }),
      ])

      renderTeams()

      await screen.findByText('Engineering')
      expect(screen.getByText('Core engineering group')).toBeInTheDocument()
      const engineeringRow = screen
        .getByText('Engineering')
        .closest('tr') as HTMLElement
      expect(within(engineeringRow).getByText('Owner')).toBeInTheDocument()
      expect(within(engineeringRow).getByText('5')).toBeInTheDocument()
      const designRow = screen.getByText('Design').closest('tr') as HTMLElement
      expect(within(designRow).getByText('Admin')).toBeInTheDocument()
      const docsRow = screen.getByText('Docs').closest('tr') as HTMLElement
      expect(within(docsRow).getByText('Member')).toBeInTheDocument()
      // The page offers Create team unconditionally (creating a team needs no
      // team-scoped permission); per-team management gating lives on the
      // details page, keyed off the server permissions array — not here.
      expect(screen.getByTestId('create-team-button')).toBeInTheDocument()
    })

    it('falls back to the member role badge and hides the member count for personal teams', async () => {
      ;(teamService.getTeams as jest.Mock).mockResolvedValue([
        buildTeam({
          name: 'Personal',
          description: '',
          is_personal: true,
          role: undefined,
          member_count: 1,
        }),
      ])

      renderTeams()

      const row = (await screen.findByText('Personal')).closest(
        'tr'
      ) as HTMLElement
      expect(within(row).getByText('Member')).toBeInTheDocument()
      // Personal workspaces show a dash instead of a member count.
      expect(within(row).getByText('-')).toBeInTheDocument()
      expect(within(row).queryByText('1')).not.toBeInTheDocument()
    })

    it('shows the error alert and empties the lists when loading fails', async () => {
      ;(teamService.getTeams as jest.Mock).mockRejectedValue(
        new Error('teams service down')
      )

      renderTeams()

      await screen.findByText('Error')
      expect(screen.getByText('teams service down')).toBeInTheDocument()
      expect(mockHandleError).toHaveBeenCalledWith(
        expect.any(Error),
        'Failed to load teams'
      )
      expect(screen.getByText('No teams yet')).toBeInTheDocument()
    })

    it('shows the empty state with a create action when there are no teams', async () => {
      renderTeams()

      await screen.findByText('No teams yet')
      expect(
        screen.getByText(
          'Create your first team or wait for an invitation to join an existing team.'
        )
      ).toBeInTheDocument()
      expect(screen.queryByText('Pending Invitations')).not.toBeInTheDocument()
    })
  })

  describe('navigation', () => {
    it('navigates to the team details when a team name is clicked', async () => {
      ;(teamService.getTeams as jest.Mock).mockResolvedValue([buildTeam()])

      renderTeams()
      const user = userEvent.setup()

      await user.click(await screen.findByText('Engineering'))

      expect(screen.getByTestId('team-details-probe')).toBeInTheDocument()
    })
  })

  describe('create team modal', () => {
    it('opens from the header button and refetches on success', async () => {
      renderTeams()
      const user = userEvent.setup()

      await screen.findByText('No teams yet')
      expect(screen.queryByTestId('create-team-modal')).not.toBeInTheDocument()

      await user.click(screen.getByTestId('create-team-button'))
      expect(screen.getByTestId('create-team-modal')).toBeInTheDocument()

      const fetchesBefore = (teamService.getTeams as jest.Mock).mock.calls
        .length
      await user.click(
        screen.getByRole('button', { name: 'probe-modal-success' })
      )

      await waitFor(() => {
        expect(
          (teamService.getTeams as jest.Mock).mock.calls.length
        ).toBeGreaterThan(fetchesBefore)
      })
    })

    it('also opens from the empty-state action', async () => {
      renderTeams()
      const user = userEvent.setup()

      await screen.findByText('No teams yet')
      // Header button + empty-state button.
      const createButtons = screen.getAllByRole('button', {
        name: /Create team/,
      })
      expect(createButtons).toHaveLength(2)
      await user.click(createButtons[1])

      expect(screen.getByTestId('create-team-modal')).toBeInTheDocument()
    })
  })

  describe('pending invitations', () => {
    it('lists pending invitations with a count badge and inviter details', async () => {
      ;(teamService.getPendingInvitations as jest.Mock).mockResolvedValue([
        buildInvitation(),
        buildInvitation({
          id: 'inv-2',
          token: 'token-def',
          team_name: 'Beta',
          invited_by: undefined,
        }),
      ])

      renderTeams()

      await screen.findByText('Pending Invitations')
      expect(screen.getByText('Acme Corp')).toBeInTheDocument()
      expect(screen.getByText('Beta')).toBeInTheDocument()
      expect(
        screen.getByText('Invited by Alice (alice@example.com)')
      ).toBeInTheDocument()
      const heading = screen
        .getByText('Pending Invitations')
        .closest('div') as HTMLElement
      expect(within(heading).getByText('2')).toBeInTheDocument()
    })

    it('accept success removes the card and notifies other surfaces', async () => {
      ;(teamService.getPendingInvitations as jest.Mock).mockResolvedValue([
        buildInvitation(),
      ])
      mockAcceptAndEnterTeam.mockResolvedValue({
        ok: true,
        team: null,
        teamId: 'team-9',
        teamName: 'Acme Corp',
      })

      renderTeams()
      const user = userEvent.setup()

      await screen.findByText('Acme Corp')
      await user.click(screen.getByRole('button', { name: /Accept/ }))

      await waitFor(() => {
        expect(screen.queryByText('Acme Corp')).not.toBeInTheDocument()
      })
      expect(mockAcceptAndEnterTeam).toHaveBeenCalledWith('token-abc')
      expect(emitInvitationsChanged).toHaveBeenCalled()
      expect(screen.queryByText('Pending Invitations')).not.toBeInTheDocument()
    })

    it('accept failure keeps the invitation card (the hook already toasts)', async () => {
      ;(teamService.getPendingInvitations as jest.Mock).mockResolvedValue([
        buildInvitation(),
      ])
      mockAcceptAndEnterTeam.mockResolvedValue({
        ok: false,
        error: new Error('expired'),
      })

      renderTeams()
      const user = userEvent.setup()

      await screen.findByText('Acme Corp')
      await user.click(screen.getByRole('button', { name: /Accept/ }))

      await waitFor(() => {
        expect(mockAcceptAndEnterTeam).toHaveBeenCalledWith('token-abc')
      })
      expect(screen.getByText('Acme Corp')).toBeInTheDocument()
      expect(emitInvitationsChanged).not.toHaveBeenCalled()
    })

    it('decline rejects via the service, toasts, removes the card and notifies', async () => {
      ;(teamService.getPendingInvitations as jest.Mock).mockResolvedValue([
        buildInvitation(),
      ])
      ;(teamService.rejectInvitation as jest.Mock).mockResolvedValue(undefined)

      renderTeams()
      const user = userEvent.setup()

      await screen.findByText('Acme Corp')
      await user.click(screen.getByRole('button', { name: /Decline/ }))

      await waitFor(() => {
        expect(teamService.rejectInvitation).toHaveBeenCalledWith('token-abc')
      })
      expect(toast.info).toHaveBeenCalledWith(
        'Invitation to Acme Corp has been declined.'
      )
      await waitFor(() => {
        expect(screen.queryByText('Acme Corp')).not.toBeInTheDocument()
      })
      expect(emitInvitationsChanged).toHaveBeenCalled()
    })

    it('decline failure reports the error and keeps the card', async () => {
      ;(teamService.getPendingInvitations as jest.Mock).mockResolvedValue([
        buildInvitation(),
      ])
      ;(teamService.rejectInvitation as jest.Mock).mockRejectedValue(
        new Error('cannot reject')
      )

      renderTeams()
      const user = userEvent.setup()

      await screen.findByText('Acme Corp')
      await user.click(screen.getByRole('button', { name: /Decline/ }))

      await waitFor(() => {
        expect(mockHandleError).toHaveBeenCalledWith(
          expect.any(Error),
          'Failed to reject invitation'
        )
      })
      expect(screen.getByText('Acme Corp')).toBeInTheDocument()
      expect(emitInvitationsChanged).not.toHaveBeenCalled()
    })
  })
})
