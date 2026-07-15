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

import { ApiError } from '@/types/errors'
import { ACCESS_RESTRICTED_MESSAGE } from '@/utils/authErrors'

import { DevLogin } from './DevLogin'

function renderDevLogin(returnTo?: string, onError?: (error: string) => void) {
  return render(
    <MemoryRouter>
      <DevLogin returnTo={returnTo} onError={onError} />
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

  it('surfaces the restriction wording when the allowlist denies the login', async () => {
    const onError = jest.fn()
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

    renderDevLogin('/', onError)
    await submitWithEmail()

    await waitFor(() => {
      expect(onError).toHaveBeenCalledWith(ACCESS_RESTRICTED_MESSAGE)
    })
    expect(mockHardRedirect).not.toHaveBeenCalled()
  })

  it('surfaces the backend detail for any other failure', async () => {
    const onError = jest.fn()
    mockDevLogin.mockRejectedValue(new Error('dev login is disabled'))

    renderDevLogin('/', onError)
    await submitWithEmail()

    await waitFor(() => {
      expect(onError).toHaveBeenCalledWith('dev login is disabled')
    })
  })
})
