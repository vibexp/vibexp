import { render, screen, waitFor } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import { ApiError } from '@/types/errors'
import type { OAuthConsentDetails } from '@/types/oauth'

// ---------------------------------------------------------------------------
// Mocks (set up before importing the component under test)
// ---------------------------------------------------------------------------

let mockLogin: string | null = 'login-abc'

jest.mock('react-router-dom', () => {
  const actual =
    jest.requireActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useSearchParams: () => [new URLSearchParams({ login: mockLogin ?? '' })],
  }
})

const mockGetConsent = jest.fn()
const mockAttach = jest.fn()
const mockSubmitConsent = jest.fn()

jest.mock('@/services/oauthService', () => ({
  oauthService: {
    getConsent: (...args: unknown[]) => mockGetConsent(...args),
    attach: (...args: unknown[]) => mockAttach(...args),
    submitConsent: (...args: unknown[]) => mockSubmitConsent(...args),
  },
}))

const mockHardRedirect = jest.fn()

jest.mock('@/utils/navigation', () => ({
  hardRedirect: (...args: unknown[]) => mockHardRedirect(...args),
}))

// ---------------------------------------------------------------------------
// Imports after mocks
// ---------------------------------------------------------------------------

import { OAuthConsentPage } from './OAuthConsentPage'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const buildDetails = (
  overrides: Partial<OAuthConsentDetails> = {}
): OAuthConsentDetails => ({
  authenticated: true,
  client_name: 'Claude Code',
  redirect_host: 'claude.ai',
  scopes: ['mcp'],
  csrf: 'csrf-token-123',
  ...overrides,
})

const unauthorizedError = () =>
  new ApiError({
    type: 'about:blank',
    title: 'Unauthorized',
    status: 401,
    detail: 'authentication required',
    code: 'AUTH_REQUIRED',
    request_id: '',
    timestamp: '2026-06-29T00:00:00Z',
  })

function renderPage() {
  return render(
    <MemoryRouter>
      <OAuthConsentPage />
    </MemoryRouter>
  )
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('OAuthConsentPage', () => {
  let consoleErrorSpy: jest.SpyInstance

  beforeEach(() => {
    jest.clearAllMocks()
    mockLogin = 'login-abc'
    consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    consoleErrorSpy.mockRestore()
  })

  it('renders the request details and Approve/Deny when already authenticated', async () => {
    mockGetConsent.mockResolvedValueOnce(buildDetails())

    renderPage()

    expect(
      await screen.findByRole('button', { name: /approve/i })
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /deny/i })).toBeInTheDocument()
    expect(screen.getAllByText('Claude Code').length).toBeGreaterThan(0)
    expect(screen.getByText('claude.ai')).toBeInTheDocument()
    expect(screen.getByText('mcp')).toBeInTheDocument()
    expect(mockGetConsent).toHaveBeenCalledWith('login-abc')
    // Already bound — no attach needed.
    expect(mockAttach).not.toHaveBeenCalled()
  })

  it('attaches the session then renders when not yet bound but signed in', async () => {
    mockGetConsent
      .mockResolvedValueOnce(buildDetails({ authenticated: false }))
      .mockResolvedValueOnce(buildDetails())
    mockAttach.mockResolvedValueOnce({ authenticated: true })

    renderPage()

    expect(
      await screen.findByRole('button', { name: /approve/i })
    ).toBeInTheDocument()
    expect(mockAttach).toHaveBeenCalledWith('login-abc', 'csrf-token-123')
    expect(mockGetConsent).toHaveBeenCalledTimes(2)
    expect(mockHardRedirect).not.toHaveBeenCalled()
  })

  it('redirects to /login with a return_to when signed out (attach 401)', async () => {
    mockGetConsent.mockResolvedValueOnce(buildDetails({ authenticated: false }))
    mockAttach.mockRejectedValueOnce(unauthorizedError())

    renderPage()

    await waitFor(() => {
      expect(mockHardRedirect).toHaveBeenCalledWith(
        '/login?return_to=%2Foauth%2Fconsent%3Flogin%3Dlogin-abc'
      )
    })
    // No approval screen and no error — we navigated away.
    expect(
      screen.queryByRole('button', { name: /approve/i })
    ).not.toBeInTheDocument()
  })

  it('shows the expired/invalid error state when the request fails to load', async () => {
    mockGetConsent.mockRejectedValueOnce(new Error('expired'))

    renderPage()

    expect(
      await screen.findByText(/expired or is no longer valid/i)
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /approve/i })
    ).not.toBeInTheDocument()
  })

  it('shows the load error when attach fails for a non-auth reason', async () => {
    mockGetConsent.mockResolvedValueOnce(buildDetails({ authenticated: false }))
    mockAttach.mockRejectedValueOnce(new Error('boom'))

    renderPage()

    expect(
      await screen.findByText(/expired or is no longer valid/i)
    ).toBeInTheDocument()
    expect(mockHardRedirect).not.toHaveBeenCalled()
  })

  it('shows the missing-login error when no login id is present', async () => {
    mockLogin = null

    renderPage()

    expect(
      await screen.findByText(/missing required information/i)
    ).toBeInTheDocument()
    expect(mockGetConsent).not.toHaveBeenCalled()
  })

  it('approves and navigates the browser to redirect_to', async () => {
    const user = userEvent.setup()
    mockGetConsent.mockResolvedValueOnce(buildDetails())
    mockSubmitConsent.mockResolvedValueOnce({
      redirect_to: 'https://claude.ai/cb?code=xyz',
    })

    renderPage()

    const approve = await screen.findByRole('button', { name: /approve/i })
    await user.click(approve)

    await waitFor(() => {
      expect(mockSubmitConsent).toHaveBeenCalledWith(
        'login-abc',
        'csrf-token-123',
        'approve'
      )
    })
    expect(mockHardRedirect).toHaveBeenCalledWith(
      'https://claude.ai/cb?code=xyz'
    )
  })

  it('denies and navigates the browser to the access_denied redirect', async () => {
    const user = userEvent.setup()
    mockGetConsent.mockResolvedValueOnce(buildDetails())
    mockSubmitConsent.mockResolvedValueOnce({
      redirect_to: 'https://claude.ai/cb?error=access_denied',
    })

    renderPage()

    const deny = await screen.findByRole('button', { name: /deny/i })
    await user.click(deny)

    await waitFor(() => {
      expect(mockSubmitConsent).toHaveBeenCalledWith(
        'login-abc',
        'csrf-token-123',
        'deny'
      )
    })
    expect(mockHardRedirect).toHaveBeenCalledWith(
      'https://claude.ai/cb?error=access_denied'
    )
  })

  it('shows an error when submitting the decision fails', async () => {
    const user = userEvent.setup()
    mockGetConsent.mockResolvedValueOnce(buildDetails())
    mockSubmitConsent.mockRejectedValueOnce(new Error('boom'))

    renderPage()

    const approve = await screen.findByRole('button', { name: /approve/i })
    await user.click(approve)

    expect(
      await screen.findByText(/could not complete the authorization/i)
    ).toBeInTheDocument()
    expect(mockHardRedirect).not.toHaveBeenCalled()
  })

  it('omits the scopes row when no scopes are requested', async () => {
    mockGetConsent.mockResolvedValueOnce(buildDetails({ scopes: [] }))

    renderPage()

    await screen.findByRole('button', { name: /approve/i })
    expect(screen.queryByText('Requested access')).not.toBeInTheDocument()
  })
})
