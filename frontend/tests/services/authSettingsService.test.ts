/**
 * AuthSettingsService and SetupService — the wire contract of Admin →
 * Settings → Authentication and the setup page (#1239): exactly what reaches
 * `generatedClient`.
 */

// Mock the generated client; `unwrap` stays real so these exercise the same
// success/error resolution production uses.
const mockGeneratedClient = vi.hoisted(() => ({
  GET: vi.fn(),
  POST: vi.fn(),
  PUT: vi.fn(),
  DELETE: vi.fn(),
}))

vi.mock('../../src/lib/apiClientGenerated', async () => {
  const actual = await vi.importActual<
    typeof import('../../src/lib/apiClientGenerated')
  >('../../src/lib/apiClientGenerated')
  return { ...actual, generatedClient: mockGeneratedClient }
})

import { authSettingsService } from '../../src/services/authSettingsService'
import { setupService } from '../../src/services/setupService'
import { ApiError } from '../../src/types/errors'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const noContent = {
  ok: true,
  status: 204,
  statusText: 'No Content',
} as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })
const empty = () => Promise.resolve({ data: undefined, response: noContent })

const PROVIDERS = '/api/v1/admin/settings/auth/providers'
const ALLOWLIST = '/api/v1/admin/settings/auth/allowlist'
const ADMINS = '/api/v1/admin/settings/auth/admins'
const ID = '11111111-1111-4111-8111-111111111111'

beforeEach(() => {
  vi.clearAllMocks()
})

describe('authSettingsService: providers', () => {
  it('lists the providers', async () => {
    const list = { providers: [], version: 7 }
    mockGeneratedClient.GET.mockReturnValue(success(list))
    await expect(authSettingsService.listProviders()).resolves.toEqual(list)
    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(PROVIDERS)
  })

  it('creates a provider', async () => {
    const body = { slug: 'okta' } as never
    mockGeneratedClient.POST.mockReturnValue(success({ version: 8 }))
    await expect(authSettingsService.createProvider(body)).resolves.toEqual({
      version: 8,
    })
    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(PROVIDERS, { body })
  })

  it('updates a provider by id', async () => {
    const body = { enabled: false, expected_version: 7 } as never
    mockGeneratedClient.PUT.mockReturnValue(success({ version: 8 }))
    await authSettingsService.updateProvider(ID, body)
    expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(`${PROVIDERS}/{id}`, {
      params: { path: { id: ID } },
      body,
    })
  })

  it('deletes a provider with the version and the lockout confirmation as query', async () => {
    mockGeneratedClient.DELETE.mockReturnValue(empty())
    await expect(
      authSettingsService.deleteProvider(ID, {
        expected_version: 7,
        confirm_lockout_risk: true,
      })
    ).resolves.toBeUndefined()
    expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
      `${PROVIDERS}/{id}`,
      {
        params: {
          path: { id: ID },
          query: { expected_version: 7, confirm_lockout_risk: true },
        },
      }
    )
  })

  it('tests a provider', async () => {
    const result = { is_valid: true, message: null }
    mockGeneratedClient.POST.mockReturnValue(success(result))
    await expect(authSettingsService.testProvider({ id: ID })).resolves.toEqual(
      result
    )
    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(`${PROVIDERS}/test`, {
      body: { id: ID },
    })
  })

  it('surfaces a 409 with its metadata, which the lockout confirm reads', async () => {
    const problem = {
      type: 'about:blank',
      title: 'Lockout Risk',
      status: 409,
      detail: 'this change leaves no enabled sign-in provider',
      code: 'lockout_risk',
      request_id: 'req-1',
      timestamp: '2026-10-10T00:00:00Z',
      metadata: { reason: 'no_enabled_provider' },
    }
    mockGeneratedClient.PUT.mockReturnValue(
      Promise.resolve({
        error: problem,
        response: { ok: false, status: 409, statusText: 'Conflict' },
      })
    )
    const failure = await authSettingsService
      .updateProvider(ID, {} as never)
      .catch((err: unknown) => err)
    expect(failure).toBeInstanceOf(ApiError)
    expect((failure as ApiError).code).toBe('lockout_risk')
    expect((failure as ApiError).metadata).toEqual({
      reason: 'no_enabled_provider',
    })
  })
})

describe('authSettingsService: allowlist', () => {
  it('reads the allowlist', async () => {
    mockGeneratedClient.GET.mockReturnValue(success({ domains: [] }))
    await authSettingsService.getAllowlist()
    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(ALLOWLIST)
  })

  it('replaces the allowlist', async () => {
    const body = { domains: ['a.io'], emails: [], expected_version: 0 }
    mockGeneratedClient.PUT.mockReturnValue(success({ version: 1 }))
    await authSettingsService.updateAllowlist(body)
    expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(ALLOWLIST, { body })
  })

  it('resets the allowlist', async () => {
    mockGeneratedClient.DELETE.mockReturnValue(empty())
    await expect(authSettingsService.resetAllowlist()).resolves.toBeUndefined()
    expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(ALLOWLIST)
  })

  it('previews a candidate', async () => {
    const body = { domains: ['a.io'], emails: [] }
    const impact = { count: 2, sample: ['x@b.io'], sample_truncated: true }
    mockGeneratedClient.POST.mockReturnValue(success(impact))
    await expect(authSettingsService.previewAllowlist(body)).resolves.toEqual(
      impact
    )
    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      `${ALLOWLIST}/preview`,
      { body }
    )
  })
})

describe('authSettingsService: admins and audit', () => {
  it('lists the admins', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({ root_admins: [], admins: [] })
    )
    await authSettingsService.listAdmins()
    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(ADMINS)
  })

  it('grants by email', async () => {
    mockGeneratedClient.POST.mockReturnValue(success({ user_id: ID }))
    await authSettingsService.grantAdmin({ email: 'ada@example.com' })
    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(ADMINS, {
      body: { email: 'ada@example.com' },
    })
  })

  it('revokes by user id', async () => {
    mockGeneratedClient.DELETE.mockReturnValue(empty())
    await authSettingsService.revokeAdmin(ID)
    expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
      `${ADMINS}/{user_id}`,
      { params: { path: { user_id: ID } } }
    )
  })

  it("lists one setting's audit page", async () => {
    const page = { entries: [], next_cursor: null }
    mockGeneratedClient.GET.mockReturnValue(success(page))
    await expect(
      authSettingsService.listAudit('auth_allowlist', {
        limit: 20,
        cursor: 'c1',
      })
    ).resolves.toEqual(page)
    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/admin/settings/auth/audit',
      {
        params: {
          query: { limit: 20, cursor: 'c1', setting: 'auth_allowlist' },
        },
      }
    )
  })

  it('defaults the audit paging', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      success({ entries: [], next_cursor: null })
    )
    await authSettingsService.listAudit('auth_setup')
    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/admin/settings/auth/audit',
      { params: { query: { setting: 'auth_setup' } } }
    )
  })
})

describe('setupService', () => {
  it('reads the setup status', async () => {
    mockGeneratedClient.GET.mockReturnValue(success({ setup_required: true }))
    await expect(setupService.getStatus()).resolves.toEqual({
      setup_required: true,
    })
    expect(mockGeneratedClient.GET).toHaveBeenCalledWith('/api/v1/setup/status')
  })

  it('exchanges the token for a setup session', async () => {
    const session = { expires_at: '2026-10-10T13:00:00Z' }
    mockGeneratedClient.POST.mockReturnValue(success(session))
    await expect(setupService.createSession('tok-123')).resolves.toEqual(
      session
    )
    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/setup/session',
      { body: { token: 'tok-123' } }
    )
  })
})
