import { useCallback, useMemo, useState } from 'react'

import type { Prompt } from '@/services/promptService'

import { useTeam } from '../contexts/TeamContext'
import { promptService } from '../services/promptService'

interface UsePromptSearchOptions {
  limit?: number
  excludeCurrentPrompt?: string // Exclude current prompt by slug when editing
}

interface PromptSearchResult {
  prompts: Prompt[]
  loading: boolean
  error: string | null
  searchPrompts: (query: string) => Promise<void>
  clearResults: () => void
}

export function usePromptSearch(
  options: UsePromptSearchOptions = {}
): PromptSearchResult {
  const { currentTeam } = useTeam()
  const { limit = 10, excludeCurrentPrompt } = options

  const [prompts, setPrompts] = useState<Prompt[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const searchPrompts = useCallback(
    async (query: string) => {
      if (!query.trim()) {
        setPrompts([])
        return
      }

      if (!currentTeam) {
        setError('No team selected')
        return
      }

      try {
        setLoading(true)
        setError(null)

        const response = await promptService.getPrompts(currentTeam.id, {
          search: query.trim(),
          limit,
          page: 1,
          status: 'published', // Only show published prompts for embedding
        })

        let searchResults = response.prompts

        // Filter out the current prompt if editing
        if (excludeCurrentPrompt) {
          searchResults = searchResults.filter(
            prompt => prompt.slug !== excludeCurrentPrompt
          )
        }

        setPrompts(searchResults)
      } catch (err) {
        const errorMessage =
          err instanceof Error ? err.message : 'Failed to search prompts'
        setError(errorMessage)
        setPrompts([])
      } finally {
        setLoading(false)
      }
    },
    [currentTeam, limit, excludeCurrentPrompt]
  )

  const clearResults = useCallback(() => {
    setPrompts([])
    setError(null)
  }, [])

  // Memoize the result to prevent unnecessary re-renders
  return useMemo(
    () => ({
      prompts,
      loading,
      error,
      searchPrompts,
      clearResults,
    }),
    [prompts, loading, error, searchPrompts, clearResults]
  )
}
