import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import { toast } from '@/lib/toast'
import { authService } from '@/services/authService'
import {
  type AdminAuthPolicy,
  authSettingsService,
} from '@/services/authSettingsService'

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('@/services/authSettingsService', () => ({
  authSettingsService: {
    getPolicy: vi.fn(),
    updatePolicy: vi.fn(),
  },
}))

vi.mock('@/services/authService', () => ({
  authService: { getSignInMethods: vi.fn() },
}))

import { AuthPolicySection } from '../AuthPolicySection'
import { apiError, lockoutRisk, policy, versionConflict } from './fixtures'

const service = vi.mocked(authSettingsService)
const auth = vi.mocked(authService)
const onSaved = vi.fn()

const stored = policy({
  password_login_enabled: true,
  registration_mode: 'invite_only',
  version: 4,
  updated_at: '2026-10-01T10:00:00Z',
})

/** How a forgotten password is reset, as the sign-in discovery reports it. */
const resetChannel = (reset: 'email' | 'admin') => {
  auth.getSignInMethods.mockResolvedValue({
    providers: [],
    password: { enabled: true, reset },
  })
}

async function renderSection(initial: AdminAuthPolicy) {
  service.getPolicy.mockResolvedValue(initial)
  render(
    <MemoryRouter>
      <AuthPolicySection onSaved={onSaved} />
    </MemoryRouter>
  )
  await screen.findByTestId('auth-policy-section')
  return userEvent.setup()
}

const save = () => screen.getByRole('button', { name: 'Save sign-in policy' })
const passwordSwitch = () =>
  screen.getByRole('switch', { name: 'Password sign-in' })
const mode = (name: string) => screen.getByRole('radio', { name })

beforeEach(() => {
  vi.clearAllMocks()
  resetChannel('email')
})

describe('AuthPolicySection', () => {
  it('shows the defaults of an instance that stored nothing', async () => {
    await renderSection(policy())
    expect(screen.getByTestId('policy-password-status')).toHaveTextContent(
      'Password sign-in off'
    )
    expect(passwordSwitch()).not.toBeChecked()
    expect(mode('Open')).toBeChecked()
    expect(mode('Invitation only')).not.toBeChecked()
    expect(save()).toBeDisabled()
  })

  it('shows the stored policy', async () => {
    await renderSection(stored)
    expect(screen.getByTestId('policy-password-status')).toHaveTextContent(
      'Password sign-in on'
    )
    expect(passwordSwitch()).toBeChecked()
    expect(mode('Invitation only')).toBeChecked()
  })

  it('enables Save only while something differs from what is stored', async () => {
    const user = await renderSection(policy())
    await user.click(passwordSwitch())
    expect(save()).toBeEnabled()
    expect(screen.getByText('You have unsaved changes.')).toBeVisible()
    await user.click(passwordSwitch())
    expect(save()).toBeDisabled()
  })

  it('saves both settings against the loaded version', async () => {
    service.updatePolicy.mockResolvedValue(
      policy({
        password_login_enabled: true,
        registration_mode: 'invite_only',
        version: 1,
      })
    )
    const user = await renderSection(policy())
    await user.click(passwordSwitch())
    await user.click(mode('Invitation only'))
    await user.click(save())

    await waitFor(() => {
      expect(service.updatePolicy).toHaveBeenCalledWith({
        password_login_enabled: true,
        registration_mode: 'invite_only',
        // Nothing was stored, so the save must find nothing stored.
        expected_version: 0,
        confirm_lockout_risk: false,
      })
    })
    expect(toast.success).toHaveBeenCalledWith('Sign-in policy saved')
    expect(onSaved).toHaveBeenCalledTimes(1)
    expect(screen.getByTestId('policy-password-status')).toHaveTextContent(
      'Password sign-in on'
    )
    expect(save()).toBeDisabled()
  })

  it('sends the stored version on a later save', async () => {
    service.updatePolicy.mockResolvedValue(
      policy({ ...stored, registration_mode: 'open', version: 5 })
    )
    const user = await renderSection(stored)
    await user.click(mode('Open'))
    await user.click(save())
    await waitFor(() => {
      expect(service.updatePolicy).toHaveBeenCalledWith(
        expect.objectContaining({ expected_version: 4 })
      )
    })
  })

  it('keeps the admin’s choices on a version conflict and reloads on request', async () => {
    service.updatePolicy.mockRejectedValue(versionConflict())
    const user = await renderSection(stored)
    await user.click(mode('Open'))
    await user.click(save())

    const conflict = await screen.findByTestId('policy-conflict')
    expect(conflict).toHaveTextContent(
      'Someone else changed the sign-in policy'
    )
    // Nothing the admin chose was thrown away, and it is not retried.
    expect(mode('Open')).toBeChecked()
    expect(save()).toBeDisabled()
    expect(service.updatePolicy).toHaveBeenCalledTimes(1)
    expect(onSaved).not.toHaveBeenCalled()

    service.getPolicy.mockResolvedValue(
      policy({ ...stored, password_login_enabled: false, version: 5 })
    )
    await user.click(within(conflict).getByRole('button', { name: 'Reload' }))
    await waitFor(() => {
      expect(screen.queryByTestId('policy-conflict')).toBeNull()
    })
    expect(passwordSwitch()).not.toBeChecked()
    expect(mode('Invitation only')).toBeChecked()
  })

  it.each([
    ['no_usable_method', {}, 'This leaves no way to sign in'],
    ['own_method', {}, 'You signed in with a password'],
    [
      'password_only_accounts',
      { password_only_accounts: 3 },
      '3 accounts can only sign in with a password',
    ],
    [
      'password_only_accounts',
      { password_only_accounts: 1 },
      '1 account can only sign in with a password',
    ],
    ['something_new', {}, 'This change could lock you out'],
  ])(
    'asks before switching password sign-in off when the server reports %s',
    async (reason, metadata, title) => {
      service.updatePolicy
        .mockRejectedValueOnce(lockoutRisk(reason, metadata))
        .mockResolvedValueOnce(
          policy({ ...stored, password_login_enabled: false, version: 5 })
        )
      const user = await renderSection(stored)
      await user.click(passwordSwitch())
      await user.click(save())

      const dialog = await screen.findByRole('alertdialog')
      expect(within(dialog).getByText(title)).toBeVisible()
      // Nothing is saved until the admin confirms.
      expect(onSaved).not.toHaveBeenCalled()

      await user.click(
        within(dialog).getByRole('button', { name: 'Switch it off anyway' })
      )
      await waitFor(() => {
        expect(service.updatePolicy).toHaveBeenLastCalledWith({
          password_login_enabled: false,
          registration_mode: 'invite_only',
          expected_version: 4,
          confirm_lockout_risk: true,
        })
      })
      await waitFor(() => {
        expect(screen.queryByRole('alertdialog')).toBeNull()
      })
      expect(onSaved).toHaveBeenCalledTimes(1)
      expect(screen.getByTestId('policy-password-status')).toHaveTextContent(
        'Password sign-in off'
      )
    }
  )

  it('keeps the unsaved choice when the lockout confirmation is cancelled', async () => {
    service.updatePolicy.mockRejectedValue(lockoutRisk('own_method'))
    const user = await renderSection(stored)
    await user.click(passwordSwitch())
    await user.click(save())

    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => {
      expect(screen.queryByRole('alertdialog')).toBeNull()
    })
    expect(service.updatePolicy).toHaveBeenCalledTimes(1)
    expect(passwordSwitch()).not.toBeChecked()
    expect(save()).toBeEnabled()
    expect(screen.getByTestId('policy-password-status')).toHaveTextContent(
      'Password sign-in on'
    )
  })

  it('reports any other failure and keeps the form', async () => {
    service.updatePolicy.mockRejectedValue(
      apiError(500, 'INTERNAL_ERROR', { detail: 'Something broke' })
    )
    const user = await renderSection(policy())
    await user.click(passwordSwitch())
    await user.click(save())

    expect(await screen.findByTestId('policy-error')).toHaveTextContent(
      'Something broke'
    )
    expect(passwordSwitch()).toBeChecked()
    expect(save()).toBeEnabled()
  })

  describe('the note about password resets', () => {
    it('shows while password sign-in is on and resets need an administrator', async () => {
      resetChannel('admin')
      await renderSection(stored)
      const note = await screen.findByTestId('policy-no-email-note')
      expect(note).toHaveTextContent('No email provider is configured')
      expect(
        within(note).getByRole('link', { name: 'Set up email' })
      ).toHaveAttribute('href', '/admin/settings/email')
    })

    it('is hidden when a reset link can be emailed', async () => {
      resetChannel('email')
      await renderSection(stored)
      await waitFor(() => {
        expect(auth.getSignInMethods).toHaveBeenCalled()
      })
      expect(screen.queryByTestId('policy-no-email-note')).toBeNull()
    })

    it('is hidden while password sign-in is off, even with no email provider', async () => {
      resetChannel('admin')
      const user = await renderSection(policy())
      await waitFor(() => {
        expect(auth.getSignInMethods).toHaveBeenCalled()
      })
      expect(screen.queryByTestId('policy-no-email-note')).toBeNull()
      // It follows what is STORED, not an unsaved toggle.
      await user.click(passwordSwitch())
      expect(screen.queryByTestId('policy-no-email-note')).toBeNull()
    })

    it('is hidden when the reset channel cannot be read', async () => {
      auth.getSignInMethods.mockRejectedValue(new Error('offline'))
      await renderSection(stored)
      await waitFor(() => {
        expect(auth.getSignInMethods).toHaveBeenCalled()
      })
      expect(screen.queryByTestId('policy-no-email-note')).toBeNull()
    })
  })

  it('reports a failed load', async () => {
    service.getPolicy.mockRejectedValue(new Error('offline'))
    render(
      <MemoryRouter>
        <AuthPolicySection />
      </MemoryRouter>
    )
    expect(await screen.findByTestId('policy-load-failed')).toHaveTextContent(
      'offline'
    )
  })
})
