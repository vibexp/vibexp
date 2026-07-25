/**
 * Routing test for the decoupled `/admin` branch (#456).
 *
 * Mounts the same tree `App.tsx` mounts — `RequireInstanceAdmin` → `AdminShell`
 * → `AdminRoutes` under a `/admin/*` route that is a **sibling** of the app
 * catch-all, with **no** `TeamProvider`/`ProjectProvider`. `useTeam`/`useProject`
 * throw outside their providers, so any page or shell component that reached for
 * team context would fail this render rather than production.
 *
 * `adminService` is mocked so the pages render deterministic content;
 * `teamService`/`projectService` are mocked only to assert they are never called.
 */
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import { ThemeProvider } from '@/lib/theme'
import { AdminRoutes } from '@/pages/admin/AdminRoutes'
import { AdminShell } from '@/pages/admin/AdminShell'
import { RequireInstanceAdmin } from '@/pages/admin/RequireInstanceAdmin'
import type {
  AdminStatsResponse,
  AdminTeamDetail,
  AdminTeamListResponse,
  AdminUserDetail,
  AdminUserListResponse,
} from '@/services/adminService'

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

jest.mock('@/services/teamService', () => ({
  teamService: { getTeams: jest.fn() },
}))

jest.mock('@/services/projectService', () => ({
  projectService: { getProjects: jest.fn() },
}))

import { adminService } from '@/services/adminService'
import { projectService } from '@/services/projectService'
import { teamService } from '@/services/teamService'

const mockAdminService = adminService as jest.Mocked<typeof adminService>

const stats: AdminStatsResponse = {
  counts: { users: 42, teams: 7, prompts: 3, artifacts: 5, memories: 9 },
  version: '1.2.3',
}

const emptyUsers: AdminUserListResponse = {
  users: [],
  total_count: 0,
  page: 1,
  per_page: 20,
  total_pages: 0,
}

const emptyTeams: AdminTeamListResponse = {
  teams: [],
  total_count: 0,
  page: 1,
  per_page: 20,
  total_pages: 0,
}

const userDetail: AdminUserDetail = {
  id: 'u1',
  email: 'alice@example.com',
  name: 'Alice',
  idp_provider: 'google',
  created_at: '2026-01-01T00:00:00Z',
  memberships: [],
}

const teamDetail: AdminTeamDetail = {
  id: 't1',
  name: 'Engineering',
  owner: { id: 'o1', email: 'owner@example.com', name: 'Owner' },
  created_at: '2026-01-01T00:00:00Z',
  members: [],
}

/**
 * Mirrors `App.tsx`: `/admin/*` first, then the catch-all standing in for
 * `MainApp`. If the mount point or its `/*` suffix regressed, these paths would
 * render "app shell" instead.
 */
function renderAt(path: string) {
  return render(
    <ThemeProvider defaultTheme="light">
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route
            path="/admin/*"
            element={
              <RequireInstanceAdmin>
                <AdminShell>
                  <AdminRoutes />
                </AdminShell>
              </RequireInstanceAdmin>
            }
          />
          <Route path="/*" element={<div>app shell</div>} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  )
}

function asAdmin() {
  mockUseAuth.mockReturnValue({
    user: { id: 'u1', email: 'admin@example.com', is_instance_admin: true },
    isLoading: false,
    logout: jest.fn(),
  })
}

const adminNav = () =>
  screen.queryByRole('navigation', { name: 'Admin sections' })

beforeEach(() => {
  jest.clearAllMocks()
  mockAdminService.getStats.mockResolvedValue(stats)
  mockAdminService.listUsers.mockResolvedValue(emptyUsers)
  mockAdminService.listTeams.mockResolvedValue(emptyTeams)
  mockAdminService.getUser.mockResolvedValue(userDetail)
  mockAdminService.getTeam.mockResolvedValue(teamDetail)
})

it('renders the Dashboard at /admin inside the admin shell', async () => {
  asAdmin()
  renderAt('/admin')

  expect(
    screen.getByRole('heading', { name: 'Dashboard', level: 1 })
  ).toBeInTheDocument()
  expect(adminNav()).toBeInTheDocument()
  // The index page loaded: its version stat resolved.
  expect(await screen.findByText('1.2.3')).toBeInTheDocument()
})

it.each([
  ['/admin/users', 'Users'],
  ['/admin/teams', 'Teams'],
])('renders %s inside the admin shell', async (path, heading) => {
  asAdmin()
  renderAt(path)

  expect(
    await screen.findByRole('heading', { name: heading, level: 1 })
  ).toBeInTheDocument()
  expect(screen.queryByText('app shell')).not.toBeInTheDocument()
})

it.each([
  ['/admin/users/u1', 'Alice'],
  ['/admin/teams/t1', 'Engineering'],
])(
  'routes %s to its detail page, not to the app catch-all',
  async (path, shown) => {
    asAdmin()
    renderAt(path)

    expect(await screen.findByText(shown)).toBeInTheDocument()
    expect(screen.queryByText('app shell')).not.toBeInTheDocument()
    // A detail page titles itself with the record, so the shell adds no section
    // heading — otherwise every detail page would be headed "Users"/"Teams".
    expect(
      screen.queryByRole('heading', { name: 'Users', level: 1 })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('heading', { name: 'Teams', level: 1 })
    ).not.toBeInTheDocument()
  }
)

it('renders an in-shell not-found for an unknown /admin path', () => {
  asAdmin()
  renderAt('/admin/nope')

  expect(
    screen.getByRole('heading', { name: 'Page not found' })
  ).toBeInTheDocument()
  // Still inside the shell rather than bounced to the product 404.
  expect(adminNav()).toBeInTheDocument()
  expect(screen.queryByText('app shell')).not.toBeInTheDocument()
})

it('needs no team or project context under /admin', async () => {
  asAdmin()
  renderAt('/admin/users')

  expect(await screen.findByText('No users yet')).toBeInTheDocument()
  expect(teamService.getTeams).not.toHaveBeenCalled()
  expect(projectService.getProjects).not.toHaveBeenCalled()
})

it('blocks a non-admin entering /admin directly by URL', () => {
  mockUseAuth.mockReturnValue({
    user: { id: 'u1', is_instance_admin: false },
    isLoading: false,
    logout: jest.fn(),
  })
  renderAt('/admin')

  expect(screen.getByText('app shell')).toBeInTheDocument()
  expect(adminNav()).not.toBeInTheDocument()
})

it('shows no admin content while the identity is still resolving', () => {
  mockUseAuth.mockReturnValue({
    user: null,
    isLoading: true,
    logout: jest.fn(),
  })
  renderAt('/admin')

  expect(adminNav()).not.toBeInTheDocument()
  expect(mockAdminService.getStats).not.toHaveBeenCalled()
})
