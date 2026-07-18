/**
 * Routing smoke test for the /admin subtree (#315 shell, real pages from #316):
 * mounts the real guard + AdminLayout + nested pages as registered in routes.tsx
 * and checks direct-URL entry for both an admin and a non-admin. adminService is
 * mocked so the pages render deterministic content.
 */
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import { AdminLayout } from '@/pages/admin/AdminLayout'
import { AdminStats } from '@/pages/admin/AdminStats'
import { AdminTeamDetail } from '@/pages/admin/AdminTeamDetail'
import { AdminTeams } from '@/pages/admin/AdminTeams'
import { AdminUserDetail } from '@/pages/admin/AdminUserDetail'
import { AdminUsers } from '@/pages/admin/AdminUsers'
import { RequireInstanceAdmin } from '@/pages/admin/RequireInstanceAdmin'

const mockUseAuth = jest.fn()
jest.mock('@/contexts/useAuth', () => ({
  useAuth: () => mockUseAuth(),
}))

jest.mock('@/services/adminService', () => ({
  adminService: {
    getStats: jest.fn(),
    listUsers: jest.fn(),
    listTeams: jest.fn(),
    getUser: jest.fn(),
    getTeam: jest.fn(),
  },
}))

import { adminService } from '@/services/adminService'

const mockAdminService = adminService as jest.Mocked<typeof adminService>

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/" element={<div>home page</div>} />
        <Route
          path="admin"
          element={
            <RequireInstanceAdmin>
              <AdminLayout />
            </RequireInstanceAdmin>
          }
        >
          <Route index element={<AdminStats />} />
          <Route path="users" element={<AdminUsers />} />
          <Route path="users/:id" element={<AdminUserDetail />} />
          <Route path="teams" element={<AdminTeams />} />
          <Route path="teams/:id" element={<AdminTeamDetail />} />
        </Route>
      </Routes>
    </MemoryRouter>
  )
}

beforeEach(() => {
  jest.clearAllMocks()
  mockAdminService.getStats.mockResolvedValue({
    counts: { users: 1, teams: 1, prompts: 0, artifacts: 0, memories: 0 },
    version: 'test',
  })
  mockAdminService.listUsers.mockResolvedValue({
    users: [],
    total_count: 0,
    page: 1,
    per_page: 20,
    total_pages: 0,
  })
})

it('renders the admin shell + Stats at /admin for an admin', async () => {
  mockUseAuth.mockReturnValue({
    user: { id: 'u1', is_instance_admin: true },
    isLoading: false,
  })
  renderAt('/admin')

  expect(
    screen.getByRole('heading', { name: 'Admin Portal' })
  ).toBeInTheDocument()
  // Stats index page loads and renders the version stat card.
  expect(await screen.findByText('test')).toBeInTheDocument()
})

it('renders the Users page at /admin/users for an admin', async () => {
  mockUseAuth.mockReturnValue({
    user: { id: 'u1', is_instance_admin: true },
    isLoading: false,
  })
  renderAt('/admin/users')

  expect(await screen.findByText('No users yet')).toBeInTheDocument()
})

it('blocks a non-admin entering /admin directly by URL', () => {
  mockUseAuth.mockReturnValue({
    user: { id: 'u1', is_instance_admin: false },
    isLoading: false,
  })
  renderAt('/admin')

  expect(screen.getByText('home page')).toBeInTheDocument()
  expect(
    screen.queryByRole('heading', { name: 'Admin Portal' })
  ).not.toBeInTheDocument()
})
