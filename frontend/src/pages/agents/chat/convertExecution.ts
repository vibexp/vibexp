import type { AgentExecution } from '@/services/agentService'
import type { A2AArtifact, A2APart, A2ATextPart } from '@/types/a2a'

import type { Message } from './types'

export function convertExecutionToMessages(
  execution: AgentExecution
): Message[] {
  const messages: Message[] = []

  // Truthy guard (not `!== undefined`): the spec types `input` as non-nullable,
  // but a runtime `null` would still crash the `'text' in input` check below.
  if (execution.input) {
    const inputText =
      typeof execution.input === 'object' && 'text' in execution.input
        ? (execution.input as { text: string }).text
        : JSON.stringify(execution.input)

    messages.push({
      role: 'user',
      text: inputText,
      timestamp: execution.started_at,
    })
  }

  const hasError = execution.status === 'error' || execution.status === 'failed'

  let responseText = 'No response'
  if (hasError && execution.error) {
    responseText = `Error: ${execution.error}`
  } else if (execution.artifacts && Array.isArray(execution.artifacts)) {
    const textParts = (execution.artifacts as A2AArtifact[])
      .flatMap((artifact: A2AArtifact) => artifact.parts ?? [])
      .filter((part: A2APart): part is A2ATextPart => part.kind === 'text')
      .map((part: A2ATextPart) => part.text)
    if (textParts.length > 0) {
      responseText = textParts.join('\n')
    }
  }

  messages.push({
    role: 'agent',
    text: responseText,
    timestamp: execution.ended_at ?? execution.started_at,
    isError: hasError,
  })

  return messages
}
