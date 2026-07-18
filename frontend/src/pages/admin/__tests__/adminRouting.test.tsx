/**
 * Routing smoke test for the /admin subtree (issue #315): mounts the real guard
 * + AdminLayout + nested stub pages exactly as registered in routes.tsx, and
 * checks direct-URL entry for both an admin and a non-admin.
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

afterEach(() => {
  jest.clearAllMocks()
})

it('renders the admin shell + Stats at /admin for an admin', () => {
  mockUseAuth.mockReturnValue({
    user: { id: 'u1', is_instance_admin: true },
    isLoading: false,
  })
  renderAt('/admin')

  expect(
    screen.getByRole('heading', { name: 'Admin Portal' })
  ).toBeInTheDocument()
  expect(
    screen.getByText('Instance statistics — coming soon.')
  ).toBeInTheDocument()
})

it('renders the Users page at /admin/users for an admin', () => {
  mockUseAuth.mockReturnValue({
    user: { id: 'u1', is_instance_admin: true },
    isLoading: false,
  })
  renderAt('/admin/users')

  expect(screen.getByText('Users — coming soon.')).toBeInTheDocument()
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
