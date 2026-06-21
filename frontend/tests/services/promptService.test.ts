import type { ApiResponse } from '../../src/types/api'
import type {
  CreatePromptRequest,
  UpdatePromptRequest,
  PromptFilters,
  PromptsResponse,
  RenderPromptResponse,
  Prompt,
} from '../../src/types'

// The real promptService now returns a bare Prompt (issue #1325). This legacy
// suite exercises a standalone TestPromptService that still asserts the raw
// {status, message, data} envelope shape, so it keeps a local alias.
type PromptResponse = ApiResponse<Prompt>

// Mock the authService
const mockAuthService = {
  getToken: jest.fn(),
  setToken: jest.fn(),
}

// Mock fetch
const mockFetch = jest.fn()
global.fetch = mockFetch

// Define dependency types for the test
interface PromptDependencyItem {
  id: string
  slug: string
  name: string
}

interface PromptDependenciesResponse {
  used_by: PromptDependencyItem[]
  uses: PromptDependencyItem[]
  data?: {
    used_by: PromptDependencyItem[]
    uses: PromptDependencyItem[]
  }
}

// Create a test implementation of PromptService to avoid import.meta issues
class TestPromptService {
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

  async getPrompts(
    teamId: string,
    filters: PromptFilters = {}
  ): Promise<PromptsResponse> {
    const params = new URLSearchParams()

    // Remove team_id from query params - it's now in the URL path
    if (filters.status) params.append('status', filters.status)
    if (filters.search) params.append('search', filters.search)
    if (filters.page) params.append('page', filters.page.toString())
    if (filters.limit) params.append('limit', filters.limit.toString())

    const queryString = params.toString()
    const endpoint = `/${teamId}/prompts${queryString ? `?${queryString}` : ''}`

    return this.makeRequest<PromptsResponse>(endpoint)
  }

  async getPrompt(teamId: string, slug: string): Promise<PromptResponse> {
    return this.makeRequest<PromptResponse>(`/${teamId}/prompts/${slug}`)
  }

  async createPrompt(
    teamId: string,
    data: CreatePromptRequest
  ): Promise<PromptResponse> {
    return this.makeRequest<PromptResponse>(`/${teamId}/prompts`, {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async updatePrompt(
    teamId: string,
    slug: string,
    data: UpdatePromptRequest
  ): Promise<PromptResponse> {
    return this.makeRequest<PromptResponse>(`/${teamId}/prompts/${slug}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    })
  }

  async deletePrompt(teamId: string, slug: string): Promise<void> {
    await this.makeRequest<void>(`/${teamId}/prompts/${slug}`, {
      method: 'DELETE',
    })
    return
  }

  async getPromptPlaceholders(teamId: string, slug: string): Promise<string[]> {
    const response = await this.makeRequest<{ placeholders: string[] }>(
      `/${teamId}/prompts/${slug}/placeholders`
    )
    return response?.placeholders || []
  }

  async renderPrompt(
    teamId: string,
    slug: string,
    placeholders: Record<string, string>
  ): Promise<RenderPromptResponse> {
    return this.makeRequest<RenderPromptResponse>(
      `/${teamId}/prompts/${slug}/render`,
      {
        method: 'POST',
        body: JSON.stringify({ placeholders }),
      }
    )
  }

  async getPromptDependencies(
    teamId: string,
    slug: string
  ): Promise<{
    used_by: PromptDependencyItem[]
    uses: PromptDependencyItem[]
  }> {
    const response = await this.makeRequest<PromptDependenciesResponse>(
      `/${teamId}/prompts/${slug}/dependencies`
    )
    // Handle both wrapped {data: {...}} and unwrapped {...} responses
    const responseData = response?.data || response

    // Ensure we always return arrays, never null
    if (!responseData) {
      return { used_by: [], uses: [] }
    }

    return {
      used_by: responseData.used_by || [],
      uses: responseData.uses || [],
    }
  }
}

describe('PromptService', () => {
  let promptService: TestPromptService
  const mockToken = 'mock-auth-token'
  const mockTeamId = 'team-123'
  const baseUrl = 'https://api.vibexp.io/api/v1'

  beforeEach(() => {
    jest.clearAllMocks()
    promptService = new TestPromptService()
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

      await expect(promptService.getPrompts(mockTeamId)).rejects.toThrow(
        'No authentication token'
      )
      expect(mockFetch).not.toHaveBeenCalled()
    })

    it('should include Bearer token in request headers', async () => {
      const mockResponse = { data: { prompts: [], total_count: 0 } }
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      await promptService.getPrompts(mockTeamId)

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/prompts`,
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: `Bearer ${mockToken}`,
            'Content-Type': 'application/json',
          }),
        })
      )
    })

    it('should handle 401 authentication expired', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 401,
        json: () => Promise.resolve({ message: 'Unauthorized' }),
      } as Response)

      await expect(promptService.getPrompts(mockTeamId)).rejects.toThrow(
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

      await expect(promptService.getPrompts(mockTeamId)).rejects.toThrow(
        errorMessage
      )
    })

    it('should handle HTTP errors without JSON response', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.reject(new Error('Not JSON')),
      } as Response)

      await expect(promptService.getPrompts(mockTeamId)).rejects.toThrow(
        'HTTP error! status: 500'
      )
    })

    it('should handle 204 No Content responses', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 204,
        headers: new Headers(),
      } as Response)

      const result = await promptService.deletePrompt(mockTeamId, 'test-slug')
      expect(result).toBeUndefined()
    })

    it('should handle responses with no content-type header', async () => {
      const mockData = { id: '1', name: 'Test' }
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        headers: new Headers(),
        text: () => Promise.resolve(JSON.stringify(mockData)),
      } as Response)

      const result = await promptService.getPrompt(mockTeamId, 'test-slug')
      expect(result).toEqual(mockData)
    })

    it('should handle responses with invalid JSON', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        headers: new Headers(),
        text: () => Promise.resolve('invalid json'),
      } as Response)

      const consoleSpy = jest.spyOn(console, 'warn').mockImplementation()
      const result = await promptService.getPrompt(mockTeamId, 'test-slug')

      expect(result).toBeNull()
      expect(consoleSpy).toHaveBeenCalledWith(
        'Failed to parse response as JSON:',
        expect.any(Error)
      )
      consoleSpy.mockRestore()
    })
  })

  describe('CRUD Operations', () => {
    describe('getPrompts', () => {
      const mockPromptsResponse: PromptsResponse = {
        status: 'success',
        message: 'Prompts retrieved successfully',
        data: {
          prompts: [
            {
              id: '1',
              name: 'Test Prompt',
              slug: 'test-prompt',
              description: 'Test description',
              body: 'Test body',
              user_id: 'user1',
              project_id: 'project-1',
              status: 'published',
              created_at: '2023-01-01T00:00:00Z',
              updated_at: '2023-01-01T00:00:00Z',
            } as Prompt,
          ],
          page: 1,
          per_page: 10,
          total_count: 1,
          total_pages: 1,
        },
      }

      it('should fetch prompts without filters', async () => {
        mockFetch.mockResolvedValue({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockPromptsResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        } as Response)

        const result = await promptService.getPrompts(mockTeamId)

        expect(mockFetch).toHaveBeenCalledWith(
          `${baseUrl}/${mockTeamId}/prompts`,
          expect.objectContaining({
            headers: expect.objectContaining({
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
        expect(result).toEqual(mockPromptsResponse)
      })

      it('should fetch prompts with all filters', async () => {
        const filters: PromptFilters = {
          status: 'published',
          search: 'test query',
          page: 2,
          limit: 5,
        }

        mockFetch.mockResolvedValue({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockPromptsResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        } as Response)

        await promptService.getPrompts(mockTeamId, filters)

        expect(mockFetch).toHaveBeenCalledWith(
          `${baseUrl}/${mockTeamId}/prompts?status=published&search=test+query&page=2&limit=5`,
          expect.any(Object)
        )
      })

      it('should fetch prompts with partial filters', async () => {
        const filters: PromptFilters = {
          search: 'partial',
          page: 1,
        }

        mockFetch.mockResolvedValue({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockPromptsResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        } as Response)

        await promptService.getPrompts(mockTeamId, filters)

        expect(mockFetch).toHaveBeenCalledWith(
          `${baseUrl}/${mockTeamId}/prompts?search=partial&page=1`,
          expect.any(Object)
        )
      })
    })

    describe('getPrompt', () => {
      const mockPromptResponse: PromptResponse = {
        status: 'success',
        message: 'Prompt retrieved successfully',
        data: {
          id: '1',
          name: 'Test Prompt',
          slug: 'test-prompt',
          description: 'Test description',
          body: 'Test body with {{placeholder}}',
          user_id: 'user1',
          project_id: 'project-1',
          status: 'published',
          mcp_expose: true,
          is_shared: false,
          labels: [],
          created_at: '2023-01-01T00:00:00Z',
          updated_at: '2023-01-01T00:00:00Z',
          version: 1,
        },
      }

      it('should fetch a single prompt by slug', async () => {
        mockFetch.mockResolvedValue({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockPromptResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        } as Response)

        const result = await promptService.getPrompt(mockTeamId, 'test-prompt')

        expect(mockFetch).toHaveBeenCalledWith(
          `${baseUrl}/${mockTeamId}/prompts/test-prompt`,
          expect.objectContaining({
            headers: expect.objectContaining({
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
        expect(result).toEqual(mockPromptResponse)
      })

      it('should handle special characters in slug', async () => {
        const slug = 'test-prompt-with-special-chars-123'
        mockFetch.mockResolvedValue({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockPromptResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        } as Response)

        await promptService.getPrompt(mockTeamId, slug)

        expect(mockFetch).toHaveBeenCalledWith(
          `${baseUrl}/${mockTeamId}/prompts/${slug}`,
          expect.any(Object)
        )
      })
    })

    describe('createPrompt', () => {
      const createRequest: CreatePromptRequest = {
        name: 'New Prompt',
        slug: 'new-prompt',
        description: 'A new test prompt',
        body: 'Prompt body with {{variable}}',
        status: 'draft',
        project_id: 'project-1',
      }

      const mockCreateResponse: PromptResponse = {
        status: 'success',
        message: 'Prompt created successfully',
        data: {
          id: '2',
          ...createRequest,
          user_id: 'user1',
          created_at: '2023-01-01T00:00:00Z',
          updated_at: '2023-01-01T00:00:00Z',
        } as Prompt,
      }

      it('should create a new prompt', async () => {
        mockFetch.mockResolvedValue({
          ok: true,
          status: 201,
          json: () => Promise.resolve(mockCreateResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        } as Response)

        const result = await promptService.createPrompt(
          mockTeamId,
          createRequest
        )

        expect(mockFetch).toHaveBeenCalledWith(
          `${baseUrl}/${mockTeamId}/prompts`,
          expect.objectContaining({
            method: 'POST',
            headers: expect.objectContaining({
              Authorization: `Bearer ${mockToken}`,
              'Content-Type': 'application/json',
            }),
            body: JSON.stringify(createRequest),
          })
        )
        expect(result).toEqual(mockCreateResponse)
      })

      it('should create prompt with minimal data', async () => {
        const minimalRequest: CreatePromptRequest = {
          name: 'Minimal Prompt',
          slug: 'minimal',
          description: 'Minimal description',
          body: 'Minimal body',
          project_id: 'project-1',
        }

        mockFetch.mockResolvedValue({
          ok: true,
          status: 201,
          json: () => Promise.resolve(mockCreateResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        } as Response)

        await promptService.createPrompt(mockTeamId, minimalRequest)

        expect(mockFetch).toHaveBeenCalledWith(
          `${baseUrl}/${mockTeamId}/prompts`,
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify(minimalRequest),
          })
        )
      })
    })

    describe('updatePrompt', () => {
      const updateRequest: UpdatePromptRequest = {
        name: 'Updated Prompt',
        description: 'Updated description',
        body: 'Updated body with {{newVariable}}',
        status: 'published',
      }

      const mockUpdateResponse: PromptResponse = {
        status: 'success',
        message: 'Prompt updated successfully',
        data: {
          id: '1',
          slug: 'test-prompt',
          user_id: 'user1',
          project_id: 'project-1',
          created_at: '2023-01-01T00:00:00Z',
          updated_at: '2023-01-01T01:00:00Z',
          ...updateRequest,
        } as Prompt,
      }

      it('should update an existing prompt', async () => {
        mockFetch.mockResolvedValue({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockUpdateResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        } as Response)

        const result = await promptService.updatePrompt(
          mockTeamId,
          'test-prompt',
          updateRequest
        )

        expect(mockFetch).toHaveBeenCalledWith(
          `${baseUrl}/${mockTeamId}/prompts/test-prompt`,
          expect.objectContaining({
            method: 'PUT',
            headers: expect.objectContaining({
              Authorization: `Bearer ${mockToken}`,
              'Content-Type': 'application/json',
            }),
            body: JSON.stringify(updateRequest),
          })
        )
        expect(result).toEqual(mockUpdateResponse)
      })

      it('should update prompt with partial data', async () => {
        const partialUpdate: UpdatePromptRequest = {
          name: 'Only Name Updated',
        }

        mockFetch.mockResolvedValue({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockUpdateResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        } as Response)

        await promptService.updatePrompt(
          mockTeamId,
          'test-prompt',
          partialUpdate
        )

        expect(mockFetch).toHaveBeenCalledWith(
          `${baseUrl}/${mockTeamId}/prompts/test-prompt`,
          expect.objectContaining({
            method: 'PUT',
            body: JSON.stringify(partialUpdate),
          })
        )
      })
    })

    describe('deletePrompt', () => {
      it('should delete a prompt', async () => {
        mockFetch.mockResolvedValue({
          ok: true,
          status: 204,
          headers: new Headers(),
        } as Response)

        const result = await promptService.deletePrompt(
          mockTeamId,
          'test-prompt'
        )

        expect(mockFetch).toHaveBeenCalledWith(
          `${baseUrl}/${mockTeamId}/prompts/test-prompt`,
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
          json: () => Promise.resolve({ message: 'Prompt not found' }),
        } as Response)

        await expect(
          promptService.deletePrompt(mockTeamId, 'nonexistent-prompt')
        ).rejects.toThrow('Prompt not found')
      })
    })
  })

  describe('Template Processing', () => {
    describe('getPromptPlaceholders', () => {
      it('should fetch placeholders for a prompt', async () => {
        const mockPlaceholders = ['name', 'email', 'company']
        const mockResponse = { placeholders: mockPlaceholders }

        mockFetch.mockResolvedValue({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        } as Response)

        const result = await promptService.getPromptPlaceholders(
          mockTeamId,
          'test-prompt'
        )

        expect(mockFetch).toHaveBeenCalledWith(
          `${baseUrl}/${mockTeamId}/prompts/test-prompt/placeholders`,
          expect.objectContaining({
            headers: expect.objectContaining({
              Authorization: `Bearer ${mockToken}`,
            }),
          })
        )
        expect(result).toEqual(mockPlaceholders)
      })

      it('should return empty array when no placeholders response', async () => {
        mockFetch.mockResolvedValue({
          ok: true,
          status: 200,
          json: () => Promise.resolve(null),
          headers: new Headers({ 'content-type': 'application/json' }),
        } as Response)

        const result = await promptService.getPromptPlaceholders(
          mockTeamId,
          'test-prompt'
        )
        expect(result).toEqual([])
      })

      it('should return empty array when placeholders is undefined', async () => {
        mockFetch.mockResolvedValue({
          ok: true,
          status: 200,
          json: () => Promise.resolve({}),
          headers: new Headers({ 'content-type': 'application/json' }),
        } as Response)

        const result = await promptService.getPromptPlaceholders(
          mockTeamId,
          'test-prompt'
        )
        expect(result).toEqual([])
      })
    })

    describe('renderPrompt', () => {
      const mockRenderResponse: RenderPromptResponse = {
        rendered_body: 'Hello John, welcome to Acme Corp!',
        placeholders_missing: [],
        references_used: ['name', 'company'],
      }

      it('should render a prompt with placeholders', async () => {
        const placeholders = {
          name: 'John',
          company: 'Acme Corp',
        }

        mockFetch.mockResolvedValue({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockRenderResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        } as Response)

        const result = await promptService.renderPrompt(
          mockTeamId,
          'test-prompt',
          placeholders
        )

        expect(mockFetch).toHaveBeenCalledWith(
          `${baseUrl}/${mockTeamId}/prompts/test-prompt/render`,
          expect.objectContaining({
            method: 'POST',
            headers: expect.objectContaining({
              Authorization: `Bearer ${mockToken}`,
              'Content-Type': 'application/json',
            }),
            body: JSON.stringify({ placeholders }),
          })
        )
        expect(result).toEqual(mockRenderResponse)
      })

      it('should handle empty placeholders', async () => {
        const placeholders = {}

        mockFetch.mockResolvedValue({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockRenderResponse),
          headers: new Headers({ 'content-type': 'application/json' }),
        } as Response)

        await promptService.renderPrompt(
          mockTeamId,
          'test-prompt',
          placeholders
        )

        expect(mockFetch).toHaveBeenCalledWith(
          `${baseUrl}/${mockTeamId}/prompts/test-prompt/render`,
          expect.objectContaining({
            body: JSON.stringify({ placeholders: {} }),
          })
        )
      })

      it('should handle render errors with missing placeholders', async () => {
        const renderError: RenderPromptResponse = {
          rendered_body: 'Hello {{name}}, welcome to {{company}}!',
          placeholders_missing: ['name', 'company'],
          references_used: [],
        }

        mockFetch.mockResolvedValue({
          ok: true,
          status: 200,
          json: () => Promise.resolve(renderError),
          headers: new Headers({ 'content-type': 'application/json' }),
        } as Response)

        const result = await promptService.renderPrompt(
          mockTeamId,
          'test-prompt',
          {}
        )
        expect(result.placeholders_missing).toEqual(['name', 'company'])
      })
    })
  })

  describe('Data Validation', () => {
    it('should validate required fields in createPrompt', async () => {
      const invalidRequest = {
        name: '',
        slug: '',
        description: '',
        body: '',
      } as CreatePromptRequest

      mockFetch.mockResolvedValue({
        ok: false,
        status: 400,
        json: () =>
          Promise.resolve({ message: 'Validation error: name is required' }),
      } as Response)

      await expect(
        promptService.createPrompt(mockTeamId, invalidRequest)
      ).rejects.toThrow('Validation error: name is required')
    })

    it('should validate data types in updatePrompt', async () => {
      const invalidUpdate = {
        status: 'invalid-status' as unknown,
      } as UpdatePromptRequest

      mockFetch.mockResolvedValue({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ message: 'Invalid status value' }),
      } as Response)

      await expect(
        promptService.updatePrompt(mockTeamId, 'test-prompt', invalidUpdate)
      ).rejects.toThrow('Invalid status value')
    })

    it('should handle server-side validation errors', async () => {
      const requestWithExistingSlug: CreatePromptRequest = {
        name: 'Test Prompt',
        slug: 'existing-slug',
        description: 'Test description',
        body: 'Test body',
        project_id: 'project-1',
      }

      mockFetch.mockResolvedValue({
        ok: false,
        status: 409,
        json: () => Promise.resolve({ message: 'Slug already exists' }),
      } as Response)

      await expect(
        promptService.createPrompt(mockTeamId, requestWithExistingSlug)
      ).rejects.toThrow('Slug already exists')
    })
  })

  describe('Network and Edge Cases', () => {
    it('should handle network errors', async () => {
      mockFetch.mockRejectedValue(new Error('Network error'))

      await expect(promptService.getPrompts(mockTeamId)).rejects.toThrow(
        'Network error'
      )
    })

    it('should handle timeout errors', async () => {
      mockFetch.mockRejectedValue(new Error('Request timeout'))

      await expect(
        promptService.getPrompt(mockTeamId, 'test-slug')
      ).rejects.toThrow('Request timeout')
    })

    it('should handle malformed URLs in slug', async () => {
      const malformedSlug = 'test/prompt%20with%20special'
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ data: {} }),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      await promptService.getPrompt(mockTeamId, malformedSlug)

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/prompts/${malformedSlug}`,
        expect.any(Object)
      )
    })

    it('should handle empty response bodies', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        headers: new Headers(),
        text: () => Promise.resolve(''),
      } as Response)

      const result = await promptService.getPrompt(mockTeamId, 'test-slug')
      expect(result).toBeNull()
    })
  })

  describe('getPromptDependencies', () => {
    it('should fetch prompt dependencies successfully', async () => {
      const slug = 'test-prompt'
      const mockDependencies = {
        used_by: [
          { id: 'dep-1', slug: 'dependent-1', name: 'Dependent Prompt 1' },
          { id: 'dep-2', slug: 'dependent-2', name: 'Dependent Prompt 2' },
        ],
        uses: [
          { id: 'ref-1', slug: 'referenced-1', name: 'Referenced Prompt 1' },
        ],
      }

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockDependencies),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await promptService.getPromptDependencies(mockTeamId, slug)

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/${mockTeamId}/prompts/${slug}/dependencies`,
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: `Bearer ${mockToken}`,
          }),
        })
      )

      expect(result).toEqual(mockDependencies)
      expect(result.used_by).toHaveLength(2)
      expect(result.uses).toHaveLength(1)
    })

    it('should return empty arrays when no dependencies exist', async () => {
      const slug = 'test-prompt'
      const mockEmptyDependencies = {
        used_by: [],
        uses: [],
      }

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockEmptyDependencies),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await promptService.getPromptDependencies(mockTeamId, slug)

      expect(result.used_by).toEqual([])
      expect(result.uses).toEqual([])
      expect(result.used_by).not.toBeNull()
      expect(result.uses).not.toBeNull()
    })

    it('should handle null arrays and return empty arrays', async () => {
      const slug = 'test-prompt'
      const mockNullDependencies = {
        used_by: null,
        uses: null,
      }

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockNullDependencies),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await promptService.getPromptDependencies(mockTeamId, slug)

      // Should convert null to empty arrays
      expect(result.used_by).toEqual([])
      expect(result.uses).toEqual([])
    })

    it('should handle wrapped API response', async () => {
      const slug = 'test-prompt'
      const mockWrappedResponse = {
        data: {
          used_by: [{ id: 'dep-1', slug: 'dependent-1', name: 'Dependent 1' }],
          uses: [],
        },
      }

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockWrappedResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await promptService.getPromptDependencies(mockTeamId, slug)

      expect(result.used_by).toHaveLength(1)
      expect(result.used_by[0].slug).toBe('dependent-1')
      expect(result.uses).toEqual([])
    })

    it('should handle unwrapped API response', async () => {
      const slug = 'test-prompt'
      const mockUnwrappedResponse = {
        used_by: [],
        uses: [{ id: 'ref-1', slug: 'referenced-1', name: 'Referenced 1' }],
      }

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockUnwrappedResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await promptService.getPromptDependencies(mockTeamId, slug)

      expect(result.used_by).toEqual([])
      expect(result.uses).toHaveLength(1)
      expect(result.uses[0].slug).toBe('referenced-1')
    })

    it('should handle 404 error when prompt not found', async () => {
      const slug = 'non-existent'

      mockFetch.mockResolvedValue({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ message: 'Prompt not found' }),
      } as Response)

      await expect(
        promptService.getPromptDependencies(mockTeamId, slug)
      ).rejects.toThrow('Prompt not found')
    })

    it('should handle network errors', async () => {
      mockFetch.mockRejectedValue(new Error('Network error'))

      await expect(
        promptService.getPromptDependencies(mockTeamId, 'test-slug')
      ).rejects.toThrow('Network error')
    })

    it('should handle null response', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(null),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await promptService.getPromptDependencies(
        mockTeamId,
        'test-slug'
      )

      expect(result.used_by).toEqual([])
      expect(result.uses).toEqual([])
    })
  })
})
