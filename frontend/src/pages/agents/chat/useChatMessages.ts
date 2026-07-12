import { useCallback, useRef, useState } from 'react'

import {
  type AgentExecutionEvent,
  useEventPolling,
} from '@/hooks/useEventPolling'
import { toast } from '@/lib/toast'
import type { Agent, AgentExecution } from '@/services/agentService'
import { agentService } from '@/services/agentService'
import {
  type A2AArtifact,
  type A2AArtifactUpdateEventData,
  type A2APart,
  isTextPart,
} from '@/types/a2a'
import { getErrorMessage } from '@/utils/errorHandling'

import { convertExecutionToMessages } from './convertExecution'
import {
  type ExecutionMetadata,
  type Message,
  PLACEHOLDER_TEXT,
  STREAMING,
} from './types'

interface UseChatMessagesArgs {
  teamId: string | undefined
  agent: Agent | null
  conversationId: string | null
  onConversationCaptured: (conversationId: string) => void
}

export function updateStreamingMessages(
  prev: Message[],
  newText: string,
  shouldAppend: boolean,
  artifactId: string
): Message[] {
  const lastMessage = prev[prev.length - 1]

  if (
    lastMessage.role === 'agent' &&
    lastMessage.timestamp === STREAMING &&
    shouldAppend &&
    lastMessage.artifactId === artifactId
  ) {
    return [
      ...prev.slice(0, -1),
      { ...lastMessage, text: lastMessage.text + newText },
    ]
  }

  if (
    lastMessage.role === 'agent' &&
    lastMessage.timestamp === STREAMING &&
    lastMessage.text === PLACEHOLDER_TEXT
  ) {
    return [
      ...prev.slice(0, -1),
      {
        role: 'agent',
        text: newText,
        timestamp: STREAMING,
        artifactId,
      },
    ]
  }

  if (lastMessage.timestamp === STREAMING && !shouldAppend) {
    return prev
  }

  return [
    ...prev.slice(0, -1),
    { ...lastMessage, timestamp: new Date().toISOString() },
    {
      role: 'agent',
      text: newText,
      timestamp: STREAMING,
      artifactId,
    },
  ]
}

export function extractResponseText(execution: AgentExecution): string {
  const hasError = execution.status === 'error' || execution.status === 'failed'
  if (hasError && execution.error) return `Error: ${execution.error}`
  if (execution.artifacts && Array.isArray(execution.artifacts)) {
    const textParts = (execution.artifacts as A2AArtifact[])
      .flatMap((artifact: A2AArtifact) => artifact.parts ?? [])
      .filter(isTextPart)
      .map(part => part.text)
    if (textParts.length > 0) return textParts.join('\n')
  }
  if (execution.status === 'cancelled') return 'Cancelled'
  return 'No response received'
}

export function useChatMessages({
  teamId,
  agent,
  conversationId,
  onConversationCaptured,
}: UseChatMessagesArgs) {
  const [messages, setMessages] = useState<Message[]>([])
  const [isExecuting, setIsExecuting] = useState(false)
  const [executionMetadata, setExecutionMetadata] =
    useState<ExecutionMetadata | null>(null)
  const [currentExecutionId, setCurrentExecutionId] = useState<string | null>(
    null
  )
  const [hasEarlierMessages, setHasEarlierMessages] = useState(false)
  const [isLoadingEarlier, setIsLoadingEarlier] = useState(false)
  const [totalMessageCount, setTotalMessageCount] = useState(0)

  const processedExecutions = useRef<Set<string>>(new Set())
  const processedEvents = useRef<Set<string>>(new Set())

  const handleStreamingEvent = useCallback((event: AgentExecutionEvent) => {
    const eventKey = `${event.execution_id}-${String(event.sequence_number)}`
    if (processedEvents.current.has(eventKey)) return
    processedEvents.current.add(eventKey)

    if (event.event_type !== 'artifact-update') return

    const eventData = event.event_data as A2AArtifactUpdateEventData
    const artifactData: A2AArtifact = eventData.artifact ?? {
      parts: [],
      artifactId: 'unknown',
    }
    const parts: A2APart[] = artifactData.parts ?? []

    const textParts = parts.filter(isTextPart).map(part => part.text)
    if (textParts.length === 0) return

    const newText = textParts.join('\n')
    const shouldAppend = eventData.append === true
    const artifactId = artifactData.artifactId

    setMessages(prev =>
      updateStreamingMessages(prev, newText, shouldAppend, artifactId)
    )
  }, [])

  const handleExecutionComplete = useCallback(
    (execution: AgentExecution) => {
      if (processedExecutions.current.has(execution.id)) return
      processedExecutions.current.add(execution.id)

      const hasError =
        execution.status === 'error' || execution.status === 'failed'
      const responseText = extractResponseText(execution)

      const metadata: ExecutionMetadata = {
        taskId: execution.id,
        status: execution.status,
      }
      if (execution.started_at) metadata.started = execution.started_at
      if (execution.duration !== null && execution.duration !== undefined) {
        metadata.duration = execution.duration / 1000
      }

      const agentMessage: Message = {
        role: 'agent',
        text: responseText,
        timestamp: new Date().toISOString(),
        isError: hasError,
      }

      setMessages(prev => {
        const lastMessage = prev[prev.length - 1]
        if (
          lastMessage.role === 'agent' &&
          lastMessage.timestamp === STREAMING
        ) {
          return [...prev.slice(0, -1), agentMessage]
        }
        return [...prev, agentMessage]
      })

      setExecutionMetadata(metadata)
      setIsExecuting(false)

      if (!conversationId && execution.conversation_id) {
        onConversationCaptured(execution.conversation_id)
      }

      if (hasError) {
        toast.error(execution.error ?? 'Agent execution failed')
      }
    },
    [conversationId, onConversationCaptured]
  )

  const { currentState } = useEventPolling({
    teamId,
    executionId: currentExecutionId,
    enabled: !!currentExecutionId,
    onEvent: handleStreamingEvent,
    onComplete: () => {
      void (async () => {
        if (currentExecutionId && teamId) {
          try {
            const response = await agentService.getExecutionStatus(
              teamId,
              currentExecutionId
            )
            const execution = response
            handleExecutionComplete(execution)
          } catch (error) {
            toast.error(
              getErrorMessage(error, 'Failed to get final execution state')
            )
          }
        }
        setCurrentExecutionId(null)
      })()
    },
  })

  const loadConversation = useCallback(
    async (convId: string, limit = 50) => {
      if (!teamId) return
      try {
        const response = await agentService.getConversationExecutions(
          teamId,
          convId,
          { limit }
        )
        setTotalMessageCount(response.total_count)
        setHasEarlierMessages(response.has_more)
        setMessages(response.executions.flatMap(convertExecutionToMessages))
      } catch (error) {
        toast.error(getErrorMessage(error, 'Failed to load conversation'))
      }
    },
    [teamId]
  )

  const loadEarlierMessages = useCallback(async () => {
    if (!conversationId || isLoadingEarlier || !hasEarlierMessages || !teamId) {
      return
    }

    try {
      setIsLoadingEarlier(true)
      if (messages.length === 0) return
      const oldestMessage = messages[0]
      if (!oldestMessage.timestamp) return

      const response = await agentService.getConversationExecutions(
        teamId,
        conversationId,
        { limit: 50, before: oldestMessage.timestamp }
      )
      const olderMessages = response.executions.flatMap(
        convertExecutionToMessages
      )
      setMessages(prev => [...olderMessages, ...prev])
      setHasEarlierMessages(response.has_more)
    } catch (error) {
      toast.error(getErrorMessage(error, 'Failed to load earlier messages'))
    } finally {
      setIsLoadingEarlier(false)
    }
  }, [teamId, conversationId, isLoadingEarlier, hasEarlierMessages, messages])

  const sendMessage = useCallback(
    async (text: string) => {
      if (!agent || !text.trim() || !teamId) return

      const userMessage: Message = {
        role: 'user',
        text: text.trim(),
        timestamp: new Date().toISOString(),
      }
      const placeholderMessage: Message = {
        role: 'agent',
        text: PLACEHOLDER_TEXT,
        timestamp: STREAMING,
      }

      setMessages(prev => [...prev, userMessage, placeholderMessage])

      try {
        setIsExecuting(true)
        const inputMode = agent.agent_card?.defaultInputModes?.[0] ?? 'text'
        const response = await agentService.executeAgent(
          teamId,
          agent.id,
          { [inputMode]: userMessage.text },
          conversationId ?? undefined
        )
        const executionData = response

        if (
          executionData.status === 'pending' ||
          executionData.status === 'submitted'
        ) {
          processedEvents.current.clear()
          setCurrentExecutionId(executionData.id)
          return
        }
        handleExecutionComplete(executionData)
      } catch (error) {
        const errorMessage = getErrorMessage(error, 'Failed to send message')
        const errorAgentMessage: Message = {
          role: 'agent',
          text: `Error: ${errorMessage}`,
          timestamp: new Date().toISOString(),
          isError: true,
        }
        setMessages(prev => {
          const lastMessage = prev[prev.length - 1]
          if (
            lastMessage.role === 'agent' &&
            lastMessage.text === PLACEHOLDER_TEXT
          ) {
            return [...prev.slice(0, -1), errorAgentMessage]
          }
          return [...prev, errorAgentMessage]
        })
        toast.error(errorMessage)
        setIsExecuting(false)
      }
    },
    [agent, teamId, conversationId, handleExecutionComplete]
  )

  const cancelExecution = useCallback(async () => {
    if (!teamId || !currentExecutionId) return
    try {
      const execution = await agentService.cancelExecution(
        teamId,
        currentExecutionId
      )
      handleExecutionComplete(execution)
      setCurrentExecutionId(null)
    } catch (error) {
      toast.error(getErrorMessage(error, 'Failed to cancel execution'))
    }
  }, [teamId, currentExecutionId, handleExecutionComplete])

  const reset = useCallback(() => {
    setMessages([])
    setHasEarlierMessages(false)
    setTotalMessageCount(0)
    setExecutionMetadata(null)
    processedEvents.current.clear()
    processedExecutions.current.clear()
  }, [])

  return {
    messages,
    isExecuting,
    executionMetadata,
    setExecutionMetadata,
    currentState,
    currentExecutionId,
    hasEarlierMessages,
    isLoadingEarlier,
    totalMessageCount,
    loadConversation,
    loadEarlierMessages,
    sendMessage,
    cancelExecution,
    reset,
  }
}
