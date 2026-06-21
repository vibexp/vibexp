import { render, screen, waitFor } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import type { APIErrorResponse } from '@/types/errors'
import { ApiError } from '@/types/errors'
import type { TeamInvitation } from '@/types/team'

// ---------------------------------------------------------------------------
// Mocks (set up before importing the component under test)
// ---------------------------------------------------------------------------

const mockNavigate = jest.fn()
let mockTokenParam: string | undefined = 'token-abc'

jest.mock('react-router-dom', () => {
  const actual =
    jest.requireActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
    useParams: () => ({ token: mockTokenParam }),
  }
})

const mockGetInvitationByToken = jest.fn()
const mockAcceptInvitation = jest.fn()
const mockRejectInvitation = jest.fn()

jest.mock('@/services/teamService', () => ({
  teamService: {
    getInvitationByToken: (...args: unknown[]) =>
      mockGetInvitationByToken(...args),
    acceptInvitation: (...args: unknown[]) => mockAcceptInvitation(...args),
    rejectInvitation: (...args: unknown[]) => mockRejectInvitation(...args),
  },
}))

let mockIsAuthenticated = true
let mockAuthLoading = false
const mockCheckPendingInvitation = jest.fn<string | null, []>(() => null)

jest.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({
    isAuthenticated: mockIsAuthenticated,
    isLoading: mockAuthLoading,
    checkPendingInvitation: mockCheckPendingInvitation,
  }),
}))

const mockSessionStoreSet = jest.fn()
const mockSessionStoreRemove = jest.fn()

jest.mock('@/utils/storage', () => ({
  sessionStore: {
    set: (...args: unknown[]) => mockSessionStoreSet(...args),
    remove: (...args: unknown[]) => mockSessionStoreRemove(...args),
  },
}))

// ---------------------------------------------------------------------------
// Imports after mocks
// ---------------------------------------------------------------------------

import { AcceptInvitation } from './AcceptInvitation'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const buildInvitation = (
  overrides: Partial<TeamInvitation> = {}
): TeamInvitation => ({
  id: 'inv-1',
  token: 'token-abc',
  team_id: 'team-123',
  team_name: 'Engineering',
  invitee_email: 'invited@example.com',
  role: 'member',
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

const buildApiError = (
  status: number,
  overrides: Partial<APIErrorResponse> = {}
): ApiError =>
  new ApiError({
    type: 'https://api.vibexp.io/errors/test',
    title: 'Test Error',
    status,
    detail: 'detail',
    code: 'TEST',
    request_id: 'req-1',
    timestamp: '2024-01-01T00:00:00Z',
    ...overrides,
  })

function renderPage() {
  return render(
    <MemoryRouter>
      <AcceptInvitation />
    </MemoryRouter>
  )
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('AcceptInvitation', () => {
  let consoleErrorSpy: jest.SpyInstance

  beforeEach(() => {
    jest.clearAllMocks()
    mockIsAuthenticated = true
    mockAuthLoading = false
    mockTokenParam = 'token-abc'
    mockCheckPendingInvitation.mockReturnValue(null)
    // Component logs a console.error on every catch — intentionally silence
    // it in tests so failure noise stays readable.
    consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    consoleErrorSpy.mockRestore()
  })

  it('renders invitation details and the Accept button on success', async () => {
    mockGetInvitationByToken.mockResolvedValueOnce({
      invitation: buildInvitation(),
    })

    renderPage()

    expect(
      await screen.findByRole('button', { name: /accept invitation/i })
    ).toBeInTheDocument()
    expect(screen.getByText('Engineering')).toBeInTheDocument()
    expect(screen.getByText('Jane Inviter')).toBeInTheDocument()
    expect(screen.getByText('invited@example.com')).toBeInTheDocument()
  })

  it('renders "expired" alert on 410', async () => {
    mockGetInvitationByToken.mockRejectedValueOnce(
      buildApiError(410, {
        title: 'Invitation Expired',
        code: 'RESOURCE_CONFLICT',
        detail: 'Invitation has expired',
      })
    )

    renderPage()

    expect(await screen.findByText(/invitation expired/i)).toBeInTheDocument()
    expect(screen.getByText('This invitation has expired.')).toBeInTheDocument()
  })

  it('renders "revoked" alert on 409 with metadata.status=revoked', async () => {
    mockGetInvitationByToken.mockRejectedValueOnce(
      buildApiError(409, {
        title: 'Resource Conflict',
        code: 'RESOURCE_CONFLICT',
        detail: 'Invitation has been revoked',
        metadata: { status: 'revoked' },
      })
    )

    renderPage()

    expect(await screen.findByText('Invitation revoked')).toBeInTheDocument()
    expect(
      screen.getByText('This invitation has been revoked.')
    ).toBeInTheDocument()
  })

  it('renders "already accepted" alert on 409 with metadata.status=accepted', async () => {
    mockGetInvitationByToken.mockRejectedValueOnce(
      buildApiError(409, {
        code: 'RESOURCE_CONFLICT',
        detail: 'Invitation has already been accepted',
        metadata: { status: 'accepted' },
      })
    )

    renderPage()

    expect(
      await screen.findByText('This invitation has already been accepted.')
    ).toBeInTheDocument()
    expect(
      screen.getByText('Invitation no longer available')
    ).toBeInTheDocument()
  })

  it('renders "already rejected" alert on 409 with metadata.status=rejected', async () => {
    mockGetInvitationByToken.mockRejectedValueOnce(
      buildApiError(409, {
        code: 'RESOURCE_CONFLICT',
        detail: 'Invitation has been rejected',
        metadata: { status: 'rejected' },
      })
    )

    renderPage()

    expect(
      await screen.findByText('This invitation has already been rejected.')
    ).toBeInTheDocument()
  })

  it('renders fallback "no longer valid" alert on 409 with unknown metadata.status', async () => {
    mockGetInvitationByToken.mockRejectedValueOnce(
      buildApiError(409, {
        code: 'RESOURCE_CONFLICT',
        detail: 'Invitation is no longer pending',
        // No metadata at all — exercises the disambiguation fallback.
      })
    )

    renderPage()

    expect(
      await screen.findByText('This invitation is no longer valid.')
    ).toBeInTheDocument()
    expect(
      screen.getByText('Invitation no longer available')
    ).toBeInTheDocument()
  })

  it('renders "not found" alert on 404', async () => {
    mockGetInvitationByToken.mockRejectedValueOnce(
      buildApiError(404, {
        code: 'RESOURCE_NOT_FOUND',
        detail: 'Invitation not found',
      })
    )

    renderPage()

    expect(await screen.findByText('Invitation not found')).toBeInTheDocument()
    expect(screen.getByText('Invitation not found.')).toBeInTheDocument()
  })

  it('renders generic retry alert on 500 / unknown failure', async () => {
    mockGetInvitationByToken.mockRejectedValueOnce(
      buildApiError(500, {
        code: 'INTERNAL_ERROR',
        detail: 'Failed to load invitation',
      })
    )

    renderPage()

    expect(
      await screen.findByText(
        "Couldn't load invitation. Please try again later."
      )
    ).toBeInTheDocument()
  })

  it('renders "session expired" alert on 401 (cookie evicted mid-mount)', async () => {
    // The unauthenticated-redirect path runs in useAuth before we ever issue
    // the GET; this case covers the rarer "auth was true, then expired" race.
    mockGetInvitationByToken.mockRejectedValueOnce(
      buildApiError(401, {
        code: 'UNAUTHORIZED',
        detail: 'Session expired',
      })
    )

    renderPage()

    expect(await screen.findByText('Sign in required')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Your session has expired. Please sign in again to continue.'
      )
    ).toBeInTheDocument()
  })

  it('renders generic retry alert when a non-ApiError (e.g. network) is thrown', async () => {
    mockGetInvitationByToken.mockRejectedValueOnce(
      new Error('Network error: Unable to connect')
    )

    renderPage()

    expect(
      await screen.findByText(
        "Couldn't load invitation. Please try again later."
      )
    ).toBeInTheDocument()
  })

  it('redirects to "/" and stashes token in sessionStore when unauthenticated', async () => {
    mockIsAuthenticated = false
    mockAuthLoading = false

    renderPage()

    await waitFor(() => {
      expect(mockSessionStoreSet).toHaveBeenCalledWith(
        expect.any(String),
        'token-abc'
      )
    })
    expect(mockNavigate).toHaveBeenCalledWith('/')
    expect(mockGetInvitationByToken).not.toHaveBeenCalled()
  })

  it('shows "Invalid invitation link" when no token is present in URL or session', async () => {
    mockTokenParam = undefined
    mockCheckPendingInvitation.mockReturnValue(null)

    renderPage()

    expect(await screen.findByText('Invalid invitation')).toBeInTheDocument()
    expect(screen.getByText('Invalid invitation link')).toBeInTheDocument()
  })

  it('stashes the just-accepted handoff and navigates to the team page after accept', async () => {
    const user = userEvent.setup()
    mockGetInvitationByToken.mockResolvedValueOnce({
      invitation: buildInvitation(),
    })
    mockAcceptInvitation.mockResolvedValueOnce({
      team_id: 'team-123',
      team_name: 'Engineering',
      message: 'Successfully joined team Engineering',
    })

    renderPage()

    const acceptButton = await screen.findByRole('button', {
      name: /accept invitation/i,
    })
    await user.click(acceptButton)

    await waitFor(() => {
      expect(mockAcceptInvitation).toHaveBeenCalledWith('token-abc')
    })
    expect(mockSessionStoreRemove).toHaveBeenCalledWith(
      'vx_pending_invitation_token'
    )
    expect(mockSessionStoreSet).toHaveBeenCalledWith(
      'vx_invitation_just_accepted',
      { team_id: 'team-123', team_name: 'Engineering' }
    )
    expect(mockNavigate).toHaveBeenCalledWith('/settings/teams/team-123')
  })

  it('shows the "couldn\'t accept" alert when accept fails with a non-ApiError', async () => {
    const user = userEvent.setup()
    mockGetInvitationByToken.mockResolvedValueOnce({
      invitation: buildInvitation(),
    })
    mockAcceptInvitation.mockRejectedValueOnce(new Error('boom'))

    renderPage()

    const acceptButton = await screen.findByRole('button', {
      name: /accept invitation/i,
    })
    await user.click(acceptButton)

    expect(
      await screen.findByText('Failed to accept invitation. Please try again.')
    ).toBeInTheDocument()
  })

  it('surfaces a wrong-account message when accept fails with email mismatch', async () => {
    const user = userEvent.setup()
    mockGetInvitationByToken.mockResolvedValueOnce({
      invitation: buildInvitation(),
    })
    mockAcceptInvitation.mockRejectedValueOnce(
      buildApiError(403, {
        code: 'INVITATION_EMAIL_MISMATCH',
        metadata: {
          invitee_email: 'invited@example.com',
          actor_email: 'someone@example.com',
        },
      })
    )

    renderPage()

    const acceptButton = await screen.findByRole('button', {
      name: /accept invitation/i,
    })
    await user.click(acceptButton)

    expect(await screen.findByText('Wrong account')).toBeInTheDocument()
    expect(screen.getByText(/invited@example\.com/)).toBeInTheDocument()
  })

  it('navigates home with rejected message after a successful reject', async () => {
    const user = userEvent.setup()
    mockGetInvitationByToken.mockResolvedValueOnce({
      invitation: buildInvitation(),
    })
    mockRejectInvitation.mockResolvedValueOnce(undefined)

    renderPage()

    const rejectButton = await screen.findByRole('button', {
      name: /^reject$/i,
    })
    await user.click(rejectButton)

    await waitFor(() => {
      expect(mockRejectInvitation).toHaveBeenCalledWith('token-abc')
    })
    expect(mockSessionStoreRemove).toHaveBeenCalled()
    expect(mockNavigate).toHaveBeenCalledWith('/', {
      state: { message: 'Invitation rejected' },
    })
  })

  it('shows the "couldn\'t reject" alert when reject fails', async () => {
    const user = userEvent.setup()
    mockGetInvitationByToken.mockResolvedValueOnce({
      invitation: buildInvitation(),
    })
    mockRejectInvitation.mockRejectedValueOnce(new Error('boom'))

    renderPage()

    const rejectButton = await screen.findByRole('button', {
      name: /^reject$/i,
    })
    await user.click(rejectButton)

    expect(
      await screen.findByText('Failed to reject invitation. Please try again.')
    ).toBeInTheDocument()
  })

  it('renders the legacy "email" field when invitee_email is missing', async () => {
    mockGetInvitationByToken.mockResolvedValueOnce({
      invitation: buildInvitation({
        invitee_email: undefined,
        email: 'legacy@example.com',
      }),
    })

    renderPage()

    expect(await screen.findByText('legacy@example.com')).toBeInTheDocument()
  })

  it('navigates home from the error state when "Go to dashboard" is clicked', async () => {
    const user = userEvent.setup()
    mockGetInvitationByToken.mockRejectedValueOnce(
      buildApiError(404, { code: 'RESOURCE_NOT_FOUND' })
    )

    renderPage()

    const goButton = await screen.findByRole('button', {
      name: /go to dashboard/i,
    })
    await user.click(goButton)

    expect(mockNavigate).toHaveBeenCalledWith('/')
  })
})
