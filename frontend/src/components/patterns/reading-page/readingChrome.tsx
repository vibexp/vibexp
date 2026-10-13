import { createContext, useContext, useEffect, useRef } from 'react'

import type { BodyViewMode } from './types'

/** A body's Rendered / Raw state, as the page chrome needs it to draw the switch. */
export interface BodyViewHandle {
  mode: BodyViewMode
  setMode: (mode: BodyViewMode) => void
}

interface ReadingChromeValue {
  /** Called by the body with its view state, and with `null` when it unmounts. */
  setBodyView: (handle: BodyViewHandle | null) => void
  /**
   * Where an edit page's header draws the body editor's controls (its
   * Rendered / Raw switch and whatever sits beside it), or `null` where the
   * header has no room for them and the editor keeps them above the text.
   */
  controlsSlot: HTMLElement | null
}

/**
 * What a reading page's chrome (its toolbar) shares with the body it frames.
 *
 * The Rendered / Raw switch belongs to the body — the body owns the state, and
 * a prompt page even drives it from outside — but it is DRAWN in the page's
 * toolbar, beside Edit and Copy, rather than on a row of its own above the
 * text. The body hands its state up through this context; outside a reading
 * page there is no context and the body keeps drawing the switch itself.
 *
 * An edit page does the same for its editor, so the body starts at the same
 * place under the header on both: the editor's controls are several and its
 * own (a prompt's reference hint, a template loader), so instead of handing
 * state up it renders them into `controlsSlot`, the end of the header's meta
 * row.
 */
export const ReadingChromeContext = createContext<ReadingChromeValue | null>(
  null
)

/**
 * Hands the body's view state to the page toolbar. Returns `true` when a
 * reading page is hosting the switch, so the body must not draw its own.
 */
export function useHostedBodyView(
  mode: BodyViewMode,
  setMode: (mode: BodyViewMode) => void
): boolean {
  const chrome = useContext(ReadingChromeContext)
  const setBodyView = chrome?.setBodyView

  // The latest setter without making its identity a reason to re-register.
  const setModeRef = useRef(setMode)
  useEffect(() => {
    setModeRef.current = setMode
  })

  useEffect(() => {
    if (!setBodyView) return
    setBodyView({
      mode,
      setMode: next => {
        setModeRef.current(next)
      },
    })
  }, [setBodyView, mode])

  useEffect(() => {
    if (!setBodyView) return
    return () => {
      setBodyView(null)
    }
  }, [setBodyView])

  return chrome !== null
}

/**
 * The element an edit page's header offers the body editor's controls, or
 * `null` when the editor should draw them itself.
 */
export function useBodyControlsSlot(): HTMLElement | null {
  return useContext(ReadingChromeContext)?.controlsSlot ?? null
}
