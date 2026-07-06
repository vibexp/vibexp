import { useCallback, useEffect, useRef, useState } from 'react'

import type { AgentExecution } from '../services/agentService'
import { agentService } from '../services/agentService'

interface UseExecutionPollingOptions {
  teamId: string | null
  executionId: string | null
  enabled?: boolean
  interval?: number // milliseconds, default 5000
  onComplete?: (execution: AgentExecution) => void
  onError?: (error: Error) => void
}

export function useExecutionPolling({
  teamId,
  executionId,
  enabled = true,
  interval = 5000,
  onComplete,
  onError,
}: UseExecutionPollingOptions) {
  const [execution, setExecution] = useState<AgentExecution | null>(null)
  const [isPolling, setIsPolling] = useState(false)
  const [error, setError] = useState<Error | null>(null)
  const timeoutRef = useRef<NodeJS.Timeout | null>(null)
  const onCompleteRef = useRef(onComplete)
  const onErrorRef = useRef(onError)

  // Update refs when callbacks change (doesn't trigger re-render)
  useEffect(() => {
    onCompleteRef.current = onComplete
  }, [onComplete])

  useEffect(() => {
    onErrorRef.current = onError
  }, [onError])

  const isTerminalState = useCallback((status: string) => {
    return ['success', 'error', 'completed', 'failed', 'cancelled'].includes(
      status
    )
  }, [])

  const pollStatus = useCallback(async () => {
    if (!executionId || !enabled || !teamId) return

    try {
      setIsPolling(true)
      const response = await agentService.getExecutionStatus(
        teamId,
        executionId
      )
      const executionData =
        (response as { data?: AgentExecution } | undefined)?.data ??
        (response as unknown as AgentExecution)

      setExecution(executionData)
      setError(null)

      if (isTerminalState(executionData.status)) {
        setIsPolling(false)
        onCompleteRef.current?.(executionData)
        return
      }

      timeoutRef.current = setTimeout(() => {
        void pollStatus()
      }, interval)
    } catch (err) {
      const error =
        err instanceof Error ? err : new Error('Failed to poll status')
      setError(error)
      setIsPolling(false)
      onErrorRef.current?.(error)
    }
  }, [teamId, executionId, enabled, interval, isTerminalState])

  useEffect(() => {
    if (executionId && enabled) {
      void pollStatus()
    }
    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current)
        timeoutRef.current = null
      }
    }
  }, [executionId, enabled, pollStatus])

  const stopPolling = useCallback(() => {
    setIsPolling(false)
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current)
      timeoutRef.current = null
    }
  }, [])

  return { execution, isPolling, error, stopPolling }
}
