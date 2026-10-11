import { ApiError } from '../../types/errors'
import type { StatusDefinition, StatusListResponse } from '../statusService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = vi.hoisted(() => ({
  GET: vi.fn(),
  POST: vi.fn(),
  PATCH: vi.fn(),
  DELETE: vi.fn(),
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

import { STATUS_DELETE_FALLBACK, statusService } from '../statusService'

const teamId = 'team-1'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response

const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

const customStatus: StatusDefinition = {
  id: 'status-1',
  resource_type: 'prompt',
  slug: 'in-review',
  name: 'In review',
  is_system: false,
  is_live: true,
  usage_count: 3,
  created_at: '2026-10-01T00:00:00Z',
  updated_at: '2026-10-01T00:00:00Z',
}

describe('StatusService', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('list fetches the statuses of one resource type and unwraps the list', async () => {
    const response: StatusListResponse = { statuses: [customStatus] }
    mockGeneratedClient.GET.mockReturnValue(success(response))

    const result = await statusService.list(teamId, 'prompt')

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/{team_id}/statuses',
      {
        params: {
          path: { team_id: teamId },
          query: { resource_type: 'prompt' },
        },
      }
    )
    expect(result).toEqual([customStatus])
  })

  it('create posts the request body to the team-scoped path', async () => {
    mockGeneratedClient.POST.mockReturnValue(success(customStatus))

    const result = await statusService.create(teamId, {
      resource_type: 'prompt',
      slug: 'in-review',
      name: 'In review',
    })

    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/{team_id}/statuses',
      {
        params: { path: { team_id: teamId } },
        body: { resource_type: 'prompt', slug: 'in-review', name: 'In review' },
      }
    )
    expect(result).toEqual(customStatus)
  })

  it('rename patches the name alone — the slug is immutable', async () => {
    const renamed = { ...customStatus, name: 'Under review' }
    mockGeneratedClient.PATCH.mockReturnValue(success(renamed))

    const result = await statusService.rename(
      teamId,
      'status-1',
      'Under review'
    )

    expect(mockGeneratedClient.PATCH).toHaveBeenCalledWith(
      '/api/v1/{team_id}/statuses/{id}',
      {
        params: { path: { team_id: teamId, id: 'status-1' } },
        body: { name: 'Under review' },
      }
    )
    expect(result).toEqual(renamed)
  })

  it('remove deletes by id and returns what the server reassigned', async () => {
    mockGeneratedClient.DELETE.mockReturnValue(
      success({ reassigned_count: 3, reassigned_to: 'draft' })
    )

    const result = await statusService.remove(teamId, 'status-1')

    expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
      '/api/v1/{team_id}/statuses/{id}',
      { params: { path: { team_id: teamId, id: 'status-1' } } }
    )
    expect(result).toEqual({ reassigned_count: 3, reassigned_to: 'draft' })
  })

  // The page shows the server's own words for a reserved or duplicate slug, so
  // the problem body must survive the service untouched.
  it('rejects with the ApiError carrying the server detail and status', async () => {
    mockGeneratedClient.POST.mockReturnValue(
      Promise.resolve({
        error: {
          type: 'about:blank',
          title: 'Resource exists',
          status: 409,
          detail: 'slug is a built-in status and cannot be used',
          code: 'RESOURCE_ALREADY_EXISTS',
          request_id: 'req-1',
          timestamp: '2026-10-01T00:00:00Z',
        },
        response: { ok: false, status: 409, statusText: 'Conflict' },
      })
    )

    const failure = await statusService
      .create(teamId, { resource_type: 'prompt', slug: 'draft', name: 'Draft' })
      .catch((error: unknown) => error)

    expect(failure).toBeInstanceOf(ApiError)
    expect((failure as ApiError).status).toBe(409)
    expect((failure as ApiError).getMessage()).toBe(
      'slug is a built-in status and cannot be used'
    )
  })

  it('names draft as the fallback for prompts and active for the rest', () => {
    expect(STATUS_DELETE_FALLBACK).toEqual({
      prompt: 'draft',
      artifact: 'active',
      blueprint: 'active',
      memory: 'active',
    })
  })
})
