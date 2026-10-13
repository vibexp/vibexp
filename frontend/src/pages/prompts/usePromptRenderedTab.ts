import { useEffect, useRef } from 'react'

import type { BodyViewMode } from '@/components/patterns/reading-page'

interface UsePromptRenderedTabOptions {
  /** The loaded prompt's slug; `undefined` until the prompt has loaded. */
  slug: string | undefined
  tab: BodyViewMode
  currentTeam: { id: string } | null
  fetchPlaceholders: (slug: string, teamId: string) => Promise<void>
  renderPrompt: (slug: string, teamId: string) => Promise<void>
  isLoadingPlaceholders: boolean
  placeholderValues: Record<string, string>
  placeholderCount: number
}

/**
 * Drives the server render behind `PromptDetail`'s Rendered tab: the first
 * time a prompt is shown there its placeholders are fetched and the prompt is
 * rendered shortly after, and from then on a changed placeholder value
 * re-renders it on a debounce. Nothing runs while the Raw tab is showing.
 */
export function usePromptRenderedTab({
  slug,
  tab,
  currentTeam,
  fetchPlaceholders,
  renderPrompt,
  isLoadingPlaceholders,
  placeholderValues,
  placeholderCount,
}: UsePromptRenderedTabOptions): void {
  const loadedRef = useRef<string | null>(null)
  const prevValuesRef = useRef<Record<string, string>>({})

  useEffect(() => {
    if (!slug || tab !== 'rendered' || !currentTeam) return
    if (loadedRef.current !== slug) {
      loadedRef.current = slug
      void fetchPlaceholders(slug, currentTeam.id).then(() => {
        setTimeout(() => {
          void renderPrompt(slug, currentTeam.id)
        }, 100)
      })
    } else if (!isLoadingPlaceholders) {
      void renderPrompt(slug, currentTeam.id)
    }
  }, [
    slug,
    tab,
    currentTeam,
    fetchPlaceholders,
    renderPrompt,
    isLoadingPlaceholders,
  ])

  useEffect(() => {
    const prev = prevValuesRef.current
    const changed = Object.keys(placeholderValues).some(
      key => placeholderValues[key] !== prev[key]
    )
    if (
      !changed ||
      !slug ||
      !currentTeam ||
      tab !== 'rendered' ||
      isLoadingPlaceholders ||
      loadedRef.current !== slug ||
      placeholderCount === 0
    ) {
      prevValuesRef.current = placeholderValues
      return
    }
    const t = setTimeout(() => {
      void renderPrompt(slug, currentTeam.id)
      prevValuesRef.current = placeholderValues
    }, 500)
    return () => {
      clearTimeout(t)
    }
  }, [
    placeholderValues,
    slug,
    tab,
    currentTeam,
    isLoadingPlaceholders,
    placeholderCount,
    renderPrompt,
  ])
}
