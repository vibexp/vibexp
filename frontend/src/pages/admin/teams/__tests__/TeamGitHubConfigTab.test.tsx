import { render, screen } from '@testing-library/react'

const mockGet = vi.hoisted(() => vi.fn())
vi.mock('@/services/adminService', () => ({
  adminService: {
    getTeamGitHubConfig: (...args: unknown[]) => mockGet(...args),
  },
}))

import { TeamGitHubConfigTab } from '../detail/TeamGitHubConfigTab'
import {
  expectNoSentinel,
  expectReadOnly,
  githubConfig,
  withSentinels,
} from './teamConfigFixtures'

beforeEach(() => {
  vi.clearAllMocks()
})

it('shows a loading skeleton while the request is pending', () => {
  mockGet.mockReturnValue(new Promise(() => {}))
  render(<TeamGitHubConfigTab teamId="t1" />)
  expect(screen.getByTestId('config-loading')).toBeInTheDocument()
})

it('shows an error alert when the request fails', async () => {
  mockGet.mockRejectedValue(new Error('gh down'))
  render(<TeamGitHubConfigTab teamId="t1" />)
  expect(
    await screen.findByText('Failed to load GitHub integration')
  ).toBeInTheDocument()
})

it('renders the app registration and installation without secrets', async () => {
  const config = githubConfig()
  mockGet.mockResolvedValue(
    withSentinels({
      app_config: config.app_config && withSentinels(config.app_config),
      installation: withSentinels(config.installation),
    })
  )
  render(<TeamGitHubConfigTab teamId="t1" />)

  expect(await screen.findByText('team-app')).toBeInTheDocument()
  expect(mockGet).toHaveBeenCalledWith('t1')
  expect(screen.getByText('123456')).toBeInTheDocument()
  expect(screen.getAllByText('Configured ✓')).toHaveLength(3)
  expect(screen.getByText('Not set')).toBeInTheDocument()
  expect(screen.getByText('acme')).toBeInTheDocument()
  expect(screen.getByText('987654')).toBeInTheDocument()
  expect(screen.getByText('Active')).toBeInTheDocument()
  expectReadOnly()
  expectNoSentinel()
})

it('says when no app is configured and nothing is installed', async () => {
  mockGet.mockResolvedValue(
    githubConfig({
      app_config: null,
      installation: {
        installed: false,
        account_login: null,
        installation_id: null,
        suspended: false,
        installed_at: null,
      },
    })
  )
  render(<TeamGitHubConfigTab teamId="t1" />)

  expect(
    await screen.findByText('No GitHub App configured for this team.')
  ).toBeInTheDocument()
  expect(screen.getByText('Not installed.')).toBeInTheDocument()
  expect(screen.queryByText(/Inherited/)).not.toBeInTheDocument()
})
