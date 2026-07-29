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
  AdminUserDetail,
} from '../../src/services/adminService'

// Mock the generated client; `unwrap` stays real so these exercise the same
// success/error resolution production uses.
const mockGeneratedClient = vi.hoisted(() => ({
  GET: vi.fn(),
  POST: vi.fn(),
  PATCH: vi.fn(),
  DELETE: vi.fn(),
}))

vi.mock('../../src/lib/apiClientGenerated', async () => {
  const actual = await vi.importActual<
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
  vi.clearAllMocks()
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

const activeUser: AdminUserDetail = {
  id: 'u1',
  email: 'ada@example.com',
  name: 'Ada',
  idp_provider: 'google',
  status: 'active',
  created_at: '2026-01-01T00:00:00Z',
  memberships: [],
}

describe('listUsers', () => {
  it('forwards every filter, sort and pagination param', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({
        users: [],
        total_count: 0,
        page: 1,
        per_page: 20,
        total_pages: 0,
      })
    )

    await adminService.listUsers({
      page: 2,
      limit: 20,
      search: 'ada',
      status: 'suspended',
      idp_provider: 'google',
      created_from: '2026-07-01T00:00:00.000Z',
      created_to: '2026-07-24T23:59:59.999Z',
      sort_by: 'team_count',
      sort_order: 'asc',
    })

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/admin/users',
      {
        params: {
          query: {
            page: 2,
            limit: 20,
            search: 'ada',
            status: 'suspended',
            idp_provider: 'google',
            created_from: '2026-07-01T00:00:00.000Z',
            created_to: '2026-07-24T23:59:59.999Z',
            sort_by: 'team_count',
            sort_order: 'asc',
          },
        },
      }
    )
  })
})

describe('the user mutations', () => {
  it('creates a user, omitting an unset provider', async () => {
    mockGeneratedClient.POST.mockReturnValue(success(activeUser))

    await adminService.createUser({ email: 'ada@example.com', name: 'Ada' })

    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/admin/users',
      { body: { email: 'ada@example.com', name: 'Ada' } }
    )
  })

  it('updates the name via PATCH', async () => {
    mockGeneratedClient.PATCH.mockReturnValue(
      success({ ...activeUser, name: 'Ada L' })
    )

    const result = await adminService.updateUser('u1', { name: 'Ada L' })

    expect(mockGeneratedClient.PATCH).toHaveBeenCalledWith(
      '/api/v1/admin/users/{id}',
      { params: { path: { id: 'u1' } }, body: { name: 'Ada L' } }
    )
    expect(result.name).toBe('Ada L')
  })

  it('suspends and reactivates through their own endpoints', async () => {
    mockGeneratedClient.POST.mockReturnValue(
      success({ ...activeUser, status: 'suspended' })
    )
    const suspended = await adminService.suspendUser('u1')
    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/admin/users/{id}/suspend',
      { params: { path: { id: 'u1' } } }
    )
    expect(suspended.status).toBe('suspended')

    mockGeneratedClient.POST.mockReturnValue(success(activeUser))
    await adminService.reactivateUser('u1')
    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/admin/users/{id}/reactivate',
      { params: { path: { id: 'u1' } } }
    )
  })
})

describe('deleteUser', () => {
  const noContent = {
    ok: true,
    status: 204,
    statusText: 'No Content',
  } as Response

  it('reports a successful delete', async () => {
    mockGeneratedClient.DELETE.mockReturnValue(
      Promise.resolve({ data: undefined, response: noContent })
    )

    await expect(adminService.deleteUser('u1')).resolves.toEqual({
      deleted: true,
    })
    expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
      '/api/v1/admin/users/{id}',
      { params: { path: { id: 'u1' } } }
    )
  })

  it('returns the 409 refusal as data, with its blockers intact', async () => {
    // The whole reason deleteUser bypasses `unwrap`: the 409 body is
    // application/json + AdminUserDeleteBlockedResponse, not problem details, so
    // unwrap's isProblemDetails check fails and it would collapse the response to
    // a generic ApiError — discarding `blockers`, the one thing the dialog needs.
    const refusal = {
      message: 'This user owns shared teams with other members.',
      blockers: [
        { team_id: 't1', team_name: 'Acme Engineering', member_count: 4 },
      ],
    }
    mockGeneratedClient.DELETE.mockReturnValue(
      Promise.resolve({
        error: refusal,
        response: {
          ok: false,
          status: 409,
          statusText: 'Conflict',
        } as Response,
      })
    )

    const result = await adminService.deleteUser('u1')

    expect(result).toEqual({ deleted: false, refusal })
    if (!('refusal' in result)) throw new Error('expected a refusal')
    expect(result.refusal.blockers[0].team_name).toBe('Acme Engineering')
    expect(result.refusal.blockers[0].member_count).toBe(4)
  })

  it('reports a refusal with no blockers, which is how self-targeting arrives', async () => {
    // The same 409 covers "you cannot delete yourself / a config admin", where
    // blockers is empty. An empty list must not read as "nothing is blocking it".
    mockGeneratedClient.DELETE.mockReturnValue(
      Promise.resolve({
        error: { message: 'You cannot delete your own account.', blockers: [] },
        response: {
          ok: false,
          status: 409,
          statusText: 'Conflict',
        } as Response,
      })
    )

    const result = await adminService.deleteUser('u1')

    expect(result.deleted).toBe(false)
  })

  it('throws for a 409 whose body is not the documented shape', async () => {
    // Failing loudly beats reporting a refusal the response never described.
    mockGeneratedClient.DELETE.mockReturnValue(
      Promise.resolve({
        error: { detail: 'nope' },
        response: {
          ok: false,
          status: 409,
          statusText: 'Conflict',
        } as Response,
      })
    )

    await expect(adminService.deleteUser('u1')).rejects.toThrow()
  })

  it('throws for a non-409 failure, like any other call', async () => {
    mockGeneratedClient.DELETE.mockReturnValue(
      Promise.resolve({
        error: {
          type: 'about:blank',
          title: 'Not Found',
          status: 404,
          detail: 'user not found',
          code: 'NOT_FOUND',
          request_id: 'r1',
          timestamp: '2026-07-25T00:00:00Z',
        },
        response: {
          ok: false,
          status: 404,
          statusText: 'Not Found',
        } as Response,
      })
    )

    await expect(adminService.deleteUser('missing')).rejects.toThrow(
      'user not found'
    )
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

    await adminService.listUsers({ page: 3, limit: 20 })

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
