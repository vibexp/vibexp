import { ApiError } from '../../types/errors'
import type { TeamSettingsAuditListResponse } from '../teamSettingsAuditService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = vi.hoisted(() => ({
  GET: vi.fn(),
}))

vi.mock('../../lib/apiClientGenerated', async () => {
  const actual = await vi.importActual<
    typeof import('../../lib/apiClientGenerated')
  >('../../lib/apiClientGenerated')
  return {
    ...actual,
    generatedClient: mockGeneratedClient,
  }
})

import { teamSettingsAuditService } from '../teamSettingsAuditService'

const teamId = 'team-1'
const path = '/api/v1/{team_id}/settings/audit'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response

const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

const problem = (status: number, detail: string, code: string) =>
  Promise.resolve({
    error: {
      type: `https://api.vibexp.io/errors/${code}`,
      title: code,
      status,
      detail,
      code,
      request_id: 'req-1',
      timestamp: '2024-01-01T00:00:00Z',
    },
    response: { ok: false, status, statusText: code } as Response,
  })

const emptyPage: TeamSettingsAuditListResponse = {
  entries: [],
  total_count: 0,
  page: 1,
  per_page: 20,
  total_pages: 0,
}

describe('teamSettingsAuditService', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('reads a page from the audit path, with paging in the query', async () => {
    mockGeneratedClient.GET.mockReturnValue(success(emptyPage))

    const result = await teamSettingsAuditService.getAudit(teamId, 3, 20)

    expect(result).toEqual(emptyPage)
    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(path, {
      params: {
        path: { team_id: teamId },
        query: { page: 3, limit: 20 },
      },
      signal: undefined,
    })
  })

  it('forwards the caller abort signal so a page change cancels the last read', async () => {
    mockGeneratedClient.GET.mockReturnValue(success(emptyPage))
    const controller = new AbortController()

    await teamSettingsAuditService.getAudit(teamId, 1, 20, controller.signal)

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      path,
      expect.objectContaining({ signal: controller.signal })
    )
  })

  it('surfaces the 403 a non-owner gets as an ApiError', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      problem(403, 'You do not have permission', 'forbidden')
    )

    await expect(
      teamSettingsAuditService.getAudit(teamId, 1, 20)
    ).rejects.toBeInstanceOf(ApiError)
  })
})
