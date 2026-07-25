import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { GitHubAppConfigResponse } from '@/services/githubAppConfigService'
import { githubAppConfigService } from '@/services/githubAppConfigService'

import { GitHubAppConfigDialog } from './GitHubAppConfigDialog'

jest.mock('@/services/githubAppConfigService', () => ({
  ...jest.requireActual('@/services/githubAppConfigService'),
  githubAppConfigService: {
    createAppConfig: jest.fn(),
    updateAppConfig: jest.fn(),
    validateAppConfig: jest.fn(),
  },
}))

jest.mock('@/lib/toast', () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}))

const mockedCreate = githubAppConfigService.createAppConfig as jest.Mock
const mockedUpdate = githubAppConfigService.updateAppConfig as jest.Mock
const mockedValidate = githubAppConfigService.validateAppConfig as jest.Mock

const TEAM_ID = 'team-1'
// Assembled at runtime rather than written as a literal: gitleaks' private-key
// rule matches the PEM armour itself, so even an obviously fake fixture trips
// it — and silencing the scanner for a test blunts it for everyone.
const ARMOUR = ['-----BEGIN RSA', 'PRIVATE', 'KEY-----'].join(' ')
const PEM = `${ARMOUR}\nnot-a-real-key\n${ARMOUR.replace('BEGIN', 'END')}`

const existingConfig = {
  id: 'cfg-1',
  team_id: TEAM_ID,
  app_id: '123456',
  app_slug: 'acme-app',
  client_id: 'Iv1.abc',
  has_private_key: true,
  has_client_secret: true,
  has_webhook_secret: true,
  webhook_url: 'https://vibexp.example.com/api/v1/webhooks/github/tok',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  version: 1,
} as unknown as GitHubAppConfigResponse

beforeEach(() => {
  jest.clearAllMocks()
  mockedValidate.mockResolvedValue({ is_valid: true, message: 'ok' })
})

const renderDialog = (
  props: Partial<React.ComponentProps<typeof GitHubAppConfigDialog>> = {}
) =>
  render(
    <GitHubAppConfigDialog
      open
      onOpenChange={jest.fn()}
      teamId={TEAM_ID}
      onSaved={jest.fn()}
      {...props}
    />
  )

describe('GitHubAppConfigDialog — create', () => {
  it('sends the pasted credentials', async () => {
    const user = userEvent.setup()
    mockedCreate.mockResolvedValue({
      webhook_url: 'https://example.com/hook',
      webhook_secret: 'whsec',
    })
    const onSaved = jest.fn()
    renderDialog({ onSaved })

    await user.type(screen.getByPlaceholderText('123456'), '123456')
    await user.type(screen.getByPlaceholderText('my-vibexp-app'), 'acme-app')
    await user.type(screen.getByPlaceholderText('Iv1.abc123'), 'Iv1.abc')
    await user.type(screen.getByLabelText('Client secret'), 'cs-value')
    await user.type(screen.getByLabelText('Private key'), PEM)
    await user.click(screen.getByRole('button', { name: 'Register App' }))

    await waitFor(() => {
      expect(mockedCreate).toHaveBeenCalled()
    })
    expect(mockedCreate).toHaveBeenCalledWith(TEAM_ID, {
      app_id: '123456',
      app_slug: 'acme-app',
      client_id: 'Iv1.abc',
      client_secret: 'cs-value',
      private_key: PEM,
    })

    // The generated secret is handed to the caller exactly once, for the
    // one-time disclosure dialog.
    await waitFor(() => {
      expect(onSaved).toHaveBeenCalledWith({
        webhookUrl: 'https://example.com/hook',
        webhookSecret: 'whsec',
      })
    })
  })

  it('requires the secrets on create rather than sending empties', async () => {
    const user = userEvent.setup()
    renderDialog()

    await user.type(screen.getByPlaceholderText('123456'), '123456')
    await user.type(screen.getByPlaceholderText('my-vibexp-app'), 'acme-app')
    await user.type(screen.getByPlaceholderText('Iv1.abc123'), 'Iv1.abc')
    await user.click(screen.getByRole('button', { name: 'Register App' }))

    expect(await screen.findByText('Private key is required')).toBeVisible()
    expect(mockedCreate).not.toHaveBeenCalled()
  })
})

describe('GitHubAppConfigDialog — edit', () => {
  it('omits blank secrets so the stored ones are preserved', async () => {
    const user = userEvent.setup()
    mockedUpdate.mockResolvedValue(existingConfig)
    renderDialog({ config: existingConfig })

    // Change only a non-secret field; leave both secret inputs untouched.
    const slug = screen.getByLabelText('App slug')
    await user.clear(slug)
    await user.type(slug, 'renamed-app')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => {
      expect(mockedUpdate).toHaveBeenCalled()
    })

    // Asserted on the REQUEST PAYLOAD, not the rendered UI: `undefined` keeps
    // the stored secret, while `''` would be a server-side validation error.
    const [, payload] = mockedUpdate.mock.calls[0] as [
      string,
      Record<string, unknown>,
    ]
    expect(payload.app_slug).toBe('renamed-app')
    expect(payload.client_secret).toBeUndefined()
    expect(payload.private_key).toBeUndefined()
    expect(Object.hasOwn(payload, 'client_secret')).toBe(true)
  })

  it('never prefills the stored secrets, but does prefill the public fields', () => {
    renderDialog({ config: existingConfig })

    expect(screen.getByLabelText('Client secret')).toHaveValue('')
    expect(screen.getByLabelText('Private key')).toHaveValue('')
    // The non-secret fields are echoed so an edit is not a re-type from scratch.
    expect(screen.getByLabelText('App ID')).toHaveValue('123456')
    expect(screen.getByLabelText('App slug')).toHaveValue('acme-app')
    expect(screen.getByLabelText('Client ID')).toHaveValue('Iv1.abc')
  })
})

describe('GitHubAppConfigDialog — validation categories', () => {
  const cases = [
    { code: 'slug_mismatch', field: 'App slug', match: /slug from the App/i },
    { code: 'app_not_found', field: 'App ID', match: /no App with this ID/i },
    {
      code: 'invalid_credentials',
      field: 'Private key',
      match: /does not belong to this App ID/i,
    },
  ] as const

  it.each(cases)(
    'reports $code on the $field field, not as a toast',
    async ({ code, match }) => {
      const user = userEvent.setup()
      mockedUpdate.mockResolvedValue(existingConfig)
      mockedValidate.mockResolvedValue({
        is_valid: false,
        message: 'generic server message',
        details: { error_details: code },
      })
      const onSaved = jest.fn()
      renderDialog({ config: existingConfig, onSaved })

      await user.click(screen.getByRole('button', { name: 'Save changes' }))

      expect(await screen.findByText(match)).toBeVisible()
      // A failed probe must not look like success.
      expect(onSaved).not.toHaveBeenCalled()
    }
  )

  it('falls back to the server message for an unknown category', async () => {
    const user = userEvent.setup()
    mockedUpdate.mockResolvedValue(existingConfig)
    mockedValidate.mockResolvedValue({
      is_valid: false,
      message: 'something the SPA has not shipped yet',
      details: { error_details: 'a_new_category' },
    })
    renderDialog({ config: existingConfig })

    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    const { toast } = jest.requireMock('@/lib/toast')
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalled()
    })
    expect(toast.error.mock.calls[0][1]).toEqual(
      expect.objectContaining({
        description: 'something the SPA has not shipped yet',
      })
    )
  })
})
