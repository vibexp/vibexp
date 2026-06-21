import type {
  PromptGalleryCategory,
  PromptGalleryListResponse,
  PromptGalleryTemplate,
} from '../../types'

// Mock fetch globally
const mockFetch = jest.fn()
global.fetch = mockFetch

// Mock authService
const mockAuthService = {
  getToken: jest.fn(),
  setToken: jest.fn(),
}

// Mock the promptGalleryService module
jest.mock('../promptGalleryService', () => {
  const API_BASE_URL = 'https://api.vibexp.io/api/v1'

  class PromptGalleryService {
    async getCategories(): Promise<PromptGalleryCategory[]> {
      const token = mockAuthService.getToken()
      if (!token) {
        throw new Error('No authentication token')
      }

      const response = await fetch(
        `${API_BASE_URL}/prompt-gallery/categories`,
        {
          headers: {
            'Content-Type': 'application/json',

            Authorization: `Bearer ${token}`,
          },
        }
      )

      if (!response.ok) {
        if (response.status === 401) {
          mockAuthService.setToken(null)
          throw new Error('Authentication expired')
        }
        throw new Error('Failed to get categories')
      }

      return response.json()
    }

    async getPrompts(
      filters: {
        category?: string
        search?: string
        tags?: string[]
        page?: number
        limit?: number
      } = {}
    ): Promise<PromptGalleryListResponse> {
      const token = mockAuthService.getToken()
      if (!token) {
        throw new Error('No authentication token')
      }

      const params = new URLSearchParams()
      if (filters.category) params.append('category', filters.category)
      if (filters.search) params.append('search', filters.search)
      if (filters.tags && filters.tags.length > 0) {
        params.append('tags', filters.tags.join(','))
      }
      if (filters.page) params.append('page', filters.page.toString())
      if (filters.limit) params.append('limit', filters.limit.toString())

      const queryString = params.toString()
      const endpoint = `${API_BASE_URL}/prompt-gallery/prompts${queryString ? `?${queryString}` : ''}`

      const response = await fetch(endpoint, {
        headers: {
          'Content-Type': 'application/json',

          Authorization: `Bearer ${token}`,
        },
      })

      if (!response.ok) {
        throw new Error('Failed to get prompts')
      }

      return response.json()
    }

    async getPromptById(id: string): Promise<PromptGalleryTemplate> {
      const token = mockAuthService.getToken()
      if (!token) {
        throw new Error('No authentication token')
      }

      const response = await fetch(
        `${API_BASE_URL}/prompt-gallery/prompts/${id}`,
        {
          headers: {
            'Content-Type': 'application/json',

            Authorization: `Bearer ${token}`,
          },
        }
      )

      if (!response.ok) {
        throw new Error('Failed to get prompt')
      }

      const data = await response.json()
      if (!data) {
        throw new Error('Prompt not found')
      }

      return data
    }

    async trackPromptUsage(promptId: string): Promise<void> {
      const token = mockAuthService.getToken()
      if (!token) {
        throw new Error('No authentication token')
      }

      const response = await fetch(
        `${API_BASE_URL}/prompt-gallery/prompts/${promptId}/use`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',

            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify({ prompt_id: promptId }),
        }
      )

      if (!response.ok) {
        if (response.status === 401) {
          mockAuthService.setToken(null)
          throw new Error('Authentication expired')
        }
        throw new Error('Failed to track usage')
      }
    }
  }

  return {
    promptGalleryService: new PromptGalleryService(),
  }
})

import { promptGalleryService } from '../promptGalleryService'

describe('PromptGalleryService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    mockAuthService.getToken.mockReturnValue('test-token')
  })

  describe('getCategories', () => {
    it('should fetch and return categories', async () => {
      const mockCategories = [
        { category: 'Engineering', count: 5 },
        { category: 'Marketing', count: 3 },
      ]

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        // eslint-disable-next-line @typescript-eslint/require-await
        json: async () => mockCategories,
      })

      const result = await promptGalleryService.getCategories()

      expect(result).toEqual(mockCategories)
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/prompt-gallery/categories'),
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: 'Bearer test-token',
          }),
        })
      )
    })

    it('should throw error when not authenticated', async () => {
      mockAuthService.getToken.mockReturnValue(null)

      await expect(promptGalleryService.getCategories()).rejects.toThrow(
        'No authentication token'
      )
    })

    it('should handle 401 unauthorized', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
      })

      await expect(promptGalleryService.getCategories()).rejects.toThrow(
        'Authentication expired'
      )
      expect(mockAuthService.setToken).toHaveBeenCalledWith(null)
    })
  })

  describe('getPrompts', () => {
    it('should fetch prompts with all filters', async () => {
      const mockResponse = {
        prompts: [
          {
            id: '123',
            title: 'Test Prompt',
            description: 'Test Description',
            content: 'Test Content',
            category: 'Engineering',
            tags: ['code-review'],
            metadata: {},
            created_at: '2025-01-01T00:00:00Z',
            updated_at: '2025-01-01T00:00:00Z',
          },
        ],
        total_count: 1,
        page: 1,
        per_page: 10,
        total_pages: 1,
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        // eslint-disable-next-line @typescript-eslint/require-await
        json: async () => mockResponse,
      })

      const result = await promptGalleryService.getPrompts({
        category: 'Engineering',
        search: 'test',
        tags: ['code-review'],
        page: 1,
        limit: 10,
      })

      expect(result).toEqual(mockResponse)
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('category=Engineering'),
        expect.any(Object)
      )
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('search=test'),
        expect.any(Object)
      )
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('tags=code-review'),
        expect.any(Object)
      )
    })

    it('should handle multiple tags', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        // eslint-disable-next-line @typescript-eslint/require-await
        json: async () => ({
          prompts: [],
          total_count: 0,
          page: 1,
          per_page: 10,
          total_pages: 0,
        }),
      })

      await promptGalleryService.getPrompts({
        tags: ['tag1', 'tag2'],
      })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('tags=tag1%2Ctag2'),
        expect.any(Object)
      )
    })
  })

  describe('getPromptById', () => {
    it('should fetch prompt by ID', async () => {
      const mockPrompt = {
        id: '123',
        title: 'Test Prompt',
        description: 'Test Description',
        content: 'Test Content',
        category: 'Engineering',
        tags: ['code-review'],
        metadata: {},
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        // eslint-disable-next-line @typescript-eslint/require-await
        json: async () => mockPrompt,
      })

      const result = await promptGalleryService.getPromptById('123')

      expect(result).toEqual(mockPrompt)
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/prompt-gallery/prompts/123'),
        expect.any(Object)
      )
    })

    it('should throw error when prompt not found', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        // eslint-disable-next-line @typescript-eslint/require-await
        json: async () => null,
      })

      await expect(promptGalleryService.getPromptById('999')).rejects.toThrow(
        'Prompt not found'
      )
    })
  })

  describe('trackPromptUsage', () => {
    it('should track prompt usage', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        // eslint-disable-next-line @typescript-eslint/require-await
        json: async () => ({ message: 'Success' }),
      })

      await promptGalleryService.trackPromptUsage('123')

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/prompt-gallery/prompts/123/use'),
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ prompt_id: '123' }),
        })
      )
    })

    it('should handle unauthorized error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
      })

      await expect(
        promptGalleryService.trackPromptUsage('123')
      ).rejects.toThrow('Authentication expired')
      expect(mockAuthService.setToken).toHaveBeenCalledWith(null)
    })
  })
})
