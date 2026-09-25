import { render, screen } from '@testing-library/react'

const mockGet = vi.hoisted(() => vi.fn())
vi.mock('@/services/adminService', () => ({
  adminService: {
    getTeamSearchConfig: (...args: unknown[]) => mockGet(...args),
  },
}))

import { TeamSearchConfigTab } from '../detail/TeamSearchConfigTab'
import {
  expectNoSentinel,
  expectReadOnly,
  searchConfig,
  withSentinels,
} from './teamConfigFixtures'

beforeEach(() => {
  vi.clearAllMocks()
})

it('shows a loading skeleton while the request is pending', () => {
  mockGet.mockReturnValue(new Promise(() => {}))
  render(<TeamSearchConfigTab teamId="t1" />)
  expect(screen.getByTestId('config-loading')).toBeInTheDocument()
})

it('shows an error alert when the request fails', async () => {
  mockGet.mockRejectedValue(new Error('search down'))
  render(<TeamSearchConfigTab teamId="t1" />)
  expect(
    await screen.findByText('Failed to load search settings')
  ).toBeInTheDocument()
  expect(screen.getByText('search down')).toBeInTheDocument()
})

it('renders team-owned values alongside the instance defaults', async () => {
  mockGet.mockResolvedValue(withSentinels(searchConfig()))
  render(<TeamSearchConfigTab teamId="t1" />)

  expect(await screen.findByText('Search ranking')).toBeInTheDocument()
  expect(mockGet).toHaveBeenCalledWith('t1')
  expect(screen.getByTestId('config-source')).toHaveTextContent('Team')
  expect(screen.getByText('0.7')).toBeInTheDocument()
  expect(screen.getByText('200')).toBeInTheDocument()
  expect(screen.getByText('Instance defaults')).toBeInTheDocument()
  expect(screen.getByText('0.9')).toBeInTheDocument()
  expectReadOnly()
  expectNoSentinel()
})

it('marks inherited values and does not repeat the defaults', async () => {
  mockGet.mockResolvedValue(searchConfig({ source: 'instance' }))
  render(<TeamSearchConfigTab teamId="t1" />)

  expect(await screen.findByTestId('config-source')).toHaveTextContent(
    'Inherited from instance'
  )
  expect(screen.queryByText('Instance defaults')).not.toBeInTheDocument()
})
