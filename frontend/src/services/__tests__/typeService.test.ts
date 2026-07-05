import type { Type, TypeListResponse } from '../typeService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = {
  GET: jest.fn(),
  POST: jest.fn(),
  DELETE: jest.fn(),
}

jest.mock('../../lib/apiClientGenerated', () => {
  const actual = jest.requireActual<
    typeof import('../../lib/apiClientGenerated')
  >('../../lib/apiClientGenerated')
  return {
    ...actual,
    generatedClient: mockGeneratedClient,
  }
})

import { typeService } from '../typeService'

const teamId = 'team-1'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const noContentResponse = {
  ok: true,
  status: 204,
  statusText: 'No Content',
} as Response

const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

const mockType: Type = {
  id: 'type-1',
  team_id: teamId,
  resource_type: 'artifact',
  slug: 'design-doc',
  name: 'Design Doc',
  is_system: false,
  created_at: '2024-01-01T00:00:00Z',
}

describe('TypeService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('getTypes fetches team-scoped types filtered by resource type and unwraps the list', async () => {
    const response: TypeListResponse = { types: [mockType], total_count: 1 }
    mockGeneratedClient.GET.mockReturnValue(success(response))

    const result = await typeService.getTypes(teamId, 'artifact')

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/{team_id}/types',
      {
        params: {
          path: { team_id: teamId },
          query: { resource_type: 'artifact' },
        },
      }
    )
    expect(result).toEqual([mockType])
  })

  it('createType posts the request body to the team-scoped path', async () => {
    mockGeneratedClient.POST.mockReturnValue(success(mockType))

    const result = await typeService.createType(teamId, {
      resource_type: 'artifact',
      slug: 'design-doc',
      name: 'Design Doc',
    })

    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/{team_id}/types',
      {
        params: { path: { team_id: teamId } },
        body: {
          resource_type: 'artifact',
          slug: 'design-doc',
          name: 'Design Doc',
        },
      }
    )
    expect(result).toEqual(mockType)
  })

  it('deleteType deletes by type id on the team-scoped path', async () => {
    mockGeneratedClient.DELETE.mockReturnValue(
      Promise.resolve({ data: undefined, response: noContentResponse })
    )

    await typeService.deleteType(teamId, 'type-1')

    expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
      '/api/v1/{team_id}/types/{id}',
      { params: { path: { team_id: teamId, id: 'type-1' } } }
    )
  })

  it('throws ApiError with backend detail on RFC 9457 error', async () => {
    mockGeneratedClient.POST.mockReturnValue(
      Promise.resolve({
        error: {
          type: 'https://api.vibexp.io/errors/BAD_REQUEST',
          title: 'Bad Request',
          status: 400,
          detail: 'slug already exists',
          code: 'BAD_REQUEST',
          request_id: 'req-1',
          timestamp: '2024-01-01T10:00:00Z',
        },
        response: { ok: false, status: 400, statusText: 'Bad Request' },
      })
    )

    await expect(
      typeService.createType(teamId, {
        resource_type: 'artifact',
        slug: 'design-doc',
        name: 'Design Doc',
      })
    ).rejects.toThrow('slug already exists')
  })
})
