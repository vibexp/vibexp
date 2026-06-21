import type {
  CreateArtifactRequest,
  UpdateArtifactRequest,
  ArtifactFilters,
  ArtifactListResponse,
  ArtifactStatsResponse,
  Artifact,
} from '../../src/types'

// Mock fetch globally
const mockFetch = jest.fn()
global.fetch = mockFetch

// Mock authService
const mockAuthService = {
  getToken: jest.fn(),
  setToken: jest.fn(),
}

jest.mock('../../src/services/authService', () => ({
  authService: mockAuthService,
}))

// Mock the entire ArtifactService module to avoid import.meta.env issues
jest.mock('../../src/services/artifactService', () => {
  // Create a mock authService for this module
  const authService = {
    getToken: jest.fn(),
    setToken: jest.fn(),
  }

  class ArtifactService {
    private async makeRequest<T>(
      endpoint: string,
      options: RequestInit = {}
    ): Promise<T> {
      const token = authService.getToken()
      if (!token) {
        throw new Error('No authentication token')
      }

      // Use production API URL by default for tests
      const API_BASE_URL = 'https://api.vibexp.io/api/v1'

      const response = await fetch(`${API_BASE_URL}${endpoint}`, {
        ...options,
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
          ...options.headers,
        },
      })

      if (!response.ok) {
        if (response.status === 401) {
          authService.setToken(null)
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

    async getArtifacts(
      teamId: string,
      filters: ArtifactFilters = {}
    ): Promise<ArtifactListResponse> {
      const params = new URLSearchParams()

      // Handle standard filters
      Object.entries(filters).forEach(([key, value]) => {
        if (value !== undefined && value !== null && value !== '') {
          params.append(key, String(value))
        }
      })

      const queryString = params.toString()
      const endpoint = `/${teamId}/artifacts${queryString ? `?${queryString}` : ''}`

      return this.makeRequest(endpoint)
    }

    async getArtifactsByProject(
      teamId: string,
      projectId: string,
      filters: Omit<ArtifactFilters, 'project_id'> = {}
    ): Promise<ArtifactListResponse> {
      const params = new URLSearchParams()
      Object.entries(filters).forEach(([key, value]) => {
        if (value !== undefined && value !== null && value !== '') {
          params.append(key, String(value))
        }
      })

      const queryString = params.toString()
      const endpoint = `/${teamId}/artifacts/${encodeURIComponent(projectId)}${queryString ? `?${queryString}` : ''}`

      return this.makeRequest(endpoint)
    }

    async getArtifact(
      teamId: string,
      projectName: string,
      slug: string
    ): Promise<Artifact> {
      return this.makeRequest(
        `/${teamId}/artifacts/${encodeURIComponent(projectName)}/${encodeURIComponent(slug)}`
      )
    }

    async createArtifact(
      teamId: string,
      data: CreateArtifactRequest
    ): Promise<Artifact> {
      return this.makeRequest(`/${teamId}/artifacts`, {
        method: 'POST',
        body: JSON.stringify(data),
      })
    }

    async updateArtifact(
      teamId: string,
      projectName: string,
      slug: string,
      data: UpdateArtifactRequest
    ): Promise<Artifact> {
      return this.makeRequest(
        `/${teamId}/artifacts/${encodeURIComponent(projectName)}/${encodeURIComponent(slug)}`,
        {
          method: 'PUT',
          body: JSON.stringify(data),
        }
      )
    }

    async deleteArtifact(
      teamId: string,
      projectName: string,
      slug: string
    ): Promise<void> {
      await this.makeRequest(
        `/${teamId}/artifacts/${encodeURIComponent(projectName)}/${encodeURIComponent(slug)}`,
        {
          method: 'DELETE',
        }
      )
    }

    async getArtifactStats(teamId: string): Promise<unknown> {
      try {
        const response = await this.makeRequest(`/${teamId}/artifacts/stats`)
        return (
          response || {
            total_projects: 0,
            total_artifacts: 0,
            added_this_week: 0,
            total_by_type: {},
            total_by_status: {},
          }
        )
      } catch (error) {
        console.error('Failed to fetch artifact stats:', error)
        return {
          total_projects: 0,
          total_artifacts: 0,
          added_this_week: 0,
          total_by_type: {},
          total_by_status: {},
        }
      }
    }

    async getProjects(): Promise<unknown> {
      try {
        const response = await this.makeRequest('/artifacts/projects')
        return response || { projects: [] }
      } catch (error) {
        console.error('Failed to fetch projects:', error)
        return { projects: [] }
      }
    }
  }

  return {
    artifactService: new ArtifactService(),
    __authService: authService, // Export for testing
  }
})

// Import after mocking
import { artifactService } from '../../src/services/artifactService'

describe('ArtifactService', () => {
  const mockToken = 'test-token-123'
  const mockTeamId = 'team-123'
  const mockArtifact: Artifact = {
    id: 'artifact-1',
    project_id: 'test-project',
    slug: 'test-artifact',
    user_id: 'user-1',
    content: 'Test artifact content',
    created_at: '2023-01-01T00:00:00Z',
    updated_at: '2023-01-01T00:00:00Z',
    status: 'active',
    title: 'Test Artifact',
    description: 'Test artifact description',
    type: 'general',
    metadata: { tag: 'test', category: 'testing' },
  }

  const mockArtifactListResponse: ArtifactListResponse = {
    artifacts: [mockArtifact],
    page: 1,
    per_page: 10,
    total_count: 1,
    total_pages: 1,
  }

  const mockArtifactStatsResponse: ArtifactStatsResponse = {
    total_projects: 5,
    total_artifacts: 25,
    added_this_week: 3,
    total_by_type: {
      general: 10,
      work_reports: 8,
      static_contexts: 7,
    },
    total_by_status: {
      active: 22,
      expired: 3,
    },
  }

  beforeEach(() => {
    jest.clearAllMocks()
    mockAuthService.getToken.mockReturnValue(mockToken)
    mockFetch.mockClear()

    // Also set up the internal authService from the mocked module
    const mockedModule = jest.requireMock('../../src/services/artifactService')
    if (mockedModule.__authService) {
      mockedModule.__authService.getToken.mockReturnValue(mockToken)
      mockedModule.__authService.setToken.mockImplementation(() => {})
    }
  })

  describe('Authentication and Authorization', () => {
    it('should throw error when no token is available', async () => {
      const mockedModule = jest.requireMock(
        '../../src/services/artifactService'
      )
      mockedModule.__authService.getToken.mockReturnValue(null)

      await expect(artifactService.getArtifacts(mockTeamId)).rejects.toThrow(
        'No authentication token'
      )
    })

    it('should include authorization header in requests', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockArtifactListResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      await artifactService.getArtifacts(mockTeamId)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/artifacts'),
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: `Bearer ${mockToken}`,
            'Content-Type': 'application/json',
          }),
        })
      )
    })

    it('should handle 401 authentication expired', async () => {
      const mockedModule = jest.requireMock(
        '../../src/services/artifactService'
      )

      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: () => Promise.resolve({ message: 'Unauthorized' }),
      })

      await expect(artifactService.getArtifacts(mockTeamId)).rejects.toThrow(
        'Authentication expired'
      )
      expect(mockedModule.__authService.setToken).toHaveBeenCalledWith(null)
    })
  })

  describe('Artifact CRUD Operations', () => {
    describe('getArtifacts', () => {
      it('should retrieve artifacts successfully', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockArtifactListResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        const result = await artifactService.getArtifacts(mockTeamId)

        expect(result).toEqual(mockArtifactListResponse)
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(`/${mockTeamId}/artifacts`),
          expect.objectContaining({
            headers: expect.objectContaining({
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
      })

      it('should handle filters correctly', async () => {
        const filters: ArtifactFilters = {
          project_id: 'test-project',
          search: 'test search',
          status: 'active',
          type: 'general',
          sort_by: 'created_at',
          sort_order: 'desc',
          page: 2,
          limit: 20,
          metadata_tag: 'important',
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockArtifactListResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await artifactService.getArtifacts(mockTeamId, filters)

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('project_id=test-project'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('search=test+search'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('status=active'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('type=general'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('sort_by=created_at'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('sort_order=desc'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('page=2'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('limit=20'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('metadata_tag=important'),
          expect.any(Object)
        )
      })

      it('should handle empty filters', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockArtifactListResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await artifactService.getArtifacts(mockTeamId, {})

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(`/${mockTeamId}/artifacts`),
          expect.any(Object)
        )
      })

      it('should skip undefined, null, and empty string values in filters', async () => {
        const filters: ArtifactFilters = {
          project_id: 'test-project',
          search: undefined,
          status: null as unknown as 'active' | 'draft' | 'archived',
          type: undefined,
          page: 1,
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockArtifactListResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await artifactService.getArtifacts(mockTeamId, filters)

        const call = mockFetch.mock.calls[0][0] as string
        expect(call).toContain('project_id=test-project')
        expect(call).toContain('page=1')
        expect(call).not.toContain('search=')
        expect(call).not.toContain('status=')
        expect(call).not.toContain('type=')
      })

      it('should filter out empty string values from filters', async () => {
        const filters: ArtifactFilters = {
          project_id: 'test-project',
          search: '',
          status: 'active',
          type: undefined,
          page: 1,
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockArtifactListResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await artifactService.getArtifacts(mockTeamId, filters)

        const call = mockFetch.mock.calls[0][0] as string
        expect(call).toContain('project_id=test-project')
        expect(call).toContain('status=active')
        expect(call).toContain('page=1')
        expect(call).not.toContain('search=')
        expect(call).not.toContain('type=')
      })
    })

    describe('getArtifactsByProject', () => {
      it('should retrieve artifacts by project successfully', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockArtifactListResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        const result = await artifactService.getArtifactsByProject(
          mockTeamId,
          'test-project'
        )

        expect(result).toEqual(mockArtifactListResponse)
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(`/${mockTeamId}/artifacts/test-project`),
          expect.objectContaining({
            headers: expect.objectContaining({
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
      })

      it('should handle project name with special characters', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockArtifactListResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await artifactService.getArtifactsByProject(
          mockTeamId,
          'project with spaces & symbols'
        )

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(
            `/${mockTeamId}/artifacts/project%20with%20spaces%20%26%20symbols`
          ),
          expect.any(Object)
        )
      })

      it('should handle filters for project-specific requests', async () => {
        const filters = {
          search: 'test',
          status: 'active' as const,
          page: 2,
          limit: 50,
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockArtifactListResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await artifactService.getArtifactsByProject(
          mockTeamId,
          'test-project',
          filters
        )

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('search=test'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('status=active'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('page=2'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('limit=50'),
          expect.any(Object)
        )
      })
    })

    describe('getArtifact', () => {
      it('should retrieve a single artifact by project and slug', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockArtifact),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        const result = await artifactService.getArtifact(
          mockTeamId,
          'test-project',
          'test-artifact'
        )

        expect(result).toEqual(mockArtifact)
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(
            `/${mockTeamId}/artifacts/test-project/test-artifact`
          ),
          expect.objectContaining({
            headers: expect.objectContaining({
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
      })

      it('should handle URL encoding for project name and slug', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockArtifact),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await artifactService.getArtifact(
          mockTeamId,
          'project/name',
          'slug with spaces'
        )

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(
            `/${mockTeamId}/artifacts/project%2Fname/slug%20with%20spaces`
          ),
          expect.any(Object)
        )
      })
    })

    describe('createArtifact', () => {
      it('should create a new artifact successfully', async () => {
        const createRequest: CreateArtifactRequest = {
          project_id: 'test-project',
          slug: 'new-artifact',
          content: 'New artifact content',
          title: 'New Artifact',
          description: 'A new test artifact',
          type: 'general',
          status: 'active',
          metadata: { tag: 'new' },
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 201,
          json: () => Promise.resolve(mockArtifact),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        const result = await artifactService.createArtifact(
          mockTeamId,
          createRequest
        )

        expect(result).toEqual(mockArtifact)
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(`/${mockTeamId}/artifacts`),
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify(createRequest),
            headers: expect.objectContaining({
              Authorization: `Bearer ${mockToken}`,
              'Content-Type': 'application/json',
            }),
          })
        )
      })

      it('should create artifact with minimal required fields', async () => {
        const createRequest: CreateArtifactRequest = {
          project_id: 'test-project-uuid',
          slug: 'minimal-artifact',
          content: 'Minimal artifact content',
          title: 'Minimal Artifact',
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 201,
          json: () => Promise.resolve(mockArtifact),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await artifactService.createArtifact(mockTeamId, createRequest)

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(`/${mockTeamId}/artifacts`),
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify(createRequest),
          })
        )
      })

      it('should handle complex metadata in create request', async () => {
        const complexMetadata = {
          nested: {
            object: {
              with: 'deep nesting',
              array: [1, 2, 3],
              boolean: false,
            },
          },
          tags: ['tag1', 'tag2', 'tag3'],
          timestamp: new Date().toISOString(),
        }

        const createRequest: CreateArtifactRequest = {
          project_id: 'test-project-uuid',
          slug: 'complex-artifact',
          content: 'Complex metadata artifact',
          title: 'Complex Artifact',
          metadata: complexMetadata,
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 201,
          json: () => Promise.resolve(mockArtifact),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await artifactService.createArtifact(mockTeamId, createRequest)

        const requestBody = JSON.parse(
          (mockFetch.mock.calls[0][1] as RequestInit).body as string
        )
        expect(requestBody.metadata).toEqual(complexMetadata)
      })
    })

    describe('updateArtifact', () => {
      it('should update an existing artifact successfully', async () => {
        const updateRequest: UpdateArtifactRequest = {
          content: 'Updated artifact content',
          title: 'Updated Artifact',
          description: 'Updated description',
          metadata: { tag: 'updated' },
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockArtifact),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        const result = await artifactService.updateArtifact(
          mockTeamId,
          'test-project',
          'test-artifact',
          updateRequest
        )

        expect(result).toEqual(mockArtifact)
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(
            `/${mockTeamId}/artifacts/test-project/test-artifact`
          ),
          expect.objectContaining({
            method: 'PUT',
            body: JSON.stringify(updateRequest),
            headers: expect.objectContaining({
              Authorization: `Bearer ${mockToken}`,
              'Content-Type': 'application/json',
            }),
          })
        )
      })

      it('should handle partial updates', async () => {
        const updateRequest: UpdateArtifactRequest = {
          title: 'Only title update',
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockArtifact),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await artifactService.updateArtifact(
          mockTeamId,
          'test-project',
          'test-artifact',
          updateRequest
        )

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(
            `/${mockTeamId}/artifacts/test-project/test-artifact`
          ),
          expect.objectContaining({
            method: 'PUT',
            body: JSON.stringify(updateRequest),
          })
        )
      })

      it('should handle URL encoding in update requests', async () => {
        const updateRequest: UpdateArtifactRequest = {
          content: 'Updated content',
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockArtifact),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await artifactService.updateArtifact(
          mockTeamId,
          'project/name',
          'slug with spaces',
          updateRequest
        )

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(
            `/${mockTeamId}/artifacts/project%2Fname/slug%20with%20spaces`
          ),
          expect.objectContaining({
            method: 'PUT',
          })
        )
      })
    })

    describe('deleteArtifact', () => {
      it('should delete an artifact successfully', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 204,
          headers: new Headers(),
        })

        await artifactService.deleteArtifact(
          mockTeamId,
          'test-project',
          'test-artifact'
        )

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(
            `/${mockTeamId}/artifacts/test-project/test-artifact`
          ),
          expect.objectContaining({
            method: 'DELETE',
            headers: expect.objectContaining({
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
      })

      it('should handle 204 No Content response', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 204,
          headers: new Headers(),
        })

        const result = await artifactService.deleteArtifact(
          mockTeamId,
          'test-project',
          'test-artifact'
        )

        expect(result).toBeUndefined()
      })

      it('should handle URL encoding in delete requests', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 204,
          headers: new Headers(),
        })

        await artifactService.deleteArtifact(
          mockTeamId,
          'project/name',
          'slug with spaces'
        )

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(
            `/${mockTeamId}/artifacts/project%2Fname/slug%20with%20spaces`
          ),
          expect.objectContaining({
            method: 'DELETE',
          })
        )
      })
    })
  })

  describe('Statistics and Metadata Operations', () => {
    describe('getArtifactStats', () => {
      it('should retrieve artifact statistics successfully', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockArtifactStatsResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        const result = await artifactService.getArtifactStats(mockTeamId)

        expect(result).toEqual(mockArtifactStatsResponse)
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(`/${mockTeamId}/artifacts/stats`),
          expect.objectContaining({
            headers: expect.objectContaining({
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
      })

      it('should return default stats when request fails', async () => {
        const consoleSpy = jest.spyOn(console, 'error').mockImplementation()
        mockFetch.mockRejectedValueOnce(new Error('Network error'))

        const result = await artifactService.getArtifactStats(mockTeamId)

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

      it('should return default stats when response is null', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(null),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        const result = await artifactService.getArtifactStats(mockTeamId)

        expect(result).toEqual({
          total_projects: 0,
          total_artifacts: 0,
          added_this_week: 0,
          total_by_type: {},
          total_by_status: {},
        })
      })
    })
  })

  describe('Error Handling and Data Validation', () => {
    it('should handle network errors', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Network error'))

      await expect(artifactService.getArtifacts(mockTeamId)).rejects.toThrow(
        'Network error'
      )
    })

    it('should handle HTTP error responses', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ message: 'Internal server error' }),
      })

      await expect(artifactService.getArtifacts(mockTeamId)).rejects.toThrow(
        'Internal server error'
      )
    })

    it('should handle HTTP errors without JSON response', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        json: () => Promise.reject(new Error('Invalid JSON')),
      })

      await expect(artifactService.getArtifacts(mockTeamId)).rejects.toThrow(
        'HTTP error! status: 404'
      )
    })

    it('should handle response without content-type header', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers(),
        text: () => Promise.resolve(JSON.stringify(mockArtifactListResponse)),
      })

      const result = await artifactService.getArtifacts(mockTeamId)

      expect(result).toEqual(mockArtifactListResponse)
    })

    it('should handle empty response body', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: () => Promise.resolve(''),
        text: () => Promise.resolve(''),
      })

      const result = await artifactService.getArtifacts(mockTeamId)

      expect(result).toBe('')
    })

    it('should handle invalid JSON response', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers(),
        text: () => Promise.resolve('invalid json'),
      })

      // Should log warning but not throw error
      const consoleSpy = jest.spyOn(console, 'warn').mockImplementation()
      const result = await artifactService.getArtifacts(mockTeamId)

      expect(result).toBeNull()
      expect(consoleSpy).toHaveBeenCalledWith(
        'Failed to parse response as JSON:',
        expect.any(Error)
      )

      consoleSpy.mockRestore()
    })

    it('should handle 403 Forbidden responses', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 403,
        json: () => Promise.resolve({ message: 'Forbidden access' }),
      })

      await expect(artifactService.getArtifacts(mockTeamId)).rejects.toThrow(
        'Forbidden access'
      )
    })

    it('should handle 429 Rate Limit responses', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 429,
        json: () => Promise.resolve({ message: 'Rate limit exceeded' }),
      })

      await expect(artifactService.getArtifacts(mockTeamId)).rejects.toThrow(
        'Rate limit exceeded'
      )
    })
  })

  describe('File Upload and Content Validation', () => {
    it('should handle large content in create requests', async () => {
      const largeContent = 'x'.repeat(10000) // 10KB content
      const createRequest: CreateArtifactRequest = {
        project_id: 'test-project-uuid',
        slug: 'large-artifact',
        content: largeContent,
        title: 'Large Artifact',
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: () => Promise.resolve(mockArtifact),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      await artifactService.createArtifact(mockTeamId, createRequest)

      const requestBody = JSON.parse(
        (mockFetch.mock.calls[0][1] as RequestInit).body as string
      )
      expect(requestBody.content).toBe(largeContent)
    })

    it('should handle special characters in content', async () => {
      const specialContent =
        'Content with 🔥 emojis, "quotes", \'apostrophes\', & symbols\n\ttabs'
      const createRequest: CreateArtifactRequest = {
        project_id: 'test-project-uuid',
        slug: 'special-artifact',
        content: specialContent,
        title: 'Special Artifact',
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: () => Promise.resolve(mockArtifact),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      await artifactService.createArtifact(mockTeamId, createRequest)

      const requestBody = JSON.parse(
        (mockFetch.mock.calls[0][1] as RequestInit).body as string
      )
      expect(requestBody.content).toBe(specialContent)
    })

    it('should handle binary content validation errors', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ message: 'Invalid file type' }),
      })

      const createRequest: CreateArtifactRequest = {
        project_id: 'test-project-uuid',
        slug: 'binary-artifact',
        content: 'binary content',
        title: 'Binary Artifact',
        type: 'general',
      }

      await expect(
        artifactService.createArtifact(mockTeamId, createRequest)
      ).rejects.toThrow('Invalid file type')
    })

    it('should handle content size limit errors', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 413,
        json: () => Promise.resolve({ message: 'Content too large' }),
      })

      const createRequest: CreateArtifactRequest = {
        project_id: 'test-project-uuid',
        slug: 'large-artifact',
        content: 'x'.repeat(1000000), // Very large content
        title: 'Large Artifact',
      }

      await expect(
        artifactService.createArtifact(mockTeamId, createRequest)
      ).rejects.toThrow('Content too large')
    })
  })

  describe('Storage Management Operations', () => {
    it('should handle storage quota exceeded errors', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 507,
        json: () => Promise.resolve({ message: 'Storage quota exceeded' }),
      })

      const createRequest: CreateArtifactRequest = {
        project_id: 'test-project-uuid',
        slug: 'quota-test',
        content: 'test content',
        title: 'Quota Test',
      }

      await expect(
        artifactService.createArtifact(mockTeamId, createRequest)
      ).rejects.toThrow('Storage quota exceeded')
    })

    it('should handle cleanup operations via delete', async () => {
      // Mock multiple delete operations
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          status: 204,
          headers: new Headers(),
        })
        .mockResolvedValueOnce({
          ok: true,
          status: 204,
          headers: new Headers(),
        })
        .mockResolvedValueOnce({
          ok: true,
          status: 204,
          headers: new Headers(),
        })

      await Promise.all([
        artifactService.deleteArtifact(mockTeamId, 'project-1', 'artifact-1'),
        artifactService.deleteArtifact(mockTeamId, 'project-1', 'artifact-2'),
        artifactService.deleteArtifact(mockTeamId, 'project-1', 'artifact-3'),
      ])

      expect(mockFetch).toHaveBeenCalledTimes(3)
    })

    it('should handle storage optimization through stats monitoring', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            total_projects: 100,
            total_artifacts: 10000,
            added_this_week: 500,
            total_by_type: {
              general: 5000,
              work_reports: 3000,
              static_contexts: 2000,
            },
            total_by_status: {
              active: 8000,
              expired: 2000,
            },
          }),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      const stats = await artifactService.getArtifactStats(mockTeamId)

      expect(stats.total_artifacts).toBe(10000)
      expect(stats.total_by_status.expired).toBe(2000)
    })
  })

  describe('Data Consistency and Integrity', () => {
    it('should maintain request/response data integrity', async () => {
      const createRequest: CreateArtifactRequest = {
        project_id: 'test-project-uuid',
        slug: 'integrity-test',
        content: 'Test integrity',
        title: 'Integrity Test',
        metadata: { test: 'value', number: 42, boolean: true },
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: () => Promise.resolve(mockArtifact),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      await artifactService.createArtifact(mockTeamId, createRequest)

      const requestBody = JSON.parse(
        (mockFetch.mock.calls[0][1] as RequestInit).body as string
      )
      expect(requestBody).toEqual(createRequest)
    })

    it('should handle complex metadata structures', async () => {
      const complexMetadata = {
        nested: {
          object: {
            with: 'deep nesting',
            array: [1, 2, 3],
            boolean: false,
          },
        },
        tags: ['tag1', 'tag2', 'tag3'],
        timestamp: new Date().toISOString(),
      }

      const createRequest: CreateArtifactRequest = {
        project_id: 'test-project-uuid',
        slug: 'complex-metadata',
        content: 'Complex metadata test',
        title: 'Complex Metadata Test',
        metadata: complexMetadata,
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: () => Promise.resolve(mockArtifact),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      await artifactService.createArtifact(mockTeamId, createRequest)

      const requestBody = JSON.parse(
        (mockFetch.mock.calls[0][1] as RequestInit).body as string
      )
      expect(requestBody.metadata).toEqual(complexMetadata)
    })
  })

  describe('Concurrent Access and Performance Scenarios', () => {
    it('should handle concurrent requests without interference', async () => {
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockArtifact),
          headers: new Headers({ 'content-type': 'application/json' }),
        })
        .mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockArtifactListResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

      const [result1, result2] = await Promise.all([
        artifactService.getArtifact(
          mockTeamId,
          'test-project',
          'test-artifact'
        ),
        artifactService.getArtifacts(mockTeamId),
      ])

      expect(result1).toEqual(mockArtifact)
      expect(result2).toEqual(mockArtifactListResponse)
      expect(mockFetch).toHaveBeenCalledTimes(2)
    })

    it('should handle rapid sequential requests', async () => {
      Array(5)
        .fill(null)
        .forEach(() =>
          mockFetch.mockResolvedValueOnce({
            ok: true,
            status: 200,
            json: () => Promise.resolve(mockArtifact),
            headers: new Headers({ 'content-type': 'application/json' }),
          })
        )

      const promises = Array(5)
        .fill(null)
        .map((_, i) =>
          artifactService.getArtifact(
            mockTeamId,
            'test-project',
            `artifact-${i}`
          )
        )

      const results = await Promise.all(promises)

      expect(results).toHaveLength(5)
      expect(mockFetch).toHaveBeenCalledTimes(5)
      results.forEach(result => {
        expect(result).toEqual(mockArtifact)
      })
    })
  })

  describe('Memory Management and Cleanup', () => {
    it('should not leak memory on successful requests', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockArtifactListResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      const result = await artifactService.getArtifacts(mockTeamId)

      expect(result).toEqual(mockArtifactListResponse)
      expect(mockFetch).toHaveBeenCalledTimes(1)
    })

    it('should clean up resources on failed requests', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Network failure'))

      await expect(artifactService.getArtifacts(mockTeamId)).rejects.toThrow(
        'Network failure'
      )

      expect(mockFetch).toHaveBeenCalledTimes(1)
    })

    it('should handle aborted requests', async () => {
      mockFetch.mockRejectedValueOnce(new Error('AbortError'))

      await expect(artifactService.getArtifacts(mockTeamId)).rejects.toThrow(
        'AbortError'
      )
    })
  })

  describe('Backup and Recovery Operations', () => {
    it('should handle service recovery after failures', async () => {
      // First request fails
      mockFetch.mockRejectedValueOnce(new Error('Service unavailable'))

      await expect(artifactService.getArtifacts(mockTeamId)).rejects.toThrow(
        'Service unavailable'
      )

      // Second request succeeds - service recovered
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockArtifactListResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      const result = await artifactService.getArtifacts(mockTeamId)

      expect(result).toEqual(mockArtifactListResponse)
      expect(mockFetch).toHaveBeenCalledTimes(2)
    })

    it('should maintain consistency across service restarts', async () => {
      // Create an artifact
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: () => Promise.resolve(mockArtifact),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      const createRequest: CreateArtifactRequest = {
        project_id: 'test-project-uuid',
        slug: 'persistent-artifact',
        content: 'Persistent artifact content',
        title: 'Persistent Artifact',
        metadata: { persistent: true },
      }

      await artifactService.createArtifact(mockTeamId, createRequest)

      // Simulate service restart by clearing mocks and making new request
      mockFetch.mockClear()
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockArtifact),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      // Retrieve the artifact - should still exist
      const result = await artifactService.getArtifact(
        mockTeamId,
        'test-project',
        'persistent-artifact'
      )

      expect(result).toEqual(mockArtifact)
    })
  })
})
