import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import React from 'react'
import { MemoryRouter } from 'react-router'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockDevLogin = vi.hoisted(() => vi.fn())
const mockHardRedirect = vi.hoisted(() => vi.fn())
let mockDevLoginEnabled = true

vi.mock('../../services/authService', () => ({
  authService: {
    devLogin: (...args: unknown[]) => mockDevLogin(...args),
  },
}))

vi.mock('../../services/environmentService', () => ({
  environmentService: {
    isDevLoginEnabled: () => mockDevLoginEnabled,
  },
}))

vi.mock('../../utils/navigation', () => ({
  hardRedirect: (...args: unknown[]) => mockHardRedirect(...args),
}))

// Passthrough mocks for the heavy/radix UI components so the form renders.
vi.mock('@/components/ui/collapsible', () => ({
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

vi.mock('@/components/ui/button', () => ({
  Button: ({
    children,
    ...props
  }: React.ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
}))

vi.mock('@/components/ui/input', () => ({
  Input: (props: React.InputHTMLAttributes<HTMLInputElement>) => (
    <input {...props} />
  ),
}))

vi.mock('@/components/ui/label', () => ({
  Label: ({
    children,
    ...props
  }: React.LabelHTMLAttributes<HTMLLabelElement>) => (
    <label {...props}>{children}</label>
  ),
}))

vi.mock('@/lib/utils', () => ({
  cn: (...args: unknown[]) => args.filter(Boolean).join(' '),
}))

// ---------------------------------------------------------------------------
// Imports after mocks
// ---------------------------------------------------------------------------

import { ApiError } from '@/types/errors'
import { ACCESS_RESTRICTED_MESSAGE } from '@/utils/authErrors'

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
    vi.clearAllMocks()
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

  it('shows the restriction wording under the email field when the allowlist denies the login', async () => {
    mockDevLogin.mockRejectedValue(
      new ApiError({
        type: 'https://api.vibexp.io/errors/access-restricted',
        title: 'Access Restricted',
        status: 403,
        detail: 'Your account is not permitted to sign in',
        code: 'access_restricted',
        request_id: 'req-1',
        timestamp: '2024-01-01T00:00:00Z',
      })
    )

    renderDevLogin('/')
    await submitWithEmail()

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(ACCESS_RESTRICTED_MESSAGE)
    // Tied to the field, so a screen reader announces it with the input.
    const email = screen.getByLabelText(/email/i)
    expect(email).toHaveAttribute('aria-invalid', 'true')
    expect(email).toHaveAttribute('aria-describedby', alert.id)
    expect(mockHardRedirect).not.toHaveBeenCalled()
  })

  it('shows the backend detail for any other failure', async () => {
    mockDevLogin.mockRejectedValue(new Error('dev login is disabled'))

    renderDevLogin('/')
    await submitWithEmail()

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'dev login is disabled'
    )
  })

  it('clears the error once the address is edited', async () => {
    mockDevLogin.mockRejectedValue(new Error('dev login is disabled'))
    renderDevLogin('/')
    await submitWithEmail()
    await screen.findByRole('alert')

    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'other@example.com' },
    })
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
