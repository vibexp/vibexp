import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import React from 'react'
import { MemoryRouter } from 'react-router'

import type { AuthProvider } from '../../services/authService'
import { SignInPage } from './SignInPage'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockLogin = vi.hoisted(() => vi.fn())
const mockTrackAuth = vi.hoisted(() => vi.fn())
const mockGetProviders = vi.hoisted(() =>
  vi.fn((): Promise<AuthProvider[]> => Promise.resolve([]))
)

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({
    login: mockLogin,
    user: null,
    isAuthenticated: false,
    isLoading: false,
    logout: vi.fn(),
    checkPendingInvitation: vi.fn(),
    markOnboardingComplete: vi.fn(),
  }),
}))

vi.mock('../../hooks/useAnalytics', () => ({
  useAnalytics: () => ({
    trackAuth: mockTrackAuth,
    track: vi.fn(),
    trackEvent: vi.fn(),
    trackPage: vi.fn(),
    trackError: vi.fn(),
    identify: vi.fn(),
    isEnabled: true,
  }),
}))

vi.mock('../../services/authService', () => ({
  authService: {
    getProviders: () => mockGetProviders(),
  },
}))

// Mock CookieConsentBanner so we don't need to stub its dependencies
vi.mock('@/components/CookieConsentBanner', () => ({
  CookieConsentBanner: () => null,
}))

// Mock theme hook
vi.mock('@/lib/theme', () => ({
  useTheme: () => ({ resolvedTheme: 'light', setTheme: vi.fn() }),
}))

// Mock DevLogin to avoid import.meta.env dependency in Jest
vi.mock('./DevLogin', () => ({
  DevLogin: () => null,
}))

// Mock UI components that pull in radix-ui and other heavy deps
vi.mock('@/components/ui/button', () => ({
  Button: ({
    children,
    onClick,
    disabled,
  }: {
    children: React.ReactNode
    onClick?: () => void
    disabled?: boolean
  }) => (
    <button onClick={onClick} disabled={disabled}>
      {children}
    </button>
  ),
}))

vi.mock('@/components/ui/alert', () => ({
  Alert: ({ children }: { children: React.ReactNode }) => (
    <div role="alert">{children}</div>
  ),
  AlertTitle: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  AlertDescription: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))

const STORAGE_KEY = 'vx_login_method'
const RETURN_TO_KEY = 'vx_return_to'

const PROVIDERS: AuthProvider[] = [
  { name: 'google', display_name: 'Google' },
  { name: 'github', display_name: 'GitHub' },
]

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderSignInPage(initialEntry = '/login') {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <SignInPage />
    </MemoryRouter>
  )
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('SignInPage — config-driven provider picker', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.sessionStorage.clear()
    // Default: login resolves immediately (redirect handled by location mock)
    mockLogin.mockResolvedValue(undefined)
    mockGetProviders.mockResolvedValue(PROVIDERS)
  })

  it('renders one button per enabled provider fetched from the backend', async () => {
    renderSignInPage()

    expect(
      await screen.findByRole('button', { name: /continue with google/i })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /continue with github/i })
    ).toBeInTheDocument()
  })

  it('writes the display name to sessionStorage and calls login() with the canonical provider name', async () => {
    renderSignInPage()

    const btn = await screen.findByRole('button', {
      name: /continue with google/i,
    })
    fireEvent.click(btn)

    // The sessionStore.set happens synchronously before the async login() call
    expect(window.sessionStorage.getItem(STORAGE_KEY)).toBe('Google')

    await waitFor(() => {
      expect(mockLogin).toHaveBeenCalledWith('google')
    })
  })

  it('sets sessionStorage and surfaces an error when login throws', async () => {
    mockLogin.mockRejectedValue(new Error('OAuth error'))

    renderSignInPage()

    const btn = await screen.findByRole('button', {
      name: /continue with github/i,
    })
    fireEvent.click(btn)

    // Storage is set synchronously before the async throw
    expect(window.sessionStorage.getItem(STORAGE_KEY)).toBe('GitHub')

    await waitFor(() => {
      expect(mockLogin).toHaveBeenCalledWith('github')
    })

    await waitFor(() => {
      expect(screen.getByText(/OAuth error/i)).toBeInTheDocument()
    })
  })

  it('shows an empty-state message when no provider is configured', async () => {
    mockGetProviders.mockResolvedValue([])

    renderSignInPage()

    expect(
      await screen.findByText(/no login providers are configured/i)
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /continue with/i })
    ).not.toBeInTheDocument()
  })

  it('shows an error when the providers request fails', async () => {
    mockGetProviders.mockRejectedValue(new Error('boom'))

    renderSignInPage()

    expect(await screen.findByText(/boom/i)).toBeInTheDocument()
  })
})

describe('SignInPage — return_to plumbing', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.sessionStorage.clear()
    mockLogin.mockResolvedValue(undefined)
    mockGetProviders.mockResolvedValue(PROVIDERS)
  })

  it('stashes a same-origin return_to before the provider redirect', async () => {
    renderSignInPage(
      '/login?return_to=' + encodeURIComponent('/oauth/consent?login=abc')
    )

    const btn = await screen.findByRole('button', {
      name: /continue with google/i,
    })
    fireEvent.click(btn)

    // Stash happens synchronously before the async login() call.
    expect(window.sessionStorage.getItem(RETURN_TO_KEY)).toBe(
      '/oauth/consent?login=abc'
    )

    await waitFor(() => {
      expect(mockLogin).toHaveBeenCalledWith('google')
    })
  })

  it('rejects an open-redirect return_to, stashing "/" instead', async () => {
    renderSignInPage('/login?return_to=' + encodeURIComponent('//evil.com'))

    const btn = await screen.findByRole('button', {
      name: /continue with google/i,
    })
    fireEvent.click(btn)

    expect(window.sessionStorage.getItem(RETURN_TO_KEY)).toBe('/')
  })

  it('defaults to "/" when no return_to is present', async () => {
    renderSignInPage('/login')

    const btn = await screen.findByRole('button', {
      name: /continue with google/i,
    })
    fireEvent.click(btn)

    expect(window.sessionStorage.getItem(RETURN_TO_KEY)).toBe('/')
  })
})
