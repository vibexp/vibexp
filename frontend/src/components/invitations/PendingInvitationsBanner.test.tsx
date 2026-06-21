import { render, screen, waitFor } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import type { TeamInvitation } from '@/types/team'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockNavigate = jest.fn()

jest.mock('react-router-dom', () => {
  const actual =
    jest.requireActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

const mockGetPendingInvitations = jest.fn()

jest.mock('@/services/teamService', () => ({
  teamService: {
    getPendingInvitations: (...args: unknown[]) =>
      mockGetPendingInvitations(...args),
  },
}))

const mockAcceptAndEnterTeam = jest.fn()

jest.mock('@/hooks/useAcceptAndEnterTeam', () => ({
  useAcceptAndEnterTeam: () => mockAcceptAndEnterTeam,
}))

const sessionStoreState = new Map<string, string>()
const mockSessionStore = {
  get: jest.fn((key: string) => sessionStoreState.get(key) ?? null),
  getJSON: jest.fn((key: string): unknown => {
    const raw = sessionStoreState.get(key)
    if (raw === undefined) return null
    try {
      return JSON.parse(raw) as unknown
    } catch {
      return null
    }
  }),
  set: jest.fn((key: string, value: unknown) => {
    sessionStoreState.set(
      key,
      typeof value === 'string' ? value : JSON.stringify(value)
    )
  }),
  remove: jest.fn((key: string) => {
    sessionStoreState.delete(key)
  }),
}

jest.mock('@/utils/storage', () => ({
  sessionStore: mockSessionStore,
}))

// ---------------------------------------------------------------------------
// Imports after mocks
// ---------------------------------------------------------------------------

import { emitInvitationsChanged } from './invitationEvents'
import { PendingInvitationsBanner } from './PendingInvitationsBanner'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const buildInvitation = (
  overrides: Partial<TeamInvitation> = {}
): TeamInvitation => ({
  id: 'inv-1',
  token: 'token-1',
  team_id: 'team-1',
  team_name: 'Engineering',
  invitee_email: 'invited@example.com',
  status: 'pending',
  created_at: '2024-01-01T00:00:00Z',
  expires_at: '2025-12-31T23:59:59Z',
  invited_by: {
    id: 'user-99',
    name: 'Jane Inviter',
    email: 'jane@example.com',
  },
  ...overrides,
})

function renderBanner() {
  return render(
    <MemoryRouter>
      <PendingInvitationsBanner />
    </MemoryRouter>
  )
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('PendingInvitationsBanner', () => {
  let consoleErrorSpy: jest.SpyInstance

  beforeEach(() => {
    jest.clearAllMocks()
    sessionStoreState.clear()
    consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    consoleErrorSpy.mockRestore()
  })

  it('renders nothing when the user has no pending invitations', async () => {
    mockGetPendingInvitations.mockResolvedValueOnce([])

    const { container } = renderBanner()

    await waitFor(() => {
      expect(mockGetPendingInvitations).toHaveBeenCalled()
    })
    expect(container.firstChild).toBeNull()
  })

  it('renders single invitation with inviter and team names', async () => {
    mockGetPendingInvitations.mockResolvedValueOnce([buildInvitation()])

    renderBanner()

    expect(await screen.findByText('Jane Inviter')).toBeInTheDocument()
    expect(screen.getByText('Engineering')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /accept/i })).toBeInTheDocument()
  })

  it('renders count and Review button when there are multiple invitations', async () => {
    mockGetPendingInvitations.mockResolvedValueOnce([
      buildInvitation({ id: 'inv-1' }),
      buildInvitation({ id: 'inv-2', team_name: 'Marketing' }),
    ])

    renderBanner()

    expect(
      await screen.findByText(/2 pending team invitations/i)
    ).toBeInTheDocument()
    const review = screen.getByRole('button', { name: /^review$/i })
    expect(review).toBeInTheDocument()
  })

  it('navigates to /settings/teams when "Review" is clicked', async () => {
    const user = userEvent.setup()
    mockGetPendingInvitations.mockResolvedValueOnce([
      buildInvitation({ id: 'inv-1' }),
      buildInvitation({ id: 'inv-2' }),
    ])

    renderBanner()

    const review = await screen.findByRole('button', { name: /^review$/i })
    await user.click(review)

    expect(mockNavigate).toHaveBeenCalledWith('/settings/teams')
  })

  it('calls useAcceptAndEnterTeam with the invitation token on Accept', async () => {
    const user = userEvent.setup()
    mockGetPendingInvitations.mockResolvedValueOnce([buildInvitation()])
    mockAcceptAndEnterTeam.mockResolvedValueOnce({
      ok: true,
      team: null,
      teamId: 'team-1',
      teamName: 'Engineering',
    })

    renderBanner()

    const accept = await screen.findByRole('button', { name: /accept/i })
    await user.click(accept)

    await waitFor(() => {
      expect(mockAcceptAndEnterTeam).toHaveBeenCalledWith('token-1')
    })
  })

  it('hides the dismissed invitation and persists ID for the session', async () => {
    const user = userEvent.setup()
    mockGetPendingInvitations.mockResolvedValueOnce([
      buildInvitation({ id: 'inv-1' }),
    ])

    renderBanner()

    const dismiss = await screen.findByRole('button', {
      name: /dismiss invitation/i,
    })
    await user.click(dismiss)

    await waitFor(() => {
      expect(screen.queryByText('Engineering')).not.toBeInTheDocument()
    })
    expect(mockSessionStore.set).toHaveBeenCalled()
    const persisted = mockSessionStore.set.mock.calls.at(-1)?.[1]
    expect(persisted).toEqual(['inv-1'])
  })

  it('keeps an undismissed invitation visible when others are dismissed', async () => {
    sessionStoreState.set(
      'vx_invitation_banner_dismissed',
      JSON.stringify(['inv-1', 'inv-2'])
    )
    mockGetPendingInvitations.mockResolvedValueOnce([
      buildInvitation({ id: 'inv-1' }),
      buildInvitation({ id: 'inv-2' }),
      buildInvitation({ id: 'inv-3', team_name: 'New Team' }),
    ])

    renderBanner()

    expect(await screen.findByText('New Team')).toBeInTheDocument()
    expect(screen.queryByText('Engineering')).not.toBeInTheDocument()
  })

  it('refetches pending invitations on emitInvitationsChanged()', async () => {
    mockGetPendingInvitations.mockResolvedValueOnce([])

    renderBanner()
    await waitFor(() => {
      expect(mockGetPendingInvitations).toHaveBeenCalledTimes(1)
    })

    mockGetPendingInvitations.mockResolvedValueOnce([buildInvitation()])
    emitInvitationsChanged()

    await waitFor(() => {
      expect(mockGetPendingInvitations).toHaveBeenCalledTimes(2)
    })
    expect(await screen.findByText('Engineering')).toBeInTheDocument()
  })

  it('survives a fetch failure without crashing', async () => {
    mockGetPendingInvitations.mockRejectedValueOnce(new Error('boom'))

    const { container } = renderBanner()

    await waitFor(() => {
      expect(mockGetPendingInvitations).toHaveBeenCalled()
    })
    expect(container.firstChild).toBeNull()
  })

  it('"Dismiss all" persists every visible invitation id', async () => {
    const user = userEvent.setup()
    mockGetPendingInvitations.mockResolvedValueOnce([
      buildInvitation({ id: 'a' }),
      buildInvitation({ id: 'b' }),
    ])

    renderBanner()

    const dismissAll = await screen.findByRole('button', {
      name: /dismiss all invitations/i,
    })
    await user.click(dismissAll)

    await waitFor(() => {
      expect(
        screen.queryByText(/pending team invitations/i)
      ).not.toBeInTheDocument()
    })
    const lastSet = mockSessionStore.set.mock.calls.at(-1)?.[1]
    expect(lastSet).toEqual(expect.arrayContaining(['a', 'b']))
  })
})
