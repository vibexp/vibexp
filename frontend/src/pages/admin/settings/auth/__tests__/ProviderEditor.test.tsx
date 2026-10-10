import { render, screen, waitFor } from '@testing-library/react'
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
    testProvider: vi.fn(),
  },
}))

import { availableProviderTypes } from '../authSettingsForm'
import { ProviderEditor } from '../ProviderDialog'
import { useAuthProviders } from '../useAuthProviders'
import { apiError, google, provider, REDIRECT_URI } from './fixtures'

const service = vi.mocked(authSettingsService)

const onDone = vi.fn()
const onCancel = vi.fn()

/** The editor outside its dialog, over the real provider state. */
function Harness({ editing }: Readonly<{ editing: string | null }>) {
  const state = useAuthProviders(0)
  if (!state.providers) return <p>loading</p>
  const stored = state.providers.find(p => p.slug === editing) ?? null
  return (
    <>
      <ProviderEditor
        stored={stored}
        types={availableProviderTypes(state.providers)}
        state={state}
        onDone={onDone}
        onCancel={onCancel}
      />
      {state.conflict && <p>conflict shown by the section</p>}
      {state.pendingLockout && <p>lockout held by the section</p>}
    </>
  )
}

async function renderEditor(
  providers: AdminAuthProvider[],
  editing: string | null = null
) {
  service.listProviders.mockResolvedValue({ providers, version: 7 })
  render(<Harness editing={editing} />)
  await screen.findByRole('form')
  return userEvent.setup()
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('ProviderEditor: adding a provider', () => {
  it('offers only the types still available', async () => {
    await renderEditor([google])
    expect(screen.queryByRole('radio', { name: 'Google' })).toBeNull()
    expect(screen.getByRole('radio', { name: 'GitHub' })).toBeChecked()
    expect(screen.getByRole('radio', { name: 'OpenID Connect' })).toBeVisible()
  })

  it('shows the issuer URL for OIDC only', async () => {
    const user = await renderEditor([])
    // Google is the first available type.
    expect(screen.getByRole('radio', { name: 'Google' })).toBeChecked()
    expect(screen.queryByLabelText('Issuer URL')).toBeNull()
    expect(screen.getByLabelText('Slug')).toHaveValue('google')

    await user.click(screen.getByRole('radio', { name: 'OpenID Connect' }))
    expect(screen.getByLabelText('Issuer URL')).toBeVisible()
    expect(screen.getByLabelText('Slug')).toHaveValue('')
  })

  it('shows the redirect URI read-only, with a copy button', async () => {
    await renderEditor([google])
    const uri = screen.getByLabelText('Redirect URI')
    expect(uri).toHaveValue(REDIRECT_URI)
    expect(uri).toHaveAttribute('readonly')
    expect(
      screen.getByRole('button', { name: 'Copy redirect URI' })
    ).toBeVisible()
    expect(screen.queryByText(/derived from the address/)).toBeNull()
  })

  it('says the redirect URI is derived while no provider is stored', async () => {
    await renderEditor([])
    expect(screen.getByLabelText('Redirect URI')).toHaveValue(
      `${globalThis.location.origin}/api/v1/auth/callback`
    )
    expect(screen.getByText(/derived from the address/)).toBeVisible()
  })

  it('blocks a save with missing fields and sends nothing', async () => {
    const user = await renderEditor([])
    await user.click(screen.getByRole('radio', { name: 'OpenID Connect' }))
    await user.click(screen.getByRole('button', { name: 'Add provider' }))

    expect(screen.getByText('Enter the client secret.')).toBeVisible()
    expect(screen.getByText('Enter the issuer URL.')).toBeVisible()
    expect(screen.getByLabelText('Client ID')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
    expect(service.createProvider).not.toHaveBeenCalled()
  })

  it('creates the provider last in order, against the loaded version', async () => {
    service.createProvider.mockResolvedValue({
      provider: provider(),
      version: 8,
    })
    const user = await renderEditor([google])
    await user.click(screen.getByRole('radio', { name: 'OpenID Connect' }))
    await user.type(screen.getByLabelText('Slug'), 'okta')
    await user.type(screen.getByLabelText('Display name'), 'Okta')
    await user.type(
      screen.getByLabelText('Issuer URL'),
      'https://example.okta.com'
    )
    await user.type(screen.getByLabelText('Client ID'), 'abc')
    await user.type(screen.getByLabelText('Client secret'), 's3cret')
    await user.click(screen.getByRole('button', { name: 'Add provider' }))

    await waitFor(() => {
      expect(onDone).toHaveBeenCalledTimes(1)
    })
    expect(service.createProvider).toHaveBeenCalledWith({
      type: 'oidc',
      slug: 'okta',
      display_name: 'Okta',
      enabled: true,
      sort_order: 2,
      client_id: 'abc',
      client_secret: 's3cret',
      issuer_url: 'https://example.okta.com',
      expected_version: 7,
    })
  })

  it('tests an unsaved provider as a full candidate and stores nothing', async () => {
    service.testProvider.mockResolvedValue({
      is_valid: false,
      message: 'discovery failed',
    })
    const user = await renderEditor([google])
    await user.click(screen.getByRole('radio', { name: 'GitHub' }))
    await user.type(screen.getByLabelText('Client ID'), 'abc')
    await user.type(screen.getByLabelText('Client secret'), 's3cret')
    await user.click(screen.getByRole('button', { name: 'Test' }))

    const result = await screen.findByTestId('provider-test-result')
    expect(result).toHaveTextContent('The test failed')
    expect(result).toHaveTextContent('discovery failed')
    expect(service.testProvider).toHaveBeenCalledWith({
      type: 'github',
      client_id: 'abc',
      client_secret: 's3cret',
    })
    expect(service.createProvider).not.toHaveBeenCalled()
  })

  it('puts a server validation error on its field', async () => {
    service.createProvider.mockRejectedValue(
      apiError(400, 'VALIDATION_FAILED', {
        validation_errors: [
          { field: 'client_id', message: 'client_id is not known' },
        ],
      })
    )
    const user = await renderEditor([google])
    await user.type(screen.getByLabelText('Client ID'), 'abc')
    await user.type(screen.getByLabelText('Client secret'), 's3cret')
    await user.click(screen.getByRole('button', { name: 'Add provider' }))

    expect(await screen.findByText('client_id is not known')).toBeVisible()
    expect(onDone).not.toHaveBeenCalled()

    // Editing the field clears the server's message.
    await user.type(screen.getByLabelText('Client ID'), 'd')
    expect(screen.queryByText('client_id is not known')).toBeNull()
  })

  it('shows a slug collision as the form error', async () => {
    service.createProvider.mockRejectedValue(
      apiError(409, 'INSTANCE_AUTH_PROVIDER_CONFLICT', {
        detail: 'A provider with this slug already exists',
      })
    )
    const user = await renderEditor([google])
    await user.type(screen.getByLabelText('Client ID'), 'abc')
    await user.type(screen.getByLabelText('Client secret'), 's3cret')
    await user.click(screen.getByRole('button', { name: 'Add provider' }))

    expect(await screen.findByTestId('provider-form-error')).toHaveTextContent(
      'A provider with this slug already exists'
    )
  })
})

describe('ProviderEditor: editing a provider', () => {
  it('fixes the type and slug and starts with an empty secret', async () => {
    await renderEditor([provider()], 'okta')
    expect(screen.queryByRole('radio')).toBeNull()
    expect(screen.queryByLabelText('Slug')).toBeNull()
    expect(screen.getByText(/type and slug\s+cannot be changed/)).toBeVisible()
    expect(screen.getByLabelText('Client secret')).toHaveValue('')
    expect(screen.getByText('Leave this blank to keep it.')).toBeVisible()
  })

  it('omits the blank secret from the save, so the stored one is kept', async () => {
    service.updateProvider.mockResolvedValue({
      provider: provider(),
      version: 8,
    })
    const stored = provider()
    const user = await renderEditor([stored], 'okta')
    const name = screen.getByLabelText('Display name')
    await user.clear(name)
    await user.type(name, 'Okta SSO')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => {
      expect(onDone).toHaveBeenCalledTimes(1)
    })
    expect(service.updateProvider).toHaveBeenCalledWith(stored.id, {
      display_name: 'Okta SSO',
      enabled: true,
      sort_order: 0,
      client_id: 'client-okta',
      issuer_url: 'https://example.okta.com',
      expected_version: 7,
      confirm_lockout_risk: false,
    })
  })

  it('requires the secret again once the issuer URL changes', async () => {
    const user = await renderEditor([provider()], 'okta')
    const issuer = screen.getByLabelText('Issuer URL')
    await user.clear(issuer)
    await user.type(issuer, 'https://other.okta.com')
    expect(screen.getByText(/the stored secret is not kept/)).toBeVisible()

    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    expect(
      screen.getByText(/not kept when the issuer URL changes/)
    ).toBeVisible()
    expect(service.updateProvider).not.toHaveBeenCalled()

    service.updateProvider.mockResolvedValue({
      provider: provider(),
      version: 8,
    })
    await user.type(screen.getByLabelText('Client secret'), 'again')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() => {
      expect(service.updateProvider).toHaveBeenCalledTimes(1)
    })
    expect(service.updateProvider.mock.calls[0][1]).toMatchObject({
      issuer_url: 'https://other.okta.com',
      client_secret: 'again',
    })
  })

  it('tests by id plus only the fields the form changed', async () => {
    service.testProvider.mockResolvedValue({ is_valid: true, message: null })
    const stored = provider()
    const user = await renderEditor([stored], 'okta')
    await user.click(screen.getByRole('button', { name: 'Test' }))
    expect(await screen.findByTestId('provider-test-result')).toHaveTextContent(
      'The test passed'
    )
    expect(service.testProvider).toHaveBeenLastCalledWith({ id: stored.id })

    await user.type(screen.getByLabelText('Client ID'), '-2')
    // An edit clears the previous result: it no longer describes the form.
    expect(screen.queryByTestId('provider-test-result')).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Test' }))
    await screen.findByTestId('provider-test-result')
    expect(service.testProvider).toHaveBeenLastCalledWith({
      id: stored.id,
      client_id: 'client-okta-2',
    })
  })

  it('reports a test that could not run', async () => {
    service.testProvider.mockRejectedValue(new Error('Network error'))
    const user = await renderEditor([provider()], 'okta')
    await user.click(screen.getByRole('button', { name: 'Test' }))
    expect(await screen.findByTestId('provider-test-error')).toHaveTextContent(
      'Network error'
    )
  })

  it('hands a version conflict to the section and never retries', async () => {
    service.updateProvider.mockRejectedValue(
      apiError(409, 'INSTANCE_SETTINGS_VERSION_CONFLICT')
    )
    const user = await renderEditor([provider()], 'okta')
    await user.type(screen.getByLabelText('Display name'), '!')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    expect(
      await screen.findByText('conflict shown by the section')
    ).toBeVisible()
    expect(onCancel).toHaveBeenCalledTimes(1)
    expect(onDone).not.toHaveBeenCalled()
    expect(service.updateProvider).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  })

  it('hands a lockout risk to the section for confirmation', async () => {
    service.updateProvider.mockRejectedValue(
      apiError(409, 'lockout_risk', {
        metadata: { reason: 'own_provider' },
      })
    )
    const user = await renderEditor([provider()], 'okta')
    await user.click(screen.getByRole('switch', { name: 'Enabled' }))
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    expect(await screen.findByText('lockout held by the section')).toBeVisible()
    expect(onCancel).toHaveBeenCalledTimes(1)
    expect(service.updateProvider).toHaveBeenCalledTimes(1)
    expect(service.updateProvider.mock.calls[0][1]).toMatchObject({
      enabled: false,
      confirm_lockout_risk: false,
    })
  })
})
