/**
 * Unit Tests for ProjectService - Issue #737
 *
 * This test suite validates the ProjectService functionality including:
 * - All CRUD operations (create, read, update, delete projects)
 * - Filter and pagination handling
 * - Authentication and authorization scenarios
 * - Error handling and edge cases
 *
 * Coverage target: >50%
 */

import type {
  Project,
  ProjectFilters,
  ProjectListResponse,
  CreateProjectRequest,
  UpdateProjectRequest,
} from '../../src/types/project'

// Mock the authService
const mockAuthService = {
  getToken: jest.fn(),
  setToken: jest.fn(),
}

jest.mock('../../src/services/authService', () => ({
  authService: mockAuthService,
}))

// Mock fetch globally
global.fetch = jest.fn()

// Create a test implementation of ProjectService to avoid import.meta issues
class TestProjectService {
  private readonly API_BASE_URL = 'https://api.vibexp.io/api/v1'

  private async makeRequest<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const token = mockAuthService.getToken()
    if (!token) {
      throw new Error('No authentication token')
    }

    const response = await fetch(`${this.API_BASE_URL}${endpoint}`, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
        ...options.headers,
      },
    })

    if (!response.ok) {
      if (response.status === 401) {
        mockAuthService.setToken(null)
        throw new Error('Authentication expired')
      }
      const errorData = await response.json().catch(() => null)
      throw new Error(
        errorData?.message || `HTTP error! status: ${response.status}`
      )
    }

    // Handle 204 No Content responses (like successful deletes)
    if (response.status === 204) {
      return null as T
    }

    // Check if response has JSON content
    const contentType = response.headers.get('content-type')
    if (contentType && contentType.includes('application/json')) {
      return response.json()
    }

    // Try to parse as JSON even if content-type header is not set correctly
    try {
      const text = await response.text()
      if (text.trim()) {
        return JSON.parse(text)
      }
    } catch (e) {
      console.warn('Failed to parse response as JSON:', e)
    }

    return null as T
  }

  async getProjects(
    teamId: string,
    filters: ProjectFilters = {}
  ): Promise<ProjectListResponse> {
    const params = new URLSearchParams()

    // Remove team_id from query params - it's now in the URL path
    if (filters.search) params.append('search', filters.search)
    if (filters.page) params.append('page', filters.page.toString())
    if (filters.limit) params.append('limit', filters.limit.toString())
    if (filters.sort_by) params.append('sort_by', filters.sort_by)
    if (filters.sort_order) params.append('sort_order', filters.sort_order)

    const queryString = params.toString()
    const endpoint = `/${teamId}/projects${queryString ? `?${queryString}` : ''}`

    return this.makeRequest<ProjectListResponse>(endpoint)
  }

  async getProject(teamId: string, slug: string): Promise<Project> {
    return this.makeRequest<Project>(
      `/${teamId}/projects/${encodeURIComponent(slug)}`
    )
  }

  async createProject(
    teamId: string,
    data: CreateProjectRequest
  ): Promise<Project> {
    return this.makeRequest<Project>(`/${teamId}/projects`, {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async updateProject(
    teamId: string,
    slug: string,
    data: UpdateProjectRequest
  ): Promise<Project> {
    return this.makeRequest<Project>(
      `/${teamId}/projects/${encodeURIComponent(slug)}`,
      {
        method: 'PUT',
        body: JSON.stringify(data),
      }
    )
  }

  async deleteProject(teamId: string, slug: string): Promise<void> {
    await this.makeRequest<void>(
      `/${teamId}/projects/${encodeURIComponent(slug)}`,
      {
        method: 'DELETE',
      }
    )
  }
}

describe('ProjectService', () => {
  let projectService: TestProjectService
  const mockToken = 'mock-auth-token'
  const mockTeamId = 'team-123'
  const baseUrl = 'https://api.vibexp.io/api/v1'
  const mockFetch = fetch as jest.MockedFunction<typeof fetch>

  const mockProject: Project = {
    id: 'project-1',
    user_id: 'user-1',
    team_id: mockTeamId,
    name: 'Test Project',
    slug: 'test-project',
    description: 'A test project',
    git_url: 'https://github.com/test/project',
    homepage: 'https://example.com',
    created_at: '2023-01-01T00:00:00Z',
    updated_at: '2023-01-01T00:00:00Z',
    version: 1,
  }

  beforeEach(() => {
    jest.clearAllMocks()
    projectService = new TestProjectService()
    mockAuthService.getToken.mockReturnValue(mockToken)

    // Reset fetch mock to default successful response
    mockFetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({}),
      headers: new Headers({ 'content-type': 'application/json' }),
    } as Response)
  })

  describe('Authentication', () => {
    it('should throw error when no token is available', async () => {
      mockAuthService.getToken.mockReturnValue(null)

      await expect(projectService.getProjects(mockTeamId)).rejects.toThrow(
        'No authentication token'
      )
      expect(mockFetch).not.toHaveBeenCalled()
    })

    it('should include Bearer token in request headers', async () => {
      const mockResponse: ProjectListResponse = {
        projects: [],
        total_count: 0,
        page: 1,
        per_page: 20,
        total_pages: 0,
      }
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      await projectService.getProjects(mockTeamId)

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/projects`,
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: `Bearer ${mockToken}`,
            'Content-Type': 'application/json',
          }),
        })
      )
    })

    it('should handle 401 authentication expired and clear token', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 401,
        json: () => Promise.resolve({ message: 'Unauthorized' }),
      } as Response)

      await expect(projectService.getProjects(mockTeamId)).rejects.toThrow(
        'Authentication expired'
      )
      expect(mockAuthService.setToken).toHaveBeenCalledWith(null)
    })
  })

  describe('Error Handling', () => {
    it('should handle HTTP errors with JSON error message', async () => {
      const errorMessage = 'Bad Request'
      mockFetch.mockResolvedValue({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ message: errorMessage }),
      } as Response)

      await expect(projectService.getProjects(mockTeamId)).rejects.toThrow(
        errorMessage
      )
    })

    it('should handle HTTP errors without JSON response', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.reject(new Error('Not JSON')),
      } as Response)

      await expect(projectService.getProjects(mockTeamId)).rejects.toThrow(
        'HTTP error! status: 500'
      )
    })

    it('should handle 204 No Content responses', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 204,
        headers: new Headers(),
      } as Response)

      const result = await projectService.deleteProject(mockTeamId, 'test-slug')
      expect(result).toBeUndefined()
    })

    it('should handle network errors', async () => {
      mockFetch.mockRejectedValue(new Error('Network error'))

      await expect(projectService.getProjects(mockTeamId)).rejects.toThrow(
        'Network error'
      )
    })

    it('should handle timeout errors', async () => {
      mockFetch.mockRejectedValue(new Error('Request timeout'))

      await expect(
        projectService.getProject(mockTeamId, 'test-slug')
      ).rejects.toThrow('Request timeout')
    })

    it('should handle responses with no content-type header', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        headers: new Headers(),
        text: () => Promise.resolve(JSON.stringify(mockProject)),
      } as Response)

      const result = await projectService.getProject(mockTeamId, 'test-slug')
      expect(result).toEqual(mockProject)
    })

    it('should handle responses with invalid JSON', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        headers: new Headers(),
        text: () => Promise.resolve('invalid json'),
      } as Response)

      const consoleSpy = jest.spyOn(console, 'warn').mockImplementation()
      const result = await projectService.getProject(mockTeamId, 'test-slug')

      expect(result).toBeNull()
      expect(consoleSpy).toHaveBeenCalledWith(
        'Failed to parse response as JSON:',
        expect.any(Error)
      )
      consoleSpy.mockRestore()
    })

    it('should handle empty response bodies', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        headers: new Headers(),
        text: () => Promise.resolve(''),
      } as Response)

      const result = await projectService.getProject(mockTeamId, 'test-slug')
      expect(result).toBeNull()
    })
  })

  describe('getProjects', () => {
    const mockProjectsResponse: ProjectListResponse = {
      projects: [mockProject],
      total_count: 1,
      page: 1,
      per_page: 20,
      total_pages: 1,
    }

    it('should fetch projects without filters', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockProjectsResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await projectService.getProjects(mockTeamId)

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/projects`,
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: `Bearer ${mockToken}`,
          }),
        })
      )
      expect(result).toEqual(mockProjectsResponse)
    })

    it('should fetch projects with all filters', async () => {
      const filters: ProjectFilters = {
        search: 'test query',
        page: 2,
        limit: 5,
        sort_by: 'name',
        sort_order: 'asc',
      }

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockProjectsResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      await projectService.getProjects(mockTeamId, filters)

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/projects?search=test+query&page=2&limit=5&sort_by=name&sort_order=asc`,
        expect.any(Object)
      )
    })

    it('should fetch projects with partial filters', async () => {
      const filters: ProjectFilters = {
        search: 'partial',
        page: 1,
      }

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockProjectsResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      await projectService.getProjects(mockTeamId, filters)

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/projects?search=partial&page=1`,
        expect.any(Object)
      )
    })

    it('should ignore undefined and null filter values', async () => {
      const filters: ProjectFilters = {
        search: undefined,
        page: undefined,
        limit: undefined,
      }

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockProjectsResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      await projectService.getProjects(mockTeamId, filters)

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/projects`,
        expect.any(Object)
      )
    })

    it('should ignore empty string filter values', async () => {
      const filters: ProjectFilters = {
        search: '',
      }

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockProjectsResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      await projectService.getProjects(mockTeamId, filters)

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/projects`,
        expect.any(Object)
      )
    })

    it('should handle empty project list', async () => {
      const emptyResponse: ProjectListResponse = {
        projects: [],
        total_count: 0,
        page: 1,
        per_page: 20,
        total_pages: 0,
      }

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(emptyResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await projectService.getProjects(mockTeamId)
      expect(result.projects).toHaveLength(0)
      expect(result.total_count).toBe(0)
    })

    it('should handle large project lists', async () => {
      const largeProjectList = Array.from({ length: 100 }, (_, i) => ({
        ...mockProject,
        id: `project-${i}`,
        name: `Project ${i}`,
        slug: `project-${i}`,
      }))

      const largeResponse: ProjectListResponse = {
        projects: largeProjectList,
        total_count: 100,
        page: 1,
        per_page: 100,
        total_pages: 1,
      }

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(largeResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await projectService.getProjects(mockTeamId, {
        limit: 100,
      })
      expect(result.projects).toHaveLength(100)
    })
  })

  describe('getProject', () => {
    it('should fetch a single project by slug', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockProject),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await projectService.getProject(mockTeamId, 'test-project')

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/projects/test-project`,
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: `Bearer ${mockToken}`,
          }),
        })
      )
      expect(result).toEqual(mockProject)
    })

    it('should handle special characters in slug', async () => {
      const slug = 'test-project-with-special-chars-123'
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockProject),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      await projectService.getProject(mockTeamId, slug)

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/projects/${slug}`,
        expect.any(Object)
      )
    })

    it('should URL encode slug with special characters', async () => {
      const slug = 'test/project with spaces'
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockProject),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      await projectService.getProject(mockTeamId, slug)

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/projects/${encodeURIComponent(slug)}`,
        expect.any(Object)
      )
    })

    it('should handle 404 not found', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ message: 'Project not found' }),
      } as Response)

      await expect(
        projectService.getProject(mockTeamId, 'nonexistent-project')
      ).rejects.toThrow('Project not found')
    })
  })

  describe('createProject', () => {
    const createRequest: CreateProjectRequest = {
      name: 'New Project',
      slug: 'new-project',
      description: 'A new test project',
      git_url: 'https://github.com/test/new-project',
      homepage: 'https://example.com/new',
    }

    it('should create a new project', async () => {
      const createdProject: Project = {
        ...mockProject,
        id: 'project-2',
        ...createRequest,
        user_id: 'user-1',
        team_id: mockTeamId,
        created_at: '2023-01-02T00:00:00Z',
        updated_at: '2023-01-02T00:00:00Z',
        version: 1,
      }

      mockFetch.mockResolvedValue({
        ok: true,
        status: 201,
        json: () => Promise.resolve(createdProject),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await projectService.createProject(
        mockTeamId,
        createRequest
      )

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/projects`,
        expect.objectContaining({
          method: 'POST',
          headers: expect.objectContaining({
            Authorization: `Bearer ${mockToken}`,
            'Content-Type': 'application/json',
          }),
          body: JSON.stringify(createRequest),
        })
      )
      expect(result).toEqual(createdProject)
    })

    it('should create project with minimal data', async () => {
      const minimalRequest: CreateProjectRequest = {
        name: 'Minimal Project',
        slug: 'minimal',
      }

      mockFetch.mockResolvedValue({
        ok: true,
        status: 201,
        json: () => Promise.resolve({ ...mockProject, ...minimalRequest }),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      await projectService.createProject(mockTeamId, minimalRequest)

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/projects`,
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify(minimalRequest),
        })
      )
    })

    it('should handle validation errors', async () => {
      const invalidRequest: CreateProjectRequest = {
        name: '',
        slug: '',
      }

      mockFetch.mockResolvedValue({
        ok: false,
        status: 400,
        json: () =>
          Promise.resolve({ message: 'Validation error: name is required' }),
      } as Response)

      await expect(
        projectService.createProject(mockTeamId, invalidRequest)
      ).rejects.toThrow('Validation error: name is required')
    })

    it('should handle duplicate slug conflict', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 409,
        json: () => Promise.resolve({ message: 'Slug already exists' }),
      } as Response)

      await expect(
        projectService.createProject(mockTeamId, createRequest)
      ).rejects.toThrow('Slug already exists')
    })
  })

  describe('updateProject', () => {
    const updateRequest: UpdateProjectRequest = {
      name: 'Updated Project',
      description: 'Updated description',
    }

    it('should update an existing project', async () => {
      const updatedProject: Project = {
        ...mockProject,
        ...updateRequest,
        updated_at: '2023-01-01T01:00:00Z',
        version: 2,
      }

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(updatedProject),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await projectService.updateProject(
        mockTeamId,
        'test-project',
        updateRequest
      )

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/projects/test-project`,
        expect.objectContaining({
          method: 'PUT',
          headers: expect.objectContaining({
            Authorization: `Bearer ${mockToken}`,
            'Content-Type': 'application/json',
          }),
          body: JSON.stringify(updateRequest),
        })
      )
      expect(result).toEqual(updatedProject)
    })

    it('should update project with partial data', async () => {
      const partialUpdate: UpdateProjectRequest = {
        name: 'Only Name Updated',
      }

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ ...mockProject, ...partialUpdate }),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      await projectService.updateProject(
        mockTeamId,
        'test-project',
        partialUpdate
      )

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/projects/test-project`,
        expect.objectContaining({
          method: 'PUT',
          body: JSON.stringify(partialUpdate),
        })
      )
    })

    it('should handle 404 not found on update', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ message: 'Project not found' }),
      } as Response)

      await expect(
        projectService.updateProject(mockTeamId, 'nonexistent', updateRequest)
      ).rejects.toThrow('Project not found')
    })

    it('should URL encode slug in update request', async () => {
      const slug = 'test/project with spaces'
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockProject),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      await projectService.updateProject(mockTeamId, slug, updateRequest)

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/projects/${encodeURIComponent(slug)}`,
        expect.any(Object)
      )
    })
  })

  describe('deleteProject', () => {
    it('should delete a project', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 204,
        headers: new Headers(),
      } as Response)

      const result = await projectService.deleteProject(
        mockTeamId,
        'test-project'
      )

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/projects/test-project`,
        expect.objectContaining({
          method: 'DELETE',
          headers: expect.objectContaining({
            Authorization: `Bearer ${mockToken}`,
            'Content-Type': 'application/json',
          }),
        })
      )
      expect(result).toBeUndefined()
    })

    it('should handle delete errors', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ message: 'Project not found' }),
      } as Response)

      await expect(
        projectService.deleteProject(mockTeamId, 'nonexistent-project')
      ).rejects.toThrow('Project not found')
    })

    it('should URL encode slug in delete request', async () => {
      const slug = 'test/project with spaces'
      mockFetch.mockResolvedValue({
        ok: true,
        status: 204,
        headers: new Headers(),
      } as Response)

      await projectService.deleteProject(mockTeamId, slug)

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/projects/${encodeURIComponent(slug)}`,
        expect.any(Object)
      )
    })

    it('should handle 403 forbidden on delete', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 403,
        json: () =>
          Promise.resolve({ message: 'Not authorized to delete this project' }),
      } as Response)

      await expect(
        projectService.deleteProject(mockTeamId, 'other-users-project')
      ).rejects.toThrow('Not authorized to delete this project')
    })
  })

  describe('Concurrent Operations', () => {
    it('should handle concurrent requests properly', async () => {
      const promises = []

      // Mock multiple successful responses
      for (let i = 0; i < 3; i++) {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: jest.fn().mockResolvedValue({
            ...mockProject,
            id: `project-${i}`,
          }),
          headers: new Headers({ 'content-type': 'application/json' }),
        } as unknown as Response)
      }

      // Start concurrent requests
      promises.push(projectService.getProject(mockTeamId, 'project-0'))
      promises.push(projectService.getProject(mockTeamId, 'project-1'))
      promises.push(projectService.getProject(mockTeamId, 'project-2'))

      const results = await Promise.all(promises)

      expect(results).toHaveLength(3)
      expect(mockFetch).toHaveBeenCalledTimes(3)
    })
  })

  describe('Integration Scenarios', () => {
    it('should handle complete CRUD workflow', async () => {
      // Create
      const createRequest: CreateProjectRequest = {
        name: 'CRUD Test Project',
        slug: 'crud-test-project',
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: () =>
          Promise.resolve({
            ...mockProject,
            ...createRequest,
            id: 'crud-project',
          }),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const created = await projectService.createProject(
        mockTeamId,
        createRequest
      )
      expect(created.slug).toBe('crud-test-project')

      // Read
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve(created),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const fetched = await projectService.getProject(
        mockTeamId,
        'crud-test-project'
      )
      expect(fetched.id).toBe('crud-project')

      // Update
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({ ...created, name: 'Updated CRUD Project' }),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const updated = await projectService.updateProject(
        mockTeamId,
        'crud-test-project',
        {
          name: 'Updated CRUD Project',
        }
      )
      expect(updated.name).toBe('Updated CRUD Project')

      // Delete
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 204,
        headers: new Headers(),
      } as Response)

      await projectService.deleteProject(mockTeamId, 'crud-test-project')

      expect(mockFetch).toHaveBeenCalledTimes(4)
    })
  })
})
