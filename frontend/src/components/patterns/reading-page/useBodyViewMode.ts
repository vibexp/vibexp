import { useCallback, useState } from 'react'

import type { BodyViewMode } from './types'

/** Where the reader is, as far as "a different page" goes. */
const currentPath = () => globalThis.location.pathname

/**
 * The Rendered / Raw view of a resource body, for the current page view only.
 *
 * A reading page always opens on Rendered. Raw is something a reader reaches
 * for on one resource — to copy its source — and it lasts exactly as long as
 * that page view: it is not stored anywhere, so a reload returns to Rendered,
 * and it does not follow the reader to the next resource. (It used to be a
 * remembered, shared preference, #901; one look at a blueprint's source then
 * opened every artifact, memory and prompt as a wall of monospace.)
 *
 * The choice is tied to the path it was made on rather than left to
 * unmounting, because navigating between two resources of the same kind keeps
 * the page component — and its state — mounted.
 *
 * `PromptDetail` needs the mode in its own render effects, so it calls this
 * directly and drives `ResourceBody` controlled; every other page lets
 * `ResourceBody` own it.
 */
export function useBodyViewMode() {
  const [choice, setChoice] = useState<{ mode: BodyViewMode; path: string }>()

  const setMode = useCallback((mode: BodyViewMode) => {
    setChoice({ mode, path: currentPath() })
  }, [])

  const mode: BodyViewMode =
    choice?.path === currentPath() ? choice.mode : 'rendered'
  return [mode, setMode] as const
}
