import { useCallback, useEffect, useRef, useState } from 'react'

/** Marks the heading that takes focus when the step changes. */
export const STEP_HEADING_ATTR = 'data-step-heading'

/** The small uppercase overline above a step's heading. */
export const EYEBROW =
  'text-xs font-semibold tracking-[0.06em] text-muted-foreground uppercase'

/** Copies through a hidden textarea, for browsers without the async API. */
function fallbackCopy(text: string) {
  const area = document.createElement('textarea')
  area.value = text
  area.setAttribute('readonly', '')
  area.style.position = 'fixed'
  area.style.opacity = '0'
  document.body.appendChild(area)
  area.select()
  try {
    // Deprecated, but the only way left when the page is not a secure
    // context (a self-hosted instance served over plain http).
    document.execCommand('copy')
  } catch {
    // Nothing more to try; the text is still selectable on screen.
  }
  area.remove()
}

const COPIED_MS = 1600

/**
 * Copy with "Copied" feedback, tracked per button: `copied` is the key of the
 * button that copied last, for a short while.
 */
export function useCopy() {
  const [copied, setCopied] = useState<string | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current)
    },
    []
  )

  const copy = useCallback((key: string, text: string) => {
    // `navigator.clipboard` is absent outside a secure context, whatever the
    // DOM typings say.
    const clipboard = (navigator as { clipboard?: Clipboard }).clipboard
    if (clipboard) {
      clipboard.writeText(text).catch(() => {
        fallbackCopy(text)
      })
    } else {
      fallbackCopy(text)
    }
    setCopied(key)
    if (timer.current) clearTimeout(timer.current)
    timer.current = setTimeout(() => {
      setCopied(null)
    }, COPIED_MS)
  }, [])

  return { copied, copy }
}
