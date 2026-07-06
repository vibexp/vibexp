import type {
  PromptGalleryCategory,
  PromptGalleryListResponse,
  PromptGalleryTemplate,
} from '../promptGalleryService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = {
  GET: jest.fn(),
  POST: jest.fn(),
}

jest.mock('../../lib/apiClientGenerated', () => {
  const actual = jest.requireActual<
    typeof import('../../lib/apiClientGenerated')
  >('../../lib/apiClientGenerated')
  return {
    ...actual,
    generatedClient: mockGeneratedClient,
  }
})

import { promptGalleryService } from '../promptGalleryService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

describe('PromptGalleryService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('getCategories', () => {
    it('fetches categories', async () => {
      const mockCategories: PromptGalleryCategory[] = [
        { category: 'Engineering', count: 5 },
        { category: 'Marketing', count: 3 },
      ]
      mockGeneratedClient.GET.mockReturnValue(success(mockCategories))

      const result = await promptGalleryService.getCategories()

      expect(result).toEqual(mockCategories)
      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/prompt-gallery/categories'
      )
    })

    it('propagates a 401', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        Promise.resolve({
          error: {
            type: 'https://api.vibexp.io/errors/UNAUTHORIZED',
            title: 'Unauthorized',
            status: 401,
            detail: 'Authentication required',
            code: 'UNAUTHORIZED',
            request_id: 'req-1',
            timestamp: '2024-01-01T10:00:00Z',
          },
          response: { ok: false, status: 401, statusText: 'Unauthorized' },
        })
      )

      await expect(promptGalleryService.getCategories()).rejects.toThrow(
        'Authentication required'
      )
    })
  })

  describe('getPrompts', () => {
    const mockResponse: PromptGalleryListResponse = {
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

    it('serializes filters (tags comma-joined) into the query', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockResponse))

      const result = await promptGalleryService.getPrompts({
        category: 'Engineering',
        search: 'test',
        tags: ['code-review'],
        page: 1,
        limit: 10,
      })

      expect(result).toEqual(mockResponse)
      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/prompt-gallery/prompts',
        {
          params: {
            query: {
              category: 'Engineering',
              search: 'test',
              page: 1,
              limit: 10,
              tags: 'code-review',
            },
          },
        }
      )
    })

    it('joins multiple tags', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        success({
          prompts: [],
          total_count: 0,
          page: 1,
          per_page: 10,
          total_pages: 0,
        })
      )

      await promptGalleryService.getPrompts({ tags: ['tag1', 'tag2'] })

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/prompt-gallery/prompts',
        { params: { query: { tags: 'tag1,tag2' } } }
      )
    })

    it('omits tags when the array is empty', async () => {
      mockGeneratedClient.GET.mockReturnValue(success(mockResponse))

      await promptGalleryService.getPrompts({ category: 'Engineering' })

      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/prompt-gallery/prompts',
        { params: { query: { category: 'Engineering' } } }
      )
    })
  })

  describe('getPromptById', () => {
    it('fetches a prompt by id', async () => {
      const mockPrompt: PromptGalleryTemplate = {
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
      mockGeneratedClient.GET.mockReturnValue(success(mockPrompt))

      const result = await promptGalleryService.getPromptById('123')

      expect(result).toEqual(mockPrompt)
      expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
        '/api/v1/prompt-gallery/prompts/{id}',
        { params: { path: { id: '123' } } }
      )
    })

    it('propagates a not-found error', async () => {
      mockGeneratedClient.GET.mockReturnValue(
        Promise.resolve({
          error: {
            type: 'https://api.vibexp.io/errors/NOT_FOUND',
            title: 'Not Found',
            status: 404,
            detail: 'Prompt not found',
            code: 'NOT_FOUND',
            request_id: 'req-1',
            timestamp: '2024-01-01T10:00:00Z',
          },
          response: { ok: false, status: 404, statusText: 'Not Found' },
        })
      )

      await expect(promptGalleryService.getPromptById('999')).rejects.toThrow(
        'Prompt not found'
      )
    })
  })

  describe('trackPromptUsage', () => {
    it('posts to the usage endpoint with the id in the path', async () => {
      mockGeneratedClient.POST.mockReturnValue(success({ message: 'Success' }))

      await promptGalleryService.trackPromptUsage('123')

      expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
        '/api/v1/prompt-gallery/prompts/{id}/use',
        { params: { path: { id: '123' } } }
      )
    })
  })
})
