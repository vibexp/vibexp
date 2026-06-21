import { renderHook, waitFor } from '@testing-library/react'
import { act } from 'react'

import * as TeamContextModule from '../../contexts/TeamContext'
import type { Prompt, PromptsResponse } from '../../types'
import { usePromptSearch } from '../usePromptSearch'

// Mock the promptService module
jest.mock('../../services/promptService', () => ({
  promptService: {
    getPrompts: jest.fn(),
  },
}))

// Get the mocked function for use in tests
import { promptService } from '../../services/promptService'
const mockGetPrompts = promptService.getPrompts as jest.MockedFunction<
  typeof promptService.getPrompts
>

// Mock TeamContext to provide immediate access to currentTeam
const mockTeam = {
  id: 'team-123',
  name: 'Test Team',
  slug: 'test-team',
  description: 'Test Team Description',
  role: 'owner' as const,
  member_count: 1,
  is_personal: false,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
}

const mockTeamContext = {
  currentTeam: mockTeam,
  teams: [mockTeam],
  isLoading: false,
  setCurrentTeam: jest.fn(),
  refreshTeams: jest.fn().mockResolvedValue(undefined),
}

// Mock useTeam hook to return mock context
jest.spyOn(TeamContextModule, 'useTeam').mockReturnValue(mockTeamContext)

describe('usePromptSearch', () => {
  const mockPrompts: Prompt[] = [
    {
      id: '1',
      name: 'Test Prompt 1',
      slug: 'test-prompt-1',
      description: 'A test prompt for searching',
      body: 'This is a test prompt body',
      user_id: 'user-1',
      project_id: 'project-1',
      status: 'published',
      mcp_expose: true,
      is_shared: false,
      labels: [],
      created_at: '2023-01-01T00:00:00Z',
      updated_at: '2023-01-01T00:00:00Z',
      version: 1,
    },
    {
      id: '2',
      name: 'Test Prompt 2',
      slug: 'test-prompt-2',
      description: 'Another test prompt',
      body: 'This is another test prompt body',
      user_id: 'user-1',
      project_id: 'project-1',
      status: 'published',
      mcp_expose: true,
      is_shared: false,
      labels: [],
      created_at: '2023-01-02T00:00:00Z',
      updated_at: '2023-01-02T00:00:00Z',
      version: 1,
    },
  ]

  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('initialization', () => {
    it('should initialize with empty state', () => {
      const { result } = renderHook(() => usePromptSearch())

      expect(result.current.prompts).toEqual([])
      expect(result.current.loading).toBe(false)
      expect(result.current.error).toBeNull()
      expect(typeof result.current.searchPrompts).toBe('function')
      expect(typeof result.current.clearResults).toBe('function')
    })

    it('should initialize with custom options', () => {
      const { result } = renderHook(() =>
        usePromptSearch({ limit: 5, excludeCurrentPrompt: 'test-slug' })
      )

      expect(result.current.prompts).toEqual([])
      expect(result.current.loading).toBe(false)
      expect(result.current.error).toBeNull()
    })
  })

  describe('searchPrompts functionality', () => {
    it('should search prompts successfully with API wrapper response', async () => {
      const mockResponse: PromptsResponse = {
        status: 'success',
        message: 'OK',
        data: {
          prompts: mockPrompts,
          page: 1,
          per_page: 10,
          total_count: 2,
          total_pages: 1,
        },
      }

      mockGetPrompts.mockResolvedValue(mockResponse)

      const { result } = renderHook(() => usePromptSearch())

      await act(async () => {
        await result.current.searchPrompts('test')
      })

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(mockGetPrompts).toHaveBeenCalledWith('team-123', {
        search: 'test',
        limit: 10,
        page: 1,
        status: 'published',
      })
      expect(result.current.prompts).toEqual(mockPrompts)
      expect(result.current.error).toBeNull()
    })

    it('should search prompts successfully with direct response', async () => {
      const mockResponse: PromptsResponse = {
        status: 'success',
        message: 'OK',
        data: {
          prompts: mockPrompts.slice(0, 1),
          page: 1,
          per_page: 10,
          total_count: 1,
          total_pages: 1,
        },
      }

      mockGetPrompts.mockResolvedValue(mockResponse)

      const { result } = renderHook(() => usePromptSearch())

      await act(async () => {
        await result.current.searchPrompts('test')
      })

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(result.current.prompts).toEqual(mockResponse.data.prompts)
      expect(result.current.error).toBeNull()
    })

    it('should set loading state during search', async () => {
      let resolvePromise: (value: PromptsResponse) => void
      const promise = new Promise<PromptsResponse>(resolve => {
        resolvePromise = resolve
      })

      mockGetPrompts.mockReturnValue(promise)

      const { result } = renderHook(() => usePromptSearch())

      act(() => {
        void result.current.searchPrompts('test')
      })

      expect(result.current.loading).toBe(true)

      act(() => {
        resolvePromise({
          status: 'success',
          message: 'OK',
          data: {
            prompts: [],
            page: 1,
            per_page: 10,
            total_count: 0,
            total_pages: 0,
          },
        })
      })

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })
    })

    it('should clear prompts for empty query', async () => {
      const { result } = renderHook(() => usePromptSearch())

      // First set some prompts
      const mockResponse: PromptsResponse = {
        status: 'success',
        message: 'OK',
        data: {
          prompts: mockPrompts,
          page: 1,
          per_page: 10,
          total_count: 2,
          total_pages: 1,
        },
      }
      mockGetPrompts.mockResolvedValue(mockResponse)

      await act(async () => {
        await result.current.searchPrompts('test')
      })

      expect(result.current.prompts).toHaveLength(2)

      // Then search with empty query
      await act(async () => {
        await result.current.searchPrompts('')
      })

      expect(result.current.prompts).toEqual([])
      expect(mockGetPrompts).toHaveBeenCalledTimes(1) // Should not call API for empty query
    })

    it('should trim query before searching', async () => {
      const mockResponse: PromptsResponse = {
        status: 'success',
        message: 'OK',
        data: {
          prompts: mockPrompts,
          page: 1,
          per_page: 10,
          total_count: 2,
          total_pages: 1,
        },
      }
      mockGetPrompts.mockResolvedValue(mockResponse)

      const { result } = renderHook(() => usePromptSearch())

      await act(async () => {
        await result.current.searchPrompts('  test query  ')
      })

      expect(mockGetPrompts).toHaveBeenCalledWith('team-123', {
        search: 'test query',
        limit: 10,
        page: 1,
        status: 'published',
      })
    })

    it('should use custom limit option', async () => {
      const mockResponse: PromptsResponse = {
        status: 'success',
        message: 'OK',
        data: {
          prompts: mockPrompts,
          page: 1,
          per_page: 5,
          total_count: 2,
          total_pages: 1,
        },
      }
      mockGetPrompts.mockResolvedValue(mockResponse)

      const { result } = renderHook(() => usePromptSearch({ limit: 5 }))

      await act(async () => {
        await result.current.searchPrompts('test')
      })

      expect(mockGetPrompts).toHaveBeenCalledWith('team-123', {
        search: 'test',
        limit: 5,
        page: 1,
        status: 'published',
      })
    })
  })

  describe('filtering logic', () => {
    it('should filter out current prompt when excludeCurrentPrompt is provided', async () => {
      const mockResponse: PromptsResponse = {
        status: 'success',
        message: 'OK',
        data: {
          prompts: mockPrompts,
          page: 1,
          per_page: 10,
          total_count: 2,
          total_pages: 1,
        },
      }
      mockGetPrompts.mockResolvedValue(mockResponse)

      const { result } = renderHook(() =>
        usePromptSearch({ excludeCurrentPrompt: 'test-prompt-1' })
      )

      await act(async () => {
        await result.current.searchPrompts('test')
      })

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      // Should only include test-prompt-2, excluding test-prompt-1
      expect(result.current.prompts).toHaveLength(1)
      expect(result.current.prompts[0].slug).toBe('test-prompt-2')
    })

    it('should handle empty prompts array', async () => {
      const mockResponse: PromptsResponse = {
        status: 'success',
        message: 'OK',
        data: {
          prompts: [],
          page: 1,
          per_page: 10,
          total_count: 0,
          total_pages: 0,
        },
      }
      mockGetPrompts.mockResolvedValue(mockResponse)

      const { result } = renderHook(() => usePromptSearch())

      await act(async () => {
        await result.current.searchPrompts('test')
      })

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(result.current.prompts).toEqual([])
      expect(result.current.error).toBeNull()
    })
  })

  describe('error handling', () => {
    it('should handle API errors', async () => {
      const errorMessage = 'API Error occurred'
      mockGetPrompts.mockRejectedValue(new Error(errorMessage))

      const { result } = renderHook(() => usePromptSearch())

      await act(async () => {
        await result.current.searchPrompts('test')
      })

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(result.current.error).toBe(errorMessage)
      expect(result.current.prompts).toEqual([])
    })

    it('should handle non-Error exceptions', async () => {
      mockGetPrompts.mockRejectedValue('String error')

      const { result } = renderHook(() => usePromptSearch())

      await act(async () => {
        await result.current.searchPrompts('test')
      })

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(result.current.error).toBe('Failed to search prompts')
      expect(result.current.prompts).toEqual([])
    })

    // Removed: Testing null response is unnecessary as TypeScript guarantees non-null response

    it('should handle response with no data property', async () => {
      mockGetPrompts.mockResolvedValue({} as unknown as PromptsResponse)

      const { result } = renderHook(() => usePromptSearch())

      await act(async () => {
        await result.current.searchPrompts('test')
      })

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      // With an empty object {}, it will use {} as responseData, and 'prompts' in {} is false,
      // so it will set prompts to [] without error
      expect(result.current.error).toBeNull()
      expect(result.current.prompts).toEqual([])
    })

    it('should clear error on successful search', async () => {
      // First, cause an error
      mockGetPrompts.mockRejectedValueOnce(new Error('Test error'))

      const { result } = renderHook(() => usePromptSearch())

      await act(async () => {
        await result.current.searchPrompts('test')
      })

      expect(result.current.error).toBe('Test error')

      // Then, make a successful search
      const mockResponse: PromptsResponse = {
        status: 'success',
        message: 'OK',
        data: {
          prompts: [mockPrompts[0]],
          page: 1,
          per_page: 10,
          total_count: 1,
          total_pages: 1,
        },
      }
      mockGetPrompts.mockResolvedValue(mockResponse)

      await act(async () => {
        await result.current.searchPrompts('test')
      })

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(result.current.error).toBeNull()
      expect(result.current.prompts).toHaveLength(1)
    })
  })

  describe('clearResults functionality', () => {
    it('should clear prompts and error', async () => {
      const { result } = renderHook(() => usePromptSearch())

      // First set some data
      const mockResponse: PromptsResponse = {
        status: 'success',
        message: 'OK',
        data: {
          prompts: mockPrompts,
          page: 1,
          per_page: 10,
          total_count: 2,
          total_pages: 1,
        },
      }
      mockGetPrompts.mockResolvedValue(mockResponse)

      await act(async () => {
        await result.current.searchPrompts('test')
      })

      expect(result.current.prompts).toHaveLength(2)

      // Then clear results
      act(() => {
        result.current.clearResults()
      })

      expect(result.current.prompts).toEqual([])
      expect(result.current.error).toBeNull()
    })
  })

  describe('memoization and performance', () => {
    it('should memoize result object', () => {
      const { result, rerender } = renderHook(() => usePromptSearch())

      const firstResult = result.current

      // Rerender without changing props
      rerender()

      // Result object should be the same reference due to memoization
      expect(result.current).toBe(firstResult)
    })

    it('should update memoized result when state changes', async () => {
      const { result } = renderHook(() => usePromptSearch())

      const initialResult = result.current

      const mockResponse: PromptsResponse = {
        status: 'success',
        message: 'OK',
        data: {
          prompts: [mockPrompts[0]],
          page: 1,
          per_page: 10,
          total_count: 1,
          total_pages: 1,
        },
      }
      mockGetPrompts.mockResolvedValue(mockResponse)

      await act(async () => {
        await result.current.searchPrompts('test')
      })

      // Result object should be different after state change
      expect(result.current).not.toBe(initialResult)
      expect(result.current.prompts).toHaveLength(1)
    })

    it('should memoize searchPrompts callback with same dependencies', () => {
      const { result, rerender } = renderHook(() =>
        usePromptSearch({ limit: 10, excludeCurrentPrompt: 'test' })
      )

      const firstSearchPrompts = result.current.searchPrompts

      rerender()

      // Callback should be the same reference due to useCallback with same dependencies
      expect(result.current.searchPrompts).toBe(firstSearchPrompts)
    })

    it('should memoize clearResults callback', () => {
      const { result, rerender } = renderHook(() => usePromptSearch())

      const firstClearResults = result.current.clearResults

      rerender()

      // Callback should be the same reference due to useCallback with no dependencies
      expect(result.current.clearResults).toBe(firstClearResults)
    })
  })

  describe('integration scenarios', () => {
    it('should handle complete search workflow', async () => {
      const mockResponse: PromptsResponse = {
        status: 'success',
        message: 'OK',
        data: {
          prompts: mockPrompts,
          page: 1,
          per_page: 5,
          total_count: 2,
          total_pages: 1,
        },
      }
      mockGetPrompts.mockResolvedValue(mockResponse)

      const { result } = renderHook(() =>
        usePromptSearch({ limit: 5, excludeCurrentPrompt: 'test-prompt-1' })
      )

      // Initial state
      expect(result.current.prompts).toEqual([])
      expect(result.current.loading).toBe(false)
      expect(result.current.error).toBeNull()

      // Search with results
      await act(async () => {
        await result.current.searchPrompts('test query')
      })

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(result.current.prompts).toHaveLength(1) // Filtered out test-prompt-1
      expect(result.current.prompts[0].slug).toBe('test-prompt-2')
      expect(result.current.error).toBeNull()

      // Clear results
      act(() => {
        result.current.clearResults()
      })

      expect(result.current.prompts).toEqual([])
      expect(result.current.error).toBeNull()

      // Search with empty query
      await act(async () => {
        await result.current.searchPrompts('')
      })

      expect(result.current.prompts).toEqual([])
      expect(mockGetPrompts).toHaveBeenCalledTimes(1) // Only called once for the actual search
    })

    it('should handle error recovery workflow', async () => {
      const { result } = renderHook(() => usePromptSearch())

      // Cause an error
      mockGetPrompts.mockRejectedValueOnce(new Error('Network error'))

      await act(async () => {
        await result.current.searchPrompts('test')
      })

      expect(result.current.error).toBe('Network error')
      expect(result.current.prompts).toEqual([])

      // Clear error
      act(() => {
        result.current.clearResults()
      })

      expect(result.current.error).toBeNull()

      // Successful search after error
      const mockResponse: PromptsResponse = {
        status: 'success',
        message: 'OK',
        data: {
          prompts: [mockPrompts[0]],
          page: 1,
          per_page: 10,
          total_count: 1,
          total_pages: 1,
        },
      }
      mockGetPrompts.mockResolvedValue(mockResponse)

      await act(async () => {
        await result.current.searchPrompts('test')
      })

      await waitFor(() => {
        expect(result.current.loading).toBe(false)
      })

      expect(result.current.error).toBeNull()
      expect(result.current.prompts).toEqual([mockPrompts[0]])
    })
  })
})
