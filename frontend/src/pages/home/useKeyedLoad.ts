import { useEffect, useLayoutEffect, useRef, useState } from 'react'

export interface KeyedLoad<T> {
  data: T | null
  loading: boolean
  error: boolean
}

/**
 * Loads one dashboard block's data, independently of every other block.
 *
 * `key` names what is being loaded (team, project, range…): a new key loads
 * again, `null` loads nothing. A response for a key that is no longer current
 * is dropped, so a slow request for the previous team or project can never
 * land on top of the new one. A failure is this block's alone — it is reported
 * as `error` and never thrown, so one failing request cannot blank the page.
 */
export function useKeyedLoad<T>(
  key: string | null,
  load: () => Promise<T>
): KeyedLoad<T> {
  // The latest loader, without making its identity a reason to reload.
  const loadRef = useRef(load)
  useLayoutEffect(() => {
    loadRef.current = load
  })

  const [settled, setSettled] = useState<{
    key: string
    data: T | null
    error: boolean
  } | null>(null)

  useEffect(() => {
    if (key === null) return
    let cancelled = false
    loadRef
      .current()
      .then(data => {
        if (!cancelled) setSettled({ key, data, error: false })
      })
      .catch((error: unknown) => {
        console.warn('Dashboard block failed to load:', error)
        if (!cancelled) setSettled({ key, data: null, error: true })
      })
    return () => {
      cancelled = true
    }
  }, [key])

  const current = settled?.key === key ? settled : null
  return {
    data: current?.data ?? null,
    loading: key !== null && current === null,
    error: current?.error ?? false,
  }
}
