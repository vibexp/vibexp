import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import {
  type AdminAuthProvider,
  authSettingsService,
} from '@/services/authSettingsService'

vi.mock('@/services/authSettingsService', () => ({
  authSettingsService: {
    listProviders: vi.fn(),
    createProvider: vi.fn(),
    updateProvider: vi.fn(),
    deleteProvider: vi.fn(),
    testProvider: vi.fn(),
  },
}))

import { ProvidersSection } from '../ProvidersSection'
import { useAuthProviders } from '../useAuthProviders'
import {
  apiError,
  github,
  google,
  lockoutRisk,
  provider,
  versionConflict,
} from './fixtures'

const service = vi.mocked(authSettingsService)
const onChanged = vi.fn()

function Harness() {
  return <ProvidersSection state={useAuthProviders(0, onChanged)} />
}

const okta = provider()
const saved = (p: AdminAuthProvider, version: number) => ({
  provider: p,
  version,
})

async function renderSection(providers: AdminAuthProvider[], version = 7) {
  service.listProviders.mockResolvedValue({ providers, version })
  render(<Harness />)
  await screen.findByTestId('auth-providers-section')
  return userEvent.setup()
}

const rows = () => screen.getAllByTestId('auth-provider-row')
const rowOf = (slug: string) => {
  const row = rows().find(r => r.dataset.providerSlug === slug)
  if (!row) throw new Error(`no row for ${slug}`)
  return within(row)
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('ProvidersSection: the list', () => {
  it('lists type, name, health and enabled state in sign-in order', async () => {
    await renderSection([
      github,
      provider({
        enabled: false,
        health: { status: 'disabled', last_error: null, checked_at: null },
      }),
      google,
    ])
    expect(rows().map(r => r.dataset.providerSlug)).toEqual([
      'okta',
      'google',
      'github',
    ])
    const row = rowOf('okta')
    expect(row.getByText('Okta')).toBeVisible()
    expect(row.getByText('OpenID Connect')).toBeVisible()
    expect(row.getByTestId('auth-provider-health')).toHaveTextContent(
      'Disabled'
    )
    expect(row.getByRole('switch', { name: 'Okta enabled' })).not.toBeChecked()
    expect(
      rowOf('google').getByRole('switch', { name: 'Google enabled' })
    ).toBeChecked()
  })

  it('says why an unhealthy provider is not offered', async () => {
    await renderSection([
      provider({
        health: {
          status: 'unhealthy',
          last_error: 'discovery failed: 404',
          checked_at: '2026-10-10T00:00:00Z',
        },
      }),
    ])
    expect(screen.getByTestId('auth-provider-health')).toHaveTextContent(
      'Unhealthy'
    )
    expect(screen.getByTestId('auth-provider-error')).toHaveTextContent(
      'Not offered for sign-in: discovery failed: 404'
    )
  })

  it('shows the empty state', async () => {
    await renderSection([])
    expect(screen.getByTestId('auth-providers-empty')).toBeVisible()
  })

  it('shows a load failure', async () => {
    service.listProviders.mockRejectedValue(new Error('boom'))
    render(<Harness />)
    expect(await screen.findByText('boom')).toBeVisible()
    expect(screen.queryByTestId('auth-providers-section')).toBeNull()
  })

  it('tests a stored provider by id', async () => {
    service.testProvider.mockResolvedValue({ is_valid: true, message: null })
    const user = await renderSection([okta, google])
    await user.click(screen.getByRole('button', { name: 'Test Okta' }))
    expect(
      await rowOf('okta').findByTestId('auth-provider-test')
    ).toHaveTextContent('The test passed')
    expect(service.testProvider).toHaveBeenCalledWith({ id: okta.id })
    expect(rowOf('google').queryByTestId('auth-provider-test')).toBeNull()
  })

  it('shows a failed test and a test that could not run', async () => {
    service.testProvider.mockResolvedValueOnce({
      is_valid: false,
      message: 'invalid client',
    })
    const user = await renderSection([okta])
    await user.click(screen.getByRole('button', { name: 'Test Okta' }))
    expect(await screen.findByTestId('auth-provider-test')).toHaveTextContent(
      'invalid client'
    )
    service.testProvider.mockRejectedValueOnce(new Error('timeout'))
    await user.click(screen.getByRole('button', { name: 'Test Okta' }))
    await waitFor(() => {
      expect(screen.getByTestId('auth-provider-test')).toHaveTextContent(
        'timeout'
      )
    })
  })
})

describe('ProvidersSection: a row test result', () => {
  it('is dropped once the provider changes, since it no longer describes it', async () => {
    service.testProvider.mockResolvedValue({ is_valid: true, message: null })
    service.updateProvider.mockResolvedValue(saved(okta, 8))
    const user = await renderSection([okta, google])
    await user.click(screen.getByRole('button', { name: 'Test Okta' }))
    await rowOf('okta').findByTestId('auth-provider-test')

    // The list is re-read after the change, with the provider now disabled.
    service.listProviders.mockResolvedValue({
      providers: [
        provider({
          enabled: false,
          health: { status: 'disabled', last_error: null, checked_at: null },
        }),
        google,
      ],
      version: 8,
    })
    await user.click(screen.getByRole('switch', { name: 'Okta enabled' }))
    await waitFor(() => {
      expect(
        rowOf('okta').getByTestId('auth-provider-health')
      ).toHaveTextContent('Disabled')
    })
    expect(rowOf('okta').queryByTestId('auth-provider-test')).toBeNull()
  })

  it('stays while nothing changed', async () => {
    service.testProvider.mockResolvedValue({ is_valid: true, message: null })
    const user = await renderSection([okta, google])
    await user.click(screen.getByRole('button', { name: 'Test Okta' }))
    await rowOf('okta').findByTestId('auth-provider-test')
    await user.click(screen.getByRole('button', { name: 'Edit Google' }))
    await user.click(
      within(await screen.findByRole('dialog')).getByRole('button', {
        name: 'Cancel',
      })
    )
    expect(rowOf('okta').getByTestId('auth-provider-test')).toBeVisible()
  })
})

describe('ProvidersSection: enable and disable', () => {
  it('saves the switch against the loaded version, then re-reads the list', async () => {
    service.updateProvider.mockResolvedValue(saved(okta, 8))
    const user = await renderSection([okta, google])
    await user.click(screen.getByRole('switch', { name: 'Google enabled' }))

    await waitFor(() => {
      expect(service.updateProvider).toHaveBeenCalledWith(google.id, {
        display_name: 'Google',
        enabled: false,
        sort_order: 1,
        client_id: 'client-google',
        expected_version: 7,
        confirm_lockout_risk: false,
      })
    })
    await waitFor(() => {
      expect(service.listProviders).toHaveBeenCalledTimes(2)
    })
    expect(onChanged).toHaveBeenCalledTimes(1)
  })

  it('explains a no_enabled_provider lockout and retries only on confirm', async () => {
    service.updateProvider
      .mockRejectedValueOnce(lockoutRisk('no_enabled_provider'))
      .mockResolvedValueOnce(saved(okta, 8))
    const user = await renderSection([okta])
    await user.click(screen.getByRole('switch', { name: 'Okta enabled' }))

    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent('This leaves no way to sign in')
    expect(dialog).toHaveTextContent('no sign-in provider is enabled')
    expect(service.updateProvider).toHaveBeenCalledTimes(1)

    await user.click(
      within(dialog).getByRole('button', { name: 'Apply anyway' })
    )
    await waitFor(() => {
      expect(service.updateProvider).toHaveBeenCalledTimes(2)
    })
    expect(service.updateProvider.mock.calls[1]).toEqual([
      okta.id,
      expect.objectContaining({
        enabled: false,
        expected_version: 7,
        confirm_lockout_risk: true,
      }),
    ])
    await waitFor(() => {
      expect(screen.queryByRole('alertdialog')).toBeNull()
    })
    expect(onChanged).toHaveBeenCalledTimes(1)
  })

  it('explains an own_provider lockout differently', async () => {
    service.updateProvider.mockRejectedValueOnce(lockoutRisk('own_provider'))
    const user = await renderSection([okta, google])
    await user.click(screen.getByRole('switch', { name: 'Okta enabled' }))

    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent('This is the provider you signed in with')
    expect(dialog).toHaveTextContent('cannot sign in through it again')
    expect(dialog).not.toHaveTextContent('no sign-in provider is enabled')
  })

  it('does not apply a lockout risk the admin cancels', async () => {
    service.updateProvider.mockRejectedValueOnce(
      lockoutRisk('no_enabled_provider')
    )
    const user = await renderSection([okta])
    await user.click(screen.getByRole('switch', { name: 'Okta enabled' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))

    await waitFor(() => {
      expect(screen.queryByRole('alertdialog')).toBeNull()
    })
    expect(service.updateProvider).toHaveBeenCalledTimes(1)
    expect(onChanged).not.toHaveBeenCalled()
  })

  it('shows the reload prompt on a version conflict and never retries', async () => {
    service.updateProvider.mockRejectedValueOnce(versionConflict())
    const user = await renderSection([okta, google])
    await user.click(screen.getByRole('switch', { name: 'Okta enabled' }))

    const prompt = await screen.findByTestId('auth-providers-conflict')
    expect(prompt).toHaveTextContent('Someone else changed')
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(service.updateProvider).toHaveBeenCalledTimes(1)
    // Nothing can be changed until the list is reloaded.
    expect(screen.getByRole('switch', { name: 'Okta enabled' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Add provider' })).toBeDisabled()

    service.listProviders.mockResolvedValue({
      providers: [okta, google],
      version: 9,
    })
    await user.click(within(prompt).getByRole('button', { name: 'Reload' }))
    await waitFor(() => {
      expect(screen.queryByTestId('auth-providers-conflict')).toBeNull()
    })
    expect(service.updateProvider).toHaveBeenCalledTimes(1)

    // The next change is sent against the version the reload read.
    service.updateProvider.mockResolvedValueOnce(saved(okta, 10))
    await user.click(screen.getByRole('switch', { name: 'Okta enabled' }))
    await waitFor(() => {
      expect(service.updateProvider).toHaveBeenCalledTimes(2)
    })
    expect(service.updateProvider.mock.calls[1][1]).toMatchObject({
      expected_version: 9,
    })
  })

  it('a conflict on the confirmed retry shows the reload prompt, not a loop', async () => {
    service.updateProvider
      .mockRejectedValueOnce(lockoutRisk('no_enabled_provider'))
      .mockRejectedValueOnce(versionConflict())
    const user = await renderSection([okta])
    await user.click(screen.getByRole('switch', { name: 'Okta enabled' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(
      within(dialog).getByRole('button', { name: 'Apply anyway' })
    )

    expect(await screen.findByTestId('auth-providers-conflict')).toBeVisible()
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(service.updateProvider).toHaveBeenCalledTimes(2)
  })

  it('shows any other failure and re-reads the list', async () => {
    service.updateProvider.mockRejectedValueOnce(
      apiError(500, 'INTERNAL_ERROR', { detail: 'database unavailable' })
    )
    const user = await renderSection([okta, google])
    await user.click(screen.getByRole('switch', { name: 'Okta enabled' }))
    expect(await screen.findByTestId('auth-providers-error')).toHaveTextContent(
      'database unavailable'
    )
    expect(service.listProviders).toHaveBeenCalledTimes(2)
    expect(onChanged).not.toHaveBeenCalled()
  })
})

describe('ProvidersSection: delete', () => {
  it('asks first, then deletes against the loaded version', async () => {
    service.deleteProvider.mockResolvedValue(undefined)
    const user = await renderSection([okta, google])
    await user.click(screen.getByRole('button', { name: 'Delete Google' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent('Delete Google?')
    expect(service.deleteProvider).not.toHaveBeenCalled()

    await user.click(
      within(dialog).getByRole('button', { name: 'Delete provider' })
    )
    await waitFor(() => {
      expect(service.deleteProvider).toHaveBeenCalledWith(google.id, {
        expected_version: 7,
      })
    })
    await waitFor(() => {
      expect(onChanged).toHaveBeenCalledTimes(1)
    })
  })

  it('confirms a lockout risk with the query flag', async () => {
    service.deleteProvider
      .mockRejectedValueOnce(lockoutRisk('no_enabled_provider'))
      .mockResolvedValueOnce(undefined)
    const user = await renderSection([okta])
    await user.click(screen.getByRole('button', { name: 'Delete Okta' }))
    await user.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', {
        name: 'Delete provider',
      })
    )

    const lockout = await screen.findByText('This leaves no way to sign in')
    expect(lockout).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Apply anyway' }))
    await waitFor(() => {
      expect(service.deleteProvider).toHaveBeenCalledTimes(2)
    })
    expect(service.deleteProvider.mock.calls[1]).toEqual([
      okta.id,
      { expected_version: 7, confirm_lockout_risk: true },
    ])
  })

  it('does not delete when the admin cancels', async () => {
    const user = await renderSection([okta])
    await user.click(screen.getByRole('button', { name: 'Delete Okta' }))
    await user.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', {
        name: 'Cancel',
      })
    )
    expect(service.deleteProvider).not.toHaveBeenCalled()
  })
})

describe('ProvidersSection: reorder', () => {
  it('disables the moves that would leave the list', async () => {
    await renderSection([okta, google, github])
    expect(screen.getByRole('button', { name: 'Move Okta up' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Move Okta down' })).toBeEnabled()
    expect(
      screen.getByRole('button', { name: 'Move GitHub down' })
    ).toBeDisabled()
  })

  it('rewrites only the positions that change, chaining the version', async () => {
    service.updateProvider
      .mockResolvedValueOnce(saved(google, 8))
      .mockResolvedValueOnce(saved(okta, 9))
    const user = await renderSection([okta, google, github])
    await user.click(screen.getByRole('button', { name: 'Move Okta down' }))

    await waitFor(() => {
      expect(service.updateProvider).toHaveBeenCalledTimes(2)
    })
    expect(service.updateProvider.mock.calls).toEqual([
      [
        google.id,
        expect.objectContaining({ sort_order: 0, expected_version: 7 }),
      ],
      [
        okta.id,
        expect.objectContaining({ sort_order: 1, expected_version: 8 }),
      ],
    ])
    // No secret travels with a reorder.
    expect(service.updateProvider.mock.calls[0][1]).not.toHaveProperty(
      'client_secret'
    )
    await waitFor(() => {
      expect(service.listProviders).toHaveBeenCalledTimes(2)
    })
  })

  it('re-reads the list and reports a reorder that failed half way', async () => {
    service.updateProvider
      .mockResolvedValueOnce(saved(google, 8))
      .mockRejectedValueOnce(new Error('connection reset'))
    const user = await renderSection([okta, google])
    await user.click(screen.getByRole('button', { name: 'Move Okta down' }))

    expect(await screen.findByTestId('auth-providers-error')).toHaveTextContent(
      'connection reset'
    )
    expect(service.listProviders).toHaveBeenCalledTimes(2)
  })

  it('shows the reload prompt when a reorder loses a version race', async () => {
    service.updateProvider.mockRejectedValueOnce(versionConflict())
    const user = await renderSection([okta, google])
    await user.click(screen.getByRole('button', { name: 'Move Google up' }))
    expect(await screen.findByTestId('auth-providers-conflict')).toBeVisible()
    expect(service.updateProvider).toHaveBeenCalledTimes(1)
  })
})

describe('ProvidersSection: the dialog', () => {
  it('adds a provider through the dialog and closes it', async () => {
    service.createProvider.mockResolvedValue(saved(github, 8))
    const user = await renderSection([okta, google])
    await user.click(screen.getByRole('button', { name: 'Add provider' }))

    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('Add a sign-in provider')
    // Google is used, so GitHub is the first type offered.
    expect(within(dialog).queryByRole('radio', { name: 'Google' })).toBeNull()
    await user.type(within(dialog).getByLabelText('Client ID'), 'abc')
    await user.type(within(dialog).getByLabelText('Client secret'), 's3cret')
    await user.click(
      within(dialog).getByRole('button', { name: 'Add provider' })
    )

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).toBeNull()
    })
    expect(service.createProvider).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'github',
        slug: 'github',
        sort_order: 2,
        expected_version: 7,
      })
    )
    expect(onChanged).toHaveBeenCalledTimes(1)
  })

  it('opens the stored provider for editing', async () => {
    const user = await renderSection([okta])
    await user.click(screen.getByRole('button', { name: 'Edit Okta' }))
    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('Edit sign-in provider')
    expect(within(dialog).getByLabelText('Display name')).toHaveValue('Okta')
    expect(within(dialog).getByLabelText('Client secret')).toHaveValue('')

    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).toBeNull()
    })
  })

  it('a disable saved in the dialog still goes through the lockout confirm', async () => {
    service.updateProvider
      .mockRejectedValueOnce(lockoutRisk('own_provider'))
      .mockResolvedValueOnce(saved(okta, 8))
    const user = await renderSection([okta, google])
    await user.click(screen.getByRole('button', { name: 'Edit Okta' }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('switch', { name: 'Enabled' }))
    await user.click(
      within(dialog).getByRole('button', { name: 'Save changes' })
    )

    const confirm = await screen.findByRole('alertdialog')
    expect(confirm).toHaveTextContent('This is the provider you signed in with')
    await user.click(
      within(confirm).getByRole('button', { name: 'Apply anyway' })
    )
    await waitFor(() => {
      expect(service.updateProvider).toHaveBeenCalledTimes(2)
    })
    expect(service.updateProvider.mock.calls[1][1]).toMatchObject({
      enabled: false,
      confirm_lockout_risk: true,
    })
  })
})
