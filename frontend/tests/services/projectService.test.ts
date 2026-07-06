/**
 * Unit tests for ProjectService.
 *
 * The service delegates to the generated openapi-fetch client; these tests mock
 * that client (leaving `unwrap` real) and assert each method targets the right
 * operation with the right path/query/body, plus success and error resolution.
 */

import type {
  Project,
  ProjectListResponse,
  ProjectStatsResponse,
  CreateProjectRequest,
  UpdateProjectRequest,
} from '../../src/services/projectService'

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

import { projectService } from '../../src/services/projectService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const noContentResponse = {
  ok: true,
  status: 204,
  statusText: 'No Content',
} as Response

const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })
const failure = (status: number, detail: string) =>
  Promise.resolve({
    error: {
      type: 'https://api.vibexp.io/errors/ERROR',
      title: 'Error',
      status,
      detail,
      code: 'ERROR',
      request_id: 'req-1',
      timestamp: '2024-01-01T10:00:00Z',
    },
    response: { ok: false, status, statusText: 'Error' },
  })

const TEAM_ID = 'team-1'

const project: Project = {
  id: 'project-1',
  user_id: 'user-1',
  team_id: TEAM_ID,
  name: 'My Project',
  slug: 'my-project',
  description: '',
  git_url: '',
  homepage: '',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  version: 1,
  github_connected: false,
}

const listResponse: ProjectListResponse = {
  projects: [project],
  total_count: 1,
  page: 1,
  per_page: 20,
  total_pages: 1,
}

describe('ProjectService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getProjects', () => {
    it('lists projects with default (empty) filters', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(listResponse))

      const result = await projectService.getProjects(TEAM_ID)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/projects',
        { params: { path: { team_id: TEAM_ID }, query: {} } }
      )
      expect(result).toEqual(listResponse)
    })

    it('forwards filters as query params', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(listResponse))
      const filters = {
        search: 'api',
        page: 2,
        limit: 50,
        sort_by: 'name',
        sort_order: 'asc',
      }

      await projectService.getProjects(TEAM_ID, filters)

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/projects',
        { params: { path: { team_id: TEAM_ID }, query: filters } }
      )
    })

    it('propagates errors', async () => {
      mockGeneratedClient.GET.mockReturnValue(failure(500, 'boom'))

      await expect(projectService.getProjects(TEAM_ID)).rejects.toThrow('boom')
    })
  })

  describe('getProject', () => {
    it('fetches a single project by slug', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(project))

      const result = await projectService.getProject(TEAM_ID, 'my-project')

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/projects/{slug}',
        { params: { path: { team_id: TEAM_ID, slug: 'my-project' } } }
      )
      expect(result).toEqual(project)
    })

    it('propagates a 404', async () => {
      mockGeneratedClient.GET.mockReturnValue(failure(404, 'not found'))

      await expect(
        projectService.getProject(TEAM_ID, 'missing')
      ).rejects.toThrow('not found')
    })
  })

  describe('createProject', () => {
    it('posts the create request body', async () => {
      mockGeneratedClient.POST.mockReturnValue(success(project))
      const body: CreateProjectRequest = {
        name: 'My Project',
        slug: 'my-project',
      }

      const result = await projectService.createProject(TEAM_ID, body)

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/{team_id}/projects',
        { params: { path: { team_id: TEAM_ID } }, body }
      )
      expect(result).toEqual(project)
    })

    it('propagates validation errors', async () => {
      mockGeneratedClient.POST.mockReturnValue(failure(400, 'slug taken'))

      await expect(
        projectService.createProject(TEAM_ID, { name: 'x', slug: 'x' })
      ).rejects.toThrow('slug taken')
    })
  })

  describe('updateProject', () => {
    it('puts the update request body', async () => {
      mockGeneratedClient.PUT.mockReturnValue(success(project))
      const body: UpdateProjectRequest = { name: 'Renamed' }

      const result = await projectService.updateProject(
        TEAM_ID,
        'my-project',
        body
      )

      expect(mockGeneratedClient.PUT).toHaveBeenCalledWith(
        '/api/v1/{team_id}/projects/{slug}',
        {
          params: { path: { team_id: TEAM_ID, slug: 'my-project' } },
          body,
        }
      )
      expect(result).toEqual(project)
    })
  })

  describe('deleteProject', () => {
    it('deletes a project by slug', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(
        Promise.resolve({ data: undefined, response: noContentResponse })
      )

      await projectService.deleteProject(TEAM_ID, 'my-project')

      expect(mockGeneratedClient.DELETE).toHaveBeenCalledWith(
        '/api/v1/{team_id}/projects/{slug}',
        { params: { path: { team_id: TEAM_ID, slug: 'my-project' } } }
      )
    })

    it('propagates errors', async () => {
      mockGeneratedClient.DELETE.mockReturnValue(failure(403, 'forbidden'))

      await expect(
        projectService.deleteProject(TEAM_ID, 'my-project')
      ).rejects.toThrow('forbidden')
    })
  })

  describe('getProjectStats', () => {
    it('fetches project stats', async () => {
      const stats: ProjectStatsResponse = {
        total_prompts: 5,
        total_artifacts: 3,
        total_blueprints: 2,
        total_memories: 7,
        total_feed_items: 1,
      }
      mockGeneratedClient.GET.mockReturnValue(success(stats))

      const result = await projectService.getProjectStats(TEAM_ID, 'my-project')

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/{team_id}/projects/{slug}/stats',
        { params: { path: { team_id: TEAM_ID, slug: 'my-project' } } }
      )
      expect(result).toEqual(stats)
    })
  })
})
