import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { AuthGate } from './AuthGate'

const mockAuth = vi.hoisted(() => ({
  isAuthenticated: false,
  isLoading: false,
}))

vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => mockAuth,
}))
vi.mock('@/components/auth/SignInPage', () => ({
  SignInPage: () => <p>sign in</p>,
}))

function renderGate() {
  return render(
    <AuthGate>
      <p>signed-in content</p>
    </AuthGate>
  )
}

beforeEach(() => {
  mockAuth.isAuthenticated = false
  mockAuth.isLoading = false
})

describe('AuthGate', () => {
  it('shows a loading line while the session is checked', () => {
    mockAuth.isLoading = true
    renderGate()
    expect(screen.getByText('Loading…')).toBeInTheDocument()
    expect(screen.queryByText('signed-in content')).toBeNull()
  })

  it('shows the sign-in page without a session', () => {
    renderGate()
    expect(screen.getByText('sign in')).toBeInTheDocument()
    expect(screen.queryByText('signed-in content')).toBeNull()
  })

  it('renders its children for a signed-in user', () => {
    mockAuth.isAuthenticated = true
    renderGate()
    expect(screen.getByText('signed-in content')).toBeInTheDocument()
  })
})
