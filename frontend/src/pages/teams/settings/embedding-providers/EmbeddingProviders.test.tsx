import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Mocked } from 'vitest'

import { toast } from '@/lib/toast'
import type {
  EmbeddingCoverageResponse,
  EmbeddingProviderResponse,
} from '@/services/embeddingProviderService'
import { embeddingProviderService } from '@/services/embeddingProviderService'
import type { Team } from '@/services/teamService'

import { EmbeddingProviders } from './EmbeddingProviders'

// The team TeamScopeLayout resolved from the URL (#584). Module-stable so a
// fresh identity per render cannot loop an effect keyed on it. `permissions` is
// what usePermissions reads — deliberately NOT mocked, so the copy gate is
// exercised through the real hook against these fixtures.
const urlTeam = {
  id: 'team-1',
  name: 'Team',
  permissions: ['team.update'],
} as unknown as Team

const sourceTeam = {
  id: 'team-2',
  name: 'Platform Team',
  permissions: ['team.update'],
} as unknown as Team

const renderPage = (team: Team = urlTeam) =>
  render(<EmbeddingProviders team={team} />)

const mockUseTeam = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

// Stable handleError reference (like the real useCallback-backed hook) so
// loadProviders isn't recreated every render — an unstable one loops the mount
// effect and remounts the section under test.
vi.mock('@/hooks/useErrorHandler', () => {
  const handleError = vi.fn()
  return { useErrorHandler: () => ({ handleError }) }
})

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

vi.mock('@/services/embeddingProviderService', async () => ({
  // Keep types + EMBEDDING_VECTOR_DIMENSIONS real; only mock the singleton.
  ...(await vi.importActual('@/services/embeddingProviderService')),
  embeddingProviderService: {
    getEmbeddingProviders: vi.fn(),
    getEmbeddingCoverage: vi.fn(),
    reprocessEmbeddingProvider: vi.fn(),
    clearEmbeddings: vi.fn(),
    validateEmbeddingProvider: vi.fn(),
    copyEmbeddingProviderFromTeam: vi.fn(),
  },
}))

const service = embeddingProviderService as Mocked<
  typeof embeddingProviderService
>
const mockedToast = toast as Mocked<typeof toast>

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

// cmdk (inside the copy dialog's popover) needs APIs jsdom does not provide.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
})

beforeEach(() => {
  vi.clearAllMocks()
  mockUseTeam.mockReturnValue({
    currentTeam: urlTeam,
    teams: [urlTeam, sourceTeam],
  })
  service.getEmbeddingProviders.mockResolvedValue([provider])
  service.getEmbeddingCoverage.mockResolvedValue(coverage)
  service.reprocessEmbeddingProvider.mockResolvedValue(undefined)
  service.clearEmbeddings.mockResolvedValue({ deleted_count: 160 })
})

describe('EmbeddingProviders coverage', () => {
  it('renders embedded / pending counts and % embedded for the active provider', async () => {
    renderPage()

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

    renderPage()

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

    renderPage()

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

    renderPage()

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

    renderPage()

    const clearButton = await screen.findByRole('button', {
      name: /clear all embeddings/i,
    })
    expect(clearButton).toBeDisabled()
  })

  it('renders an inline error alert when coverage fails to load', async () => {
    service.getEmbeddingCoverage.mockRejectedValue(new Error('boom'))

    renderPage()

    expect(
      await screen.findByText(/couldn.t load embedding coverage/i)
    ).toBeInTheDocument()
    expect(screen.getByText('boom')).toBeInTheDocument()
    // Providers table still renders — a status hiccup must not blank the page.
    expect(screen.getByText('OpenAI')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// #584 — cold deep-link: act on the URL's team, never the ambient one
// ---------------------------------------------------------------------------
// React fires child effects before parent effects, so on a cold deep-link to
// /teams/B/... this page's load effect runs BEFORE TeamScopeLayout's
// setCurrentTeam sync and the ambient team is still A. These tests pin that the
// page reads the team from its prop by driving the two apart.

describe('cold deep-link (#584)', () => {
  const urlTeamB = {
    id: 'team-b',
    name: 'Team B',
    permissions: [],
  } as unknown as Team

  it('fetches providers and coverage for the URL team, never the ambient one', async () => {
    mockUseTeam.mockReturnValue({
      currentTeam: { id: 'team-a', name: 'Team A' },
      teams: [],
    })

    renderPage(urlTeamB)

    await waitFor(() => {
      expect(service.getEmbeddingProviders).toHaveBeenCalledWith('team-b')
    })
    await waitFor(() => {
      expect(service.getEmbeddingCoverage).toHaveBeenCalledWith('team-b')
    })
    expect(service.getEmbeddingProviders).not.toHaveBeenCalledWith('team-a')
    expect(service.getEmbeddingCoverage).not.toHaveBeenCalledWith('team-a')
  })
})

// ---------------------------------------------------------------------------
// #835 — copy a provider in from another team, and the activation warning
// ---------------------------------------------------------------------------

const sourceProvider: EmbeddingProviderResponse = {
  ...provider,
  id: 'provider-9',
  name: 'Shared mxbai',
  model: 'mxbai-embed-large',
  chunk_size: 512,
  chunk_overlap: 64,
  query_prefix: 'query: ',
  document_prefix: 'passage: ',
  is_default: true,
}

const copiedProvider: EmbeddingProviderResponse = {
  ...sourceProvider,
  id: 'provider-copy',
  is_default: false,
}

// The verdict shape #831 returns when nothing about the team's search changed.
const inertActivation = {
  becomes_active: false,
  displaced_model: null,
  displaced_embedded_resources: 0,
  reprocess_enqueued: false,
  embeddings_wiped: false,
}

describe('copy from another team (#835)', () => {
  // Walks the two-step flow up to the pre-filled provider dialog: pick the
  // source team, then the one provider the copy will move.
  const openCopyFlow = async (user: ReturnType<typeof userEvent.setup>) => {
    await user.click(screen.getByTestId('copy-embedding-provider-button'))
    await user.click(await screen.findByTestId('source-team-picker'))
    await user.click(await screen.findByText('Platform Team'))
    await user.click(await screen.findByTestId('copy-source-provider'))
    await user.click(screen.getByTestId('confirm-copy-from-team-button'))
  }

  beforeEach(() => {
    service.getEmbeddingProviders.mockImplementation((id: string) =>
      Promise.resolve(id === 'team-2' ? [sourceProvider] : [provider])
    )
    service.copyEmbeddingProviderFromTeam.mockResolvedValue({
      provider: copiedProvider,
      activation: inertActivation,
    })
  })

  it('offers the action beside "Add provider" and in the empty state', async () => {
    service.getEmbeddingProviders.mockResolvedValue([])
    renderPage()

    expect(
      await screen.findByTestId('copy-embedding-provider-button')
    ).toBeInTheDocument()
    expect(
      await screen.findByTestId('copy-embedding-provider-button-empty')
    ).toBeInTheDocument()
  })

  it('hides the action without team.update on this team', async () => {
    const noPerms = {
      id: 'team-1',
      name: 'Team',
      permissions: [],
    } as unknown as Team
    mockUseTeam.mockReturnValue({
      currentTeam: noPerms,
      teams: [noPerms, sourceTeam],
    })

    renderPage(noPerms)

    await screen.findByText('OpenAI')
    expect(screen.queryByTestId('copy-embedding-provider-button')).toBeNull()
  })

  it('hides the action when no other team grants team.update on the source', async () => {
    const readOnlyOther = {
      id: 'team-3',
      name: 'Read Only',
      permissions: [],
    } as unknown as Team
    mockUseTeam.mockReturnValue({
      currentTeam: urlTeam,
      teams: [urlTeam, readOnlyOther],
    })

    renderPage()

    await screen.findByText('OpenAI')
    expect(screen.queryByTestId('copy-embedding-provider-button')).toBeNull()
  })

  it('lists the source team’s providers once a source team is picked', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('OpenAI')

    await user.click(screen.getByTestId('copy-embedding-provider-button'))
    await user.click(await screen.findByTestId('source-team-picker'))
    await user.click(await screen.findByText('Platform Team'))

    expect(await screen.findByText('Shared mxbai')).toBeInTheDocument()
    expect(service.getEmbeddingProviders).toHaveBeenCalledWith('team-2')
    // Nothing picked yet, so the copy is not confirmable.
    expect(screen.getByTestId('confirm-copy-from-team-button')).toBeDisabled()
  })

  it('pre-fills every non-secret field, never probes the provider, and posts the copy', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('OpenAI')

    await openCopyFlow(user)

    expect(
      await screen.findByText('Copy embedding provider')
    ).toBeInTheDocument()
    // Key is read-only: the ciphertext moves server-side, never through here.
    expect(screen.getByTestId('copy-api-key-field')).toHaveValue(
      'Will be copied from Platform Team'
    )
    // Chunk sizing and the prefixes come across pre-filled and overridable.
    expect(screen.getByLabelText('Chunk size')).toHaveValue(512)
    expect(screen.getByLabelText('Chunk overlap')).toHaveValue(64)
    expect(screen.getByLabelText('Query prefix')).toHaveValue('query: ')
    expect(screen.getByLabelText('Document prefix')).toHaveValue('passage: ')

    await user.click(screen.getByRole('button', { name: 'Copy provider' }))

    await waitFor(() => {
      expect(service.copyEmbeddingProviderFromTeam).toHaveBeenCalledWith(
        'team-1',
        {
          source_team_id: 'team-2',
          source_provider_id: 'provider-9',
          name: 'Shared mxbai',
          provider_type: 'openai',
          model: 'mxbai-embed-large',
          base_url: 'https://api.openai.com/v1',
          chunk_size: 512,
          chunk_overlap: 64,
          concurrency: 1,
          query_prefix: 'query: ',
          document_prefix: 'passage: ',
          reprocess: false,
        }
      )
    })
    // The copy path must never run the validate-on-save probe: with no key in
    // the SPA's hands it could only ever fail with an auth error.
    expect(service.validateEmbeddingProvider).not.toHaveBeenCalled()
    expect(mockedToast.success).toHaveBeenCalledWith(
      'Provider copied from Platform Team'
    )
  })

  it('posts reprocess: true when the re-process checkbox is ticked', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('OpenAI')

    await openCopyFlow(user)
    await screen.findByText('Copy embedding provider')
    await user.click(screen.getByTestId('copy-reprocess-checkbox'))
    await user.click(screen.getByRole('button', { name: 'Copy provider' }))

    await waitFor(() => {
      expect(service.copyEmbeddingProviderFromTeam).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ reprocess: true })
      )
    })
  })

  it('shows no warning when the server reports activation is unchanged', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('OpenAI')

    await openCopyFlow(user)
    await screen.findByText('Copy embedding provider')
    await user.click(screen.getByRole('button', { name: 'Copy provider' }))

    await waitFor(() => {
      expect(service.copyEmbeddingProviderFromTeam).toHaveBeenCalled()
    })
    // becomes_active: false ⇒ nothing about search changed, so no alertdialog.
    expect(screen.queryByRole('alertdialog')).toBeNull()
  })

  it('warns — naming team, displaced model and count — whenever the server reports activation changed, and re-embeds on confirm', async () => {
    // The recency case the issue is about: the copy lands NON-default and yet
    // becomes the active provider, so a client-side is_default check would
    // wrongly stay silent here.
    service.copyEmbeddingProviderFromTeam.mockResolvedValue({
      provider: copiedProvider,
      activation: {
        becomes_active: true,
        displaced_model: 'text-embedding-3-small',
        displaced_embedded_resources: 412,
        reprocess_enqueued: false,
        embeddings_wiped: false,
      },
    })

    const user = userEvent.setup()
    renderPage()
    await screen.findByText('OpenAI')

    await openCopyFlow(user)
    await screen.findByText('Copy embedding provider')
    await user.click(screen.getByRole('button', { name: 'Copy provider' }))

    const warning = await screen.findByRole('alertdialog')
    expect(warning).toHaveTextContent("Re-embed Team's resources?")
    expect(warning).toHaveTextContent('text-embedding-3-small')
    expect(warning).toHaveTextContent('412')

    await user.click(
      within(warning).getByRole('button', { name: 'Re-embed now' })
    )

    await waitFor(() => {
      expect(service.reprocessEmbeddingProvider).toHaveBeenCalledWith(
        'team-1',
        'provider-copy'
      )
    })
  })

  it('reports, without offering the remedy again, when the copy already enqueued a re-embed', async () => {
    service.copyEmbeddingProviderFromTeam.mockResolvedValue({
      provider: copiedProvider,
      activation: {
        becomes_active: true,
        displaced_model: 'text-embedding-3-small',
        displaced_embedded_resources: 412,
        reprocess_enqueued: true,
        embeddings_wiped: true,
      },
    })

    const user = userEvent.setup()
    renderPage()
    await screen.findByText('OpenAI')

    await openCopyFlow(user)
    await screen.findByText('Copy embedding provider')
    await user.click(screen.getByRole('button', { name: 'Copy provider' }))

    const warning = await screen.findByRole('alertdialog')
    expect(warning).toHaveTextContent('search now uses the copied provider')
    expect(warning).toHaveTextContent('stale vectors were cleared first')
    expect(
      within(warning).queryByRole('button', { name: 'Re-embed now' })
    ).toBeNull()

    await user.click(within(warning).getByRole('button', { name: 'Got it' }))
    expect(service.reprocessEmbeddingProvider).not.toHaveBeenCalled()
  })
})
