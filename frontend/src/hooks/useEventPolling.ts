import { useCallback, useEffect, useRef, useState } from 'react'

import { getApiBaseUrl } from '../utils/environment'

// Polling interval constant (in milliseconds)
// Adjust this value to control how frequently events are fetched
export const EVENT_POLLING_INTERVAL_MS = 3000 // 3 seconds

export interface AgentExecutionEvent {
  id: string
  execution_id: string
  event_type: string
  event_data: Record<string, unknown>
  sequence_number: number
  received_at: string
}

export interface EventPollingResponse {
  execution_id: string
  status: string
  current_state: string | null
  events: AgentExecutionEvent[]
  has_more: boolean
  next_sequence: number
}

// Simple fetch wrapper for event polling
async function fetchEvents(
  executionId: string,
  since: number
): Promise<EventPollingResponse> {
  const url = `${getApiBaseUrl()}/agents/executions/${executionId}/events?since=${String(since)}`
  const response = await fetch(url, {
    headers: {
      'Content-Type': 'application/json',
    },
    credentials: 'include',
  })

  if (!response.ok) {
    if (response.status === 401) {
      window.location.href = '/sign-in'
      throw new Error('Session expired. Please sign in again.')
    }
    // Try to parse error response, fallback to generic error
    let errorMessage = 'Network error'
    try {
      const errorData = (await response.json()) as { message?: string }
      if (errorData.message) {
        errorMessage = errorData.message
      }
    } catch {
      // Use default error message if JSON parsing fails
    }
    throw new Error(
      `${errorMessage} (HTTP ${String(response.status)}: ${response.statusText})`
    )
  }

  return response.json() as Promise<EventPollingResponse>
}

interface UseEventPollingOptions {
  executionId: string | null
  enabled?: boolean
  onEvent?: (event: AgentExecutionEvent) => void
  onComplete?: (status: string) => void
  pollingInterval?: number
}

export function useEventPolling({
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
    if (!executionId || !enabled || pollingRef.current) {
      return
    }

    pollingRef.current = true
    setIsPolling(true)

    console.log(
      `[EventPolling] Polling events since sequence ${String(nextSequenceRef.current)}`
    )

    try {
      const response = await fetchEvents(executionId, nextSequenceRef.current)

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
      setCurrentState(current_state)
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
      setError(error.message)
      setIsPolling(false)
      pollingRef.current = false

      // Retry on error after interval
      timeoutRef.current = setTimeout(() => {
        pollingRef.current = false
        void poll()
      }, pollingIntervalRef.current * 2) // Back off on error
    }
  }, [executionId, enabled]) // Only depend on executionId and enabled

  // Start polling when enabled and executionId is set
  useEffect(() => {
    if (enabled && executionId) {
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
  }, [executionId, enabled, poll]) // poll is now stable with refs

  return {
    events,
    status,
    currentState,
    error,
    isPolling,
  }
}
