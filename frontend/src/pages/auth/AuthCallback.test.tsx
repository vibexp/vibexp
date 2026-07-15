import { fireEvent, render, screen } from '@testing-library/react'
import React from 'react'
import { MemoryRouter } from 'react-router-dom'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockHardRedirect = jest.fn()
const mockConsumeReturnTo = jest.fn(() => '/')

jest.mock('@/utils/navigation', () => ({
  hardRedirect: (...args: unknown[]) => mockHardRedirect(...args),
}))

jest.mock('@/utils/returnTo', () => ({
  consumeReturnTo: () => mockConsumeReturnTo(),
}))

// `@/utils/environment` is not covered by jest's relative-path moduleNameMapper
// entries, so stub it explicitly.
jest.mock('@/utils/environment', () => ({
  getApiBaseUrl: () => 'http://api.test/api/v1',
}))

// Passthrough mocks for the UI primitives so the alert renders as plain DOM.
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

jest.mock('@/components/ui/button', () => ({
  Button: ({
    children,
    ...props
  }: React.ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
}))

jest.mock('@/components/ui/card', () => ({
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
    jest.clearAllMocks()
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
