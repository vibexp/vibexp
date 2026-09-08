import { useCallback, useRef, useState } from 'react'

import { useTeam } from '@/contexts/TeamContext'
import { promptService } from '@/services/promptService'

export interface UsePromptLabelsResult {
  labels: string[]
  loading: boolean
  error: string | null
  /** Loads the catalog once, on first use. Safe to call on every popover open. */
  load: () => void
}

/**
 * The team's prompt label catalog, for the prompts list taxonomy filter (#908).
 *
 * Lazy on purpose: the catalog is only needed once the filter's popover opens,
 * and every prompts page view would otherwise pay a request for a control most
 * visits never touch. `loadedRef` makes repeated opens free — the labels of a
 * team change rarely enough that a per-open refetch buys nothing — and doubles
 * as the staleness guard: switching team does not remount the prompts page, so
 * without it a slow response for the previous team could land on the new one's
 * filter (the same `cancelled` discipline `useTypes` keeps).
 *
 * Not exported from the `@/hooks` barrel: page suites mock that barrel
 * wholesale, and Vitest's strict export validation turns a new member into a
 * hard error in every one of them.
 */
export function usePromptLabels(): UsePromptLabelsResult {
  const { currentTeam } = useTeam()
  const teamId = currentTeam?.id
  const [labels, setLabels] = useState<string[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const loadedRef = useRef<string | null>(null)

  const load = useCallback(() => {
    if (!teamId || loadedRef.current === teamId) return
    loadedRef.current = teamId
    setLoading(true)
    setError(null)
    const isCurrent = () => loadedRef.current === teamId
    promptService
      .getPromptLabels(teamId)
      .then(next => {
        if (isCurrent()) setLabels(next)
      })
      .catch(() => {
        // A failed catalog must not re-arm on the next open only to fail again;
        // it re-arms when the team changes, which is when it could differ.
        if (isCurrent()) setError('Failed to load labels')
      })
      .finally(() => {
        if (isCurrent()) setLoading(false)
      })
  }, [teamId])

  return { labels, loading, error, load }
}
