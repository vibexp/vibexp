import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'

const mockGet = vi.hoisted(() => vi.fn())
vi.mock('@/services/adminService', () => ({
  adminService: {
    getTeamFreshnessConfig: (...args: unknown[]) => mockGet(...args),
  },
}))

import { TeamFreshnessConfigTab } from '../detail/TeamFreshnessConfigTab'
import {
  expectNoSentinel,
  expectReadOnly,
  freshnessConfig,
  withSentinels,
} from './teamConfigFixtures'

function renderTab() {
  return render(
    <MemoryRouter>
      <TeamFreshnessConfigTab teamId="t1" />
    </MemoryRouter>
  )
}

beforeEach(() => {
  vi.clearAllMocks()
})

it('shows a loading skeleton while the request is pending', () => {
  mockGet.mockReturnValue(new Promise(() => {}))
  renderTab()
  expect(screen.getByTestId('config-loading')).toBeInTheDocument()
})

it('shows an error alert when the request fails', async () => {
  mockGet.mockRejectedValue(new Error('fresh down'))
  renderTab()
  expect(
    await screen.findByText('Failed to load freshness settings')
  ).toBeInTheDocument()
})

it('renders the settings and a sentence per rule', async () => {
  const config = freshnessConfig()
  mockGet.mockResolvedValue(
    withSentinels({ ...config, rules: config.rules.map(withSentinels) })
  )
  renderTab()

  expect(await screen.findByText('Daily')).toBeInTheDocument()
  expect(mockGet).toHaveBeenCalledWith('t1')
  expect(screen.getByTestId('config-source')).toHaveTextContent('Team')
  expect(screen.getAllByTestId('freshness-rule')).toHaveLength(2)
  expect(
    screen.getByText(
      'Artifacts in any project not accessed via any medium for 90 days'
    )
  ).toBeInTheDocument()
  expect(screen.getByText('Team-wide')).toBeInTheDocument()
  expect(screen.getByText('Enabled')).toBeInTheDocument()
  expect(screen.getByText('Disabled')).toBeInTheDocument()
  expectReadOnly()
  expectNoSentinel()
})

it('links a project-scoped rule to the admin project page', async () => {
  mockGet.mockResolvedValue(freshnessConfig())
  renderTab()

  const link = await screen.findByRole('link', { name: '3f2a9c1e' })
  expect(link).toHaveAttribute(
    'href',
    '/admin/projects/3f2a9c1e-0000-4000-8000-000000000001'
  )
  expect(
    screen.getByText(
      'Prompts, Memories in project 3f2a9c1e not accessed via CLI for 1 day'
    )
  ).toBeInTheDocument()
})

it('marks inherited settings and says when there are no rules', async () => {
  mockGet.mockResolvedValue(freshnessConfig({ source: 'instance', rules: [] }))
  renderTab()

  expect(await screen.findByTestId('config-source')).toHaveTextContent(
    'Inherited from instance'
  )
  expect(
    screen.getByText('This team has no freshness rules.')
  ).toBeInTheDocument()
})
