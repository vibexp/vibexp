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
import type { Mocked } from 'vitest'

import { ThemeProvider } from '@/lib/theme'
import { AdminRoutes } from '@/pages/admin/AdminRoutes'
import { AdminShell } from '@/pages/admin/AdminShell'
import { RequireInstanceAdmin } from '@/pages/admin/RequireInstanceAdmin'
import type {
  AdminDashboardOverview,
  AdminProjectDetail,
  AdminProjectListResponse,
  AdminStatsResponse,
  AdminTeamDetail,
  AdminTeamListResponse,
  AdminUserDetail,
  AdminUserListResponse,
} from '@/services/adminService'

const mockUseAuth = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => mockUseAuth(),
}))

vi.mock('@/services/adminService', () => ({
  adminService: {
    getStats: vi.fn(),
    getDashboardOverview: vi.fn(),
    getDashboardTimeseries: vi.fn(),
    listUsers: vi.fn(),
    listTeams: vi.fn(),
    getUser: vi.fn(),
    getTeam: vi.fn(),
    listProjects: vi.fn(),
    getProject: vi.fn(),
  },
}))

vi.mock('@/services/teamService', () => ({
  teamService: { getTeams: vi.fn() },
}))

vi.mock('@/services/projectService', () => ({
  projectService: { getProjects: vi.fn() },
}))

import { adminService } from '@/services/adminService'
import { projectService } from '@/services/projectService'
import { teamService } from '@/services/teamService'

const mockAdminService = adminService as Mocked<typeof adminService>

const stats: AdminStatsResponse = {
  counts: { users: 42, teams: 7, prompts: 3, artifacts: 5, memories: 9 },
  version: '1.2.3',
}

const overview: AdminDashboardOverview = {
  counts: {
    users: 42,
    teams: 7,
    projects: 30,
    prompts: 3,
    artifacts: 5,
    memories: 9,
    blueprints: 2,
    agents: 1,
    feeds: 4,
    api_keys: 6,
  },
  breakdowns: [],
  system_health: { database_size_bytes: 1024, tables: [] },
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
  status: 'active',
  created_at: '2026-01-01T00:00:00Z',
  memberships: [],
}

const emptyProjects: AdminProjectListResponse = {
  projects: [],
  total_count: 0,
  page: 1,
  per_page: 20,
  total_pages: 0,
}

const projectDetail: AdminProjectDetail = {
  id: 'p1',
  name: 'Platform',
  slug: 'platform',
  description: '',
  git_url: '',
  homepage: '',
  team: { id: 't1', name: 'Engineering', slug: 'engineering' },
  owner: { id: 'u1', email: 'creator@example.com', name: 'Creator' },
  resource_counts: { prompts: 1, artifacts: 0, memories: 0, blueprints: 0 },
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
}

const teamDetail: AdminTeamDetail = {
  id: 't1',
  name: 'Engineering',
  slug: 'engineering',
  is_personal: false,
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
    logout: vi.fn(),
  })
}

const adminNav = () =>
  screen.queryByRole('navigation', { name: 'Admin sections' })

beforeEach(() => {
  vi.clearAllMocks()
  mockAdminService.getStats.mockResolvedValue(stats)
  mockAdminService.getDashboardOverview.mockResolvedValue(overview)
  mockAdminService.getDashboardTimeseries.mockResolvedValue({
    from: '2026-06-25T00:00:00Z',
    to: '2026-07-25T00:00:00Z',
    granularity: 'day',
    growth: [],
    sign_ins: [],
    access_by_source: [],
    data_window: {
      sign_ins_earliest_retained_at: '2026-04-26T00:00:00Z',
      access_by_source_earliest_retained_at: '2026-04-26T00:00:00Z',
    },
  })
  mockAdminService.listUsers.mockResolvedValue(emptyUsers)
  mockAdminService.listTeams.mockResolvedValue(emptyTeams)
  mockAdminService.getUser.mockResolvedValue(userDetail)
  mockAdminService.getTeam.mockResolvedValue(teamDetail)
  mockAdminService.listProjects.mockResolvedValue(emptyProjects)
  mockAdminService.getProject.mockResolvedValue(projectDetail)
})

it('renders the Dashboard at /admin inside the admin shell', async () => {
  asAdmin()
  renderAt('/admin')

  expect(
    screen.getByRole('heading', { name: 'Dashboard', level: 1 })
  ).toBeInTheDocument()
  expect(adminNav()).toBeInTheDocument()
  // The dashboard loaded: a totals card and the backend version resolved.
  expect(await screen.findByText('42')).toBeInTheDocument()
  expect(screen.getByText(/Backend version 1\.2\.3/)).toBeInTheDocument()
})

it.each([
  ['/admin/users', 'Users'],
  ['/admin/teams', 'Teams'],
  // #456 added the Projects nav entry, which rendered the in-shell 404 until
  // #461 added the route it points at.
  ['/admin/projects', 'Projects'],
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
  ['/admin/projects/p1', 'Platform'],
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
    logout: vi.fn(),
  })
  renderAt('/admin')

  expect(screen.getByText('app shell')).toBeInTheDocument()
  expect(adminNav()).not.toBeInTheDocument()
})

it('shows no admin content while the identity is still resolving', () => {
  mockUseAuth.mockReturnValue({
    user: null,
    isLoading: true,
    logout: vi.fn(),
  })
  renderAt('/admin')

  expect(adminNav()).not.toBeInTheDocument()
  expect(mockAdminService.getDashboardOverview).not.toHaveBeenCalled()
})
