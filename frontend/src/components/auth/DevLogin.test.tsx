import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import React from 'react'
import { MemoryRouter } from 'react-router-dom'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockDevLogin = jest.fn()
const mockHardRedirect = jest.fn()
let mockDevLoginEnabled = true

jest.mock('../../services/authService', () => ({
  authService: {
    devLogin: (...args: unknown[]) => mockDevLogin(...args),
  },
}))

jest.mock('../../services/environmentService', () => ({
  environmentService: {
    isDevLoginEnabled: () => mockDevLoginEnabled,
  },
}))

jest.mock('../../utils/navigation', () => ({
  hardRedirect: (...args: unknown[]) => mockHardRedirect(...args),
}))

// Passthrough mocks for the heavy/radix UI components so the form renders.
jest.mock('@/components/ui/collapsible', () => ({
  Collapsible: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  CollapsibleTrigger: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  CollapsibleContent: ({ children }: { children: React.ReactNode }) => (
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

jest.mock('@/components/ui/input', () => ({
  Input: (props: React.InputHTMLAttributes<HTMLInputElement>) => (
    <input {...props} />
  ),
}))

jest.mock('@/components/ui/label', () => ({
  Label: ({
    children,
    ...props
  }: React.LabelHTMLAttributes<HTMLLabelElement>) => (
    <label {...props}>{children}</label>
  ),
}))

jest.mock('@/lib/utils', () => ({
  cn: (...args: unknown[]) => args.filter(Boolean).join(' '),
}))

// ---------------------------------------------------------------------------
// Imports after mocks
// ---------------------------------------------------------------------------

import { DevLogin } from './DevLogin'

function renderDevLogin(returnTo?: string) {
  return render(
    <MemoryRouter>
      <DevLogin returnTo={returnTo} />
    </MemoryRouter>
  )
}

async function submitWithEmail() {
  const email = await screen.findByLabelText(/email/i)
  fireEvent.change(email, { target: { value: 'dev@example.com' } })
  fireEvent.click(screen.getByRole('button', { name: /dev login/i }))
}

describe('DevLogin', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    mockDevLoginEnabled = true
    mockDevLogin.mockResolvedValue({ id: 'u1' })
  })

  it('redirects to the validated return_to after a successful dev login', async () => {
    renderDevLogin('/oauth/consent?login=abc')

    await submitWithEmail()

    await waitFor(() => {
      expect(mockDevLogin).toHaveBeenCalledWith('dev@example.com', undefined)
    })
    expect(mockHardRedirect).toHaveBeenCalledWith('/oauth/consent?login=abc')
  })

  it('defaults to "/" when no return_to is provided', async () => {
    renderDevLogin()

    await submitWithEmail()

    await waitFor(() => {
      expect(mockHardRedirect).toHaveBeenCalledWith('/')
    })
  })

  it('rejects an open-redirect return_to, landing on "/"', async () => {
    renderDevLogin('//evil.com')

    await submitWithEmail()

    await waitFor(() => {
      expect(mockHardRedirect).toHaveBeenCalledWith('/')
    })
  })

  it('renders nothing when dev login is disabled', () => {
    mockDevLoginEnabled = false
    const { container } = renderDevLogin('/x')
    expect(container).toBeEmptyDOMElement()
  })
})
