import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { MockedFunction } from 'vitest'

import { toast } from '@/lib/toast'
import { modelProviderService } from '@/services/modelProviderService'

import { ModelProviderDialog } from './ModelProviderDialog'

vi.mock('@/services/modelProviderService', async () => ({
  // Keep any other exports real; only the service singleton is mocked so
  // validate-on-save can be asserted.
  ...(await vi.importActual('@/services/modelProviderService')),
  modelProviderService: {
    validateModelProvider: vi.fn(),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

// Radix Select relies on browser APIs jsdom doesn't implement.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

const mockedValidate =
  modelProviderService.validateModelProvider as MockedFunction<
    typeof modelProviderService.validateModelProvider
  >
const mockedToastError = toast.error as MockedFunction<typeof toast.error>

const fillValidForm = async (user: ReturnType<typeof userEvent.setup>) => {
  await user.type(
    screen.getByPlaceholderText('e.g., OpenAI GPT-4o'),
    'My Provider'
  )
  await user.type(
    screen.getByPlaceholderText('e.g., gpt-4o-mini'),
    'gpt-4o-mini'
  )
  await user.type(
    screen.getByPlaceholderText('https://api.openai.com/v1'),
    'https://api.openai.com/v1'
  )
  await user.type(screen.getByPlaceholderText('Enter API key'), 'sk-test')
}

const renderDialog = (onSubmit = vi.fn().mockResolvedValue(undefined)) => {
  render(
    <ModelProviderDialog
      teamId="team-1"
      open
      onOpenChange={vi.fn()}
      submitting={false}
      onSubmit={onSubmit}
    />
  )
  return onSubmit
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('ModelProviderDialog', () => {
  it('shows the model field and a masked (password) API key input', () => {
    renderDialog()
    expect(screen.getByPlaceholderText('e.g., gpt-4o-mini')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('Enter API key')).toHaveAttribute(
      'type',
      'password'
    )
  })

  it('validates on save and submits when the provider is valid', async () => {
    const user = userEvent.setup()
    mockedValidate.mockResolvedValue({ is_valid: true, message: 'ok' })
    const onSubmit = renderDialog()

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'Add provider' }))

    await waitFor(() => {
      expect(mockedValidate).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ model: 'gpt-4o-mini' })
      )
    })
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ model: 'gpt-4o-mini' })
    )
  })

  it('blocks submit and shows an error when validation fails', async () => {
    const user = userEvent.setup()
    mockedValidate.mockResolvedValue({
      is_valid: false,
      message: 'Could not reach the provider',
    })
    const onSubmit = renderDialog()

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'Add provider' }))

    await waitFor(() => {
      expect(mockedToastError).toHaveBeenCalledWith(
        'Could not reach the provider',
        expect.anything()
      )
    })
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('prefills the Base URL when a preset is selected', async () => {
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('button', { name: 'Groq' }))

    expect(
      screen.getByPlaceholderText('https://api.openai.com/v1')
    ).toHaveValue('https://api.groq.com/openai/v1')
  })

  const existingProvider = {
    id: 'p1',
    user_id: 'u1',
    name: 'Existing',
    provider_type: 'openai_compatible',
    model: 'gpt-4o-mini',
    is_default: false,
    base_url: 'https://api.openai.com/v1',
    configuration: '{}',
    has_api_key: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    version: 1,
  }

  it('skips validation on a name-only edit (identity unchanged)', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn().mockResolvedValue(undefined)
    render(
      <ModelProviderDialog
        teamId="team-1"
        open
        onOpenChange={vi.fn()}
        submitting={false}
        provider={existingProvider}
        onSubmit={onSubmit}
      />
    )

    const name = screen.getByPlaceholderText('e.g., OpenAI GPT-4o')
    await user.clear(name)
    await user.type(name, 'Renamed')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalled()
    })
    expect(mockedValidate).not.toHaveBeenCalled()
  })

  // -------------------------------------------------------------------------
  // #834 — copy mode
  // -------------------------------------------------------------------------

  const copySource = {
    provider: { ...existingProvider, name: 'Shared OpenAI' },
    sourceTeamName: 'Platform Team',
  }

  const renderCopyDialog = (
    overrides: Partial<typeof copySource> = {},
    onSubmit = vi.fn().mockResolvedValue(undefined)
  ) => {
    render(
      <ModelProviderDialog
        teamId="team-1"
        open
        onOpenChange={vi.fn()}
        submitting={false}
        copySource={{ ...copySource, ...overrides }}
        onSubmit={onSubmit}
      />
    )
    return onSubmit
  }

  it('prefills name, type, model and base URL from the source provider', () => {
    renderCopyDialog()

    expect(screen.getByPlaceholderText('e.g., OpenAI GPT-4o')).toHaveValue(
      'Shared OpenAI'
    )
    expect(screen.getByPlaceholderText('e.g., gpt-4o-mini')).toHaveValue(
      'gpt-4o-mini'
    )
    expect(
      screen.getByPlaceholderText('https://api.openai.com/v1')
    ).toHaveValue('https://api.openai.com/v1')
    expect(screen.getByRole('combobox')).toHaveTextContent('OpenAI-compatible')
  })

  it('renders the API key as non-editable, naming the source team', () => {
    renderCopyDialog()

    const field = screen.getByTestId('copy-api-key-field')
    expect(field).toBeDisabled()
    expect(field).toHaveAttribute('readonly')
    expect(field).toHaveValue('Will be copied from Platform Team')
    // The create path's key input must be gone, not merely hidden.
    expect(screen.queryByPlaceholderText('Enter API key')).toBeNull()
  })

  it('states before confirming that the source key becomes usable here', () => {
    renderCopyDialog()

    expect(screen.getByTestId('copy-credential-warning')).toHaveTextContent(
      /Platform Team's API key will be copied across and every member of this team will be able to use it/
    )
  })

  it('does NOT run the validation probe on the copy path', async () => {
    const user = userEvent.setup()
    const onSubmit = renderCopyDialog()

    await user.click(screen.getByRole('button', { name: 'Copy provider' }))

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          name: 'Shared OpenAI',
          provider_type: 'openai_compatible',
          model: 'gpt-4o-mini',
          base_url: 'https://api.openai.com/v1',
        })
      )
    })
    // The whole point of the exception: the SPA holds no key, so the probe
    // could only fail with an auth error and block a valid copy.
    expect(mockedValidate).not.toHaveBeenCalled()
  })

  it('still probes on the create path (the copy exception is not over-broad)', async () => {
    const user = userEvent.setup()
    mockedValidate.mockResolvedValue({ is_valid: true, message: 'ok' })
    const onSubmit = renderDialog()

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'Add provider' }))

    await waitFor(() => {
      expect(mockedValidate).toHaveBeenCalledTimes(1)
    })
    expect(onSubmit).toHaveBeenCalled()
  })

  it('hides the default checkbox — a copy always lands non-default', () => {
    renderCopyDialog()
    expect(screen.queryByText('Use as default')).toBeNull()
  })

  it('says so when the source provider has no key stored', () => {
    renderCopyDialog({
      provider: { ...existingProvider, has_api_key: false },
    })

    expect(
      screen.getByText(
        'That provider has no key stored, so the copy will not have one either.'
      )
    ).toBeInTheDocument()
  })

  it('validates when the model changes on edit', async () => {
    const user = userEvent.setup()
    mockedValidate.mockResolvedValue({ is_valid: true, message: 'ok' })
    const onSubmit = vi.fn().mockResolvedValue(undefined)
    render(
      <ModelProviderDialog
        teamId="team-1"
        open
        onOpenChange={vi.fn()}
        submitting={false}
        provider={existingProvider}
        onSubmit={onSubmit}
      />
    )

    const model = screen.getByPlaceholderText('e.g., gpt-4o-mini')
    await user.clear(model)
    await user.type(model, 'gpt-4o')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => {
      expect(mockedValidate).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ model: 'gpt-4o' })
      )
    })
    expect(onSubmit).toHaveBeenCalled()
  })
})
