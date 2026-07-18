/**
 * AdminTeamDetail (#316): renders the team, owner, and member list, and the
 * error state (e.g. a 404 for an unknown id).
 */
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { AdminTeamDetail as AdminTeamDetailType } from '@/services/adminService'

jest.mock('@/services/adminService', () => ({
  adminService: { getTeam: jest.fn() },
}))

import { adminService } from '@/services/adminService'

import { AdminTeamDetail } from '../AdminTeamDetail'

const mockAdminService = adminService as jest.Mocked<typeof adminService>

const team: AdminTeamDetailType = {
  id: 't1',
  name: 'Engineering',
  owner: { id: 'o1', email: 'owner@example.com', name: 'Owner' },
  created_at: '2026-01-01T00:00:00Z',
  members: [
    {
      user_id: 'u1',
      email: 'alice@example.com',
      name: 'Alice',
      role: 'owner',
      joined_at: '2026-01-02T00:00:00Z',
    },
  ],
}

function renderDetail() {
  return render(
    <MemoryRouter initialEntries={['/admin/teams/t1']}>
      <Routes>
        <Route path="/admin/teams/:id" element={<AdminTeamDetail />} />
      </Routes>
    </MemoryRouter>
  )
}

afterEach(() => {
  jest.clearAllMocks()
})

it('renders the team, owner, and members', async () => {
  mockAdminService.getTeam.mockResolvedValue(team)
  renderDetail()

  expect(
    await screen.findByRole('heading', { name: 'Engineering' })
  ).toBeInTheDocument()
  expect(screen.getByText('owner@example.com')).toBeInTheDocument()
  expect(screen.getByText('alice@example.com')).toBeInTheDocument()
  expect(mockAdminService.getTeam).toHaveBeenCalledWith('t1')
})

it('shows an error state when the team is not found', async () => {
  mockAdminService.getTeam.mockRejectedValue(new Error('404'))
  renderDetail()

  expect(await screen.findByText('Failed to load team')).toBeInTheDocument()
})
