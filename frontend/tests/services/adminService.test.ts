/**
 * AdminService — the wire contract for the instance-admin pages.
 *
 * There was no test for this service; #460 changes `listTeams` from
 * `(page, limit)` to a full filter object, and a param dropped on the way to the
 * client is invisible in a page test that mocks the service. These assertions are
 * about exactly what reaches `generatedClient`.
 */
import type {
  AdminProjectDetail,
  AdminProjectListResponse,
  AdminTeamListResponse,
} from '../../src/services/adminService'

// Mock the generated client; `unwrap` stays real so these exercise the same
// success/error resolution production uses.
const mockGeneratedClient = {
  GET: jest.fn(),
  POST: jest.fn(),
  PATCH: jest.fn(),
  DELETE: jest.fn(),
}

jest.mock('../../src/lib/apiClientGenerated', () => {
  const actual = jest.requireActual<
    typeof import('../../src/lib/apiClientGenerated')
  >('../../src/lib/apiClientGenerated')
  return {
    ...actual,
    generatedClient: mockGeneratedClient,
  }
})

import { adminService } from '../../src/services/adminService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

const emptyTeamPage: AdminTeamListResponse = {
  teams: [],
  total_count: 0,
  page: 1,
  per_page: 20,
  total_pages: 0,
}

beforeEach(() => {
  jest.clearAllMocks()
})

describe('listTeams', () => {
  it('forwards every filter, sort and pagination param to the query', async () => {
    mockGeneratedClient.GET.mockReturnValue(success(emptyTeamPage))

    await adminService.listTeams({
      page: 2,
      limit: 20,
      search: 'eng',
      is_personal: false,
      created_from: '2026-07-01T00:00:00.000Z',
      created_to: '2026-07-24T23:59:59.999Z',
      sort_by: 'member_count',
      sort_order: 'asc',
    })

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/admin/teams',
      {
        params: {
          query: {
            page: 2,
            limit: 20,
            search: 'eng',
            is_personal: false,
            created_from: '2026-07-01T00:00:00.000Z',
            created_to: '2026-07-24T23:59:59.999Z',
            sort_by: 'member_count',
            sort_order: 'asc',
          },
        },
      }
    )
  })

  it('passes is_personal: false through rather than dropping it as falsy', async () => {
    mockGeneratedClient.GET.mockReturnValue(success(emptyTeamPage))

    await adminService.listTeams({ page: 1, limit: 20, is_personal: false })

    // `false` means "shared only" — a truthiness check anywhere on this path
    // would turn that into "all teams" without any visible symptom.
    const [, options] = mockGeneratedClient.GET.mock.calls[0] as [
      string,
      { params: { query: Record<string, unknown> } },
    ]
    expect(options.params.query).toHaveProperty('is_personal', false)
  })

  it('returns the envelope untouched', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({ ...emptyTeamPage, total_count: 7, total_pages: 1 })
    )

    const result = await adminService.listTeams({ page: 1, limit: 20 })

    expect(result.total_count).toBe(7)
    expect(result.total_pages).toBe(1)
  })
})

const emptyProjectPage: AdminProjectListResponse = {
  projects: [],
  total_count: 0,
  page: 1,
  per_page: 20,
  total_pages: 0,
}

describe('listProjects', () => {
  it('forwards every filter, sort and pagination param to the query', async () => {
    mockGeneratedClient.GET.mockReturnValue(success(emptyProjectPage))

    await adminService.listProjects({
      page: 2,
      limit: 20,
      search: 'plat',
      team_id: 'team-1',
      created_from: '2026-07-01T00:00:00.000Z',
      created_to: '2026-07-24T23:59:59.999Z',
      sort_by: 'name',
      sort_order: 'asc',
    })

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/admin/projects',
      {
        params: {
          query: {
            page: 2,
            limit: 20,
            search: 'plat',
            team_id: 'team-1',
            created_from: '2026-07-01T00:00:00.000Z',
            created_to: '2026-07-24T23:59:59.999Z',
            sort_by: 'name',
            sort_order: 'asc',
          },
        },
      }
    )
  })

  it('returns the envelope untouched', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({ ...emptyProjectPage, total_count: 30, total_pages: 2 })
    )

    const result = await adminService.listProjects({ page: 1, limit: 20 })

    expect(result.total_count).toBe(30)
    expect(result.total_pages).toBe(2)
  })
})

describe('getProject', () => {
  it('sends the id as a path param and returns the counts', async () => {
    const project: AdminProjectDetail = {
      id: 'p1',
      name: 'Platform',
      slug: 'platform',
      description: '',
      git_url: '',
      homepage: '',
      team: { id: 't1', name: 'Engineering', slug: 'engineering' },
      owner: { id: 'u1', email: 'creator@example.com', name: 'Creator' },
      resource_counts: {
        prompts: 12,
        artifacts: 4,
        memories: 27,
        blueprints: 3,
      },
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-02T00:00:00Z',
    }
    mockGeneratedClient.GET.mockReturnValue(success(project))

    const result = await adminService.getProject('p1')

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/admin/projects/{id}',
      { params: { path: { id: 'p1' } } }
    )
    // Distinct values so a transposed mapping cannot pass.
    expect(result.resource_counts).toEqual({
      prompts: 12,
      artifacts: 4,
      memories: 27,
      blueprints: 3,
    })
  })
})

describe('the other admin reads', () => {
  it('gets instance stats', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({
        counts: { users: 1, teams: 1, prompts: 0, artifacts: 0, memories: 0 },
        version: 'test',
      })
    )

    const stats = await adminService.getStats()

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/admin/stats',
      {}
    )
    expect(stats.version).toBe('test')
  })

  it('lists users by page', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({
        users: [],
        total_count: 0,
        page: 1,
        per_page: 20,
        total_pages: 0,
      })
    )

    await adminService.listUsers(3, 20)

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/admin/users',
      { params: { query: { page: 3, limit: 20 } } }
    )
  })

  it('gets a team by id as a path param', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({
        id: 't1',
        name: 'Engineering',
        slug: 'engineering',
        is_personal: false,
        owner: { id: 'o1', email: 'owner@example.com', name: 'Owner' },
        created_at: '2026-01-01T00:00:00Z',
        members: [],
      })
    )

    await adminService.getTeam('t1')

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/admin/teams/{id}',
      { params: { path: { id: 't1' } } }
    )
  })

  it('gets a user by id as a path param', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({
        id: 'u1',
        email: 'a@example.com',
        name: 'Ada',
        status: 'active',
        created_at: '2026-01-01T00:00:00Z',
        memberships: [],
      })
    )

    await adminService.getUser('u1')

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/admin/users/{id}',
      { params: { path: { id: 'u1' } } }
    )
  })
})
