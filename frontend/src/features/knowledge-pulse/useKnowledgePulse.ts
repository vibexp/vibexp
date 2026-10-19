import { useCallback, useEffect, useReducer, useRef, useState } from 'react'

import { fetchKnowledgePulse } from './api'
import {
  initialPulseState,
  pulseReducer,
  type PulseState,
  sortEventsByTime,
  spreadDelays,
} from './pulseState'

export const POLL_INTERVALS_MS = [10000, 20000, 30000] as const
export type PollInterval = (typeof POLL_INTERVALS_MS)[number]

const PRUNE_EVERY_MS = 400
/** Most events one poll replays with animation; the rest land silently. */
const MAX_ANIMATED_PER_POLL = 30

export interface KnowledgePulse {
  state: PulseState
  playing: boolean
  setPlaying: (playing: boolean) => void
  intervalMs: PollInterval
  setIntervalMs: (ms: PollInterval) => void
  loading: boolean
  error: string | null
  /** True while the tab is hidden: polling is suspended, not paused. */
  hidden: boolean
  lastPolledAt: number | null
}

function useDocumentHidden(): boolean {
  const [hidden, setHidden] = useState(
    () => typeof document !== 'undefined' && document.hidden
  )
  useEffect(() => {
    const onChange = () => {
      setHidden(document.hidden)
    }
    document.addEventListener('visibilitychange', onChange)
    return () => {
      document.removeEventListener('visibilitychange', onChange)
    }
  }, [])
  return hidden
}

/**
 * Drives the Knowledge Pulse page: loads the snapshot plus the last few
 * minutes of events, then (only while playing and the tab is visible) polls
 * for deltas every `intervalMs` and replays each batch spread across the
 * interval so the ring moves continuously instead of flashing once per poll.
 */
export function useKnowledgePulse(teamId: string | undefined): KnowledgePulse {
  const [state, dispatch] = useReducer(
    pulseReducer,
    undefined,
    initialPulseState
  )
  const [playing, setPlaying] = useState(false)
  const [intervalMs, setIntervalMs] = useState<PollInterval>(10000)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [lastPolledAt, setLastPolledAt] = useState<number | null>(null)
  const hidden = useDocumentHidden()

  const cursorRef = useRef<string | undefined>(undefined)
  const pendingRef = useRef(
    new Map<ReturnType<typeof setTimeout>, () => void>()
  )

  /** Apply every scheduled-but-unplayed event now, silently. */
  const flushPending = useCallback(() => {
    for (const [timer, apply] of pendingRef.current) {
      clearTimeout(timer)
      apply()
    }
    pendingRef.current.clear()
  }, [])

  // Initial load: the snapshot plus a silent backfill of the recent events.
  useEffect(() => {
    if (!teamId) return
    const controller = new AbortController()
    dispatch({ type: 'reset' })
    cursorRef.current = undefined
    setLoading(true)
    setError(null)
    fetchKnowledgePulse(teamId, {}, controller.signal)
      .then(res => {
        cursorRef.current = res.cursor
        if (res.snapshot) dispatch({ type: 'snapshot', snapshot: res.snapshot })
        const now = Date.now()
        for (const event of sortEventsByTime(res.events)) {
          dispatch({ type: 'event', event, now, animate: false })
        }
        dispatch({ type: 'stats', stats: res.stats_today, at: now })
        setLastPolledAt(now)
      })
      .catch((err: unknown) => {
        if (controller.signal.aborted) return
        console.error('Failed to load knowledge pulse:', err)
        setError(err instanceof Error ? err.message : 'Failed to load')
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => {
      controller.abort()
    }
  }, [teamId])

  // Polling loop: runs only while playing, visible and loaded.
  const active = playing && !hidden && !loading && teamId !== undefined
  useEffect(() => {
    if (!active || !teamId) return
    const controller = new AbortController()
    let stopped = false
    let pollTimer: ReturnType<typeof setTimeout> | undefined

    const poll = async () => {
      try {
        const res = await fetchKnowledgePulse(
          teamId,
          { since: cursorRef.current },
          controller.signal
        )
        if (controller.signal.aborted) return
        cursorRef.current = res.cursor
        if (res.snapshot) dispatch({ type: 'snapshot', snapshot: res.snapshot })
        dispatch({ type: 'stats', stats: res.stats_today, at: Date.now() })
        setLastPolledAt(Date.now())
        setError(null)

        // A catch-up batch (tab was hidden, a burst of agents) would turn
        // into a strobe if every event animated: apply the older ones
        // silently and play only the newest few across the interval.
        const sorted = sortEventsByTime(res.events)
        const cut = Math.max(0, sorted.length - MAX_ANIMATED_PER_POLL)
        const now = Date.now()
        for (const event of sorted.slice(0, cut)) {
          dispatch({ type: 'event', event, now, animate: false })
        }
        const events = sorted.slice(cut)
        const delays = spreadDelays(events.length, intervalMs)
        events.forEach((event, i) => {
          const apply = () => {
            dispatch({ type: 'event', event, now: Date.now(), animate: false })
          }
          const timer = setTimeout(() => {
            pendingRef.current.delete(timer)
            dispatch({ type: 'event', event, now: Date.now(), animate: true })
          }, delays[i])
          pendingRef.current.set(timer, apply)
        })
      } catch (err) {
        if (controller.signal.aborted) return
        console.error('Knowledge pulse poll failed:', err)
        setError(err instanceof Error ? err.message : 'Poll failed')
      }
      if (!stopped) {
        pollTimer = setTimeout(() => void poll(), intervalMs)
      }
    }

    void poll()
    return () => {
      stopped = true
      controller.abort()
      if (pollTimer) clearTimeout(pollTimer)
      // Pausing must not lose what already arrived: show it, unanimated.
      flushPending()
    }
  }, [active, teamId, intervalMs, flushPending])

  // Expire finished pulses.
  useEffect(() => {
    const id = setInterval(() => {
      dispatch({ type: 'prune', now: Date.now() })
    }, PRUNE_EVERY_MS)
    return () => {
      clearInterval(id)
    }
  }, [])

  return {
    state,
    playing,
    setPlaying,
    intervalMs,
    setIntervalMs,
    loading,
    error,
    hidden,
    lastPolledAt,
  }
}
