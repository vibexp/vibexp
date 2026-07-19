import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import type { APIKey } from '@/services/apiKeyService'

jest.mock('@/services/apiKeyService', () => ({
  apiKeyService: {
    getAPIKeys: jest.fn(),
    createAPIKey: jest.fn(),
    deleteAPIKey: jest.fn(),
  },
}))

jest.mock('@/hooks', () => {
  const trackEvent = jest.fn()
  const showSuccess = jest.fn()
  const showError = jest.fn()
  return {
    useAnalytics: () => ({ trackEvent }),
    useAlerts: () => ({ showSuccess, showError }),
  }
})

// handleError's return value feeds Object.entries() in the create flow, so the
// mock must return an object (field → message map) by default.
jest.mock('@/hooks/useErrorHandler', () => {
  const handleError = jest.fn(() => ({}))
  return {
    useErrorHandler: () => ({ handleError }),
  }
})

jest.mock('@/lib/toast', () => ({
  toast: {
    success: jest.fn(),
    error: jest.fn(),
    info: jest.fn(),
    warning: jest.fn(),
    message: jest.fn(),
  },
}))

import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toast } from '@/lib/toast'
import { apiKeyService } from '@/services/apiKeyService'

import { APIKeys } from '../APIKeys'

const { handleError } = useErrorHandler()

function buildKey(overrides: Partial<APIKey> = {}): APIKey {
  return {
    id: 'key-1',
    user_id: 'user-1',
    name: 'Development Setup',
    key_prefix: 'vxk_abc123',
    integrations: ['ai_tools', 'cli'],
    is_legacy: false,
    created_at: '2026-01-15T10:30:00Z',
    updated_at: '2026-01-15T10:30:00Z',
    last_used_at: '2026-02-01T08:00:00Z',
    ...overrides,
  }
}

function renderAPIKeys() {
  return render(
    <MemoryRouter>
      <APIKeys />
    </MemoryRouter>
  )
}

beforeAll(() => {
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText: jest.fn().mockResolvedValue(undefined) },
    configurable: true,
  })
})

beforeEach(() => {
  jest.clearAllMocks()
  ;(apiKeyService.getAPIKeys as jest.Mock).mockResolvedValue([])
})

describe('APIKeys page — list rendering', () => {
  it('shows the loading skeleton while keys are loading, then the list', async () => {
    let resolveKeys: (keys: APIKey[]) => void = () => {}
    ;(apiKeyService.getAPIKeys as jest.Mock).mockReturnValue(
      new Promise<APIKey[]>(resolve => {
        resolveKeys = resolve
      })
    )

    renderAPIKeys()

    // While loading, neither the empty state nor any rows exist.
    expect(screen.queryByText('No API keys yet')).not.toBeInTheDocument()
    expect(screen.queryAllByTestId('api-key-item')).toHaveLength(0)

    resolveKeys([buildKey()])

    expect(await screen.findByText('Development Setup')).toBeInTheDocument()
  })

  it('renders rows with masked key, integration badges, and dates', async () => {
    ;(apiKeyService.getAPIKeys as jest.Mock).mockResolvedValue([
      buildKey(),
      buildKey({
        id: 'key-2',
        name: 'Old Key',
        key_prefix: 'vxk_old999',
        integrations: [],
        is_legacy: true,
        last_used_at: undefined,
      }),
    ])

    renderAPIKeys()

    const rows = await screen.findAllByTestId('api-key-item')
    expect(rows).toHaveLength(2)

    const first = rows[0]
    expect(within(first).getByText('Development Setup')).toBeInTheDocument()
    expect(within(first).getByTestId('masked-api-key')).toHaveTextContent(
      'vxk_abc123***'
    )
    expect(within(first).getByText('AI Tools')).toBeInTheDocument()
    expect(within(first).getByText('CLI')).toBeInTheDocument()

    // Legacy key with no integrations
    const second = rows[1]
    expect(
      within(second).getByText('Legacy key (no integrations)')
    ).toBeInTheDocument()
    expect(within(second).getByText('Never')).toBeInTheDocument()
  })

  it('shows the empty state with a create CTA when there are no keys', async () => {
    renderAPIKeys()

    expect(await screen.findByText('No API keys yet')).toBeInTheDocument()

    const user = userEvent.setup()
    await user.click(
      screen.getByRole('button', { name: 'Create your first API key' })
    )

    const dialog = await screen.findByRole('dialog')
    expect(
      within(dialog).getByText('Create API key', { selector: 'h2' })
    ).toBeInTheDocument()
    expect(screen.getByTestId('create-api-key-form')).toBeInTheDocument()
  })

  it('reports a load failure and falls back to the empty list', async () => {
    ;(apiKeyService.getAPIKeys as jest.Mock).mockRejectedValue(
      new Error('boom')
    )

    renderAPIKeys()

    await waitFor(() => {
      expect(handleError).toHaveBeenCalledWith(
        expect.any(Error),
        'Failed to load API keys'
      )
    })
    expect(await screen.findByText('No API keys yet')).toBeInTheDocument()
  })
})

describe('APIKeys page — create flow and show-once secret', () => {
  async function openCreateDialogAndFill(
    user: ReturnType<typeof userEvent.setup>
  ) {
    await user.click(screen.getByTestId('create-api-key-button'))
    await screen.findByTestId('create-api-key-form')
    await user.type(
      screen.getByTestId('api-key-name-input'),
      'Development Setup'
    )
    await user.click(screen.getByTestId('integration-checkbox-ai_tools'))
  }

  it('disables submit until a name and at least one integration are set', async () => {
    renderAPIKeys()
    await screen.findByText('No API keys yet')

    const user = userEvent.setup()
    await user.click(screen.getByTestId('create-api-key-button'))
    await screen.findByTestId('create-api-key-form')

    const submit = screen.getByTestId('submit-create-api-key-button')
    expect(submit).toBeDisabled()

    await user.type(screen.getByTestId('api-key-name-input'), 'My key')
    expect(submit).toBeDisabled()

    await user.click(screen.getByTestId('integration-checkbox-cli'))
    expect(submit).toBeEnabled()

    // Unchecking drops back below the minimum
    await user.click(screen.getByTestId('integration-checkbox-cli'))
    expect(submit).toBeDisabled()
  })

  it('creates the key with the typed name and selected integrations, then shows the secret exactly once', async () => {
    const created = buildKey({ id: 'key-new', key_prefix: 'vxk_new123' })
    ;(apiKeyService.createAPIKey as jest.Mock).mockResolvedValue({
      api_key: created,
      full_key: 'vxk_new123_full_secret_value',
      key_prefix: 'vxk_new123',
    })
    ;(apiKeyService.getAPIKeys as jest.Mock)
      .mockResolvedValueOnce([])
      .mockResolvedValue([created])

    renderAPIKeys()
    await screen.findByText('No API keys yet')

    const user = userEvent.setup()
    await openCreateDialogAndFill(user)
    await user.click(screen.getByTestId('integration-checkbox-mcp_server'))
    await user.click(screen.getByTestId('submit-create-api-key-button'))

    await waitFor(() => {
      expect(apiKeyService.createAPIKey).toHaveBeenCalledWith({
        name: 'Development Setup',
        integration_codes: ['ai_tools', 'mcp_server'],
      })
    })

    // Show-once secret: the full key comes only from the create response and
    // is rendered in a dedicated alert with a "copy it now" warning.
    const card = await screen.findByTestId('created-api-key-card')
    expect(within(card).getByTestId('api-key-display')).toHaveTextContent(
      'vxk_new123_full_secret_value'
    )
    expect(
      within(card).getByText(/you won.t be able to see it again/i)
    ).toBeInTheDocument()

    // The dialog closed and the list was reloaded.
    expect(screen.queryByTestId('create-api-key-form')).not.toBeInTheDocument()
    expect(apiKeyService.getAPIKeys).toHaveBeenCalledTimes(2)

    // The reloaded list only ever shows the masked prefix — the full secret is
    // not retrievable from the list payload.
    const row = await screen.findByTestId('api-key-item')
    expect(within(row).getByTestId('masked-api-key')).toHaveTextContent(
      'vxk_new123***'
    )
    expect(
      within(row).queryByText('vxk_new123_full_secret_value')
    ).not.toBeInTheDocument()
  })

  it('copies the freshly created secret to the clipboard', async () => {
    ;(apiKeyService.createAPIKey as jest.Mock).mockResolvedValue({
      api_key: buildKey({ id: 'key-new' }),
      full_key: 'vxk_copy_me_secret',
      key_prefix: 'vxk_copy',
    })

    renderAPIKeys()
    await screen.findByText('No API keys yet')

    const user = userEvent.setup()
    await openCreateDialogAndFill(user)
    await user.click(screen.getByTestId('submit-create-api-key-button'))

    await screen.findByTestId('created-api-key-card')
    // userEvent.setup() installs its own clipboard stub — spy on that.
    const writeText = jest.spyOn(navigator.clipboard, 'writeText')
    await user.click(screen.getByTestId('copy-api-key-button'))

    expect(writeText).toHaveBeenCalledWith('vxk_copy_me_secret')
  })

  it('dismissing the secret card removes the full key from the page for good', async () => {
    ;(apiKeyService.createAPIKey as jest.Mock).mockResolvedValue({
      api_key: buildKey({ id: 'key-new' }),
      full_key: 'vxk_gone_after_close',
      key_prefix: 'vxk_gone',
    })

    renderAPIKeys()
    await screen.findByText('No API keys yet')

    const user = userEvent.setup()
    await openCreateDialogAndFill(user)
    await user.click(screen.getByTestId('submit-create-api-key-button'))

    await screen.findByTestId('created-api-key-card')
    await user.click(screen.getByTestId('close-api-key-modal-button'))

    expect(screen.queryByTestId('created-api-key-card')).not.toBeInTheDocument()
    expect(screen.queryByText('vxk_gone_after_close')).not.toBeInTheDocument()
  })

  it('maps a create failure back onto the form fields and keeps the dialog open', async () => {
    ;(apiKeyService.createAPIKey as jest.Mock).mockRejectedValue(
      new Error('conflict')
    )
    ;(handleError as jest.Mock).mockReturnValueOnce({
      name: 'An API key with this name already exists',
    })

    renderAPIKeys()
    await screen.findByText('No API keys yet')

    const user = userEvent.setup()
    await openCreateDialogAndFill(user)
    await user.click(screen.getByTestId('submit-create-api-key-button'))

    await waitFor(() => {
      expect(handleError).toHaveBeenCalledWith(
        expect.any(Error),
        'Failed to create API key'
      )
    })

    expect(
      await screen.findByText('An API key with this name already exists')
    ).toBeInTheDocument()
    expect(screen.getByTestId('create-api-key-form')).toBeInTheDocument()
    expect(screen.queryByTestId('created-api-key-card')).not.toBeInTheDocument()
  })
})

describe('APIKeys page — delete flow', () => {
  it('confirms and deletes via the service, then re-fetches and toasts', async () => {
    ;(apiKeyService.getAPIKeys as jest.Mock).mockResolvedValue([buildKey()])
    ;(apiKeyService.deleteAPIKey as jest.Mock).mockResolvedValue(undefined)

    renderAPIKeys()

    const user = userEvent.setup()
    await user.click(await screen.findByLabelText('Delete Development Setup'))

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Delete API key?')).toBeInTheDocument()

    const fetchesBefore = (apiKeyService.getAPIKeys as jest.Mock).mock.calls
      .length
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

    await waitFor(() => {
      expect(apiKeyService.deleteAPIKey).toHaveBeenCalledWith('key-1')
    })
    await waitFor(() => {
      expect(
        (apiKeyService.getAPIKeys as jest.Mock).mock.calls.length
      ).toBeGreaterThan(fetchesBefore)
    })
    expect(toast.success).toHaveBeenCalledWith('API key deleted')
  })

  it('cancelling the confirmation closes it without deleting', async () => {
    ;(apiKeyService.getAPIKeys as jest.Mock).mockResolvedValue([buildKey()])

    renderAPIKeys()

    const user = userEvent.setup()
    await user.click(await screen.findByLabelText('Delete Development Setup'))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))

    await waitFor(() => {
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    })
    expect(apiKeyService.deleteAPIKey).not.toHaveBeenCalled()
  })

  it('reports a delete failure and closes the confirmation', async () => {
    ;(apiKeyService.getAPIKeys as jest.Mock).mockResolvedValue([buildKey()])
    ;(apiKeyService.deleteAPIKey as jest.Mock).mockRejectedValue(
      new Error('forbidden')
    )

    renderAPIKeys()

    const user = userEvent.setup()
    await user.click(await screen.findByLabelText('Delete Development Setup'))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

    await waitFor(() => {
      expect(handleError).toHaveBeenCalledWith(
        expect.any(Error),
        'Failed to delete API key'
      )
    })
    await waitFor(() => {
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    })
    expect(toast.success).not.toHaveBeenCalled()
  })
})
