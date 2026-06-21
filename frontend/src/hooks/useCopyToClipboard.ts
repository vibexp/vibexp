import { useCallback, useEffect, useRef, useState } from 'react'

/**
 * Copy-to-clipboard with transient "copied" feedback. Returns the current
 * `copied` flag and a `copy(value)` action; the flag reverts after `resetMs`.
 * Shared by `CopyButton` and the metadata slug chip so both behave identically.
 */
export function useCopyToClipboard(resetMs = 1500) {
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
      navigator.clipboard
        .writeText(value)
        .then(() => {
          setCopied(true)
          if (timerRef.current) clearTimeout(timerRef.current)
          timerRef.current = setTimeout(() => {
            setCopied(false)
          }, resetMs)
        })
        .catch((err: unknown) => {
          console.error('Copy failed:', err)
        })
    },
    [resetMs]
  )

  return { copied, copy }
}
