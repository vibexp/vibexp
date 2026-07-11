import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { toast } from '@/lib/toast'
import type {
  EmbeddingCoverageResponse,
  EmbeddingProviderResponse,
} from '@/services/embeddingProviderService'
import { embeddingProviderService } from '@/services/embeddingProviderService'

import { EmbeddingProviders } from './EmbeddingProviders'

const mockUseTeam = jest.fn()
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

// Stable handleError reference (like the real useCallback-backed hook) so
// loadProviders isn't recreated every render — an unstable one loops the mount
// effect and remounts the section under test.
jest.mock('@/hooks/useErrorHandler', () => {
  const handleError = jest.fn()
  return { useErrorHandler: () => ({ handleError }) }
})

jest.mock('@/lib/toast', () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}))

jest.mock('@/services/embeddingProviderService', () => ({
  // Keep types + EMBEDDING_VECTOR_DIMENSIONS real; only mock the singleton.
  ...jest.requireActual('@/services/embeddingProviderService'),
  embeddingProviderService: {
    getEmbeddingProviders: jest.fn(),
    getEmbeddingCoverage: jest.fn(),
    reprocessEmbeddingProvider: jest.fn(),
    clearEmbeddings: jest.fn(),
    validateEmbeddingProvider: jest.fn(),
  },
}))

const service = embeddingProviderService as jest.Mocked<
  typeof embeddingProviderService
>
const mockedToast = toast as jest.Mocked<typeof toast>

const provider: EmbeddingProviderResponse = {
  id: 'provider-1',
  user_id: 'user-1',
  name: 'OpenAI',
  provider_type: 'openai',
  model: 'text-embedding-3-small',
  chunk_size: 1000,
  chunk_overlap: 200,
  concurrency: 1,
  is_default: true,
  base_url: 'https://api.openai.com/v1',
  configuration: '{}',
  created_at: '2023-01-01T00:00:00Z',
  updated_at: '2023-01-01T00:00:00Z',
  version: 1,
  has_api_key: true,
}

const coverage: EmbeddingCoverageResponse = {
  has_active_provider: true,
  active_model: 'text-embedding-3-small',
  // Aggregate is deliberately distinct from every per-type value so the summary
  // cards can be asserted with plain text queries: embedded 160 / total 200 /
  // pending 40 / 80%, while per-type rows read 90% and 70%.
  coverage: [
    {
      entity_type: 'prompt',
      total: 100,
      embedded: 90,
      pending: 10,
      embedded_percent: 90,
    },
    {
      entity_type: 'artifact',
      total: 100,
      embedded: 70,
      pending: 30,
      embedded_percent: 70,
    },
  ],
}

beforeEach(() => {
  jest.clearAllMocks()
  mockUseTeam.mockReturnValue({ currentTeam: { id: 'team-1', name: 'Team' } })
  service.getEmbeddingProviders.mockResolvedValue([provider])
  service.getEmbeddingCoverage.mockResolvedValue(coverage)
  service.reprocessEmbeddingProvider.mockResolvedValue(undefined)
  service.clearEmbeddings.mockResolvedValue({ deleted_count: 160 })
})

describe('EmbeddingProviders coverage', () => {
  it('renders embedded / pending counts and % embedded for the active provider', async () => {
    render(<EmbeddingProviders />)

    // Aggregate across types: 160 embedded of 200, 40 pending, 80%.
    expect(await screen.findByText('Embedding coverage')).toBeInTheDocument()
    expect(screen.getByText('Embedded')).toBeInTheDocument()
    expect(screen.getByText('160')).toBeInTheDocument()
    expect(screen.getByText('of 200 items')).toBeInTheDocument()
    expect(screen.getByText('Pending')).toBeInTheDocument()
    expect(screen.getByText('40')).toBeInTheDocument()
    expect(screen.getByText('% embedded')).toBeInTheDocument()
    expect(screen.getByText('80%')).toBeInTheDocument()

    // Per-type breakdown is present.
    expect(screen.getByText('Prompts')).toBeInTheDocument()
    expect(screen.getByText('Artifacts')).toBeInTheDocument()
    expect(screen.getByText('90%')).toBeInTheDocument()
    expect(screen.getByText('70%')).toBeInTheDocument()
  })

  it('shows 0% without NaN when there are no entities', async () => {
    service.getEmbeddingCoverage.mockResolvedValue({
      has_active_provider: true,
      active_model: 'text-embedding-3-small',
      coverage: [
        {
          entity_type: 'prompt',
          total: 0,
          embedded: 0,
          pending: 0,
          embedded_percent: 0,
        },
      ],
    })

    render(<EmbeddingProviders />)

    // Overall + the single per-type card both read 0% — no NaN anywhere.
    await screen.findByText('% embedded')
    expect(screen.getAllByText('0%').length).toBeGreaterThan(0)
    expect(screen.queryByText(/NaN/)).not.toBeInTheDocument()
  })

  it('reprocesses via the default provider, disables while running, and refetches coverage', async () => {
    const user = userEvent.setup()
    let resolveReprocess: () => void = () => {}
    service.reprocessEmbeddingProvider.mockReturnValue(
      new Promise<void>(resolve => {
        resolveReprocess = resolve
      })
    )

    render(<EmbeddingProviders />)

    const button = await screen.findByRole('button', {
      name: /reprocess pending/i,
    })
    await waitFor(() => {
      expect(button).toBeEnabled()
    })

    await user.click(button)

    expect(service.reprocessEmbeddingProvider).toHaveBeenCalledWith(
      'team-1',
      'provider-1'
    )
    // Disabled + spinner while the request is in flight.
    expect(button).toBeDisabled()

    resolveReprocess()

    await waitFor(() => {
      expect(mockedToast.success).toHaveBeenCalledWith(
        'Reprocessing started',
        expect.any(Object)
      )
    })
    // Mount fetch + post-reprocess refetch.
    await waitFor(() => {
      expect(service.getEmbeddingCoverage).toHaveBeenCalledTimes(2)
    })
    await waitFor(() => {
      expect(button).toBeEnabled()
    })
  })

  it('clears all embeddings after confirmation and refetches coverage', async () => {
    const user = userEvent.setup()

    render(<EmbeddingProviders />)

    const clearButton = await screen.findByRole('button', {
      name: /clear all embeddings/i,
    })
    // Enabled because the team has embedded content (160 embedded).
    await waitFor(() => {
      expect(clearButton).toBeEnabled()
    })

    await user.click(clearButton)

    // Confirmation dialog gates the destructive action.
    const confirm = await screen.findByRole('button', { name: /^clear all$/i })
    expect(service.clearEmbeddings).not.toHaveBeenCalled()

    await user.click(confirm)

    expect(service.clearEmbeddings).toHaveBeenCalledWith('team-1')
    await waitFor(() => {
      expect(mockedToast.success).toHaveBeenCalledWith(
        'Embeddings cleared',
        expect.any(Object)
      )
    })
    // Mount fetch + post-clear refetch.
    await waitFor(() => {
      expect(service.getEmbeddingCoverage).toHaveBeenCalledTimes(2)
    })
  })

  it('disables clear all embeddings when nothing is embedded', async () => {
    service.getEmbeddingCoverage.mockResolvedValue({
      has_active_provider: true,
      active_model: 'text-embedding-3-small',
      coverage: [
        {
          entity_type: 'prompt',
          total: 10,
          embedded: 0,
          pending: 10,
          embedded_percent: 0,
        },
      ],
    })

    render(<EmbeddingProviders />)

    const clearButton = await screen.findByRole('button', {
      name: /clear all embeddings/i,
    })
    expect(clearButton).toBeDisabled()
  })

  it('renders an inline error alert when coverage fails to load', async () => {
    service.getEmbeddingCoverage.mockRejectedValue(new Error('boom'))

    render(<EmbeddingProviders />)

    expect(
      await screen.findByText(/couldn.t load embedding coverage/i)
    ).toBeInTheDocument()
    expect(screen.getByText('boom')).toBeInTheDocument()
    // Providers table still renders — a status hiccup must not blank the page.
    expect(screen.getByText('OpenAI')).toBeInTheDocument()
  })
})
