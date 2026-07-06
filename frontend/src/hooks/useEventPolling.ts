import type { components } from '@vibexp/api-client'
import { useCallback, useEffect, useRef, useState } from 'react'

import { generatedClient, unwrap } from '@/lib/apiClientGenerated'
import { ApiError } from '@/types/errors'

// Polling interval constant (in milliseconds)
// Adjust this value to control how frequently events are fetched
export const EVENT_POLLING_INTERVAL_MS = 3000 // 3 seconds

export type AgentExecutionEvent = components['schemas']['AgentExecutionEvent']

// Cursor-based polling response (the `since` query selects this variant over the
// page-based one). The generated 200 type is a union of the two shapes.
type EventPollingResponse =
  components['schemas']['AgentExecutionEventsPollResponse']

// Fetch the next batch of execution events through the generated, team-scoped
// operation. Passing `since` selects cursor-based polling, so the response is
// always the poll variant of the union.
async function fetchEvents(
  teamId: string,
  executionId: string,
  since: number
): Promise<EventPollingResponse> {
  const response = await unwrap(
    generatedClient.GET('/api/v1/{team_id}/agents/executions/{id}/events', {
      params: { path: { team_id: teamId, id: executionId }, query: { since } },
    })
  )
  return response as EventPollingResponse
}

interface UseEventPollingOptions {
  teamId: string | undefined
  executionId: string | null
  enabled?: boolean
  onEvent?: (event: AgentExecutionEvent) => void
  onComplete?: (status: string) => void
  pollingInterval?: number
}

export function useEventPolling({
  teamId,
  executionId,
  enabled = true,
  onEvent,
  onComplete,
  pollingInterval = EVENT_POLLING_INTERVAL_MS,
}: UseEventPollingOptions) {
  const [events, setEvents] = useState<AgentExecutionEvent[]>([])
  const [status, setStatus] = useState<string>('pending')
  const [currentState, setCurrentState] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [isPolling, setIsPolling] = useState(false)

  const nextSequenceRef = useRef(0)
  const timeoutRef = useRef<NodeJS.Timeout | null>(null)
  const pollingRef = useRef(false)
  const onEventRef = useRef(onEvent)
  const onCompleteRef = useRef(onComplete)
  const pollingIntervalRef = useRef(pollingInterval)

  // Update refs when callbacks change
  useEffect(() => {
    onEventRef.current = onEvent
    onCompleteRef.current = onComplete
    pollingIntervalRef.current = pollingInterval
  }, [onEvent, onComplete, pollingInterval])

  const poll = useCallback(async () => {
    if (!teamId || !executionId || !enabled || pollingRef.current) {
      return
    }

    pollingRef.current = true
    setIsPolling(true)

    console.log(
      `[EventPolling] Polling events since sequence ${String(nextSequenceRef.current)}`
    )

    try {
      const response = await fetchEvents(
        teamId,
        executionId,
        nextSequenceRef.current
      )

      const {
        events: newEvents,
        status: execStatus,
        current_state,
        has_more,
        next_sequence,
      } = response

      console.log(
        `[EventPolling] Received ${String(newEvents.length)} new events, status: ${execStatus}`
      )

      // Update state
      setStatus(execStatus)
      setCurrentState(current_state ?? null)
      nextSequenceRef.current = next_sequence

      // Process new events
      if (newEvents.length > 0) {
        setEvents(prev => [...prev, ...newEvents])

        // Call event callback for each new event
        const currentOnEvent = onEventRef.current
        if (currentOnEvent) {
          newEvents.forEach((event: AgentExecutionEvent) => {
            currentOnEvent(event)
          })
        }
      }

      // Check if execution is complete
      const isComplete =
        execStatus === 'success' ||
        execStatus === 'error' ||
        execStatus === 'failed'

      if (isComplete) {
        const currentOnComplete = onCompleteRef.current
        if (currentOnComplete) {
          currentOnComplete(execStatus)
        }
        setIsPolling(false)
        pollingRef.current = false
        return
      }

      // Continue polling if has_more
      if (has_more) {
        timeoutRef.current = setTimeout(() => {
          pollingRef.current = false
          void poll()
        }, pollingIntervalRef.current)
      } else {
        setIsPolling(false)
        pollingRef.current = false
      }
    } catch (err) {
      const error = err as Error
      console.error('Event polling error:', error)

      // Session expired — send the user back to sign in (preserves the
      // hand-written client's 401 behaviour now that the transport throws
      // ApiError instead of redirecting itself).
      if (error instanceof ApiError && error.status === 401) {
        setIsPolling(false)
        pollingRef.current = false
        window.location.href = '/sign-in'
        return
      }

      setError(error.message)
      setIsPolling(false)
      pollingRef.current = false

      // Retry on error after interval
      timeoutRef.current = setTimeout(() => {
        pollingRef.current = false
        void poll()
      }, pollingIntervalRef.current * 2) // Back off on error
    }
  }, [teamId, executionId, enabled]) // Only depend on teamId, executionId and enabled

  // Start polling when enabled and both teamId and executionId are set
  useEffect(() => {
    if (enabled && teamId && executionId) {
      // Reset state for new execution
      setEvents([])
      setStatus('pending')
      setCurrentState(null)
      setError(null)
      nextSequenceRef.current = 0
      pollingRef.current = false

      // Start polling
      void poll()
    }

    // Cleanup on unmount or when executionId/enabled changes
    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current)
        timeoutRef.current = null
      }
      pollingRef.current = false
      setIsPolling(false)
    }
  }, [teamId, executionId, enabled, poll]) // poll is now stable with refs

  return {
    events,
    status,
    currentState,
    error,
    isPolling,
  }
}
