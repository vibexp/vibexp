import type {
  Artifact,
  ArtifactFilters,
  ArtifactListResponse,
  ArtifactStatsResponse,
  CreateArtifactRequest,
  UpdateArtifactRequest,
} from '../../src/services/artifactService'

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

import { artifactService } from '../../src/services/artifactService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

describe('ArtifactService', () => {
  const teamId = 'team-123'
  const projectId = 'my-project'
  const slug = 'api-docs'

  const mockArtifact: Artifact = {
    id: 'artifact-1',
    project_id: projectId,
    slug,
    user_id: 'user-123',
    title: 'API Documentation',
    description: 'Docs',
    type: 'general',
    status: 'active',
    content: 'hello',
    metadata: {},
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  }

  const mockListResponse: ArtifactListResponse = {
    artifacts: [mockArtifact],
    page: 1,
    per_page: 20,
    total_count: 1,
    total_pages: 1,
  }

  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getArtifacts', () => {
    it('lists artifacts with team_id in the path and empty query by default', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockListResponse))

      const result = await artifactService.getArtifacts(teamId)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/artifacts',
        { params: { path: { team_id: teamId }, query: {} } }
      )
      expect(result).toEqual(mockListResponse)
    })

    it('forwards filters as query params', async () => {
      const filters: ArtifactFilters = {
        search: 'api',
        type: 'general',
        status: 'active',
        sort_by: 'updated_at',
        sort_order: 'desc',
        page: 2,
        limit: 10,
      }
      mockGeneratedClient.GET.mockReturnValue(success(mockListResponse))

      await artifactService.getArtifacts(teamId, filters)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/artifacts',
        { params: { path: { team_id: teamId }, query: filters } }
      )
    })
  })

  describe('getArtifactsByProject', () => {
    it('lists artifacts for a project with project_id in the path', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockListResponse))

      const result = await artifactService.getArtifactsByProject(
        teamId,
        projectId
      )

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/artifacts/{project_id}',
        {
          params: {
            path: { team_id: teamId, project_id: projectId },
            query: {},
          },
        }
      )
      expect(result).toEqual(mockListResponse)
    })
  })

  describe('getArtifact', () => {
    it('fetches a single artifact by project and slug', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockArtifact))

      const result = await artifactService.getArtifact(teamId, projectId, slug)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/artifacts/{project_id}/{slug}',
        {
          params: {
            path: { team_id: teamId, project_id: projectId, slug },
          },
        }
      )
      expect(result).toEqual(mockArtifact)
    })
  })

  describe('createArtifact', () => {
    it('posts the create request body', async () => {
      const createRequest: CreateArtifactRequest = {
        project_id: projectId,
        slug: 'new-doc',
        title: 'New Doc',
        type: 'general',
        status: 'active',
        content: 'body',
      }
      mockGeneratedClient.POST.mockReturnValue(success(mockArtifact))

      const result = await artifactService.createArtifact(teamId, createRequest)

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/artifacts',
        { params: { path: { team_id: teamId } }, body: createRequest }
      )
      expect(result).toEqual(mockArtifact)
    })
  })

  describe('updateArtifact', () => {
    it('puts the update request body', async () => {
      const updateRequest: UpdateArtifactRequest = {
        title: 'Updated Title',
        description: 'Updated description',
      }
      mockGeneratedClient.PUT.mockReturnValue(success(mockArtifact))

      const result = await artifactService.updateArtifact(
        teamId,
        projectId,
        slug,
        updateRequest
      )

      expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(
        '/api/v1/{team_id}/artifacts/{project_id}/{slug}',
        {
          params: {
            path: { team_id: teamId, project_id: projectId, slug },
          },
          body: updateRequest,
        }
      )
      expect(result).toEqual(mockArtifact)
    })
  })

  describe('deleteArtifact', () => {
    it('deletes an artifact by project and slug', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(
        Promise.resolve({
          data: undefined,
          response: { ok: true, status: 204 } as Response,
        })
      )

      await artifactService.deleteArtifact(teamId, projectId, slug)

      expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
        '/api/v1/{team_id}/artifacts/{project_id}/{slug}',
        {
          params: {
            path: { team_id: teamId, project_id: projectId, slug },
          },
        }
      )
    })
  })

  describe('version history', () => {
    const versionList = {
      versions: [
        {
          id: 'ver-1',
          team_id: teamId,
          resource_type: 'artifact',
          resource_id: 'artifact-1',
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

    it('getArtifactVersions fetches the versions endpoint', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(versionList))

      const result = await artifactService.getArtifactVersions(
        teamId,
        projectId,
        slug
      )

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/artifacts/{project_id}/{slug}/versions',
        {
          params: {
            path: { team_id: teamId, project_id: projectId, slug },
          },
        }
      )
      expect(result).toEqual(versionList)
    })

    it('getArtifactVersion fetches a single version by number', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(versionList.versions[0]))

      await artifactService.getArtifactVersion(teamId, projectId, slug, 2)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/artifacts/{project_id}/{slug}/versions/{version_number}',
        {
          params: {
            path: {
              team_id: teamId,
              project_id: projectId,
              slug,
              version_number: 2,
            },
          },
        }
      )
    })

    it('restoreArtifactVersion posts to the restore endpoint', async () => {
      mockGeneratedClient.POST.mockReturnValue(success(mockArtifact))

      const result = await artifactService.restoreArtifactVersion(
        teamId,
        projectId,
        slug,
        1
      )

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/artifacts/{project_id}/{slug}/versions/{version_number}/restore',
        {
          params: {
            path: {
              team_id: teamId,
              project_id: projectId,
              slug,
              version_number: 1,
            },
          },
        }
      )
      expect(result).toEqual(mockArtifact)
    })
  })

  describe('getArtifactStats', () => {
    it('fetches artifact stats', async () => {
      const mockStats: ArtifactStatsResponse = {
        total_projects: 5,
        total_artifacts: 15,
        added_this_week: 3,
        total_by_type: { general: 10, work_reports: 5 },
        total_by_status: { active: 12, draft: 3 },
      }
      mockGeneratedClient.GET.mockReturnValue(success(mockStats))

      const result = await artifactService.getArtifactStats(teamId)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/artifacts/stats',
        { params: { path: { team_id: teamId } } }
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

      const result = await artifactService.getArtifactStats(teamId)

      expect(result).toEqual({
        total_projects: 0,
        total_artifacts: 0,
        added_this_week: 0,
        total_by_type: {},
        total_by_status: {},
      })
      expect(consoleSpy).toHaveBeenCalledWith(
        'Failed to fetch artifact stats:',
        expect.any(Error)
      )

      consoleSpy.mockRestore()
    })
  })
})
