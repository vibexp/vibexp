/**
 * AdminSettingsService — the wire contract of the instance search and AI
 * summary settings pages (#1202): exactly what reaches `generatedClient`.
 */

// Mock the generated client; `unwrap` stays real so these exercise the same
// success/error resolution production uses.
const mockGeneratedClient = vi.hoisted(() => ({
  GET: vi.fn(),
  PUT: vi.fn(),
  DELETE: vi.fn(),
}))

vi.mock('../../src/lib/apiClientGenerated', async () => {
  const actual = await vi.importActual<
    typeof import('../../src/lib/apiClientGenerated')
  >('../../src/lib/apiClientGenerated')
  return { ...actual, generatedClient: mockGeneratedClient }
})

import { adminSettingsService } from '../../src/services/adminSettingsService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

beforeEach(() => {
  vi.clearAllMocks()
})

describe('instance search + AI summary settings (#1202)', () => {
  const noContent = {
    ok: true,
    status: 204,
    statusText: 'No Content',
  } as Response

  describe.each([
    {
      section: 'search',
      path: '/api/v1/admin/settings/search',
      get: () => adminSettingsService.getSearchSettings(),
      update: (body: never) => adminSettingsService.updateSearchSettings(body),
      reset: () => adminSettingsService.resetSearchSettings(),
      audit: (params: { cursor?: string; limit?: number }) =>
        adminSettingsService.listSearchSettingsAudit(params),
    },
    {
      section: 'ai-summary',
      path: '/api/v1/admin/settings/ai-summary',
      get: () => adminSettingsService.getAISummarySettings(),
      update: (body: never) =>
        adminSettingsService.updateAISummarySettings(body),
      reset: () => adminSettingsService.resetAISummarySettings(),
      audit: (params: { cursor?: string; limit?: number }) =>
        adminSettingsService.listAISummarySettingsAudit(params),
    },
  ])('$section', ({ path, get, update, reset, audit }) => {
    it('GETs the settings', async () => {
      mockGeneratedClient.GET.mockReturnValue(success({ source: 'default' }))

      await expect(get()).resolves.toEqual({ source: 'default' })
      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(path)
    })

    it('PUTs the body, expected_version included', async () => {
      const body = { enabled: true, expected_version: 3 }
      mockGeneratedClient.PUT.mockReturnValue(success({ source: 'instance' }))

      await update(body as never)

      expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(path, { body })
    })

    it('surfaces a version conflict as a 409 ApiError', async () => {
      mockGeneratedClient.PUT.mockReturnValue(
        Promise.resolve({
          error: {
            type: 'about:blank',
            title: 'Conflict',
            status: 409,
            detail: 'changed',
            code: 'INSTANCE_SETTINGS_VERSION_CONFLICT',
            request_id: 'r1',
            timestamp: '2026-01-01T00:00:00Z',
          },
          response: {
            ok: false,
            status: 409,
            statusText: 'Conflict',
          } as Response,
        })
      )

      await expect(update({} as never)).rejects.toMatchObject({
        status: 409,
        code: 'INSTANCE_SETTINGS_VERSION_CONFLICT',
      })
    })

    it('DELETEs to reset and resolves on 204', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(
        Promise.resolve({ data: undefined, response: noContent })
      )

      await expect(reset()).resolves.toBeUndefined()
      expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(path)
    })

    it('lists the audit log with the cursor and limit', async () => {
      const page = { entries: [], next_cursor: null }
      mockGeneratedClient.GET.mockReturnValue(success(page))

      await expect(audit({ cursor: 'c1', limit: 20 })).resolves.toEqual(page)
      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(`${path}/audit`, {
        params: { query: { cursor: 'c1', limit: 20 } },
      })
    })
  })
})
