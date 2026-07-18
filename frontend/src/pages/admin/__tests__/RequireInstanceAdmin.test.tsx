/**
 * RequireInstanceAdmin guard (issue #315): renders children only for an instance
 * admin, redirects everyone else to `/`, and — critically — fails closed while
 * `/auth/me` is still loading (never flashes admin content).
 */
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import { RequireInstanceAdmin } from '@/pages/admin/RequireInstanceAdmin'

const mockUseAuth = jest.fn()
jest.mock('@/contexts/useAuth', () => ({
  useAuth: () => mockUseAuth(),
}))

function renderGuard() {
  return render(
    <MemoryRouter initialEntries={['/admin']}>
      <Routes>
        <Route path="/" element={<div>home page</div>} />
        <Route
          path="/admin"
          element={
            <RequireInstanceAdmin>
              <div>admin content</div>
            </RequireInstanceAdmin>
          }
        />
      </Routes>
    </MemoryRouter>
  )
}

afterEach(() => {
  jest.clearAllMocks()
})

it('renders children for an instance admin', () => {
  mockUseAuth.mockReturnValue({
    user: { id: 'u1', is_instance_admin: true },
    isLoading: false,
  })
  renderGuard()

  expect(screen.getByText('admin content')).toBeInTheDocument()
  expect(screen.queryByText('home page')).not.toBeInTheDocument()
})

it('redirects a non-admin to /', () => {
  mockUseAuth.mockReturnValue({
    user: { id: 'u1', is_instance_admin: false },
    isLoading: false,
  })
  renderGuard()

  expect(screen.getByText('home page')).toBeInTheDocument()
  expect(screen.queryByText('admin content')).not.toBeInTheDocument()
})

it('redirects a logged-out user (null user) to /', () => {
  mockUseAuth.mockReturnValue({ user: null, isLoading: false })
  renderGuard()

  expect(screen.getByText('home page')).toBeInTheDocument()
  expect(screen.queryByText('admin content')).not.toBeInTheDocument()
})

it('fails closed while /auth/me is loading', () => {
  // Even an admin flag must not render children before loading resolves.
  mockUseAuth.mockReturnValue({
    user: { id: 'u1', is_instance_admin: true },
    isLoading: true,
  })
  renderGuard()

  expect(screen.queryByText('admin content')).not.toBeInTheDocument()
  expect(screen.queryByText('home page')).not.toBeInTheDocument()
  expect(screen.getByText('Loading…')).toBeInTheDocument()
})
