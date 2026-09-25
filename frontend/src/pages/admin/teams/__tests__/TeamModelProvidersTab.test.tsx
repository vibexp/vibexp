import { render, screen } from '@testing-library/react'

const mockGet = vi.hoisted(() => vi.fn())
vi.mock('@/services/adminService', () => ({
  adminService: {
    getTeamModelProviders: (...args: unknown[]) => mockGet(...args),
  },
}))

import { TeamModelProvidersTab } from '../detail/TeamModelProvidersTab'
import {
  expectNoSentinel,
  expectReadOnly,
  modelProvider,
  withSentinels,
} from './teamConfigFixtures'

beforeEach(() => {
  vi.clearAllMocks()
})

it('shows a loading skeleton while the request is pending', () => {
  mockGet.mockReturnValue(new Promise(() => {}))
  render(<TeamModelProvidersTab teamId="t1" />)
  expect(screen.getByTestId('config-loading')).toBeInTheDocument()
})

it('shows an error alert when the request fails', async () => {
  mockGet.mockRejectedValue(new Error('mp down'))
  render(<TeamModelProvidersTab teamId="t1" />)
  expect(
    await screen.findByText('Failed to load model providers')
  ).toBeInTheDocument()
})

it('says so when the team has no providers, without claiming inheritance', async () => {
  mockGet.mockResolvedValue({ providers: [] })
  render(<TeamModelProvidersTab teamId="t1" />)
  expect(
    await screen.findByText('No model providers configured for this team.')
  ).toBeInTheDocument()
  expect(screen.queryByText(/Inherited/)).not.toBeInTheDocument()
})

it('renders each provider with key names only and API-key state', async () => {
  mockGet.mockResolvedValue({
    providers: [
      withSentinels(modelProvider()),
      withSentinels(
        modelProvider({
          id: 'mp-2',
          name: 'Backup',
          is_default: false,
          has_api_key: false,
          base_url: null,
          configuration_keys: [],
        })
      ),
    ],
  })
  render(<TeamModelProvidersTab teamId="t1" />)

  expect(await screen.findByText('OpenAI prod')).toBeInTheDocument()
  expect(mockGet).toHaveBeenCalledWith('t1')
  expect(screen.getAllByTestId('provider-row')).toHaveLength(2)
  expect(screen.getByText('Default')).toBeInTheDocument()
  expect(screen.getByText('https://api.openai.com/v1')).toBeInTheDocument()
  expect(screen.getByText('organization')).toBeInTheDocument()
  expect(screen.getByText('temperature')).toBeInTheDocument()
  expect(screen.getByText('Configured ✓')).toBeInTheDocument()
  expect(screen.getByText('Not set')).toBeInTheDocument()
  expectReadOnly()
  expectNoSentinel()
})
