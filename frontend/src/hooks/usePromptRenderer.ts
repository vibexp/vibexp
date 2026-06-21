import { marked } from 'marked'
import { useCallback, useRef, useState } from 'react'

import { promptService } from '../services/promptService'

interface UsePromptRendererReturn {
  // State
  renderedBody: string
  renderError: string | null
  isRendering: boolean
  allPlaceholders: string[]
  placeholderValues: Record<string, string>
  isLoadingPlaceholders: boolean

  // Actions
  renderPrompt: (
    slug: string,
    teamId: string,
    placeholders?: Record<string, string>
  ) => Promise<void>
  fetchPlaceholders: (slug: string, teamId: string) => Promise<void>
  updatePlaceholderValue: (placeholder: string, value: string) => void
  renderPreviewContent: (content: string) => string
  renderMarkdown: (content: string) => Promise<string>
}

export function usePromptRenderer(): UsePromptRendererReturn {
  const [renderedBody, setRenderedBody] = useState('')
  const [renderError, setRenderError] = useState<string | null>(null)
  const [isRendering, setIsRendering] = useState(false)
  const [allPlaceholders, setAllPlaceholders] = useState<string[]>([])
  const [placeholderValues, setPlaceholderValues] = useState<
    Record<string, string>
  >({})
  const [isLoadingPlaceholders, setIsLoadingPlaceholders] = useState(false)

  // Keep ref to current placeholder values for callbacks
  const placeholderValuesRef = useRef(placeholderValues)
  placeholderValuesRef.current = placeholderValues

  // Render markdown content locally (for basic markdown without backend processing)
  const renderMarkdown = useCallback(
    async (content: string): Promise<string> => {
      try {
        return await marked(content, {
          breaks: true,
          gfm: true,
        })
      } catch (error) {
        console.error('Error rendering markdown:', error)
        return content
      }
    },
    []
  )

  // Enhanced preview with markdown rendering and syntax highlighting
  const renderPreviewContent = useCallback((content: string): string => {
    if (!content) return 'No content to preview...'

    try {
      // First, enhance @ mentions before markdown processing
      const contentWithMentions = content.replace(/@([\w-]+)/g, match => {
        return `<span class="bg-info-subtle text-info px-1 py-0.5 rounded text-sm font-mono border border-info">${match}</span>`
      })

      // Parse markdown
      return marked(contentWithMentions) as string
    } catch (error) {
      console.error('Error parsing markdown:', error)
      return content
    }
  }, [])

  // Fetch all placeholders for a prompt (including from referenced prompts)
  const fetchPlaceholders = useCallback(
    async (slug: string, teamId: string) => {
      try {
        setIsLoadingPlaceholders(true)
        const placeholders = await promptService.getPromptPlaceholders(
          teamId,
          slug
        )
        setAllPlaceholders(placeholders)

        // Initialize placeholder values - use functional update to avoid dependency
        setPlaceholderValues(currentValues => {
          // Add new placeholders with empty values
          const newPlaceholders = placeholders.reduce<Record<string, string>>(
            (acc, placeholder) => {
              if (!(placeholder in currentValues)) {
                return { ...acc, [placeholder]: '' }
              }
              return acc
            },
            {}
          )

          // Filter out values for placeholders that no longer exist
          const validValues = Object.entries(currentValues).reduce<
            Record<string, string>
          >((acc, [key, value]) => {
            if (placeholders.includes(key)) {
              return { ...acc, [key]: value }
            }
            return acc
          }, {})

          return { ...validValues, ...newPlaceholders }
        })
      } catch (error) {
        console.error('Failed to fetch placeholders:', error)
        setAllPlaceholders([])
      } finally {
        setIsLoadingPlaceholders(false)
      }
    },
    []
  )

  // Render the prompt with current placeholder values using backend API
  const renderPrompt = useCallback(
    async (
      slug: string,
      teamId: string,
      placeholders?: Record<string, string>
    ) => {
      try {
        setIsRendering(true)
        setRenderError(null)

        // If placeholders are provided, use them; otherwise get current values
        const valuesToUse = placeholders ?? placeholderValuesRef.current
        const response = await promptService.renderPrompt(
          teamId,
          slug,
          valuesToUse
        )
        setRenderedBody(response.rendered_body)
      } catch (error) {
        console.error('Error rendering prompt:', error)
        const errorMessage =
          error instanceof Error ? error.message : 'Failed to render prompt'
        setRenderError(errorMessage)
        setRenderedBody('')
      } finally {
        setIsRendering(false)
      }
    },
    []
  )

  // Update a single placeholder value
  const updatePlaceholderValue = useCallback(
    (placeholder: string, value: string) => {
      setPlaceholderValues(prev => ({
        ...prev,
        [placeholder]: value,
      }))
    },
    []
  )

  return {
    // State
    renderedBody,
    renderError,
    isRendering,
    allPlaceholders,
    placeholderValues,
    isLoadingPlaceholders,

    // Actions
    renderPrompt,
    fetchPlaceholders,
    updatePlaceholderValue,
    renderPreviewContent,
    renderMarkdown,
  }
}
