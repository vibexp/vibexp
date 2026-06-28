import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import React from 'react'
import { MemoryRouter } from 'react-router-dom'

import type { AuthProvider } from '../../types'
import { SignInPage } from './SignInPage'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockLogin = jest.fn()
const mockTrackAuth = jest.fn()
const mockGetProviders = jest.fn<Promise<AuthProvider[]>, []>()

jest.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({
    login: mockLogin,
    user: null,
    isAuthenticated: false,
    isLoading: false,
    logout: jest.fn(),
    checkPendingInvitation: jest.fn(),
    markOnboardingComplete: jest.fn(),
  }),
}))

jest.mock('../../hooks/useAnalytics', () => ({
  useAnalytics: () => ({
    trackAuth: mockTrackAuth,
    track: jest.fn(),
    trackEvent: jest.fn(),
    trackPage: jest.fn(),
    trackError: jest.fn(),
    identify: jest.fn(),
    isEnabled: true,
  }),
}))

jest.mock('../../services/authService', () => ({
  authService: {
    getProviders: () => mockGetProviders(),
  },
}))

// Mock CookieConsentBanner so we don't need to stub its dependencies
jest.mock('@/components/CookieConsentBanner', () => ({
  CookieConsentBanner: () => null,
}))

// Mock theme hook
jest.mock('@/lib/theme', () => ({
  useTheme: () => ({ resolvedTheme: 'light', setTheme: jest.fn() }),
}))

// Mock DevLogin to avoid import.meta.env dependency in Jest
jest.mock('./DevLogin', () => ({
  DevLogin: () => null,
}))

// Mock UI components that pull in radix-ui and other heavy deps
jest.mock('@/components/ui/button', () => ({
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

jest.mock('@/components/ui/alert', () => ({
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

const PROVIDERS: AuthProvider[] = [
  { name: 'google', display_name: 'Google' },
  { name: 'github', display_name: 'GitHub' },
]

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderSignInPage() {
  return render(
    <MemoryRouter>
      <SignInPage />
    </MemoryRouter>
  )
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('SignInPage — config-driven provider picker', () => {
  beforeEach(() => {
    jest.clearAllMocks()
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
