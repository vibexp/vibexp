import { renderHook, act } from '@testing-library/react'
import { usePromptRenderer } from '../../src/hooks/usePromptRenderer'
import type { RenderPromptResponse } from '../../src/types'

// Mock marked library
jest.mock('marked', () => ({
  marked: jest.fn(),
}))

// Mock promptService
jest.mock('../../src/services/promptService', () => ({
  promptService: {
    getPromptPlaceholders: jest.fn(),
    renderPrompt: jest.fn(),
  },
}))

import { promptService } from '../../src/services/promptService'
import { marked } from 'marked'
const mockPromptService = promptService as jest.Mocked<typeof promptService>
const mockMarked = marked as jest.MockedFunction<typeof marked>

// Mock console.error to suppress expected error logs in tests
const originalConsoleError = console.error
beforeEach(() => {
  console.error = jest.fn()
})

afterEach(() => {
  console.error = originalConsoleError
})

describe('usePromptRenderer', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    // Setup default mock implementations
    mockMarked.mockImplementation((content: string) =>
      Promise.resolve(`<p>${content}</p>`)
    )
  })

  describe('Initial State', () => {
    it('should initialize with default state values', () => {
      const { result } = renderHook(() => usePromptRenderer())

      expect(result.current.renderedBody).toBe('')
      expect(result.current.renderError).toBeNull()
      expect(result.current.isRendering).toBe(false)
      expect(result.current.allPlaceholders).toEqual([])
      expect(result.current.placeholderValues).toEqual({})
      expect(result.current.isLoadingPlaceholders).toBe(false)
    })

    it('should provide all expected functions', () => {
      const { result } = renderHook(() => usePromptRenderer())

      expect(typeof result.current.renderPrompt).toBe('function')
      expect(typeof result.current.fetchPlaceholders).toBe('function')
      expect(typeof result.current.updatePlaceholderValue).toBe('function')
      expect(typeof result.current.renderPreviewContent).toBe('function')
      expect(typeof result.current.renderMarkdown).toBe('function')
    })
  })

  describe('renderMarkdown', () => {
    it('should render markdown content successfully', async () => {
      mockMarked.mockResolvedValueOnce('<h1>Title</h1>')

      const { result } = renderHook(() => usePromptRenderer())
      const content = '# Title'

      const html = await result.current.renderMarkdown(content)

      expect(marked).toHaveBeenCalledWith(content, {
        breaks: true,
        gfm: true,
      })
      expect(html).toBe('<h1>Title</h1>')
    })

    it('should handle markdown parsing errors gracefully', async () => {
      const error = new Error('Markdown parsing failed')
      mockMarked.mockRejectedValueOnce(error)

      const { result } = renderHook(() => usePromptRenderer())
      const content = '# Invalid markdown'

      const html = await result.current.renderMarkdown(content)

      expect(html).toBe(content)
      expect(console.error).toHaveBeenCalledWith(
        'Error rendering markdown:',
        error
      )
    })

    it('should handle empty content', async () => {
      const { result } = renderHook(() => usePromptRenderer())

      const html = await result.current.renderMarkdown('')

      expect(html).toBe('<p></p>')
    })

    it('should maintain stable function reference', () => {
      const { result, rerender } = renderHook(() => usePromptRenderer())

      const firstRender = result.current.renderMarkdown
      rerender()
      const secondRender = result.current.renderMarkdown

      expect(firstRender).toBe(secondRender)
    })
  })

  describe('renderPreviewContent', () => {
    it('should return default message for empty content', () => {
      const { result } = renderHook(() => usePromptRenderer())

      const html = result.current.renderPreviewContent('')

      expect(html).toBe('No content to preview...')
    })

    it('should enhance @mentions before markdown processing', () => {
      mockMarked.mockReturnValueOnce('<p>Enhanced content</p>')

      const { result } = renderHook(() => usePromptRenderer())
      const content = 'Hello @user-name and @another-user'

      const html = result.current.renderPreviewContent(content)

      expect(marked).toHaveBeenCalledWith(
        'Hello <span class="bg-info-subtle text-info px-1 py-0.5 rounded text-sm font-mono border border-info">@user-name</span> and <span class="bg-info-subtle text-info px-1 py-0.5 rounded text-sm font-mono border border-info">@another-user</span>'
      )
      expect(html).toBe('<p>Enhanced content</p>')
    })

    it('should handle content without mentions', () => {
      mockMarked.mockReturnValueOnce('<p>Regular content</p>')

      const { result } = renderHook(() => usePromptRenderer())
      const content = 'Regular markdown content'

      const html = result.current.renderPreviewContent(content)

      expect(marked).toHaveBeenCalledWith(content)
      expect(html).toBe('<p>Regular content</p>')
    })

    it('should handle markdown parsing errors in preview', () => {
      const error = new Error('Preview parsing failed')
      mockMarked.mockImplementationOnce(() => {
        throw error
      })

      const { result } = renderHook(() => usePromptRenderer())
      const content = 'Content with error'

      const html = result.current.renderPreviewContent(content)

      expect(html).toBe(content)
      expect(console.error).toHaveBeenCalledWith(
        'Error parsing markdown:',
        error
      )
    })

    it('should maintain stable function reference', () => {
      const { result, rerender } = renderHook(() => usePromptRenderer())

      const firstRender = result.current.renderPreviewContent
      rerender()
      const secondRender = result.current.renderPreviewContent

      expect(firstRender).toBe(secondRender)
    })

    it('should handle complex mention patterns', () => {
      mockMarked.mockReturnValueOnce('<p>Complex mentions</p>')

      const { result } = renderHook(() => usePromptRenderer())
      const content =
        '@user-123 @user_with_underscores @hyphenated-user @a1b2c3'

      result.current.renderPreviewContent(content)

      const expectedContent =
        '<span class="bg-info-subtle text-info px-1 py-0.5 rounded text-sm font-mono border border-info">@user-123</span> <span class="bg-info-subtle text-info px-1 py-0.5 rounded text-sm font-mono border border-info">@user_with_underscores</span> <span class="bg-info-subtle text-info px-1 py-0.5 rounded text-sm font-mono border border-info">@hyphenated-user</span> <span class="bg-info-subtle text-info px-1 py-0.5 rounded text-sm font-mono border border-info">@a1b2c3</span>'
      expect(marked).toHaveBeenCalledWith(expectedContent)
    })
  })

  describe('fetchPlaceholders', () => {
    it('should fetch placeholders successfully', async () => {
      const mockPlaceholders = ['name', 'email', 'company']
      mockPromptService.getPromptPlaceholders.mockResolvedValueOnce(
        mockPlaceholders
      )

      const { result } = renderHook(() => usePromptRenderer())

      await act(async () => {
        await result.current.fetchPlaceholders('test-slug', 'test-team-id')
      })

      expect(mockPromptService.getPromptPlaceholders).toHaveBeenCalledWith(
        'test-team-id',
        'test-slug'
      )
      expect(result.current.allPlaceholders).toEqual(mockPlaceholders)
      expect(result.current.isLoadingPlaceholders).toBe(false)
      expect(result.current.placeholderValues).toEqual({
        name: '',
        email: '',
        company: '',
      })
    })

    it('should handle loading state correctly', async () => {
      let resolvePromise: (value: string[]) => void
      const promise = new Promise<string[]>(resolve => {
        resolvePromise = resolve
      })
      mockPromptService.getPromptPlaceholders.mockReturnValueOnce(promise)

      const { result } = renderHook(() => usePromptRenderer())

      // Start fetching
      act(() => {
        result.current.fetchPlaceholders('test-slug', 'test-team-id')
      })

      // Should be loading
      expect(result.current.isLoadingPlaceholders).toBe(true)

      // Resolve the promise
      await act(async () => {
        resolvePromise!(['placeholder1'])
        await promise
      })

      // Should not be loading anymore
      expect(result.current.isLoadingPlaceholders).toBe(false)
    })

    it('should preserve existing placeholder values', async () => {
      const { result } = renderHook(() => usePromptRenderer())

      // Set initial values
      act(() => {
        result.current.updatePlaceholderValue('name', 'John')
        result.current.updatePlaceholderValue('email', 'john@example.com')
      })

      // Fetch new placeholders that include some existing ones
      mockPromptService.getPromptPlaceholders.mockResolvedValueOnce([
        'name',
        'company',
      ])

      await act(async () => {
        await result.current.fetchPlaceholders('test-slug', 'test-team-id')
      })

      expect(result.current.placeholderValues).toEqual({
        name: 'John', // Preserved
        company: '', // New placeholder with empty value
        // email was removed as it's not in new placeholders
      })
    })

    it('should remove obsolete placeholder values', async () => {
      const { result } = renderHook(() => usePromptRenderer())

      // Set initial placeholders
      mockPromptService.getPromptPlaceholders.mockResolvedValueOnce([
        'name',
        'email',
        'company',
      ])
      await act(async () => {
        await result.current.fetchPlaceholders('initial-slug', 'test-team-id')
      })

      // Update some values
      act(() => {
        result.current.updatePlaceholderValue('name', 'John')
        result.current.updatePlaceholderValue('email', 'john@example.com')
        result.current.updatePlaceholderValue('company', 'Acme Inc')
      })

      // Fetch new placeholders with fewer items
      mockPromptService.getPromptPlaceholders.mockResolvedValueOnce([
        'name',
        'title',
      ])

      await act(async () => {
        await result.current.fetchPlaceholders('updated-slug', 'test-team-id')
      })

      expect(result.current.placeholderValues).toEqual({
        name: 'John', // Preserved
        title: '', // New placeholder
        // email and company removed
      })
    })

    it('should handle fetch errors gracefully', async () => {
      const error = new Error('Failed to fetch placeholders')
      mockPromptService.getPromptPlaceholders.mockRejectedValueOnce(error)

      const { result } = renderHook(() => usePromptRenderer())

      await act(async () => {
        await result.current.fetchPlaceholders('test-slug', 'test-team-id')
      })

      expect(result.current.allPlaceholders).toEqual([])
      expect(result.current.isLoadingPlaceholders).toBe(false)
      expect(console.error).toHaveBeenCalledWith(
        'Failed to fetch placeholders:',
        error
      )
    })

    it('should maintain stable function reference', () => {
      const { result, rerender } = renderHook(() => usePromptRenderer())

      const firstRender = result.current.fetchPlaceholders
      rerender()
      const secondRender = result.current.fetchPlaceholders

      expect(firstRender).toBe(secondRender)
    })
  })

  describe('renderPrompt', () => {
    const mockResponse: RenderPromptResponse = {
      rendered_body: 'Hello John from Acme Inc!',
      placeholders_missing: [],
      references_used: [],
    }

    it('should render prompt successfully with current placeholder values', async () => {
      mockPromptService.renderPrompt.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => usePromptRenderer())

      // Set some placeholder values
      act(() => {
        result.current.updatePlaceholderValue('name', 'John')
        result.current.updatePlaceholderValue('company', 'Acme Inc')
      })

      await act(async () => {
        await result.current.renderPrompt('test-slug', 'test-team-id')
      })

      expect(mockPromptService.renderPrompt).toHaveBeenCalledWith(
        'test-team-id',
        'test-slug',
        {
          name: 'John',
          company: 'Acme Inc',
        }
      )
      expect(result.current.renderedBody).toBe(mockResponse.rendered_body)
      expect(result.current.renderError).toBeNull()
      expect(result.current.isRendering).toBe(false)
    })

    it('should render prompt with provided placeholder values', async () => {
      mockPromptService.renderPrompt.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => usePromptRenderer())
      const customPlaceholders = { name: 'Jane', company: 'Beta Corp' }

      await act(async () => {
        await result.current.renderPrompt(
          'test-slug',
          'test-team-id',
          customPlaceholders
        )
      })

      expect(mockPromptService.renderPrompt).toHaveBeenCalledWith(
        'test-team-id',
        'test-slug',
        customPlaceholders
      )
      expect(result.current.renderedBody).toBe(mockResponse.rendered_body)
    })

    it('should handle loading state correctly', async () => {
      let resolvePromise: (value: RenderPromptResponse) => void
      const promise = new Promise<RenderPromptResponse>(resolve => {
        resolvePromise = resolve
      })
      mockPromptService.renderPrompt.mockReturnValueOnce(promise)

      const { result } = renderHook(() => usePromptRenderer())

      // Start rendering
      act(() => {
        result.current.renderPrompt('test-slug', 'test-team-id')
      })

      // Should be rendering
      expect(result.current.isRendering).toBe(true)
      expect(result.current.renderError).toBeNull()

      // Resolve the promise
      await act(async () => {
        resolvePromise!(mockResponse)
        await promise
      })

      // Should not be rendering anymore
      expect(result.current.isRendering).toBe(false)
    })

    it('should handle render errors with Error objects', async () => {
      const error = new Error('Rendering failed')
      mockPromptService.renderPrompt.mockRejectedValueOnce(error)

      const { result } = renderHook(() => usePromptRenderer())

      await act(async () => {
        await result.current.renderPrompt('test-slug', 'test-team-id')
      })

      expect(result.current.renderError).toBe('Rendering failed')
      expect(result.current.renderedBody).toBe('')
      expect(result.current.isRendering).toBe(false)
      expect(console.error).toHaveBeenCalledWith(
        'Error rendering prompt:',
        error
      )
    })

    it('should handle render errors with non-Error objects', async () => {
      const error = 'String error'
      mockPromptService.renderPrompt.mockRejectedValueOnce(error)

      const { result } = renderHook(() => usePromptRenderer())

      await act(async () => {
        await result.current.renderPrompt('test-slug', 'test-team-id')
      })

      expect(result.current.renderError).toBe('Failed to render prompt')
      expect(result.current.renderedBody).toBe('')
      expect(result.current.isRendering).toBe(false)
    })

    it('should clear previous errors on successful render', async () => {
      const { result } = renderHook(() => usePromptRenderer())

      // First render with error
      mockPromptService.renderPrompt.mockRejectedValueOnce(
        new Error('First error')
      )
      await act(async () => {
        await result.current.renderPrompt('test-slug', 'test-team-id')
      })
      expect(result.current.renderError).toBe('First error')

      // Second render successful
      mockPromptService.renderPrompt.mockResolvedValueOnce(mockResponse)
      await act(async () => {
        await result.current.renderPrompt('test-slug', 'test-team-id')
      })
      expect(result.current.renderError).toBeNull()
      expect(result.current.renderedBody).toBe(mockResponse.rendered_body)
    })

    it('should maintain stable function reference', () => {
      const { result, rerender } = renderHook(() => usePromptRenderer())

      const firstRender = result.current.renderPrompt
      rerender()
      const secondRender = result.current.renderPrompt

      expect(firstRender).toBe(secondRender)
    })

    it('should use ref to access current placeholder values', async () => {
      mockPromptService.renderPrompt.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => usePromptRenderer())

      // Set placeholder values
      act(() => {
        result.current.updatePlaceholderValue('name', 'Initial')
      })

      // Create a render function that will be called later
      let renderFunction: () => Promise<void>
      act(() => {
        renderFunction = () =>
          result.current.renderPrompt('test-slug', 'test-team-id')
      })

      // Update placeholder values
      act(() => {
        result.current.updatePlaceholderValue('name', 'Updated')
      })

      // Call the render function - should use updated values
      await act(async () => {
        await renderFunction!()
      })

      expect(mockPromptService.renderPrompt).toHaveBeenCalledWith(
        'test-team-id',
        'test-slug',
        {
          name: 'Updated',
        }
      )
    })
  })

  describe('updatePlaceholderValue', () => {
    it('should update a single placeholder value', () => {
      const { result } = renderHook(() => usePromptRenderer())

      act(() => {
        result.current.updatePlaceholderValue('name', 'John')
      })

      expect(result.current.placeholderValues).toEqual({
        name: 'John',
      })
    })

    it('should update multiple placeholder values independently', () => {
      const { result } = renderHook(() => usePromptRenderer())

      act(() => {
        result.current.updatePlaceholderValue('name', 'John')
        result.current.updatePlaceholderValue('email', 'john@example.com')
        result.current.updatePlaceholderValue('company', 'Acme Inc')
      })

      expect(result.current.placeholderValues).toEqual({
        name: 'John',
        email: 'john@example.com',
        company: 'Acme Inc',
      })
    })

    it('should overwrite existing placeholder values', () => {
      const { result } = renderHook(() => usePromptRenderer())

      act(() => {
        result.current.updatePlaceholderValue('name', 'John')
      })

      expect(result.current.placeholderValues.name).toBe('John')

      act(() => {
        result.current.updatePlaceholderValue('name', 'Jane')
      })

      expect(result.current.placeholderValues.name).toBe('Jane')
    })

    it('should handle empty string values', () => {
      const { result } = renderHook(() => usePromptRenderer())

      act(() => {
        result.current.updatePlaceholderValue('name', 'John')
        result.current.updatePlaceholderValue('name', '')
      })

      expect(result.current.placeholderValues.name).toBe('')
    })

    it('should maintain stable function reference', () => {
      const { result, rerender } = renderHook(() => usePromptRenderer())

      const firstRender = result.current.updatePlaceholderValue
      rerender()
      const secondRender = result.current.updatePlaceholderValue

      expect(firstRender).toBe(secondRender)
    })
  })

  describe('Integration Tests', () => {
    it('should work with complete workflow', async () => {
      const mockPlaceholders = ['name', 'company']
      const mockRenderResponse: RenderPromptResponse = {
        rendered_body: 'Hello John from Acme Inc!',
        placeholders_missing: [],
        references_used: [],
      }

      mockPromptService.getPromptPlaceholders.mockResolvedValueOnce(
        mockPlaceholders
      )
      mockPromptService.renderPrompt.mockResolvedValueOnce(mockRenderResponse)

      const { result } = renderHook(() => usePromptRenderer())

      // 1. Fetch placeholders
      await act(async () => {
        await result.current.fetchPlaceholders('test-slug', 'test-team-id')
      })

      expect(result.current.allPlaceholders).toEqual(mockPlaceholders)
      expect(result.current.placeholderValues).toEqual({
        name: '',
        company: '',
      })

      // 2. Update placeholder values
      act(() => {
        result.current.updatePlaceholderValue('name', 'John')
        result.current.updatePlaceholderValue('company', 'Acme Inc')
      })

      expect(result.current.placeholderValues).toEqual({
        name: 'John',
        company: 'Acme Inc',
      })

      // 3. Render prompt
      await act(async () => {
        await result.current.renderPrompt('test-slug', 'test-team-id')
      })

      expect(result.current.renderedBody).toBe(mockRenderResponse.rendered_body)
      expect(result.current.renderError).toBeNull()
    })

    it('should handle mixed success and error scenarios', async () => {
      const { result } = renderHook(() => usePromptRenderer())

      // Successful placeholder fetch
      mockPromptService.getPromptPlaceholders.mockResolvedValueOnce(['name'])
      await act(async () => {
        await result.current.fetchPlaceholders('test-slug', 'test-team-id')
      })
      expect(result.current.allPlaceholders).toEqual(['name'])

      // Failed render
      mockPromptService.renderPrompt.mockRejectedValueOnce(
        new Error('Render failed')
      )
      await act(async () => {
        await result.current.renderPrompt('test-slug', 'test-team-id')
      })
      expect(result.current.renderError).toBe('Render failed')

      // Successful render after error
      const mockRenderResponse: RenderPromptResponse = {
        rendered_body: 'Success!',
        placeholders_missing: [],
        references_used: [],
      }
      mockPromptService.renderPrompt.mockResolvedValueOnce(mockRenderResponse)
      await act(async () => {
        await result.current.renderPrompt('test-slug', 'test-team-id')
      })
      expect(result.current.renderError).toBeNull()
      expect(result.current.renderedBody).toBe('Success!')
    })
  })

  describe('Performance and Stability', () => {
    it('should maintain stable function references across re-renders', () => {
      const { result, rerender } = renderHook(() => usePromptRenderer())

      const initialFunctions = {
        renderPrompt: result.current.renderPrompt,
        fetchPlaceholders: result.current.fetchPlaceholders,
        updatePlaceholderValue: result.current.updatePlaceholderValue,
        renderPreviewContent: result.current.renderPreviewContent,
        renderMarkdown: result.current.renderMarkdown,
      }

      // Trigger re-render
      rerender()

      expect(result.current.renderPrompt).toBe(initialFunctions.renderPrompt)
      expect(result.current.fetchPlaceholders).toBe(
        initialFunctions.fetchPlaceholders
      )
      expect(result.current.updatePlaceholderValue).toBe(
        initialFunctions.updatePlaceholderValue
      )
      expect(result.current.renderPreviewContent).toBe(
        initialFunctions.renderPreviewContent
      )
      expect(result.current.renderMarkdown).toBe(
        initialFunctions.renderMarkdown
      )
    })

    it('should handle rapid consecutive calls gracefully', async () => {
      const mockRenderResponse: RenderPromptResponse = {
        rendered_body: 'Final result',
        placeholders_missing: [],
        references_used: [],
      }
      mockPromptService.renderPrompt.mockResolvedValue(mockRenderResponse)

      const { result } = renderHook(() => usePromptRenderer())

      // Make multiple rapid calls
      await act(async () => {
        const promises = [
          result.current.renderPrompt('slug1', 'test-team-id'),
          result.current.renderPrompt('slug2', 'test-team-id'),
          result.current.renderPrompt('slug3', 'test-team-id'),
        ]
        await Promise.all(promises)
      })

      expect(mockPromptService.renderPrompt).toHaveBeenCalledTimes(3)
      expect(result.current.isRendering).toBe(false)
    })

    it('should cleanup properly on unmount', () => {
      const { unmount } = renderHook(() => usePromptRenderer())

      // Should not throw any errors
      expect(() => unmount()).not.toThrow()
    })
  })

  describe('Edge Cases', () => {
    it('should handle undefined and null values in updatePlaceholderValue', () => {
      const { result } = renderHook(() => usePromptRenderer())

      act(() => {
        result.current.updatePlaceholderValue(
          'test',
          undefined as unknown as string
        )
      })

      expect(result.current.placeholderValues.test).toBeUndefined()

      act(() => {
        result.current.updatePlaceholderValue('test', null as unknown as string)
      })

      expect(result.current.placeholderValues.test).toBeNull()
    })

    it('should handle empty slug in fetchPlaceholders', async () => {
      mockPromptService.getPromptPlaceholders.mockResolvedValueOnce([])

      const { result } = renderHook(() => usePromptRenderer())

      await act(async () => {
        await result.current.fetchPlaceholders('', 'test-team-id')
      })

      expect(mockPromptService.getPromptPlaceholders).toHaveBeenCalledWith(
        'test-team-id',
        ''
      )
      expect(result.current.allPlaceholders).toEqual([])
    })

    it('should handle empty slug in renderPrompt', async () => {
      const mockResponse: RenderPromptResponse = {
        rendered_body: '',
        placeholders_missing: [],
        references_used: [],
      }
      mockPromptService.renderPrompt.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => usePromptRenderer())

      await act(async () => {
        await result.current.renderPrompt('', 'test-team-id')
      })

      expect(mockPromptService.renderPrompt).toHaveBeenCalledWith(
        'test-team-id',
        '',
        {}
      )
    })

    it('should handle very large placeholder objects', async () => {
      const { result } = renderHook(() => usePromptRenderer())

      // Create large placeholder object
      const largePlaceholders: Record<string, string> = {}
      for (let i = 0; i < 1000; i++) {
        largePlaceholders[`placeholder${i}`] = `value${i}`
      }

      mockPromptService.renderPrompt.mockResolvedValueOnce({
        rendered_body: 'Success',
        placeholders_missing: [],
        references_used: [],
      })

      await act(async () => {
        await result.current.renderPrompt(
          'test-slug',
          'test-team-id',
          largePlaceholders
        )
      })

      expect(mockPromptService.renderPrompt).toHaveBeenCalledWith(
        'test-team-id',
        'test-slug',
        largePlaceholders
      )
      expect(result.current.renderedBody).toBe('Success')
    })
  })
})
