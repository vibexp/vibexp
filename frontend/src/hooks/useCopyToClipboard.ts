import { useCallback, useEffect, useRef, useState } from 'react'

/**
 * Copy-to-clipboard with transient "copied" feedback. Returns the current
 * `copied` flag and a `copy(value)` action; the flag reverts after `resetMs`.
 * Shared by `CopyButton` and the metadata slug chip so both behave identically.
 *
 * `onFailure` runs when the text could not be copied: the browser refused,
 * or there is no clipboard API at all, as on a self-hosted instance served
 * over plain http (not a secure context). A caller can offer a manual copy
 * there; without it the failure is only logged.
 */
export function useCopyToClipboard(
  resetMs = 1500,
  onFailure?: (value: string) => void
) {
  const [copied, setCopied] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(
    () => () => {
      if (timerRef.current) clearTimeout(timerRef.current)
    },
    []
  )

  const copy = useCallback(
    (value: string) => {
      const fail = (err: unknown) => {
        console.error('Copy failed:', err)
        onFailure?.(value)
      }
      // Absent outside a secure context, whatever the DOM typings say.
      const clipboard = (navigator as { clipboard?: Clipboard }).clipboard
      if (!clipboard) {
        fail(new Error('Clipboard API unavailable'))
        return
      }
      clipboard
        .writeText(value)
        .then(() => {
          setCopied(true)
          if (timerRef.current) clearTimeout(timerRef.current)
          timerRef.current = setTimeout(() => {
            setCopied(false)
          }, resetMs)
        })
        .catch(fail)
    },
    [resetMs, onFailure]
  )

  return { copied, copy }
}
