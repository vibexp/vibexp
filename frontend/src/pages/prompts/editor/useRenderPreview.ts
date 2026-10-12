import { useCallback, useEffect, useState } from 'react'

import type { Prompt } from '@/services/promptService'
import { promptService } from '@/services/promptService'
import { getErrorMessage } from '@/utils/errorHandling'

import type { EditorView } from './types'

interface UseRenderPreviewArgs {
  teamId: string | undefined
  prompt: Prompt | null
  view: EditorView
  isEditing: boolean
}

export function useRenderPreview({
  teamId,
  prompt,
  view,
  isEditing,
}: UseRenderPreviewArgs) {
  const [placeholderValues, setPlaceholderValues] = useState<
    Record<string, string>
  >({})
  const [renderedBody, setRenderedBody] = useState('')
  const [renderError, setRenderError] = useState<string | null>(null)
  const [isRendering, setIsRendering] = useState(false)
  const [allPlaceholders, setAllPlaceholders] = useState<string[]>([])
  const [isLoadingPlaceholders, setIsLoadingPlaceholders] = useState(false)

  const fetchAllPlaceholders = useCallback(async () => {
    if (!isEditing || !prompt || !teamId) return
    try {
      setIsLoadingPlaceholders(true)
      const placeholders = await promptService.getPromptPlaceholders(
        teamId,
        prompt.slug
      )
      setAllPlaceholders(placeholders)
    } catch {
      setAllPlaceholders([])
    } finally {
      setIsLoadingPlaceholders(false)
    }
  }, [isEditing, prompt, teamId])

  useEffect(() => {
    setPlaceholderValues(prev =>
      Object.fromEntries(allPlaceholders.map(p => [p, prev[p] ?? '']))
    )
  }, [allPlaceholders])

  const renderPrompt = useCallback(async () => {
    if (!isEditing || !prompt || !teamId) {
      setRenderError(
        'Cannot render unsaved prompt. Please save the prompt first.'
      )
      return
    }
    try {
      setIsRendering(true)
      setRenderError(null)
      // No values are sent: the server expands the @references and leaves
      // every {{placeholder}} in place, and the Render tab substitutes them on
      // screen so that an unfilled one stays visible instead of vanishing.
      const response = await promptService.renderPrompt(teamId, prompt.slug, {})
      setRenderedBody(response.rendered_body)
    } catch (error) {
      setRenderError(getErrorMessage(error, 'Failed to render prompt'))
      setRenderedBody('')
    } finally {
      setIsRendering(false)
    }
  }, [isEditing, prompt, teamId])

  useEffect(() => {
    if (view !== 'render' || !isEditing || !prompt) return
    const timeoutId = setTimeout(() => {
      void renderPrompt()
    }, 500)
    return () => {
      clearTimeout(timeoutId)
    }
  }, [view, isEditing, prompt, renderPrompt])

  const setPlaceholderValue = useCallback(
    (placeholder: string, value: string) => {
      setPlaceholderValues(prev => ({ ...prev, [placeholder]: value }))
    },
    []
  )

  return {
    allPlaceholders,
    placeholderValues,
    setPlaceholderValue,
    renderedBody,
    renderError,
    isRendering,
    isLoadingPlaceholders,
    fetchAllPlaceholders,
  }
}
