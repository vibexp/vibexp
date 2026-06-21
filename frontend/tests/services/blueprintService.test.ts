import type {
  Blueprint,
  BlueprintListResponse,
  BlueprintStatsResponse,
  CreateBlueprintRequest,
  UpdateBlueprintRequest,
  BlueprintFilters,
} from '../../src/types'

// Mock apiClient
const mockApiClient = {
  get: jest.fn(),
  post: jest.fn(),
  put: jest.fn(),
  delete: jest.fn(),
}

jest.mock('../../src/lib/apiClient', () => ({
  apiClient: mockApiClient,
}))

// Import after mocking
import { blueprintService } from '../../src/services/blueprintService'

describe('BlueprintService', () => {
  const mockTeamId = 'team-123'
  const mockProjectId = 'my-project'
  const mockSlug = 'api-spec'

  const mockBlueprint: Blueprint = {
    id: 'spec-123',
    project_id: 'my-project',
    slug: 'api-spec',
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
    it('should fetch blueprints with teamId in URL path', async () => {
      mockApiClient.get.mockResolvedValue(mockListResponse)

      const result = await blueprintService.getBlueprints(mockTeamId)

      expect(mockApiClient.get).toHaveBeenCalledWith(
        `/${mockTeamId}/blueprints`
      )
      expect(result).toEqual(mockListResponse)
    })

    it('should apply filters correctly without team_id in query params', async () => {
      const filters: BlueprintFilters = {
        search: 'api',
        type: 'general',
        status: 'active',
        page: 2,
        limit: 10,
      }

      mockApiClient.get.mockResolvedValue(mockListResponse)

      await blueprintService.getBlueprints(mockTeamId, filters)

      const callArg = mockApiClient.get.mock.calls[0][0] as string
      expect(callArg).toContain(`/${mockTeamId}/blueprints?`)
      expect(callArg).not.toContain('team_id=')
      expect(callArg).toContain('search=api')
      expect(callArg).toContain('type=general')
      expect(callArg).toContain('status=active')
      expect(callArg).toContain('page=2')
      expect(callArg).toContain('limit=10')
    })

    it('should ignore empty filter values', async () => {
      const filters: BlueprintFilters = {
        search: '',
        page: 1,
      }

      mockApiClient.get.mockResolvedValue(mockListResponse)

      await blueprintService.getBlueprints(mockTeamId, filters)

      const callArg = mockApiClient.get.mock.calls[0][0] as string
      expect(callArg).not.toContain('search=')
      expect(callArg).toContain('page=1')
    })
  })

  describe('getBlueprintsByProject', () => {
    it('should fetch blueprints for a specific project with teamId in URL path', async () => {
      mockApiClient.get.mockResolvedValue(mockListResponse)

      const result = await blueprintService.getBlueprintsByProject(
        mockTeamId,
        mockProjectId
      )

      expect(mockApiClient.get).toHaveBeenCalledWith(
        `/${mockTeamId}/blueprints/${mockProjectId}`
      )
      expect(result).toEqual(mockListResponse)
    })

    it('should encode special characters in project ID', async () => {
      mockApiClient.get.mockResolvedValue(mockListResponse)

      await blueprintService.getBlueprintsByProject(
        mockTeamId,
        'project/with/slashes'
      )

      const callArg = mockApiClient.get.mock.calls[0][0] as string
      expect(callArg).toContain(`/${mockTeamId}/blueprints/`)
      expect(callArg).toContain('project%2Fwith%2Fslashes')
    })
  })

  describe('getBlueprint', () => {
    it('should fetch a single blueprint with teamId in URL path', async () => {
      mockApiClient.get.mockResolvedValue(mockBlueprint)

      const result = await blueprintService.getBlueprint(
        mockTeamId,
        mockProjectId,
        mockSlug
      )

      expect(mockApiClient.get).toHaveBeenCalledWith(
        `/${mockTeamId}/blueprints/${mockProjectId}/${mockSlug}`
      )
      expect(result).toEqual(mockBlueprint)
    })

    it('should encode special characters in slug', async () => {
      mockApiClient.get.mockResolvedValue(mockBlueprint)

      await blueprintService.getBlueprint(mockTeamId, mockProjectId, 'api spec')

      expect(mockApiClient.get).toHaveBeenCalledWith(
        `/${mockTeamId}/blueprints/${mockProjectId}/api%20spec`
      )
    })
  })

  describe('createBlueprint', () => {
    it('should create a new blueprint with teamId in URL path', async () => {
      const createRequest: CreateBlueprintRequest = {
        project_id: mockProjectId,
        slug: 'new-spec',
        title: 'New Specification',
        type: 'general',
        content: '{"openapi": "3.0.0"}',
      }

      mockApiClient.post.mockResolvedValue(mockBlueprint)

      const result = await blueprintService.createBlueprint(
        mockTeamId,
        createRequest
      )

      expect(mockApiClient.post).toHaveBeenCalledWith(
        `/${mockTeamId}/blueprints`,
        createRequest
      )
      expect(result).toEqual(mockBlueprint)
    })

    it('should create a claude-code type blueprint', async () => {
      const createRequest: CreateBlueprintRequest = {
        project_id: mockProjectId,
        slug: 'claude-spec',
        title: 'Claude Code Specification',
        type: 'claude-code',
        subtype: 'sub-agents',
        content: 'agent content',
      }

      mockApiClient.post.mockResolvedValue({
        ...mockBlueprint,
        type: 'claude-code',
        subtype: 'sub-agents',
      })

      const result = await blueprintService.createBlueprint(
        mockTeamId,
        createRequest
      )

      expect(mockApiClient.post).toHaveBeenCalledWith(
        `/${mockTeamId}/blueprints`,
        createRequest
      )
      expect(result.type).toBe('claude-code')
    })
  })

  describe('updateBlueprint', () => {
    it('should update an existing blueprint with teamId in URL path', async () => {
      const updateRequest: UpdateBlueprintRequest = {
        title: 'Updated Title',
        description: 'Updated description',
      }

      mockApiClient.put.mockResolvedValue(mockBlueprint)

      const result = await blueprintService.updateBlueprint(
        mockTeamId,
        mockProjectId,
        mockSlug,
        updateRequest
      )

      expect(mockApiClient.put).toHaveBeenCalledWith(
        `/${mockTeamId}/blueprints/${mockProjectId}/${mockSlug}`,
        updateRequest
      )
      expect(result).toEqual(mockBlueprint)
    })
  })

  describe('deleteBlueprint', () => {
    it('should delete a blueprint with teamId in URL path', async () => {
      mockApiClient.delete.mockResolvedValue(undefined)

      await blueprintService.deleteBlueprint(
        mockTeamId,
        mockProjectId,
        mockSlug
      )

      expect(mockApiClient.delete).toHaveBeenCalledWith(
        `/${mockTeamId}/blueprints/${mockProjectId}/${mockSlug}`
      )
    })
  })

  describe('getBlueprintStats', () => {
    it('should fetch blueprint stats with teamId in URL path', async () => {
      const mockStats: BlueprintStatsResponse = {
        total_projects: 5,
        total_blueprints: 15,
        added_this_week: 3,
        total_by_type: {
          general: 10,
          'claude-code': 5,
        },
        total_by_status: {
          active: 12,
          expired: 3,
        },
      }

      mockApiClient.get.mockResolvedValue(mockStats)

      const result = await blueprintService.getBlueprintStats(mockTeamId)

      expect(mockApiClient.get).toHaveBeenCalledWith(
        `/${mockTeamId}/blueprints/stats`
      )
      expect(result).toEqual(mockStats)
    })

    it('should return default stats on error', async () => {
      const consoleSpy = jest.spyOn(console, 'error').mockImplementation()
      mockApiClient.get.mockRejectedValue(new Error('Network error'))

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
      mockApiClient.get.mockResolvedValue(versionList)

      const result = await blueprintService.getBlueprintVersions(
        mockTeamId,
        mockProjectId,
        mockSlug
      )

      expect(mockApiClient.get).toHaveBeenCalledWith(
        `/${mockTeamId}/blueprints/${mockProjectId}/${mockSlug}/versions`
      )
      expect(result).toEqual(versionList)
    })

    it('getBlueprintVersion fetches a single version by number', async () => {
      mockApiClient.get.mockResolvedValue(versionList.versions[0])

      await blueprintService.getBlueprintVersion(
        mockTeamId,
        mockProjectId,
        mockSlug,
        2
      )

      expect(mockApiClient.get).toHaveBeenCalledWith(
        `/${mockTeamId}/blueprints/${mockProjectId}/${mockSlug}/versions/2`
      )
    })

    it('restoreBlueprintVersion posts to the restore endpoint', async () => {
      mockApiClient.post.mockResolvedValue(mockBlueprint)

      const result = await blueprintService.restoreBlueprintVersion(
        mockTeamId,
        mockProjectId,
        mockSlug,
        1
      )

      expect(mockApiClient.post).toHaveBeenCalledWith(
        `/${mockTeamId}/blueprints/${mockProjectId}/${mockSlug}/versions/1/restore`
      )
      expect(result).toEqual(mockBlueprint)
    })

    it('encodes special characters in the slug', async () => {
      mockApiClient.get.mockResolvedValue(versionList)

      await blueprintService.getBlueprintVersions(
        mockTeamId,
        mockProjectId,
        'api spec'
      )

      expect(mockApiClient.get).toHaveBeenCalledWith(
        `/${mockTeamId}/blueprints/${mockProjectId}/api%20spec/versions`
      )
    })
  })
})
