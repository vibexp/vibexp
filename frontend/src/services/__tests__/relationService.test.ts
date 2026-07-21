import type {
  RelatedResource,
  Relation,
  RelationListResponse,
} from '../relationService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = {
  GET: jest.fn(),
  POST: jest.fn(),
  PUT: jest.fn(),
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

import { relationService } from '../relationService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

const TEAM_ID = 'team-123'
const RELATION_ID = 'rel-abc'

const mockRelation: Relation = {
  id: RELATION_ID,
  team_id: TEAM_ID,
  project_id: 'p1',
  from_type: 'artifact',
  from_id: 'a1',
  to_type: 'blueprint',
  to_id: 'b1',
  relation_type: 'governed-by',
  origin: 'human',
  status: 'confirmed',
  created_at: '2026-07-21T00:00:00Z',
  updated_at: '2026-07-21T00:00:00Z',
}

const mockRelated: RelatedResource = {
  relation_id: RELATION_ID,
  relation_type: 'governed-by',
  direction: 'outgoing',
  origin: 'ai',
  status: 'suggested',
  resource_type: 'blueprint',
  resource_id: 'b1',
  title: 'Go standards',
  created_at: '2026-07-21T00:00:00Z',
}

beforeEach(() => {
  jest.clearAllMocks()
})

describe('relationService', () => {
  test('list passes team + resource + paging params to the relations GET', async () => {
    const resp: RelationListResponse = {
      relations: [mockRelated],
      total_count: 1,
      page: 1,
      per_page: 100,
      total_pages: 1,
    }
    mockGeneratedClient.GET.mockReturnValue(success(resp))

    const out = await relationService.list(TEAM_ID, 'artifact', 'a1', 2, 50)

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/{team_id}/relations',
      {
        params: {
          path: { team_id: TEAM_ID },
          query: {
            resource_type: 'artifact',
            resource_id: 'a1',
            page: 2,
            limit: 50,
          },
        },
      }
    )
    expect(out).toEqual(resp)
  })

  test('create posts the edge body under the team path', async () => {
    mockGeneratedClient.POST.mockReturnValue(success(mockRelation))
    const body = {
      from_type: 'artifact' as const,
      from_id: 'a1',
      to_type: 'blueprint' as const,
      to_id: 'b1',
      relation_type: 'governed-by' as const,
      origin: 'human' as const,
    }

    const out = await relationService.create(TEAM_ID, body)

    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/{team_id}/relations',
      { params: { path: { team_id: TEAM_ID } }, body }
    )
    expect(out).toEqual(mockRelation)
  })

  test('confirm posts to the relation confirm endpoint', async () => {
    mockGeneratedClient.POST.mockReturnValue(
      success({ ...mockRelation, status: 'confirmed' })
    )

    await relationService.confirm(TEAM_ID, RELATION_ID)

    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/{team_id}/relations/{relation_id}/confirm',
      { params: { path: { team_id: TEAM_ID, relation_id: RELATION_ID } } }
    )
  })

  test('remove deletes the relation by id', async () => {
    mockGeneratedClient.DELETE.mockReturnValue(success(undefined))

    await relationService.remove(TEAM_ID, RELATION_ID)

    expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
      '/api/v1/{team_id}/relations/{relation_id}',
      { params: { path: { team_id: TEAM_ID, relation_id: RELATION_ID } } }
    )
  })

  test('a failed response rejects via the shared unwrap', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      Promise.resolve({
        error: { message: 'boom' },
        response: { ok: false, status: 500, statusText: 'err' } as Response,
      })
    )

    await expect(
      relationService.list(TEAM_ID, 'artifact', 'a1')
    ).rejects.toBeDefined()
  })
})
