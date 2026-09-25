import { render, screen } from '@testing-library/react'

const mockGet = vi.hoisted(() => vi.fn())
vi.mock('@/services/adminService', () => ({
  adminService: {
    getTeamAISummaryConfig: (...args: unknown[]) => mockGet(...args),
  },
}))

import { TeamAISummaryConfigTab } from '../detail/TeamAISummaryConfigTab'
import {
  aiSummaryConfig,
  expectNoSentinel,
  expectReadOnly,
  withSentinels,
} from './teamConfigFixtures'

beforeEach(() => {
  vi.clearAllMocks()
})

it('shows a loading skeleton while the request is pending', () => {
  mockGet.mockReturnValue(new Promise(() => {}))
  render(<TeamAISummaryConfigTab teamId="t1" />)
  expect(screen.getByTestId('config-loading')).toBeInTheDocument()
})

it('shows an error alert when the request fails', async () => {
  mockGet.mockRejectedValue(new Error('ai down'))
  render(<TeamAISummaryConfigTab teamId="t1" />)
  expect(
    await screen.findByText('Failed to load AI summary settings')
  ).toBeInTheDocument()
})

it('renders the team-owned settings', async () => {
  mockGet.mockResolvedValue(withSentinels(aiSummaryConfig()))
  render(<TeamAISummaryConfigTab teamId="t1" />)

  expect(await screen.findByText('OpenAI prod')).toBeInTheDocument()
  expect(mockGet).toHaveBeenCalledWith('t1')
  expect(screen.getByTestId('config-source')).toHaveTextContent('Team')
  expect(screen.getByText('Detailed')).toBeInTheDocument()
  expect(screen.getByText('Available')).toBeInTheDocument()
  expect(screen.getByText(/5 \(max 10\)/)).toBeInTheDocument()
  expectReadOnly()
  expectNoSentinel()
})

it('marks inherited settings', async () => {
  mockGet.mockResolvedValue(aiSummaryConfig({ source: 'instance' }))
  render(<TeamAISummaryConfigTab teamId="t1" />)
  expect(await screen.findByTestId('config-source')).toHaveTextContent(
    'Inherited from instance'
  )
})

it('says "Provider removed" when the selected provider no longer resolves', async () => {
  mockGet.mockResolvedValue(
    aiSummaryConfig({ model_provider_name: null, available: false })
  )
  render(<TeamAISummaryConfigTab teamId="t1" />)
  expect(await screen.findByText('Provider removed')).toBeInTheDocument()
  expect(screen.getByText('No model provider')).toBeInTheDocument()
})

it('says "Team default" when none is selected and the team has a provider', async () => {
  const config = aiSummaryConfig({ model_provider_name: null })
  config.values.model_provider_id = null
  mockGet.mockResolvedValue(config)
  render(<TeamAISummaryConfigTab teamId="t1" />)
  expect(await screen.findByText('Team default')).toBeInTheDocument()
})

it('says "No provider" when none is selected and the team has none', async () => {
  const config = aiSummaryConfig({
    model_provider_name: null,
    available: false,
  })
  config.values.model_provider_id = null
  mockGet.mockResolvedValue(config)
  render(<TeamAISummaryConfigTab teamId="t1" />)
  expect(await screen.findByText('No provider')).toBeInTheDocument()
})
