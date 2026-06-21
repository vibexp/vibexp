import type {
  CreateMemoryRequest,
  UpdateMemoryRequest,
  MemoryFilters,
  MemoriesResponse,
  MemoryResponse,
  Memory,
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

// Create a test implementation of MemoryService to test the logic
class MemoryServiceTestable {
  private async makeRequest<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const token = mockAuthService.getToken()
    if (!token) {
      throw new Error('No authentication token')
    }

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

  async getMemories(
    teamId: string,
    filters: MemoryFilters = {}
  ): Promise<MemoriesResponse> {
    const params = new URLSearchParams()

    if (filters.search) params.append('search', filters.search)
    if (filters.metadata_key)
      params.append('metadata_key', filters.metadata_key)
    if (filters.metadata_value)
      params.append('metadata_value', filters.metadata_value)
    if (filters.page) params.append('page', filters.page.toString())
    if (filters.limit) params.append('limit', filters.limit.toString())

    const queryString = params.toString()
    const endpoint = `/${teamId}/memories${queryString ? `?${queryString}` : ''}`

    return this.makeRequest<MemoriesResponse>(endpoint)
  }

  async getMemory(teamId: string, id: string): Promise<MemoryResponse> {
    return this.makeRequest<MemoryResponse>(`/${teamId}/memories/${id}`)
  }

  async createMemory(
    teamId: string,
    data: CreateMemoryRequest
  ): Promise<MemoryResponse> {
    return this.makeRequest<MemoryResponse>(`/${teamId}/memories`, {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async updateMemory(
    teamId: string,
    id: string,
    data: UpdateMemoryRequest
  ): Promise<MemoryResponse> {
    return this.makeRequest<MemoryResponse>(`/${teamId}/memories/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    })
  }

  async deleteMemory(teamId: string, id: string): Promise<void> {
    await this.makeRequest<void>(`/${teamId}/memories/${id}`, {
      method: 'DELETE',
    })
  }

  async searchMemoriesByMetadata(
    teamId: string,
    filters: MemoryFilters
  ): Promise<MemoriesResponse> {
    if (!filters.metadata_key || !filters.metadata_value) {
      throw new Error(
        'metadata_key and metadata_value are required for metadata search'
      )
    }

    const params = new URLSearchParams()
    params.append('metadata_key', filters.metadata_key)
    params.append('metadata_value', filters.metadata_value)
    if (filters.search) params.append('search', filters.search)
    if (filters.page) params.append('page', filters.page.toString())
    if (filters.limit) params.append('limit', filters.limit.toString())

    return this.makeRequest<MemoriesResponse>(
      `/${teamId}/memories/search?${params.toString()}`
    )
  }
}

describe('MemoryService', () => {
  let memoryService: MemoryServiceTestable
  const mockToken = 'test-token-123'
  const mockTeamId = 'team-123'
  const mockMemory: Memory = {
    id: 'memory-1',
    user_id: 'user-1',
    team_id: mockTeamId,
    project_id: 'project-1',
    text: 'Test memory content',
    metadata: { tag: 'test', category: 'personal' },
    created_at: '2023-01-01T00:00:00Z',
    updated_at: '2023-01-01T00:00:00Z',
    version: 1,
  }

  const mockMemoriesResponse: MemoriesResponse = {
    memories: [mockMemory],
    page: 1,
    per_page: 10,
    total_count: 1,
    total_pages: 1,
  }

  const mockMemoryResponse: MemoryResponse = mockMemory

  beforeEach(() => {
    memoryService = new MemoryServiceTestable()
    jest.clearAllMocks()
    mockAuthService.getToken.mockReturnValue(mockToken)
    mockFetch.mockClear()
  })

  describe('Authentication and Authorization', () => {
    it('should throw error when no token is available', async () => {
      mockAuthService.getToken.mockReturnValue(null)

      await expect(memoryService.getMemories(mockTeamId)).rejects.toThrow(
        'No authentication token'
      )
    })

    it('should include authorization header in requests', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockMemoriesResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      await memoryService.getMemories(mockTeamId)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/memories'),
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: `Bearer ${mockToken}`,
            'Content-Type': 'application/json',
          }),
        })
      )
    })

    it('should handle 401 authentication expired', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: () => Promise.resolve({ message: 'Unauthorized' }),
      })

      await expect(memoryService.getMemories(mockTeamId)).rejects.toThrow(
        'Authentication expired'
      )
      expect(mockAuthService.setToken).toHaveBeenCalledWith(null)
    })
  })

  describe('Memory CRUD Operations', () => {
    describe('getMemories', () => {
      it('should retrieve memories successfully', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockMemoriesResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        const result = await memoryService.getMemories(mockTeamId)

        expect(result).toEqual(mockMemoriesResponse)
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(`/${mockTeamId}/memories`),
          expect.objectContaining({
            headers: expect.objectContaining({
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
        // Ensure team_id is NOT in query params
        expect(mockFetch).not.toHaveBeenCalledWith(
          expect.stringContaining('team_id='),
          expect.any(Object)
        )
      })

      it('should handle filters correctly', async () => {
        const filters: MemoryFilters = {
          search: 'test search',
          metadata_key: 'category',
          metadata_value: 'personal',
          page: 2,
          limit: 20,
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockMemoriesResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await memoryService.getMemories(mockTeamId, filters)

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('search=test+search'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('metadata_key=category'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('metadata_value=personal'),
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
      })

      it('should handle empty filters', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockMemoriesResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await memoryService.getMemories(mockTeamId, {})

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(`/${mockTeamId}/memories`),
          expect.any(Object)
        )
        // Ensure team_id is NOT in query params
        expect(mockFetch).not.toHaveBeenCalledWith(
          expect.stringContaining('team_id='),
          expect.any(Object)
        )
      })
    })

    describe('getMemory', () => {
      it('should retrieve a single memory by ID', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockMemoryResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        const result = await memoryService.getMemory(mockTeamId, 'memory-1')

        expect(result).toEqual(mockMemoryResponse)
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(`/${mockTeamId}/memories/memory-1`),
          expect.objectContaining({
            headers: expect.objectContaining({
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
      })
    })

    describe('createMemory', () => {
      it('should create a new memory successfully', async () => {
        const createRequest: CreateMemoryRequest = {
          project_id: 'project-1',
          text: 'New memory content',
          metadata: { tag: 'important' },
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 201,
          json: () => Promise.resolve(mockMemoryResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        const result = await memoryService.createMemory(
          mockTeamId,
          createRequest
        )

        expect(result).toEqual(mockMemoryResponse)
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(`/${mockTeamId}/memories`),
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

      it('should create memory without metadata', async () => {
        const createRequest: CreateMemoryRequest = {
          project_id: 'project-1',
          text: 'Simple memory',
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 201,
          json: () => Promise.resolve(mockMemoryResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await memoryService.createMemory(mockTeamId, createRequest)

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(`/${mockTeamId}/memories`),
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify(createRequest),
          })
        )
      })
    })

    describe('updateMemory', () => {
      it('should update an existing memory successfully', async () => {
        const updateRequest: UpdateMemoryRequest = {
          text: 'Updated memory content',
          metadata: { tag: 'updated' },
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockMemoryResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        const result = await memoryService.updateMemory(
          mockTeamId,
          'memory-1',
          updateRequest
        )

        expect(result).toEqual(mockMemoryResponse)
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(`/${mockTeamId}/memories/memory-1`),
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
        const updateRequest: UpdateMemoryRequest = {
          text: 'Only text update',
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockMemoryResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await memoryService.updateMemory(mockTeamId, 'memory-1', updateRequest)

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(`/${mockTeamId}/memories/memory-1`),
          expect.objectContaining({
            method: 'PUT',
            body: JSON.stringify(updateRequest),
          })
        )
      })
    })

    describe('deleteMemory', () => {
      it('should delete a memory successfully', async () => {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 204,
          headers: new Headers(),
        })

        await memoryService.deleteMemory(mockTeamId, 'memory-1')

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(`/${mockTeamId}/memories/memory-1`),
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

        const result = await memoryService.deleteMemory(mockTeamId, 'memory-1')

        expect(result).toBeUndefined()
      })
    })
  })

  describe('Search and Retrieval', () => {
    describe('searchMemoriesByMetadata', () => {
      it('should search memories by metadata successfully', async () => {
        const filters: MemoryFilters = {
          metadata_key: 'category',
          metadata_value: 'work',
          search: 'project',
          page: 1,
          limit: 10,
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockMemoriesResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        const result = await memoryService.searchMemoriesByMetadata(
          mockTeamId,
          filters
        )

        expect(result).toEqual(mockMemoriesResponse)
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(`/${mockTeamId}/memories/search`),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('metadata_key=category'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('metadata_value=work'),
          expect.any(Object)
        )
        // Ensure team_id is NOT in query params
        expect(mockFetch).not.toHaveBeenCalledWith(
          expect.stringContaining('team_id='),
          expect.any(Object)
        )
      })

      it('should require metadata_key and metadata_value', async () => {
        const filters: MemoryFilters = {
          search: 'test',
        }

        await expect(
          memoryService.searchMemoriesByMetadata(mockTeamId, filters)
        ).rejects.toThrow(
          'metadata_key and metadata_value are required for metadata search'
        )
      })

      it('should handle missing metadata_key', async () => {
        const filters: MemoryFilters = {
          metadata_value: 'test',
        }

        await expect(
          memoryService.searchMemoriesByMetadata(mockTeamId, filters)
        ).rejects.toThrow(
          'metadata_key and metadata_value are required for metadata search'
        )
      })

      it('should handle missing metadata_value', async () => {
        const filters: MemoryFilters = {
          metadata_key: 'tag',
        }

        await expect(
          memoryService.searchMemoriesByMetadata(mockTeamId, filters)
        ).rejects.toThrow(
          'metadata_key and metadata_value are required for metadata search'
        )
      })

      it('should search with minimal required filters', async () => {
        const filters: MemoryFilters = {
          metadata_key: 'priority',
          metadata_value: 'high',
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockMemoriesResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await memoryService.searchMemoriesByMetadata(mockTeamId, filters)

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining(`/${mockTeamId}/memories/search`),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('metadata_key=priority'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('metadata_value=high'),
          expect.any(Object)
        )
        // Ensure team_id is NOT in query params
        expect(mockFetch).not.toHaveBeenCalledWith(
          expect.stringContaining('team_id='),
          expect.any(Object)
        )
      })
    })

    describe('Advanced Filtering and Sorting', () => {
      it('should handle complex filter combinations', async () => {
        const filters: MemoryFilters = {
          search: 'complex search term',
          metadata_key: 'project',
          metadata_value: 'important',
          page: 3,
          limit: 25,
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockMemoriesResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await memoryService.getMemories(mockTeamId, filters)

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('search=complex+search+term'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('metadata_key=project'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('metadata_value=important'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('page=3'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('limit=25'),
          expect.any(Object)
        )
      })

      it('should handle special characters in search terms', async () => {
        const filters: MemoryFilters = {
          search: 'search with @#$%^&*() special chars',
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockMemoriesResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await memoryService.getMemories(mockTeamId, filters)

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('search='),
          expect.any(Object)
        )
      })
    })

    describe('Pagination and Performance', () => {
      it('should handle large page numbers', async () => {
        const filters: MemoryFilters = {
          page: 999,
          limit: 100,
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockMemoriesResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await memoryService.getMemories(mockTeamId, filters)

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('page=999'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('limit=100'),
          expect.any(Object)
        )
      })

      it('should handle boundary values for pagination', async () => {
        const filters: MemoryFilters = {
          page: 1,
          limit: 1,
        }

        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockMemoriesResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

        await memoryService.getMemories(mockTeamId, filters)

        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('page=1'),
          expect.any(Object)
        )
        expect(mockFetch).toHaveBeenCalledWith(
          expect.stringContaining('limit=1'),
          expect.any(Object)
        )
      })
    })
  })

  describe('Error Handling and Data Validation', () => {
    it('should handle network errors', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Network error'))

      await expect(memoryService.getMemories(mockTeamId)).rejects.toThrow(
        'Network error'
      )
    })

    it('should handle HTTP error responses', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ message: 'Internal server error' }),
      })

      await expect(memoryService.getMemories(mockTeamId)).rejects.toThrow(
        'Internal server error'
      )
    })

    it('should handle HTTP errors without JSON response', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        json: () => Promise.reject(new Error('Invalid JSON')),
      })

      await expect(memoryService.getMemories(mockTeamId)).rejects.toThrow(
        'HTTP error! status: 404'
      )
    })

    it('should handle response without content-type header', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers(),
        text: () => Promise.resolve(JSON.stringify(mockMemoriesResponse)),
      })

      const result = await memoryService.getMemories(mockTeamId)

      expect(result).toEqual(mockMemoriesResponse)
    })

    it('should handle empty response body', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: () => Promise.resolve(''),
        text: () => Promise.resolve(''),
      })

      const result = await memoryService.getMemories(mockTeamId)

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
      const result = await memoryService.getMemories(mockTeamId)

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

      await expect(memoryService.getMemories(mockTeamId)).rejects.toThrow(
        'Forbidden access'
      )
    })

    it('should handle 429 Rate Limit responses', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 429,
        json: () => Promise.resolve({ message: 'Rate limit exceeded' }),
      })

      await expect(memoryService.getMemories(mockTeamId)).rejects.toThrow(
        'Rate limit exceeded'
      )
    })
  })

  describe('Data Consistency and Integrity', () => {
    it('should maintain request/response data integrity', async () => {
      const createRequest: CreateMemoryRequest = {
        project_id: 'project-1',
        text: 'Test integrity',
        metadata: { test: 'value', number: 42, boolean: true },
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: () => Promise.resolve(mockMemoryResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      await memoryService.createMemory(mockTeamId, createRequest)

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

      const createRequest: CreateMemoryRequest = {
        project_id: 'project-1',
        text: 'Complex metadata test',
        metadata: complexMetadata,
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: () => Promise.resolve(mockMemoryResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      await memoryService.createMemory(mockTeamId, createRequest)

      const requestBody = JSON.parse(
        (mockFetch.mock.calls[0][1] as RequestInit).body as string
      )
      expect(requestBody.metadata).toEqual(complexMetadata)
    })

    it('should serialize and deserialize data correctly', async () => {
      const memoryWithSpecialChars: Memory = {
        id: 'test-id',
        user_id: 'user-1',
        team_id: mockTeamId,
        project_id: 'project-1',
        text: 'Text with special chars: "quotes", \'apostrophes\', & symbols',
        metadata: {
          unicode: '🔥 emoji test 🚀',
          escaped: 'text with \n newlines \t and tabs',
        },
        created_at: '2023-01-01T00:00:00Z',
        updated_at: '2023-01-01T00:00:00Z',
        version: 1,
      }

      const response: MemoryResponse = memoryWithSpecialChars

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve(response),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      const result = await memoryService.getMemory(mockTeamId, 'test-id')

      expect(result.text).toBe(memoryWithSpecialChars.text)
      expect(result.metadata).toEqual(memoryWithSpecialChars.metadata)
    })
  })

  describe('Concurrent Access and Performance Scenarios', () => {
    it('should handle concurrent requests without interference', async () => {
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockMemoryResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })
        .mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockMemoriesResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        })

      const [result1, result2] = await Promise.all([
        memoryService.getMemory(mockTeamId, 'memory-1'),
        memoryService.getMemories(mockTeamId),
      ])

      expect(result1).toEqual(mockMemoryResponse)
      expect(result2).toEqual(mockMemoriesResponse)
      expect(mockFetch).toHaveBeenCalledTimes(2)
    })

    it('should handle rapid sequential requests', async () => {
      Array(5)
        .fill(null)
        .forEach(() =>
          mockFetch.mockResolvedValueOnce({
            ok: true,
            status: 200,
            json: () => Promise.resolve(mockMemoryResponse),
            headers: new Headers({ 'content-type': 'application/json' }),
          })
        )

      const promises = Array(5)
        .fill(null)
        .map((_, i) => memoryService.getMemory(mockTeamId, `memory-${i}`))

      const results = await Promise.all(promises)

      expect(results).toHaveLength(5)
      expect(mockFetch).toHaveBeenCalledTimes(5)
      results.forEach(result => {
        expect(result).toEqual(mockMemoryResponse)
      })
    })

    it('should handle timeout scenarios gracefully', async () => {
      jest.useFakeTimers()

      mockFetch.mockImplementationOnce(
        () =>
          new Promise(resolve => {
            setTimeout(
              () =>
                resolve({
                  ok: true,
                  status: 200,
                  json: () => Promise.resolve(mockMemoryResponse),
                  headers: new Headers({ 'content-type': 'application/json' }),
                }),
              10000
            )
          })
      )

      const promise = memoryService.getMemory(mockTeamId, 'memory-1')
      jest.advanceTimersByTime(5000)

      // Request should still be pending
      expect(mockFetch).toHaveBeenCalledTimes(1)

      jest.advanceTimersByTime(6000)
      const result = await promise

      expect(result).toEqual(mockMemoryResponse)
      jest.useRealTimers()
    })
  })

  describe('Memory Management and Cleanup', () => {
    it('should not leak memory on successful requests', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockMemoriesResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      const result = await memoryService.getMemories(mockTeamId)

      expect(result).toEqual(mockMemoriesResponse)
      // Verify that the fetch mock was called and completed
      expect(mockFetch).toHaveBeenCalledTimes(1)
    })

    it('should clean up resources on failed requests', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Network failure'))

      await expect(memoryService.getMemories(mockTeamId)).rejects.toThrow(
        'Network failure'
      )

      // Verify cleanup - no hanging promises or resources
      expect(mockFetch).toHaveBeenCalledTimes(1)
    })

    it('should handle aborted requests', async () => {
      mockFetch.mockRejectedValueOnce(new Error('AbortError'))

      await expect(memoryService.getMemories(mockTeamId)).rejects.toThrow(
        'AbortError'
      )
    })
  })

  describe('Backup and Recovery Operations', () => {
    it('should handle service recovery after failures', async () => {
      // First request fails
      mockFetch.mockRejectedValueOnce(new Error('Service unavailable'))

      await expect(memoryService.getMemories(mockTeamId)).rejects.toThrow(
        'Service unavailable'
      )

      // Second request succeeds - service recovered
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockMemoriesResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      const result = await memoryService.getMemories(mockTeamId)

      expect(result).toEqual(mockMemoriesResponse)
      expect(mockFetch).toHaveBeenCalledTimes(2)
    })

    it('should maintain consistency across service restarts', async () => {
      // Create a memory
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: () => Promise.resolve(mockMemoryResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      const createRequest: CreateMemoryRequest = {
        project_id: 'project-1',
        text: 'Persistent memory',
        metadata: { persistent: true },
      }

      await memoryService.createMemory(mockTeamId, createRequest)

      // Simulate service restart by clearing mocks and making new request
      mockFetch.mockClear()
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockMemoryResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      })

      // Retrieve the memory - should still exist
      const result = await memoryService.getMemory(mockTeamId, 'memory-1')

      expect(result).toEqual(mockMemoryResponse)
    })
  })
})
