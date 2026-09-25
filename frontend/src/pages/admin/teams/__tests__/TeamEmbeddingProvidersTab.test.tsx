import { render, screen } from '@testing-library/react'

const mockGet = vi.hoisted(() => vi.fn())
vi.mock('@/services/adminService', () => ({
  adminService: {
    getTeamEmbeddingProviders: (...args: unknown[]) => mockGet(...args),
  },
}))

import { TeamEmbeddingProvidersTab } from '../detail/TeamEmbeddingProvidersTab'
import {
  embeddingConfig,
  embeddingProvider,
  expectNoSentinel,
  expectReadOnly,
  withSentinels,
} from './teamConfigFixtures'

beforeEach(() => {
  vi.clearAllMocks()
})

it('shows a loading skeleton while the request is pending', () => {
  mockGet.mockReturnValue(new Promise(() => {}))
  render(<TeamEmbeddingProvidersTab teamId="t1" />)
  expect(screen.getByTestId('config-loading')).toBeInTheDocument()
})

it('shows an error alert when the request fails', async () => {
  mockGet.mockRejectedValue(new Error('ep down'))
  render(<TeamEmbeddingProvidersTab teamId="t1" />)
  expect(
    await screen.findByText('Failed to load embedding providers')
  ).toBeInTheDocument()
})

it('renders providers with chunking details and the coverage table', async () => {
  mockGet.mockResolvedValue(
    withSentinels(
      embeddingConfig({
        providers: [
          withSentinels(
            embeddingProvider({
              has_api_key: true,
              configuration_keys: ['dimensions'],
            })
          ),
        ],
      })
    )
  )
  render(<TeamEmbeddingProvidersTab teamId="t1" />)

  expect(await screen.findByText('Local embeddings')).toBeInTheDocument()
  expect(mockGet).toHaveBeenCalledWith('t1')
  expect(screen.getByText('512 / 64 overlap')).toBeInTheDocument()
  expect(screen.getByText('search_query:')).toBeInTheDocument()
  expect(screen.getByText('dimensions')).toBeInTheDocument()
  expect(screen.getByText('Configured ✓')).toBeInTheDocument()
  expect(screen.getAllByText('nomic-embed-text').length).toBeGreaterThan(0)
  expect(screen.getByTestId('coverage-row')).toHaveTextContent('80%')
  expectReadOnly()
  expectNoSentinel()
})

it('says so when the team has no providers', async () => {
  mockGet.mockResolvedValue(
    embeddingConfig({
      providers: [],
      coverage: { has_active_provider: false, active_model: null, items: [] },
    })
  )
  render(<TeamEmbeddingProvidersTab teamId="t1" />)

  expect(
    await screen.findByText('No embedding providers configured for this team.')
  ).toBeInTheDocument()
  expect(screen.getByText('No active provider')).toBeInTheDocument()
  expect(screen.queryByText(/Inherited/)).not.toBeInTheDocument()
})
