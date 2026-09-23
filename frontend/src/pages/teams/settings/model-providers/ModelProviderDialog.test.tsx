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
    listProviderModels: vi.fn(),
  },
}))

// The real combobox (Popover + cmdk) is covered by ModelCombobox.test.tsx,
// outside a Dialog: Radix popper primitives inside a Dialog are the jsdom
// heap trap. Here a plain stub stands in so the dialog's wiring is testable.
vi.mock('./ModelCombobox', () => ({
  ModelCombobox: ({
    value,
    onChange,
    models,
  }: {
    value: string
    onChange: (model: string) => void
    models: { id: string }[]
  }) => (
    <div data-testid="model-combobox" data-value={value}>
      {models.map(model => (
        <button
          key={model.id}
          type="button"
          onClick={() => {
            onChange(model.id)
          }}
        >
          pick {model.id}
        </button>
      ))}
    </div>
  ),
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
const mockedListModels =
  modelProviderService.listProviderModels as MockedFunction<
    typeof modelProviderService.listProviderModels
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
    sourceTeamId: 'team-source',
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

  it('describes the copy source with explicit spacing and punctuation (#1004)', () => {
    renderCopyDialog()

    expect(screen.getByText(/^Pre-filled from/)).toHaveTextContent(
      "Pre-filled from Shared OpenAI in Platform Team. Adjust anything you want to differ here — the copy is a snapshot and changing it won't affect the other team.",
      { normalizeWhitespace: false }
    )
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

  // -------------------------------------------------------------------------
  // #1076 — on-demand model listing
  // -------------------------------------------------------------------------

  const loadModelsButton = () => screen.getByRole('button', { name: /models$/ })

  const typeCreateConfig = async (user: ReturnType<typeof userEvent.setup>) => {
    await user.type(
      screen.getByPlaceholderText('https://api.openai.com/v1'),
      'https://api.openai.com/v1'
    )
    await user.type(screen.getByPlaceholderText('Enter API key'), 'sk-test')
  }

  it('keeps Load models disabled until the base URL is a valid URL', async () => {
    const user = userEvent.setup()
    renderDialog()

    expect(screen.getByRole('button', { name: 'Load models' })).toBeDisabled()
    await user.type(
      screen.getByPlaceholderText('https://api.openai.com/v1'),
      'https://api.openai.com/v1'
    )
    expect(screen.getByRole('button', { name: 'Load models' })).toBeEnabled()
    expect(mockedListModels).not.toHaveBeenCalled()
  })

  it('loads models before saving and submits the picked one (create)', async () => {
    const user = userEvent.setup()
    mockedListModels.mockResolvedValue({
      supported: true,
      models: [{ id: 'gpt-4o' }, { id: 'gpt-4o-mini', owned_by: 'openai' }],
    })
    mockedValidate.mockResolvedValue({ is_valid: true, message: 'ok' })
    const onSubmit = renderDialog()

    await user.type(
      screen.getByPlaceholderText('e.g., OpenAI GPT-4o'),
      'My Provider'
    )
    await typeCreateConfig(user)
    await user.click(screen.getByRole('button', { name: 'Load models' }))

    expect(await screen.findByTestId('model-combobox')).toBeInTheDocument()
    expect(mockedListModels).toHaveBeenCalledTimes(1)
    expect(mockedListModels).toHaveBeenCalledWith('team-1', {
      provider_type: 'openai_compatible',
      base_url: 'https://api.openai.com/v1',
      api_key: 'sk-test',
    })
    expect(
      screen.queryByPlaceholderText('e.g., gpt-4o-mini')
    ).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'pick gpt-4o' }))
    await user.click(screen.getByRole('button', { name: 'Add provider' }))

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ model: 'gpt-4o' })
      )
    })
  })

  it('falls back to the text input with a note when the endpoint does not list models', async () => {
    const user = userEvent.setup()
    mockedListModels.mockResolvedValue({ supported: false, models: [] })
    renderDialog()

    await typeCreateConfig(user)
    await user.click(screen.getByRole('button', { name: 'Load models' }))

    expect(
      await screen.findByText(
        "This endpoint doesn't list models — enter the model id manually."
      )
    ).toBeInTheDocument()
    expect(screen.getByPlaceholderText('e.g., gpt-4o-mini')).toBeInTheDocument()
    expect(screen.queryByTestId('model-list-error')).not.toBeInTheDocument()
  })

  it.each([
    ['connection_failed', "Couldn't reach the provider."],
    ['unauthorized', 'The provider rejected the API key.'],
    [
      'misconfigured_provider',
      "The provider's response wasn't understood — check the base URL.",
    ],
    ['destination_not_allowed', "This base URL isn't allowed by the server."],
  ] as const)(
    'shows a fixed sentence for the %s failure and retries',
    async (category, sentence) => {
      const user = userEvent.setup()
      mockedListModels.mockResolvedValue({
        supported: false,
        models: [],
        message: category,
      })
      renderDialog()

      await typeCreateConfig(user)
      await user.click(screen.getByRole('button', { name: 'Load models' }))

      const error = await screen.findByTestId('model-list-error')
      expect(error).toHaveTextContent(sentence)
      expect(error).not.toHaveTextContent(category)

      await user.click(screen.getByRole('button', { name: 'Retry' }))
      await waitFor(() => {
        expect(mockedListModels).toHaveBeenCalledTimes(2)
      })
    }
  )

  it('shows a generic message on a thrown request and still saves a typed model', async () => {
    const user = userEvent.setup()
    mockedListModels.mockRejectedValue(new Error('403'))
    mockedValidate.mockResolvedValue({ is_valid: true, message: 'ok' })
    const onSubmit = renderDialog()

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'Load models' }))

    expect(await screen.findByTestId('model-list-error')).toHaveTextContent(
      "Couldn't load models."
    )
    await user.click(screen.getByRole('button', { name: 'Add provider' }))

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ model: 'gpt-4o-mini' })
      )
    })
  })

  it('edit mode sends the saved provider and omits a blank key', async () => {
    const user = userEvent.setup()
    mockedListModels.mockResolvedValue({ supported: true, models: [] })
    render(
      <ModelProviderDialog
        teamId="team-1"
        open
        onOpenChange={vi.fn()}
        submitting={false}
        provider={existingProvider}
        onSubmit={vi.fn()}
      />
    )

    await user.click(screen.getByRole('button', { name: 'Load models' }))

    await waitFor(() => {
      expect(mockedListModels).toHaveBeenCalledWith('team-1', {
        provider_type: 'openai_compatible',
        base_url: 'https://api.openai.com/v1',
        provider_id: 'p1',
      })
    })
    expect(await screen.findByTestId('model-combobox')).toHaveAttribute(
      'data-value',
      'gpt-4o-mini'
    )
  })

  it('copy mode lists against the source team with the source provider', async () => {
    const user = userEvent.setup()
    mockedListModels.mockResolvedValue({
      supported: true,
      models: [{ id: 'gpt-4o' }],
    })
    renderCopyDialog()

    await user.click(screen.getByRole('button', { name: 'Load models' }))

    await waitFor(() => {
      expect(mockedListModels).toHaveBeenCalledWith('team-source', {
        provider_type: 'openai_compatible',
        base_url: 'https://api.openai.com/v1',
        provider_id: 'p1',
      })
    })
    expect(await screen.findByTestId('model-combobox')).toBeInTheDocument()
  })

  it('drops a loaded list when the base URL changes, keeping the model', async () => {
    const user = userEvent.setup()
    mockedListModels.mockResolvedValue({
      supported: true,
      models: [{ id: 'gpt-4o' }],
    })
    renderDialog()

    await typeCreateConfig(user)
    await user.click(screen.getByRole('button', { name: 'Load models' }))
    await user.click(await screen.findByRole('button', { name: 'pick gpt-4o' }))

    await user.type(
      screen.getByPlaceholderText('https://api.openai.com/v1'),
      '/x'
    )

    expect(screen.queryByTestId('model-combobox')).not.toBeInTheDocument()
    expect(screen.getByPlaceholderText('e.g., gpt-4o-mini')).toHaveValue(
      'gpt-4o'
    )
    expect(loadModelsButton()).toHaveTextContent('Load models')
  })
})
