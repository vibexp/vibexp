import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { toast } from '@/lib/toast'
import { embeddingProviderService } from '@/services/embeddingProviderService'

import { EmbeddingProviderDialog } from './EmbeddingProviderDialog'

jest.mock('@/services/embeddingProviderService', () => ({
  // Keep EMBEDDING_VECTOR_DIMENSIONS (and any other exports) real; only the
  // service singleton is mocked so validate-on-save can be asserted.
  ...jest.requireActual('@/services/embeddingProviderService'),
  embeddingProviderService: {
    validateEmbeddingProvider: jest.fn(),
  },
}))

jest.mock('@/lib/toast', () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}))

const mockedValidate =
  embeddingProviderService.validateEmbeddingProvider as jest.MockedFunction<
    typeof embeddingProviderService.validateEmbeddingProvider
  >
const mockedToastError = toast.error as jest.MockedFunction<typeof toast.error>

const fillValidForm = async (user: ReturnType<typeof userEvent.setup>) => {
  await user.type(
    screen.getByPlaceholderText('e.g., OpenAI Embeddings'),
    'My Provider'
  )
  await user.type(
    screen.getByPlaceholderText('e.g., text-embedding-3-small'),
    'text-embedding-3-small'
  )
  await user.type(
    screen.getByPlaceholderText('https://api.openai.com/v1'),
    'https://api.openai.com/v1'
  )
  await user.type(screen.getByPlaceholderText('Enter API key'), 'sk-test')
}

const renderDialog = (onSubmit = jest.fn().mockResolvedValue(undefined)) => {
  render(
    <EmbeddingProviderDialog
      teamId="team-1"
      open
      onOpenChange={jest.fn()}
      submitting={false}
      onSubmit={onSubmit}
    />
  )
  return onSubmit
}

beforeEach(() => {
  jest.clearAllMocks()
})

describe('EmbeddingProviderDialog', () => {
  it('shows the model field and a read-only 1024 dimension', () => {
    renderDialog()
    expect(
      screen.getByPlaceholderText('e.g., text-embedding-3-small')
    ).toBeInTheDocument()
    const dimension = screen.getByLabelText('Embedding vector dimension')
    expect(dimension).toHaveValue('1024')
    expect(dimension).toBeDisabled()
  })

  it('validates on save and submits when the provider returns 1024 dims', async () => {
    const user = userEvent.setup()
    mockedValidate.mockResolvedValue({
      is_valid: true,
      message: 'ok',
      details: { dimension: 1024 },
    })
    const onSubmit = renderDialog()

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'Add provider' }))

    await waitFor(() => {
      expect(mockedValidate).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ model: 'text-embedding-3-small' })
      )
    })
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ model: 'text-embedding-3-small' })
    )
  })

  it('blocks submit and shows an error when validation fails', async () => {
    const user = userEvent.setup()
    mockedValidate.mockResolvedValue({
      is_valid: false,
      message: 'Provider must return 1024-dimensional embeddings',
    })
    const onSubmit = renderDialog()

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'Add provider' }))

    await waitFor(() => {
      expect(mockedToastError).toHaveBeenCalledWith(
        'Provider must return 1024-dimensional embeddings',
        expect.anything()
      )
    })
    expect(onSubmit).not.toHaveBeenCalled()
  })

  const existingProvider = {
    id: 'p1',
    user_id: 'u1',
    name: 'Existing',
    provider_type: 'openai_compatible',
    model: 'text-embedding-3-small',
    chunk_size: 1000,
    chunk_overlap: 200,
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
    const onSubmit = jest.fn().mockResolvedValue(undefined)
    render(
      <EmbeddingProviderDialog
        teamId="team-1"
        open
        onOpenChange={jest.fn()}
        submitting={false}
        provider={existingProvider}
        onSubmit={onSubmit}
      />
    )

    const name = screen.getByPlaceholderText('e.g., OpenAI Embeddings')
    await user.clear(name)
    await user.type(name, 'Renamed')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalled()
    })
    expect(mockedValidate).not.toHaveBeenCalled()
  })

  it('confirms re-embed, then validates, when the model changes on edit', async () => {
    const user = userEvent.setup()
    mockedValidate.mockResolvedValue({ is_valid: true, message: 'ok' })
    const onSubmit = jest.fn().mockResolvedValue(undefined)
    render(
      <EmbeddingProviderDialog
        teamId="team-1"
        open
        onOpenChange={jest.fn()}
        submitting={false}
        provider={existingProvider}
        onSubmit={onSubmit}
      />
    )

    const model = screen.getByPlaceholderText('e.g., text-embedding-3-small')
    await user.clear(model)
    await user.type(model, 'text-embedding-3-large')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    // A model change first prompts the re-embed confirmation; nothing runs yet.
    const confirm = await screen.findByRole('button', {
      name: 'Save & re-embed',
    })
    expect(mockedValidate).not.toHaveBeenCalled()

    await user.click(confirm)

    await waitFor(() => {
      expect(mockedValidate).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ model: 'text-embedding-3-large' })
      )
    })
    expect(onSubmit).toHaveBeenCalled()
  })
})
