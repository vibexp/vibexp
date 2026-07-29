import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { MockedFunction } from 'vitest'

import { toast } from '@/lib/toast'
import { embeddingProviderService } from '@/services/embeddingProviderService'

import { EmbeddingProviderDialog } from './EmbeddingProviderDialog'

vi.mock('@/services/embeddingProviderService', async () => ({
  // Keep EMBEDDING_VECTOR_DIMENSIONS (and any other exports) real; only the
  // service singleton is mocked so validate-on-save can be asserted.
  ...(await vi.importActual('@/services/embeddingProviderService')),
  embeddingProviderService: {
    validateEmbeddingProvider: vi.fn(),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

const mockedValidate =
  embeddingProviderService.validateEmbeddingProvider as MockedFunction<
    typeof embeddingProviderService.validateEmbeddingProvider
  >
const mockedToastError = toast.error as MockedFunction<typeof toast.error>

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

const renderDialog = (onSubmit = vi.fn().mockResolvedValue(undefined)) => {
  render(
    <EmbeddingProviderDialog
      teamId="team-1"
      teamName="Team"
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

  it('defaults concurrency to 1 and threads an edited value into submit', async () => {
    const user = userEvent.setup()
    mockedValidate.mockResolvedValue({ is_valid: true, message: 'ok' })
    const onSubmit = renderDialog()

    const concurrency = screen.getByLabelText('Concurrency')
    expect(concurrency).toHaveValue(1)

    await fillValidForm(user)
    await user.clear(concurrency)
    await user.type(concurrency, '4')
    await user.click(screen.getByRole('button', { name: 'Add provider' }))

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ concurrency: 4 })
      )
    })
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
    concurrency: 5,
    is_default: false,
    base_url: 'https://api.openai.com/v1',
    configuration: '{}',
    has_api_key: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    version: 1,
  }

  it('prefills concurrency from the provider on edit', () => {
    render(
      <EmbeddingProviderDialog
        teamId="team-1"
        teamName="Team"
        open
        onOpenChange={vi.fn()}
        submitting={false}
        provider={existingProvider}
        onSubmit={vi.fn()}
      />
    )
    expect(screen.getByLabelText('Concurrency')).toHaveValue(5)
  })

  it('skips validation on a name-only edit (identity unchanged)', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn().mockResolvedValue(undefined)
    render(
      <EmbeddingProviderDialog
        teamId="team-1"
        teamName="Team"
        open
        onOpenChange={vi.fn()}
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
    const onSubmit = vi.fn().mockResolvedValue(undefined)
    render(
      <EmbeddingProviderDialog
        teamId="team-1"
        teamName="Team"
        open
        onOpenChange={vi.fn()}
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

  it('applies a family preset to both prefix fields and submits them verbatim', async () => {
    const user = userEvent.setup()
    mockedValidate.mockResolvedValue({ is_valid: true, message: 'ok' })
    const onSubmit = renderDialog()

    await fillValidForm(user)
    // The E5 preset sets query "query: " and document "passage: " (trailing
    // spaces are significant and must be preserved, not trimmed).
    await user.click(screen.getByRole('button', { name: 'E5' }))

    expect(screen.getByLabelText('Query prefix')).toHaveValue('query: ')
    expect(screen.getByLabelText('Document prefix')).toHaveValue('passage: ')

    await user.click(screen.getByRole('button', { name: 'Add provider' }))

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          query_prefix: 'query: ',
          document_prefix: 'passage: ',
        })
      )
    })
  })

  it('confirms re-embed and skips validation on a document-prefix-only edit', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn().mockResolvedValue(undefined)
    render(
      <EmbeddingProviderDialog
        teamId="team-1"
        teamName="Team"
        open
        onOpenChange={vi.fn()}
        submitting={false}
        provider={existingProvider}
        onSubmit={onSubmit}
      />
    )

    await user.type(screen.getByLabelText('Document prefix'), 'passage: ')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    // A document-prefix change re-indexes documents, so it prompts the same
    // re-embed confirmation as an identity change.
    const confirm = await screen.findByRole('button', {
      name: 'Save & re-embed',
    })
    await user.click(confirm)

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ document_prefix: 'passage: ' })
      )
    })
    // The embedding identity did not change, so no validate probe is needed.
    expect(mockedValidate).not.toHaveBeenCalled()
  })

  it('does not re-embed for a query-prefix-only edit and submits directly', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn().mockResolvedValue(undefined)
    render(
      <EmbeddingProviderDialog
        teamId="team-1"
        teamName="Team"
        open
        onOpenChange={vi.fn()}
        submitting={false}
        provider={existingProvider}
        onSubmit={onSubmit}
      />
    )

    await user.type(screen.getByLabelText('Query prefix'), 'query: ')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ query_prefix: 'query: ' })
      )
    })
    // A query-prefix change affects only the query side: no confirmation, no probe.
    expect(
      screen.queryByRole('button', { name: 'Save & re-embed' })
    ).not.toBeInTheDocument()
    expect(mockedValidate).not.toHaveBeenCalled()
  })
})
