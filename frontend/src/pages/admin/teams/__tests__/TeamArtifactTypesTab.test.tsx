import { render, screen } from '@testing-library/react'

const mockGet = vi.hoisted(() => vi.fn())
vi.mock('@/services/adminService', () => ({
  adminService: {
    getTeamArtifactTypes: (...args: unknown[]) => mockGet(...args),
  },
}))

import { TeamArtifactTypesTab } from '../detail/TeamArtifactTypesTab'
import {
  artifactTypes,
  expectNoSentinel,
  expectReadOnly,
  withSentinels,
} from './teamConfigFixtures'

beforeEach(() => {
  vi.clearAllMocks()
})

it('shows a loading skeleton while the request is pending', () => {
  mockGet.mockReturnValue(new Promise(() => {}))
  render(<TeamArtifactTypesTab teamId="t1" />)
  expect(screen.getByTestId('config-loading')).toBeInTheDocument()
})

it('shows an error alert when the request fails', async () => {
  mockGet.mockRejectedValue(new Error('types down'))
  render(<TeamArtifactTypesTab teamId="t1" />)
  expect(
    await screen.findByText('Failed to load artifact types')
  ).toBeInTheDocument()
})

it('shows the empty state', async () => {
  mockGet.mockResolvedValue({ types: [] })
  render(<TeamArtifactTypesTab teamId="t1" />)
  expect(await screen.findByTestId('config-empty')).toBeInTheDocument()
})

it('renders system and team types', async () => {
  const data = artifactTypes()
  mockGet.mockResolvedValue(
    withSentinels({ types: data.types.map(withSentinels) })
  )
  render(<TeamArtifactTypesTab teamId="t1" />)

  expect(await screen.findByText('Report')).toBeInTheDocument()
  expect(mockGet).toHaveBeenCalledWith('t1')
  expect(screen.getAllByTestId('artifact-type-row')).toHaveLength(2)
  expect(screen.getByText('runbook')).toBeInTheDocument()
  expect(screen.getByText('System')).toBeInTheDocument()
  expect(screen.getByText('Team')).toBeInTheDocument()
  expectReadOnly()
  expectNoSentinel()
})
