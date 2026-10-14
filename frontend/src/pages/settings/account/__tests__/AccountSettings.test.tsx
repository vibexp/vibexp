import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import { toast } from '@/lib/toast'
import { accountService } from '@/services/accountService'

const { refreshUser } = vi.hoisted(() => ({ refreshUser: vi.fn() }))

vi.mock('@/services/accountService', () => ({
  accountService: {
    get: vi.fn(),
    updateName: vi.fn(),
    setPassword: vi.fn(),
    removePassword: vi.fn(),
    disconnectIdentity: vi.fn(),
  },
}))

vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ refreshUser }),
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import { AccountSettings } from '../AccountSettings'
import { account, apiError, identity, withPassword } from './fixtures'

const service = vi.mocked(accountService)
const NEW_PASSWORD = 'a brand new passphrase'

const leave = vi.fn()

function renderPage() {
  render(<AccountSettings leave={leave} />, { wrapper: MemoryRouter })
  return userEvent.setup()
}

/** A promise the test settles by hand, to hold a request in flight. */
function deferred<T>() {
  let resolve: (value: T) => void = () => {}
  const promise = new Promise<T>(done => {
    resolve = done
  })
  return { promise, resolve }
}

async function fillChange(
  user: ReturnType<typeof userEvent.setup>,
  current = 'the old one'
) {
  const form = await passwordForm('Change password')
  await user.type(form.getByLabelText('Current password'), current)
  await user.type(form.getByLabelText('New password'), NEW_PASSWORD)
  await user.type(form.getByLabelText('Repeat the new password'), NEW_PASSWORD)
  return form
}

async function passwordForm(name: 'Set a password' | 'Change password') {
  return within(await screen.findByRole('form', { name }))
}

beforeEach(() => {
  vi.clearAllMocks()
  refreshUser.mockResolvedValue(undefined)
  service.setPassword.mockResolvedValue(undefined)
  service.removePassword.mockResolvedValue(undefined)
  service.disconnectIdentity.mockResolvedValue(undefined)
})

describe('AccountSettings: loading', () => {
  it('offers to try again when the account cannot be read', async () => {
    service.get
      .mockRejectedValueOnce(new Error('Network Error'))
      .mockResolvedValueOnce(account())
    const user = renderPage()

    expect(await screen.findByTestId('account-load-error')).toHaveTextContent(
      'Network Error'
    )
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByTestId('account-profile-card')).toBeVisible()
  })
})

describe('AccountSettings: profile', () => {
  it('saves the name and refreshes the signed-in user', async () => {
    service.get.mockResolvedValue(account())
    service.updateName.mockResolvedValue(account({ name: 'Ada King' }))
    const user = renderPage()

    const name = await screen.findByLabelText('Name')
    const save = screen.getByRole('button', { name: 'Save name' })
    expect(save).toBeDisabled()
    expect(screen.getByLabelText('Email')).toHaveValue('ada@example.com')
    expect(screen.getByLabelText('Email')).toBeDisabled()

    await user.clear(name)
    await user.type(name, 'Ada King')
    await user.click(save)

    await waitFor(() => {
      expect(service.updateName).toHaveBeenCalledWith('Ada King')
    })
    await waitFor(() => {
      expect(refreshUser).toHaveBeenCalledTimes(1)
    })
    expect(name).toHaveValue('Ada King')
    expect(screen.getByRole('button', { name: 'Save name' })).toBeDisabled()
  })

  it('refuses a blank name before any request and shows a server error on the field', async () => {
    service.get.mockResolvedValue(account())
    const user = renderPage()
    const name = await screen.findByLabelText('Name')

    await user.clear(name)
    await user.type(name, '   ')
    await user.click(screen.getByRole('button', { name: 'Save name' }))
    expect(await screen.findByText('Enter your name.')).toBeVisible()
    expect(service.updateName).not.toHaveBeenCalled()

    service.updateName.mockRejectedValue(
      apiError(400, 'VALIDATION_FAILED', 'Invalid', [
        { field: 'name', message: 'The name is too long', code: 'too_long' },
      ])
    )
    await user.clear(name)
    await user.type(name, 'Ada King')
    await user.click(screen.getByRole('button', { name: 'Save name' }))
    expect(await screen.findByText('The name is too long')).toBeVisible()
    expect(refreshUser).not.toHaveBeenCalled()
  })
})

describe('AccountSettings: the three password cases', () => {
  it('explains that password sign-in is off, and still lets an existing password be removed', async () => {
    service.get.mockResolvedValue(
      account({
        password: {
          set: true,
          changed_at: '2026-10-01T10:00:00Z',
          login_enabled: false,
        },
      })
    )
    renderPage()

    expect(
      await screen.findByTestId('account-password-disabled')
    ).toHaveTextContent('has not switched password sign-in on')
    expect(screen.queryByRole('form', { name: 'Change password' })).toBeNull()
    expect(
      screen.getByRole('button', { name: 'Remove password' })
    ).toBeEnabled()
    // With the password unusable, the provider is the only way in.
    expect(
      screen.getByRole('button', { name: 'Disconnect Google' })
    ).toBeDisabled()
  })

  it('sets a first password, with nothing else in flight until it answers', async () => {
    service.get
      .mockResolvedValueOnce(account())
      .mockResolvedValueOnce(withPassword())
    let finish: () => void = () => {}
    service.setPassword.mockReturnValue(
      new Promise<void>(resolve => {
        finish = resolve
      })
    )
    const user = renderPage()
    const form = await passwordForm('Set a password')

    expect(screen.getByTestId('account-password-row')).toHaveTextContent(
      'Not set'
    )
    expect(form.queryByLabelText('Current password')).toBeNull()
    expect(form.getByText(/At least 12 characters/)).toBeVisible()

    await user.type(form.getByLabelText('New password'), NEW_PASSWORD)
    await user.type(
      form.getByLabelText('Repeat the new password'),
      NEW_PASSWORD
    )
    await user.click(form.getByRole('button', { name: 'Set password' }))

    await waitFor(() => {
      expect(service.setPassword).toHaveBeenCalledWith({
        new_password: NEW_PASSWORD,
        revoke_api_access: false,
      })
    })
    // The call ends other sessions and re-issues the cookie: no re-read, and
    // no other control, until it has answered.
    expect(service.get).toHaveBeenCalledTimes(1)
    expect(
      screen.getByRole('button', { name: 'Disconnect Google' })
    ).toBeDisabled()

    finish()
    await waitFor(() => {
      expect(service.get).toHaveBeenCalledTimes(2)
    })
    expect(
      await screen.findByRole('form', { name: 'Change password' })
    ).toBeVisible()
    expect(toast.success).toHaveBeenCalledWith(
      'Password set. Your other devices were signed out.'
    )
  })

  it('changes a password and says what was revoked', async () => {
    service.get.mockResolvedValue(withPassword())
    const user = renderPage()
    const form = await passwordForm('Change password')

    await user.type(form.getByLabelText('Current password'), 'the old one')
    await user.type(form.getByLabelText('New password'), NEW_PASSWORD)
    await user.type(
      form.getByLabelText('Repeat the new password'),
      NEW_PASSWORD
    )
    await user.click(
      form.getByRole('checkbox', {
        name: 'Also revoke API keys and connected tools',
      })
    )
    await user.click(form.getByRole('button', { name: 'Change password' }))

    await waitFor(() => {
      expect(service.setPassword).toHaveBeenCalledWith({
        current_password: 'the old one',
        new_password: NEW_PASSWORD,
        revoke_api_access: true,
      })
    })
    await waitFor(() => {
      expect(toast.success).toHaveBeenCalledWith(
        'Password changed. Your other devices were signed out, and your API keys and connected tools were revoked.'
      )
    })
    expect(form.getByLabelText('New password')).toHaveValue('')
  })
})

describe('AccountSettings: password errors', () => {
  it('catches a short password and a mismatch before any request', async () => {
    service.get.mockResolvedValue(withPassword())
    const user = renderPage()
    const form = await passwordForm('Change password')

    await user.type(form.getByLabelText('New password'), 'short')
    await user.type(form.getByLabelText('Repeat the new password'), 'shorter')
    await user.click(form.getByRole('button', { name: 'Change password' }))

    expect(await form.findByText('Enter your current password.')).toBeVisible()
    expect(form.getByText('Use at least 12 characters.')).toBeVisible()
    expect(form.getByText('The two passwords do not match.')).toBeVisible()
    expect(service.setPassword).not.toHaveBeenCalled()
  })

  it('puts server field errors on their fields and keeps what was typed', async () => {
    service.get.mockResolvedValue(withPassword())
    service.setPassword.mockRejectedValue(
      apiError(400, 'VALIDATION_FAILED', 'Invalid', [
        {
          field: 'current_password',
          message: 'The current password is not correct',
          code: 'invalid_current_password',
        },
        {
          field: 'new_password',
          message: 'This password is too common',
          code: 'common',
        },
      ])
    )
    const user = renderPage()
    const form = await passwordForm('Change password')

    await user.type(form.getByLabelText('Current password'), 'wrong one')
    await user.type(form.getByLabelText('New password'), NEW_PASSWORD)
    await user.type(
      form.getByLabelText('Repeat the new password'),
      NEW_PASSWORD
    )
    await user.click(form.getByRole('button', { name: 'Change password' }))

    expect(
      await form.findByText('The current password is not correct')
    ).toBeVisible()
    expect(form.getByText('This password is too common')).toBeVisible()
    expect(form.getByLabelText('Current password')).toHaveValue('wrong one')
    expect(toast.success).not.toHaveBeenCalled()
    // Nothing changed, so nothing is read again.
    expect(service.get).toHaveBeenCalledTimes(1)
  })

  it('says to wait when the account is locked', async () => {
    service.get.mockResolvedValue(withPassword())
    service.setPassword.mockRejectedValue(
      apiError(429, 'too_many_attempts', 'Too many attempts')
    )
    const user = renderPage()
    const form = await passwordForm('Change password')

    await user.type(form.getByLabelText('Current password'), 'wrong one')
    await user.type(form.getByLabelText('New password'), NEW_PASSWORD)
    await user.type(
      form.getByLabelText('Repeat the new password'),
      NEW_PASSWORD
    )
    await user.click(form.getByRole('button', { name: 'Change password' }))

    expect(
      await screen.findByTestId('account-password-error')
    ).toHaveTextContent('Too many wrong passwords')
  })
})

describe('AccountSettings: recent sign-in', () => {
  it('says beforehand that the guarded changes need a new sign-in', async () => {
    service.get.mockResolvedValue(
      account({
        recent_sign_in: false,
        identities: [
          identity(),
          identity({ id: 'gh', provider: 'github', display_name: 'GitHub' }),
        ],
      })
    )
    renderPage()

    const note = await screen.findByTestId('account-reauth-note')
    expect(
      within(note).getByRole('link', { name: 'Sign in again' })
    ).toHaveAttribute('href', '/login?return_to=/settings/account')
    expect(
      screen.getByRole('button', { name: 'Disconnect GitHub' })
    ).toBeDisabled()
    const form = await passwordForm('Set a password')
    expect(form.getByRole('button', { name: 'Set password' })).toBeDisabled()
  })

  it('does not hold back a password CHANGE, which needs the current password instead', async () => {
    service.get.mockResolvedValue(withPassword({ recent_sign_in: false }))
    renderPage()

    const form = await passwordForm('Change password')
    expect(form.getByRole('button', { name: 'Change password' })).toBeEnabled()
    expect(
      screen.getByRole('button', { name: 'Remove password' })
    ).toBeDisabled()
  })

  it('shows the note when the server refuses although the page thought the sign-in recent', async () => {
    service.get.mockResolvedValue(withPassword())
    service.removePassword.mockRejectedValue(
      apiError(403, 'reauthentication_required', 'Sign in again')
    )
    const user = renderPage()

    await user.click(
      await screen.findByRole('button', { name: 'Remove password' })
    )
    await user.click(
      within(screen.getByRole('alertdialog')).getByRole('button', {
        name: 'Remove password',
      })
    )

    expect(await screen.findByTestId('account-reauth-note')).toBeVisible()
    expect(screen.getByTestId('account-method-error')).toHaveTextContent(
      'recent sign-in'
    )
    expect(screen.queryByRole('alertdialog')).toBeNull()
  })
})

describe('AccountSettings: removing a sign-in method', () => {
  it('keeps the last method and says why', async () => {
    service.get.mockResolvedValue(account())
    renderPage()

    const row = await screen.findByTestId('account-identity-google')
    expect(
      within(row).getByRole('button', { name: 'Disconnect Google' })
    ).toBeDisabled()
    expect(row).toHaveTextContent('your only way to sign in')
    expect(screen.queryByRole('button', { name: 'Remove password' })).toBeNull()
  })

  it('disconnects a provider after a confirmation and re-reads the account', async () => {
    service.get
      .mockResolvedValueOnce(withPassword())
      .mockResolvedValueOnce(withPassword({ identities: [] }))
    const user = renderPage()

    await user.click(
      await screen.findByRole('button', { name: 'Disconnect Google' })
    )
    const dialog = screen.getByRole('alertdialog')
    expect(dialog).toHaveTextContent('your other devices are signed out')
    expect(service.disconnectIdentity).not.toHaveBeenCalled()

    await user.click(
      within(dialog).getByRole('button', { name: 'Disconnect Google' })
    )
    await waitFor(() => {
      expect(service.disconnectIdentity).toHaveBeenCalledWith(identity().id)
    })
    await waitFor(() => {
      expect(screen.queryByTestId('account-identity-google')).toBeNull()
    })
    expect(toast.success).toHaveBeenCalledWith(
      'Google disconnected. Your other devices were signed out.'
    )
    // Now the password is the only method left.
    expect(
      screen.getByRole('button', { name: 'Remove password' })
    ).toBeDisabled()
    expect(screen.getByTestId('account-password-row')).toHaveTextContent(
      'No enabled sign-in provider is connected'
    )
  })

  it('cancelling a confirmation changes nothing', async () => {
    service.get.mockResolvedValue(withPassword())
    const user = renderPage()

    await user.click(
      await screen.findByRole('button', { name: 'Remove password' })
    )
    await user.click(
      within(screen.getByRole('alertdialog')).getByRole('button', {
        name: 'Cancel',
      })
    )
    expect(service.removePassword).not.toHaveBeenCalled()
    expect(screen.queryByRole('alertdialog')).toBeNull()
  })

  it('explains the server refusing the last method the page could not see', async () => {
    // An administrator disabled the password: the page still counts it.
    service.get.mockResolvedValue(withPassword())
    service.disconnectIdentity.mockRejectedValue(
      apiError(409, 'last_sign_in_method', 'No other method')
    )
    const user = renderPage()

    await user.click(
      await screen.findByRole('button', { name: 'Disconnect Google' })
    )
    await user.click(
      within(screen.getByRole('alertdialog')).getByRole('button', {
        name: 'Disconnect Google',
      })
    )
    expect(await screen.findByTestId('account-method-error')).toHaveTextContent(
      'only way left to sign in'
    )
    expect(screen.getByTestId('account-identity-google')).toBeVisible()
  })

  it('marks an identity whose provider is disabled', async () => {
    service.get.mockResolvedValue(
      withPassword({
        identities: [identity({ available: false, last_used_at: null })],
      })
    )
    renderPage()

    const row = await screen.findByTestId('account-identity-google')
    expect(row).toHaveTextContent('Provider disabled')
    expect(row).toHaveTextContent('Not used to sign in yet.')
  })

  it('renders an account with no password and no identity (dev login)', async () => {
    service.get.mockResolvedValue(account({ identities: [] }))
    renderPage()

    expect(await screen.findByTestId('account-password-row')).toHaveTextContent(
      'You have not set a password.'
    )
    expect(screen.queryByRole('button', { name: /Disconnect/ })).toBeNull()
    expect(
      await screen.findByRole('form', { name: 'Set a password' })
    ).toBeVisible()
  })
})

describe('AccountSettings: one request at a time', () => {
  it('stays busy through the re-read that follows a change', async () => {
    const reread = deferred<ReturnType<typeof withPassword>>()
    service.get
      .mockResolvedValueOnce(withPassword())
      .mockReturnValueOnce(reread.promise)
    const user = renderPage()

    await user.click(
      await screen.findByRole('button', { name: 'Remove password' })
    )
    await user.click(
      within(screen.getByRole('alertdialog')).getByRole('button', {
        name: 'Remove password',
      })
    )
    await waitFor(() => {
      expect(service.get).toHaveBeenCalledTimes(2)
    })

    // The change answered and the re-read is in flight: a second click must
    // not send another session-ending call, and nothing else may start.
    const confirm = within(screen.getByRole('alertdialog')).getByRole(
      'button',
      { name: /Remove password|Working/ }
    )
    expect(confirm).toBeDisabled()
    // The page behind the open dialog is hidden from the accessibility tree.
    expect(screen.getByLabelText('Name')).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Change password', hidden: true })
    ).toBeDisabled()
    expect(service.removePassword).toHaveBeenCalledTimes(1)

    reread.resolve(account())
    await waitFor(() => {
      expect(screen.queryByRole('alertdialog')).toBeNull()
    })
    expect(
      await screen.findByRole('form', { name: 'Set a password' })
    ).toBeVisible()
    expect(service.removePassword).toHaveBeenCalledTimes(1)
  })

  it('holds the sign-in controls while a name save and its user refresh are in flight', async () => {
    service.get.mockResolvedValue(withPassword())
    const saving = deferred<ReturnType<typeof withPassword>>()
    const refreshing = deferred<undefined>()
    service.updateName.mockReturnValue(saving.promise)
    refreshUser.mockReturnValue(refreshing.promise)
    const user = renderPage()
    const form = await fillChange(user)

    const name = screen.getByLabelText('Name')
    await user.clear(name)
    await user.type(name, 'Ada King')
    await user.click(screen.getByRole('button', { name: 'Save name' }))

    const change = form.getByRole('button', { name: 'Change password' })
    expect(change).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Disconnect Google' })
    ).toBeDisabled()

    saving.resolve(withPassword({ name: 'Ada King' }))
    await waitFor(() => {
      expect(refreshUser).toHaveBeenCalledTimes(1)
    })
    // GET /auth/me is still out.
    expect(change).toBeDisabled()
    expect(service.setPassword).not.toHaveBeenCalled()

    refreshing.resolve(undefined)
    await waitFor(() => {
      expect(change).toBeEnabled()
    })
    expect(toast.success).toHaveBeenCalledWith('Name saved')
  })

  it('holds the profile while a password change is in flight', async () => {
    service.get.mockResolvedValue(withPassword())
    const changing = deferred<undefined>()
    service.setPassword.mockReturnValue(changing.promise)
    const user = renderPage()
    const form = await fillChange(user)

    await user.click(form.getByRole('button', { name: 'Change password' }))
    await waitFor(() => {
      expect(service.setPassword).toHaveBeenCalledTimes(1)
    })
    expect(screen.getByLabelText('Name')).toBeDisabled()
    expect(service.get).toHaveBeenCalledTimes(1)

    changing.resolve(undefined)
    await waitFor(() => {
      expect(screen.getByLabelText('Name')).toBeEnabled()
    })
  })
})

describe('AccountSettings: a signed-out or stale page', () => {
  it('leaves for the sign-in page when a change answers 401', async () => {
    service.get.mockResolvedValue(withPassword())
    service.setPassword.mockRejectedValue(
      apiError(401, 'AUTH_REQUIRED', 'Not signed in')
    )
    const user = renderPage()
    const form = await fillChange(user)
    await user.click(form.getByRole('button', { name: 'Change password' }))

    await waitFor(() => {
      expect(leave).toHaveBeenCalledWith('/login?return_to=/settings/account')
    })
    // A 401 is a real sign-out: nothing is retried or read again.
    expect(service.get).toHaveBeenCalledTimes(1)
    expect(service.setPassword).toHaveBeenCalledTimes(1)
  })

  it('leaves for the sign-in page when the name save answers 401', async () => {
    service.get.mockResolvedValue(account())
    service.updateName.mockRejectedValue(
      apiError(401, 'AUTH_REQUIRED', 'Authentication required')
    )
    const user = renderPage()
    const name = await screen.findByLabelText('Name')
    await user.clear(name)
    await user.type(name, 'Ada King')
    await user.click(screen.getByRole('button', { name: 'Save name' }))

    await waitFor(() => {
      expect(leave).toHaveBeenCalledWith('/login?return_to=/settings/account')
    })
    expect(refreshUser).not.toHaveBeenCalled()
  })

  it('reads the account once, whatever the caller passes as leave', async () => {
    service.get.mockResolvedValue(account())
    const { rerender } = render(<AccountSettings leave={url => leave(url)} />, {
      wrapper: MemoryRouter,
    })
    await screen.findByTestId('account-profile-card')
    rerender(<AccountSettings leave={url => leave(url)} />)
    await new Promise(resolve => setTimeout(resolve, 50))
    expect(service.get).toHaveBeenCalledTimes(1)
  })

  it('leaves for the sign-in page when the account read answers 401', async () => {
    service.get.mockRejectedValue(
      apiError(401, 'AUTH_REQUIRED', 'Not signed in')
    )
    renderPage()

    await waitFor(() => {
      expect(leave).toHaveBeenCalledWith('/login?return_to=/settings/account')
    })
    expect(screen.queryByTestId('account-load-error')).toBeNull()
  })

  it('reads the account again after a failure that may have changed it', async () => {
    // The password was changed but revoking the API access failed.
    service.get
      .mockResolvedValueOnce(account())
      .mockResolvedValueOnce(withPassword())
    service.setPassword.mockRejectedValue(
      apiError(
        500,
        'INTERNAL',
        'The password was changed, but API access could not be revoked'
      )
    )
    const user = renderPage()
    const form = await passwordForm('Set a password')
    await user.type(form.getByLabelText('New password'), NEW_PASSWORD)
    await user.type(
      form.getByLabelText('Repeat the new password'),
      NEW_PASSWORD
    )
    await user.click(form.getByRole('button', { name: 'Set password' }))

    expect(
      await screen.findByRole('form', { name: 'Change password' })
    ).toBeVisible()
    expect(screen.getByTestId('account-password-row')).toHaveTextContent('Set')
    expect(screen.getByTestId('account-password-error')).toHaveTextContent(
      'API access could not be revoked'
    )
    expect(toast.success).not.toHaveBeenCalled()
  })

  it('says so when the account turns out to have a password already', async () => {
    // Set in another tab: the page still shows "Set a password".
    service.get
      .mockResolvedValueOnce(account())
      .mockResolvedValueOnce(withPassword())
    service.setPassword.mockRejectedValue(
      apiError(400, 'VALIDATION_FAILED', 'Invalid', [
        {
          field: 'current_password',
          message: 'The current password is required',
          code: 'invalid_current_password',
        },
      ])
    )
    const user = renderPage()
    const form = await passwordForm('Set a password')
    await user.type(form.getByLabelText('New password'), NEW_PASSWORD)
    await user.type(
      form.getByLabelText('Repeat the new password'),
      NEW_PASSWORD
    )
    await user.click(form.getByRole('button', { name: 'Set password' }))

    expect(
      await screen.findByTestId('account-password-error')
    ).toHaveTextContent('Your account already has a password')
    expect(
      await screen.findByRole('form', { name: 'Change password' })
    ).toBeVisible()
  })

  it('shows the administrator note when password sign-in was switched off meanwhile', async () => {
    service.get.mockResolvedValueOnce(withPassword()).mockResolvedValueOnce(
      account({
        password: {
          set: true,
          changed_at: '2026-10-01T10:00:00Z',
          login_enabled: false,
        },
      })
    )
    service.setPassword.mockRejectedValue(
      apiError(409, 'password_login_disabled', 'Password sign-in is off')
    )
    const user = renderPage()
    const form = await fillChange(user)
    await user.click(form.getByRole('button', { name: 'Change password' }))

    expect(await screen.findByTestId('account-password-disabled')).toBeVisible()
    expect(screen.queryByRole('form', { name: 'Change password' })).toBeNull()
  })

  it('does not keep a stale account on screen when the re-read after a change fails', async () => {
    service.get
      .mockResolvedValueOnce(withPassword())
      .mockRejectedValueOnce(new Error('Network Error'))
      .mockResolvedValueOnce(withPassword({ identities: [] }))
    const user = renderPage()

    await user.click(
      await screen.findByRole('button', { name: 'Disconnect Google' })
    )
    await user.click(
      within(screen.getByRole('alertdialog')).getByRole('button', {
        name: 'Disconnect Google',
      })
    )

    expect(await screen.findByTestId('account-load-error')).toHaveTextContent(
      'Network Error'
    )
    expect(screen.queryByTestId('account-sign-in-card')).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByTestId('account-sign-in-card')).toBeVisible()
    expect(screen.queryByTestId('account-identity-google')).toBeNull()
  })

  it('explains a refusal for a session that is not a browser session', async () => {
    service.get.mockResolvedValue(withPassword())
    service.removePassword.mockRejectedValue(
      apiError(403, 'cookie_session_required', 'Cookie session required')
    )
    const user = renderPage()
    await user.click(
      await screen.findByRole('button', { name: 'Remove password' })
    )
    await user.click(
      within(screen.getByRole('alertdialog')).getByRole('button', {
        name: 'Remove password',
      })
    )
    expect(await screen.findByTestId('account-method-error')).toHaveTextContent(
      'browser session'
    )
    // Not the recent sign-in refusal: no note.
    expect(screen.queryByTestId('account-reauth-note')).toBeNull()
  })
})

describe('AccountSettings: the sign-in-again note only where it helps', () => {
  it('is not shown when signing in again would unlock nothing', async () => {
    // One provider, password sign-in off: nothing can be set or removed.
    service.get.mockResolvedValue(
      account({
        recent_sign_in: false,
        password: { set: false, changed_at: null, login_enabled: false },
      })
    )
    renderPage()

    await screen.findByTestId('account-sign-in-card')
    expect(screen.queryByTestId('account-reauth-note')).toBeNull()
  })

  it('is not shown to a password-only account, which can only change its password', async () => {
    service.get.mockResolvedValue(
      withPassword({ recent_sign_in: false, identities: [] })
    )
    renderPage()

    const form = await passwordForm('Change password')
    expect(screen.queryByTestId('account-reauth-note')).toBeNull()
    expect(form.getByRole('button', { name: 'Change password' })).toBeEnabled()
  })

  it('ties the reason to the disabled control', async () => {
    service.get.mockResolvedValue(account())
    renderPage()

    expect(
      await screen.findByRole('button', { name: 'Disconnect Google' })
    ).toHaveAccessibleDescription(
      'This is your only way to sign in, so it cannot be removed.'
    )
  })
})
