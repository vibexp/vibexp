import { useEffect, useState } from 'react'

import { getErrorMessage } from '@/utils/errorHandling'

/**
 * Fetches one admin team configuration section when the tab mounts, and again
 * whenever `load` changes (memoise it on its inputs with `useCallback`).
 *
 * Radix unmounts an inactive `TabsContent`, so a tab's request is made only
 * when the tab is opened. A late response for a superseded request is dropped.
 */
export function useAdminTeamSection<T>(
  load: () => Promise<T>,
  fallbackError: string
): { data: T | null; loading: boolean; error: string | null } {
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    load()
      .then(result => {
        if (!cancelled) setData(result)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(getErrorMessage(err, fallbackError))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [load, fallbackError])

  return { data, loading, error }
}
