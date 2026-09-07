import { useCallback } from 'react'

import { STORAGE_KEYS } from '@/constants/storageKeys'
import { useLocalStorage } from '@/hooks/useLocalStorage'

import type { BodyViewMode } from './types'

/**
 * The reader's Rendered / Raw preference, remembered per browser and shared by
 * every resource body (#901) — the details-column collapse precedent.
 *
 * Persisted as a boolean rather than the mode string: `storage.set` writes a
 * string value verbatim (no quotes) while `storage.getJSON` always
 * `JSON.parse`s, so `'raw'` is written fine and then throws on the way back
 * out, silently resetting to the default on every reload.
 *
 * `PromptDetail` needs the mode in its own render effects, so it calls this
 * directly and drives `ResourceBody` controlled; every other page lets
 * `ResourceBody` own it.
 */
export function useBodyViewMode() {
  const [raw, setRaw] = useLocalStorage<boolean>(
    STORAGE_KEYS.BODY_VIEW_RAW,
    false
  )

  const setMode = useCallback(
    (mode: BodyViewMode) => {
      setRaw(mode === 'raw')
    },
    [setRaw]
  )

  const mode: BodyViewMode = raw ? 'raw' : 'rendered'
  return [mode, setMode] as const
}
