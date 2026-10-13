import { fireEvent, render, screen } from '@testing-library/react'
import React from 'react'
import { MemoryRouter } from 'react-router'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockHardRedirect = vi.hoisted(() => vi.fn())
const mockConsumeReturnTo = vi.hoisted(() => vi.fn(() => '/'))

vi.mock('@/utils/navigation', () => ({
  hardRedirect: (...args: unknown[]) => mockHardRedirect(...args),
}))

vi.mock('@/utils/returnTo', () => ({
  consumeReturnTo: () => mockConsumeReturnTo(),
}))

// `@/utils/environment` is not covered by jest's relative-path moduleNameMapper
// entries, so stub it explicitly.
vi.mock('@/utils/environment', () => ({
  getApiBaseUrl: () => 'http://api.test/api/v1',
}))

// Passthrough mocks for the UI primitives so the alert renders as plain DOM.
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

vi.mock('@/components/ui/button', () => ({
  Button: ({
    children,
    ...props
  }: React.ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
}))

vi.mock('@/components/ui/card', () => ({
  Card: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  CardContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))

// ---------------------------------------------------------------------------
// Imports after mocks
// ---------------------------------------------------------------------------

import { ACCESS_RESTRICTED_MESSAGE } from '@/utils/authErrors'

import { AuthCallback } from './AuthCallback'

function renderCallback(search: string) {
  return render(
    <MemoryRouter initialEntries={[`/auth/callback${search}`]}>
      <AuthCallback />
    </MemoryRouter>
  )
}

describe('AuthCallback', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockConsumeReturnTo.mockReturnValue('/')
  })

  it('renders the restriction message and a back-to-sign-in action for access_restricted', () => {
    renderCallback('?error=access_restricted')

    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('Access restricted')).toBeInTheDocument()
    expect(screen.getByText(ACCESS_RESTRICTED_MESSAGE)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /back to sign in/i }))
    expect(mockHardRedirect).toHaveBeenCalledWith('/login')
  })

  it('explains an unverified email and offers the way back to sign in', () => {
    renderCallback('?error=email_not_verified')

    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('Email not verified')).toBeInTheDocument()
    expect(
      screen.getByText(/has not verified the address/i)
    ).toBeInTheDocument()
    expect(screen.queryByText('Authentication failed')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /back to sign in/i }))
    expect(mockHardRedirect).toHaveBeenCalledWith('/login')
  })

  it('asks for the existing sign-in method when the account is not verified yet', () => {
    renderCallback('?error=existing_sign_in_required')

    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(
      screen.getByText('Sign in the way you did before')
    ).toBeInTheDocument()
    expect(
      screen.getByText(/sign in with that method first/i)
    ).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /back to sign in/i }))
    expect(mockHardRedirect).toHaveBeenCalledWith('/login')
  })

  it('explains that sign-up needs an invitation', () => {
    renderCallback('?error=registration_closed')

    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(
      screen.getByText('Sign-up is by invitation only')
    ).toBeInTheDocument()
    expect(
      screen.getByText(/new accounts need an invitation/i)
    ).toBeInTheDocument()
    expect(screen.queryByText('Authentication failed')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /back to sign in/i }))
    expect(mockHardRedirect).toHaveBeenCalledWith('/login')
  })

  it('keeps the generic message for an unknown error code', () => {
    renderCallback('?error=access_denied')

    expect(screen.getByText('Authentication failed')).toBeInTheDocument()
    expect(
      screen.getByText('Authentication was cancelled or failed')
    ).toBeInTheDocument()
    expect(
      screen.queryByText(ACCESS_RESTRICTED_MESSAGE)
    ).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /try again/i }))
    expect(mockHardRedirect).toHaveBeenCalledWith('/')
  })

  it('redirects to the stashed return path when there is no error param', () => {
    mockConsumeReturnTo.mockReturnValue('/prompts')

    renderCallback('')

    expect(mockHardRedirect).toHaveBeenCalledWith('/prompts')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByText('Signing you in…')).toBeInTheDocument()
  })
})
