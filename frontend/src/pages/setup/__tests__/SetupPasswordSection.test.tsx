import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import { authService } from '@/services/authService'
import { authSettingsService } from '@/services/authSettingsService'
import { ApiError } from '@/types/errors'

vi.mock('@/services/authSettingsService', () => ({
  authSettingsService: { bootstrapAdminPassword: vi.fn() },
}))

vi.mock('@/services/authService', () => ({
  authService: { passwordLogin: vi.fn(), logout: vi.fn() },
}))

const authState = vi.hoisted(() => ({
  isAuthenticated: false,
  user: null as { email: string } | null,
}))
vi.mock('@/contexts/useAuth', () => ({ useAuth: () => authState }))

import { SetupPasswordSection } from '../SetupPasswordSection'

const settings = vi.mocked(authSettingsService)
const auth = vi.mocked(authService)
const navigate = vi.fn()

const PASSPHRASE = 'a long setup passphrase'

function validationError(
  errors: { field: string; message: string; code: string }[]
): ApiError {
  return new ApiError({
    type: 'about:blank',
    title: 'Validation Failed',
    status: 400,
    detail: 'The request is not valid',
    code: 'VALIDATION_FAILED',
    request_id: 'req-1',
    timestamp: '2026-10-14T00:00:00Z',
    validation_errors: errors,
  })
}

function renderSection() {
  render(
    <MemoryRouter>
      <SetupPasswordSection navigate={navigate} />
    </MemoryRouter>
  )
  return userEvent.setup()
}

const email = () => screen.getByLabelText('Root admin email')
const password = () => screen.getByLabelText('Password')
const repeat = () => screen.getByLabelText('Repeat the password')
const submit = () =>
  screen.getByRole('button', { name: 'Set up admin and sign in' })

async function fill(
  user: ReturnType<typeof userEvent.setup>,
  values: { email?: string; password?: string; repeat?: string }
) {
  if (values.email) await user.type(email(), values.email)
  if (values.password) await user.type(password(), values.password)
  if (values.repeat) await user.type(repeat(), values.repeat)
}

beforeEach(() => {
  vi.clearAllMocks()
  settings.bootstrapAdminPassword.mockResolvedValue(undefined)
  auth.passwordLogin.mockResolvedValue({ id: 'u1' } as never)
})

describe('SetupPasswordSection', () => {
  it('creates the admin, signs in with the same credentials and leaves the page', async () => {
    const user = renderSection()
    await fill(user, {
      email: '  Root@Example.com ',
      password: PASSPHRASE,
      repeat: PASSPHRASE,
    })
    await user.click(submit())

    await waitFor(() => {
      expect(navigate).toHaveBeenCalledWith('/')
    })
    expect(settings.bootstrapAdminPassword).toHaveBeenCalledWith({
      email: 'Root@Example.com',
      password: PASSPHRASE,
    })
    expect(auth.passwordLogin).toHaveBeenCalledWith(
      'Root@Example.com',
      PASSPHRASE
    )
    // The password is set before the sign-in is tried.
    expect(
      settings.bootstrapAdminPassword.mock.invocationCallOrder[0]
    ).toBeLessThan(auth.passwordLogin.mock.invocationCallOrder[0])
  })

  it('does not mark an untouched form as wrong', () => {
    renderSection()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(email()).not.toHaveAttribute('aria-invalid')
  })

  it('asks for every field before calling the server', async () => {
    const user = renderSection()
    await user.click(submit())

    expect(email()).toHaveAttribute('aria-invalid', 'true')
    expect(password()).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByText('Enter the email address.')).toHaveAttribute(
      'role',
      'alert'
    )
    expect(screen.getByText('Enter a password.')).toBeVisible()
    expect(settings.bootstrapAdminPassword).not.toHaveBeenCalled()
  })

  it('refuses two different passwords without calling the server, and clears once they match', async () => {
    const user = renderSection()
    await fill(user, {
      email: 'root@example.com',
      password: PASSPHRASE,
      repeat: 'something else entirely',
    })
    await user.click(submit())

    expect(screen.getByText('The two passwords do not match.')).toBeVisible()
    expect(repeat()).toHaveAttribute('aria-invalid', 'true')
    expect(settings.bootstrapAdminPassword).not.toHaveBeenCalled()

    await user.clear(repeat())
    await user.type(repeat(), PASSPHRASE)
    expect(screen.queryByText('The two passwords do not match.')).toBeNull()
  })

  it('explains an address that is not a root admin on the email field, keeping what was typed', async () => {
    settings.bootstrapAdminPassword.mockRejectedValue(
      validationError([
        {
          field: 'email',
          message:
            'The email is not a root instance admin (auth.instance_admins)',
          code: 'not_root_admin',
        },
      ])
    )
    const user = renderSection()
    await fill(user, {
      email: 'someone@example.com',
      password: PASSPHRASE,
      repeat: PASSPHRASE,
    })
    await user.click(submit())

    const message = await screen.findByText(
      /This address is not a root instance admin/
    )
    expect(message).toHaveAttribute('role', 'alert')
    expect(email()).toHaveAttribute('aria-invalid', 'true')
    expect(email()).toHaveValue('someone@example.com')
    expect(password()).toHaveValue(PASSPHRASE)
    expect(auth.passwordLogin).not.toHaveBeenCalled()
    expect(navigate).not.toHaveBeenCalled()

    // Editing the field takes its server error away.
    await user.type(email(), 'x')
    expect(
      screen.queryByText(/This address is not a root instance admin/)
    ).toBeNull()
  })

  it('shows the password policy’s own message on the password field', async () => {
    settings.bootstrapAdminPassword.mockRejectedValue(
      validationError([
        {
          field: 'password',
          message: 'Use at least 12 characters.',
          code: 'too_short',
        },
      ])
    )
    const user = renderSection()
    await fill(user, {
      email: 'root@example.com',
      password: 'short',
      repeat: 'short',
    })
    await user.click(submit())

    expect(await screen.findByText('Use at least 12 characters.')).toBeVisible()
    expect(password()).toHaveAttribute('aria-invalid', 'true')
    expect(auth.passwordLogin).not.toHaveBeenCalled()
  })

  it('reports any other refusal at form level', async () => {
    settings.bootstrapAdminPassword.mockRejectedValue(
      new ApiError({
        type: 'about:blank',
        title: 'Conflict',
        status: 409,
        detail: 'The account with this email is suspended',
        code: 'RESOURCE_CONFLICT',
        request_id: 'req-1',
        timestamp: '2026-10-14T00:00:00Z',
      })
    )
    const user = renderSection()
    await fill(user, {
      email: 'root@example.com',
      password: PASSPHRASE,
      repeat: PASSPHRASE,
    })
    await user.click(submit())

    expect(await screen.findByTestId('setup-password-error')).toHaveTextContent(
      'The account with this email is suspended'
    )
    expect(submit()).toBeEnabled()
  })

  it('catches a malformed email before any request', async () => {
    const user = renderSection()
    await fill(user, {
      email: 'root@',
      password: PASSPHRASE,
      repeat: PASSPHRASE,
    })
    await user.click(submit())

    expect(email()).toHaveAccessibleDescription(
      'Enter a complete email address, such as admin@example.com.'
    )
    expect(settings.bootstrapAdminPassword).not.toHaveBeenCalled()
    expect(email()).toHaveValue('root@')
  })

  it('accepts an address whose domain has no dot', async () => {
    const user = renderSection()
    await fill(user, {
      email: 'root@localhost',
      password: PASSPHRASE,
      repeat: PASSPHRASE,
    })
    await user.click(submit())

    await waitFor(() => {
      expect(settings.bootstrapAdminPassword).toHaveBeenCalledWith({
        email: 'root@localhost',
        password: PASSPHRASE,
      })
    })
  })

  it('says the account is ready when only the sign-in failed, and offers to try again', async () => {
    auth.passwordLogin.mockRejectedValue(new Error('Network Error'))
    const user = renderSection()
    await fill(user, {
      email: 'root@example.com',
      password: PASSPHRASE,
      repeat: PASSPHRASE,
    })
    await user.click(submit())

    const partial = await screen.findByTestId('setup-password-partial')
    expect(partial).toHaveTextContent(
      'The admin account for root@example.com has the password you chose'
    )
    expect(partial).toHaveTextContent('Network Error')
    expect(
      screen.getByRole('link', { name: 'go to the sign-in page' })
    ).toHaveAttribute('href', '/login')
    expect(navigate).not.toHaveBeenCalled()
    // The password is never shown back.
    expect(partial).not.toHaveTextContent(PASSPHRASE)

    // Trying again signs in with the same credentials and never repeats the
    // bootstrap, which would end the sessions it just made possible.
    auth.passwordLogin.mockResolvedValue({ id: 'u1' } as never)
    await user.click(
      screen.getByRole('button', { name: 'Try signing in again' })
    )
    await waitFor(() => {
      expect(navigate).toHaveBeenCalledWith('/')
    })
    expect(auth.passwordLogin).toHaveBeenLastCalledWith(
      'root@example.com',
      PASSPHRASE
    )
    expect(settings.bootstrapAdminPassword).toHaveBeenCalledTimes(1)
  })

  it('disables the form while it is working', async () => {
    let finish: () => void = () => {}
    settings.bootstrapAdminPassword.mockReturnValue(
      new Promise<void>(resolve => {
        finish = resolve
      })
    )
    const user = renderSection()
    await fill(user, {
      email: 'root@example.com',
      password: PASSPHRASE,
      repeat: PASSPHRASE,
    })
    await user.click(submit())

    expect(screen.getByRole('button', { name: 'Setting up…' })).toBeDisabled()
    expect(email()).toBeDisabled()
    finish()
    await waitFor(() => {
      expect(navigate).toHaveBeenCalled()
    })
  })
})

describe('SetupPasswordSection in a signed-in browser (#1473)', () => {
  const reload = vi.fn()

  function renderSignedIn() {
    render(
      <MemoryRouter>
        <SetupPasswordSection navigate={navigate} reload={reload} />
      </MemoryRouter>
    )
    return userEvent.setup()
  }

  afterEach(() => {
    authState.isAuthenticated = false
    authState.user = null
  })

  it('says beforehand that the signed-in session is used, and signs out here', async () => {
    authState.isAuthenticated = true
    authState.user = { email: 'admin@example.com' }
    auth.logout.mockResolvedValue(undefined)
    const user = renderSignedIn()

    const note = screen.getByTestId('setup-signed-in-note')
    expect(note).toHaveTextContent('You are signed in as admin@example.com')
    expect(note).toHaveTextContent('private window')
    await user.click(screen.getByRole('button', { name: 'Sign out here' }))
    await waitFor(() => {
      expect(reload).toHaveBeenCalledTimes(1)
    })
    expect(auth.logout).toHaveBeenCalledTimes(1)
  })

  it('explains the refusal instead of showing it raw when the server finds a signed-in session', async () => {
    settings.bootstrapAdminPassword.mockRejectedValue(
      new ApiError({
        type: 'about:blank',
        title: 'Forbidden',
        status: 403,
        detail: "Only a setup session may set a root admin's password",
        code: 'FORBIDDEN',
        request_id: 'req-1',
        timestamp: '2026-10-14T00:00:00Z',
      })
    )
    const user = renderSignedIn()
    expect(screen.queryByTestId('setup-signed-in-note')).toBeNull()
    await fill(user, {
      email: 'root@example.com',
      password: PASSPHRASE,
      repeat: PASSPHRASE,
    })
    await user.click(submit())

    expect(await screen.findByTestId('setup-signed-in-note')).toBeVisible()
    expect(screen.queryByTestId('setup-password-error')).toBeNull()
    expect(screen.queryByText(/Only a setup session may set/)).toBeNull()
  })

  it('keeps the page when signing out fails', async () => {
    authState.isAuthenticated = true
    auth.logout.mockRejectedValue(new Error('Network Error'))
    const user = renderSignedIn()

    await user.click(screen.getByRole('button', { name: 'Sign out here' }))
    expect(
      await screen.findByText('Network Error', { selector: '[role=alert]' })
    ).toBeVisible()
    expect(reload).not.toHaveBeenCalled()
  })
})
