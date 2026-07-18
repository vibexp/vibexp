/**
 * AdminUserDetail (#316): renders the user profile + team memberships, and the
 * error state (e.g. a 404 for an unknown id).
 */
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { AdminUserDetail as AdminUserDetailType } from '@/services/adminService'

jest.mock('@/services/adminService', () => ({
  adminService: { getUser: jest.fn() },
}))

import { adminService } from '@/services/adminService'

import { AdminUserDetail } from '../AdminUserDetail'

const mockAdminService = adminService as jest.Mocked<typeof adminService>

const user: AdminUserDetailType = {
  id: 'u1',
  email: 'alice@example.com',
  name: 'Alice',
  idp_provider: 'google',
  created_at: '2026-01-01T00:00:00Z',
  memberships: [
    { team_id: 't1', team_name: 'Engineering', role: 'owner' },
    { team_id: 't2', team_name: 'Design', role: 'member' },
  ],
}

function renderDetail() {
  return render(
    <MemoryRouter initialEntries={['/admin/users/u1']}>
      <Routes>
        <Route path="/admin/users/:id" element={<AdminUserDetail />} />
      </Routes>
    </MemoryRouter>
  )
}

afterEach(() => {
  jest.clearAllMocks()
})

it('renders the user and their memberships', async () => {
  mockAdminService.getUser.mockResolvedValue(user)
  renderDetail()

  expect(await screen.findByText('Engineering')).toBeInTheDocument()
  expect(screen.getByText('Design')).toBeInTheDocument()
  expect(screen.getByText('owner')).toBeInTheDocument()
  expect(mockAdminService.getUser).toHaveBeenCalledWith('u1')
})

it('shows an error state when the user is not found', async () => {
  mockAdminService.getUser.mockRejectedValue(new Error('404'))
  renderDetail()

  expect(await screen.findByText('Failed to load user')).toBeInTheDocument()
})
