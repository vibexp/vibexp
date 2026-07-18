/**
 * AdminStats (#316): loading skeletons, rendered counts + version, and the
 * error alert.
 */
import { render, screen } from '@testing-library/react'

import type { AdminStatsResponse } from '@/services/adminService'

jest.mock('@/services/adminService', () => ({
  adminService: { getStats: jest.fn() },
}))

import { adminService } from '@/services/adminService'

import { AdminStats } from '../AdminStats'

const mockAdminService = adminService as jest.Mocked<typeof adminService>

const stats: AdminStatsResponse = {
  counts: { users: 42, teams: 7, prompts: 3, artifacts: 5, memories: 9 },
  version: '1.2.3',
}

afterEach(() => {
  jest.clearAllMocks()
})

it('shows skeletons while loading', () => {
  mockAdminService.getStats.mockReturnValue(
    new Promise<AdminStatsResponse>(() => {})
  )
  render(<AdminStats />)

  expect(screen.getAllByTestId('stat-skeleton').length).toBeGreaterThan(0)
})

it('renders counts and version', async () => {
  mockAdminService.getStats.mockResolvedValue(stats)
  render(<AdminStats />)

  expect(await screen.findByText('42')).toBeInTheDocument()
  expect(screen.getByText('1.2.3')).toBeInTheDocument()
  expect(screen.getByText('Users')).toBeInTheDocument()
  expect(screen.getByText('Version')).toBeInTheDocument()
})

it('shows an error alert on failure', async () => {
  mockAdminService.getStats.mockRejectedValue(new Error('boom'))
  render(<AdminStats />)

  expect(await screen.findByText('Failed to load stats')).toBeInTheDocument()
})
