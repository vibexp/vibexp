import { useEffect, useRef } from 'react'

import type { BodyViewMode } from '@/components/patterns/reading-page'

/**
 * The server is asked to render with NO values: it expands the @references and
 * leaves every {{placeholder}} in place. The values are then substituted on
 * screen by the markdown renderer, which is what keeps an unfilled placeholder
 * visible as a chip (an empty value sent to the server would erase it) and
 * makes typing a value update the body without a round trip.
 */
const NO_PLACEHOLDER_VALUES: Record<string, string> = {}

interface UsePromptRenderedTabOptions {
  /** The loaded prompt's slug; `undefined` until the prompt has loaded. */
  slug: string | undefined
  tab: BodyViewMode
  currentTeam: { id: string } | null
  fetchPlaceholders: (slug: string, teamId: string) => Promise<void>
  renderPrompt: (
    slug: string,
    teamId: string,
    values?: Record<string, string>
  ) => Promise<void>
  isLoadingPlaceholders: boolean
}

/**
 * Drives the server render behind `PromptDetail`'s Rendered tab: the first
 * time a prompt is shown there its placeholders are fetched and the prompt is
 * rendered shortly after. A placeholder value never triggers another render:
 * the page substitutes values into the rendered body itself. Nothing runs
 * while the Raw tab is showing.
 */
export function usePromptRenderedTab({
  slug,
  tab,
  currentTeam,
  fetchPlaceholders,
  renderPrompt,
  isLoadingPlaceholders,
}: UsePromptRenderedTabOptions): void {
  const loadedRef = useRef<string | null>(null)

  useEffect(() => {
    if (!slug || tab !== 'rendered' || !currentTeam) return
    if (loadedRef.current !== slug) {
      loadedRef.current = slug
      void fetchPlaceholders(slug, currentTeam.id).then(() => {
        setTimeout(() => {
          void renderPrompt(slug, currentTeam.id, NO_PLACEHOLDER_VALUES)
        }, 100)
      })
    } else if (!isLoadingPlaceholders) {
      void renderPrompt(slug, currentTeam.id, NO_PLACEHOLDER_VALUES)
    }
  }, [
    slug,
    tab,
    currentTeam,
    fetchPlaceholders,
    renderPrompt,
    isLoadingPlaceholders,
  ])
}
