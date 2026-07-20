import type {
  Blueprint,
  BlueprintListResponse,
  BlueprintStatsResponse,
  CreateBlueprintRequest,
  UpdateBlueprintRequest,
  BlueprintFilters,
} from '../../src/services/blueprintService'

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

import { blueprintService } from '../../src/services/blueprintService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

describe('BlueprintService', () => {
  const mockTeamId = 'team-123'
  const mockProjectId = 'my-project'
  const mockSlug = 'api-spec'

  const mockBlueprint: Blueprint = {
    id: 'spec-123',
    project_id: 'my-project',
    slug: 'api-spec',
    path: 'api-spec.md',
    user_id: 'user-123',
    title: 'API Specification',
    type: 'general',
    status: 'active',
    description: 'API specification document',
    content: '{"openapi": "3.0.0"}',
    metadata: {},
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  }

  const mockListResponse: BlueprintListResponse = {
    blueprints: [mockBlueprint],
    page: 1,
    per_page: 20,
    total_count: 1,
    total_pages: 1,
  }

  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getBlueprints', () => {
    it('lists blueprints with team_id in the path and empty query by default', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockListResponse))

      const result = await blueprintService.getBlueprints(mockTeamId)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/blueprints',
        { params: { path: { team_id: mockTeamId }, query: {} } }
      )
      expect(result).toEqual(mockListResponse)
    })

    it('forwards filters as query params', async () => {
      const filters: BlueprintFilters = {
        search: 'api',
        type: 'general',
        status: 'active',
        page: 2,
        limit: 10,
      }
      mockGeneratedClient.GET.mockReturnValue(success(mockListResponse))

      await blueprintService.getBlueprints(mockTeamId, filters)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/blueprints',
        { params: { path: { team_id: mockTeamId }, query: filters } }
      )
    })
  })

  describe('getBlueprintsByProject', () => {
    it('lists blueprints for a project with project_id in the path', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockListResponse))

      const result = await blueprintService.getBlueprintsByProject(
        mockTeamId,
        mockProjectId
      )

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/blueprints/{project_id}',
        {
          params: {
            path: { team_id: mockTeamId, project_id: mockProjectId },
            query: {},
          },
        }
      )
      expect(result).toEqual(mockListResponse)
    })
  })

  describe('getBlueprint', () => {
    it('fetches a single blueprint by project and slug', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockBlueprint))

      const result = await blueprintService.getBlueprint(
        mockTeamId,
        mockProjectId,
        mockSlug
      )

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/blueprints/{project_id}/{slug}',
        {
          params: {
            path: {
              team_id: mockTeamId,
              project_id: mockProjectId,
              slug: mockSlug,
            },
          },
        }
      )
      expect(result).toEqual(mockBlueprint)
    })
  })

  describe('createBlueprint', () => {
    it('posts the create request body', async () => {
      const createRequest: CreateBlueprintRequest = {
        project_id: mockProjectId,
        slug: 'new-spec',
        title: 'New Specification',
        type: 'general',
        status: 'active',
        content: '{"openapi": "3.0.0"}',
      }
      mockGeneratedClient.POST.mockReturnValue(success(mockBlueprint))

      const result = await blueprintService.createBlueprint(
        mockTeamId,
        createRequest
      )

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/blueprints',
        { params: { path: { team_id: mockTeamId } }, body: createRequest }
      )
      expect(result).toEqual(mockBlueprint)
    })
  })

  describe('updateBlueprint', () => {
    it('puts the update request body', async () => {
      const updateRequest: UpdateBlueprintRequest = {
        title: 'Updated Title',
        description: 'Updated description',
      }
      mockGeneratedClient.PUT.mockReturnValue(success(mockBlueprint))

      const result = await blueprintService.updateBlueprint(
        mockTeamId,
        mockProjectId,
        mockSlug,
        updateRequest
      )

      expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(
        '/api/v1/{team_id}/blueprints/{project_id}/{slug}',
        {
          params: {
            path: {
              team_id: mockTeamId,
              project_id: mockProjectId,
              slug: mockSlug,
            },
          },
          body: updateRequest,
        }
      )
      expect(result).toEqual(mockBlueprint)
    })
  })

  describe('deleteBlueprint', () => {
    it('deletes a blueprint by project and slug', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(
        Promise.resolve({
          data: undefined,
          response: { ok: true, status: 204 } as Response,
        })
      )

      await blueprintService.deleteBlueprint(
        mockTeamId,
        mockProjectId,
        mockSlug
      )

      expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
        '/api/v1/{team_id}/blueprints/{project_id}/{slug}',
        {
          params: {
            path: {
              team_id: mockTeamId,
              project_id: mockProjectId,
              slug: mockSlug,
            },
          },
        }
      )
    })
  })

  describe('getBlueprintStats', () => {
    it('fetches blueprint stats', async () => {
      const mockStats: BlueprintStatsResponse = {
        total_projects: 5,
        total_blueprints: 15,
        added_this_week: 3,
        total_by_type: { general: 10, 'claude-code': 5 },
        total_by_status: { active: 12, expired: 3 },
      }
      mockGeneratedClient.GET.mockReturnValue(success(mockStats))

      const result = await blueprintService.getBlueprintStats(mockTeamId)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/blueprints/stats',
        { params: { path: { team_id: mockTeamId } } }
      )
      expect(result).toEqual(mockStats)
    })

    it('returns default stats on error', async () => {
      const consoleSpy = jest.spyOn(console, 'error').mockImplementation()
      mockGeneratedClient.GET.mockReturnValue(
        Promise.resolve({
          error: {
            type: 'https://api.vibexp.io/errors/INTERNAL_ERROR',
            title: 'Internal Server Error',
            status: 500,
            detail: 'Network error',
            code: 'INTERNAL_ERROR',
            request_id: 'req-1',
            timestamp: '2024-01-01T10:00:00Z',
          },
          response: { ok: false, status: 500, statusText: 'Server Error' },
        })
      )

      const result = await blueprintService.getBlueprintStats(mockTeamId)

      expect(result).toEqual({
        total_projects: 0,
        total_blueprints: 0,
        added_this_week: 0,
        total_by_type: {},
        total_by_status: {},
      })
      expect(consoleSpy).toHaveBeenCalledWith(
        'Failed to fetch blueprint stats:',
        expect.any(Error)
      )

      consoleSpy.mockRestore()
    })
  })

  describe('version history', () => {
    const versionList = {
      versions: [
        {
          id: 'ver-1',
          team_id: mockTeamId,
          resource_type: 'blueprint',
          resource_id: 'spec-123',
          version_number: 1,
          content: 'old content',
          change_summary: null,
          actor_type: 'human' as const,
          created_by: 'user-123',
          author: null,
          created_at: '2026-01-01T00:00:00Z',
        },
      ],
    }

    it('getBlueprintVersions fetches the versions endpoint', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(versionList))

      const result = await blueprintService.getBlueprintVersions(
        mockTeamId,
        mockProjectId,
        mockSlug
      )

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/blueprints/{project_id}/{slug}/versions',
        {
          params: {
            path: {
              team_id: mockTeamId,
              project_id: mockProjectId,
              slug: mockSlug,
            },
          },
        }
      )
      expect(result).toEqual(versionList)
    })

    it('getBlueprintVersion fetches a single version by number', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(versionList.versions[0]))

      await blueprintService.getBlueprintVersion(
        mockTeamId,
        mockProjectId,
        mockSlug,
        2
      )

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/blueprints/{project_id}/{slug}/versions/{version_number}',
        {
          params: {
            path: {
              team_id: mockTeamId,
              project_id: mockProjectId,
              slug: mockSlug,
              version_number: 2,
            },
          },
        }
      )
    })

    it('restoreBlueprintVersion posts to the restore endpoint', async () => {
      mockGeneratedClient.POST.mockReturnValue(success(mockBlueprint))

      const result = await blueprintService.restoreBlueprintVersion(
        mockTeamId,
        mockProjectId,
        mockSlug,
        1
      )

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/blueprints/{project_id}/{slug}/versions/{version_number}/restore',
        {
          params: {
            path: {
              team_id: mockTeamId,
              project_id: mockProjectId,
              slug: mockSlug,
              version_number: 1,
            },
          },
        }
      )
      expect(result).toEqual(mockBlueprint)
    })
  })
})
