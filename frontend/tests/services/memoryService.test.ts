import type {
  CreateMemoryRequest,
  Memory,
  MemoriesResponse,
  MemoryFilters,
  UpdateMemoryRequest,
} from '../../src/services/memoryService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = {
  GET: jest.fn(),
  POST: jest.fn(),
  PUT: jest.fn(),
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

import { memoryService } from '../../src/services/memoryService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

describe('MemoryService', () => {
  const teamId = 'team-123'
  const projectId = '7c9e6679-7425-40de-944b-e07fc1f90ae7'
  const id = 'mem-1'

  const mockMemory: Memory = {
    id,
    user_id: 'user-123',
    team_id: teamId,
    project_id: projectId,
    text: 'remember this',
    status: 'active',
    metadata: {},
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    version: 1,
  }

  const mockListResponse: MemoriesResponse = {
    memories: [mockMemory],
    page: 1,
    per_page: 50,
    total_count: 1,
    total_pages: 1,
  }

  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getMemories', () => {
    it('lists memories with team_id in the path and empty query by default', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockListResponse))

      const result = await memoryService.getMemories(teamId)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/memories',
        { params: { path: { team_id: teamId }, query: {} } }
      )
      expect(result).toEqual(mockListResponse)
    })

    it('forwards filters as query params', async () => {
      const filters: MemoryFilters = {
        search: 'api',
        status: 'active',
        sort_by: 'updated_at',
        sort_order: 'desc',
        page: 2,
        limit: 10,
      }
      mockGeneratedClient.GET.mockReturnValue(success(mockListResponse))

      await memoryService.getMemories(teamId, filters)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/memories',
        { params: { path: { team_id: teamId }, query: filters } }
      )
    })
  })

  describe('getMemory', () => {
    it('fetches a single memory by id', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockMemory))

      const result = await memoryService.getMemory(teamId, id)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/memories/{id}',
        { params: { path: { team_id: teamId, id } } }
      )
      expect(result).toEqual(mockMemory)
    })
  })

  describe('createMemory', () => {
    it('posts the create request body including the required project_id', async () => {
      const createRequest: CreateMemoryRequest = {
        project_id: projectId,
        text: 'new memory',
        status: 'active',
      }
      mockGeneratedClient.POST.mockReturnValue(success(mockMemory))

      const result = await memoryService.createMemory(teamId, createRequest)

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/memories',
        { params: { path: { team_id: teamId } }, body: createRequest }
      )
      expect(result).toEqual(mockMemory)
    })
  })

  describe('updateMemory', () => {
    it('puts the update request body', async () => {
      const updateRequest: UpdateMemoryRequest = {
        text: 'updated memory',
        status: 'archived',
      }
      mockGeneratedClient.PUT.mockReturnValue(success(mockMemory))

      const result = await memoryService.updateMemory(teamId, id, updateRequest)

      expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(
        '/api/v1/{team_id}/memories/{id}',
        {
          params: { path: { team_id: teamId, id } },
          body: updateRequest,
        }
      )
      expect(result).toEqual(mockMemory)
    })
  })

  describe('deleteMemory', () => {
    it('deletes a memory by id', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(
        Promise.resolve({
          data: undefined,
          response: { ok: true, status: 204 } as Response,
        })
      )

      await memoryService.deleteMemory(teamId, id)

      expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
        '/api/v1/{team_id}/memories/{id}',
        { params: { path: { team_id: teamId, id } } }
      )
    })
  })

  describe('searchMemoriesByMetadata', () => {
    it('requires metadata_key and metadata_value', async () => {
      await expect(
        memoryService.searchMemoriesByMetadata(teamId, { search: 'x' })
      ).rejects.toThrow('metadata_key and metadata_value are required')
      expect(mockGeneratedClient.GET).not.toHaveBeenCalled()
    })

    it('queries the metadata-search endpoint with the metadata filters', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockListResponse))

      const result = await memoryService.searchMemoriesByMetadata(teamId, {
        metadata_key: 'category',
        metadata_value: 'reminder',
        search: 'api',
        page: 1,
        limit: 20,
      })

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/memories/search',
        {
          params: {
            path: { team_id: teamId },
            query: {
              metadata_key: 'category',
              metadata_value: 'reminder',
              search: 'api',
              page: 1,
              limit: 20,
            },
          },
        }
      )
      expect(result).toEqual(mockListResponse)
    })
  })

  describe('version history', () => {
    const versionList = {
      versions: [
        {
          id: 'ver-1',
          team_id: teamId,
          resource_type: 'memory',
          resource_id: id,
          version_number: 1,
          content: 'old text',
          change_summary: null,
          actor_type: 'human' as const,
          created_by: 'user-123',
          author: null,
          created_at: '2026-01-01T00:00:00Z',
        },
      ],
    }

    it('getMemoryVersions fetches the versions endpoint', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(versionList))

      const result = await memoryService.getMemoryVersions(teamId, id)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/memories/{id}/versions',
        { params: { path: { team_id: teamId, id } } }
      )
      expect(result).toEqual(versionList)
    })

    it('getMemoryVersion fetches a single version by number', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(versionList.versions[0]))

      await memoryService.getMemoryVersion(teamId, id, 2)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/memories/{id}/versions/{version_number}',
        {
          params: {
            path: { team_id: teamId, id, version_number: 2 },
          },
        }
      )
    })

    it('restoreMemoryVersion posts to the restore endpoint', async () => {
      mockGeneratedClient.POST.mockReturnValue(success(mockMemory))

      const result = await memoryService.restoreMemoryVersion(teamId, id, 1)

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/memories/{id}/versions/{version_number}/restore',
        {
          params: {
            path: { team_id: teamId, id, version_number: 1 },
          },
        }
      )
      expect(result).toEqual(mockMemory)
    })
  })
})
