import { useCallback, useEffect, useRef, useState } from 'react'

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
 * team change rarely enough that a per-open refetch buys nothing.
 *
 * Switching team does NOT remount the prompts page, so the catalog must be
 * dropped and re-armed on a team change, and a response must be discarded when
 * it arrives after one — the same `cancelled` discipline `useTypes` keeps. The
 * guard reads the CURRENT team from a ref rather than comparing against
 * `loadedRef`: `loadedRef` still holds the team that started the request, so it
 * would happily accept its own stale answer.
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
  const currentTeamRef = useRef(teamId)

  useEffect(() => {
    currentTeamRef.current = teamId
    loadedRef.current = null
    // Identity-preserving so an unchanged empty catalog does not re-render:
    // a fresh `[]` on mount churns the render count page suites depend on.
    setLabels(prev => (prev.length === 0 ? prev : []))
    setError(null)
  }, [teamId])

  const load = useCallback(() => {
    if (!teamId || loadedRef.current === teamId) return
    loadedRef.current = teamId
    setLoading(true)
    setError(null)
    const isCurrent = () => currentTeamRef.current === teamId
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
