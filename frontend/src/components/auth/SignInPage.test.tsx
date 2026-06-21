import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import React from 'react'
import { MemoryRouter } from 'react-router-dom'

import { SignInPage } from './SignInPage'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockLogin = jest.fn()
const mockTrackAuth = jest.fn()

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

describe('SignInPage — OAuth provider sessionStorage', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    window.sessionStorage.clear()
    // Default: login resolves immediately (redirect handled by location mock)
    mockLogin.mockResolvedValue(undefined)
  })

  it('renders the Google sign-in button', () => {
    renderSignInPage()
    expect(
      screen.getByRole('button', { name: /continue with google/i })
    ).toBeInTheDocument()
  })

  it('writes "Google" to sessionStorage and calls login() with GoogleOAuth slug', async () => {
    renderSignInPage()

    const btn = screen.getByRole('button', { name: /continue with google/i })
    fireEvent.click(btn)

    // The sessionStore.set happens synchronously before the async login() call
    expect(window.sessionStorage.getItem(STORAGE_KEY)).toBe('Google')

    // login() is called with the WorkOS provider slug
    await waitFor(() => {
      expect(mockLogin).toHaveBeenCalledWith('GoogleOAuth')
    })
  })

  it('sets sessionStorage and calls login() with slug even when login throws', async () => {
    mockLogin.mockRejectedValue(new Error('OAuth error'))

    renderSignInPage()

    const btn = screen.getByRole('button', { name: /continue with google/i })
    fireEvent.click(btn)

    // Storage is set synchronously before the async throw
    expect(window.sessionStorage.getItem(STORAGE_KEY)).toBe('Google')

    // login() is called with the WorkOS provider slug even when it throws
    await waitFor(() => {
      expect(mockLogin).toHaveBeenCalledWith('GoogleOAuth')
    })

    // Wait for error state to settle
    await waitFor(() => {
      expect(screen.getByText(/OAuth error/i)).toBeInTheDocument()
    })
  })
})
