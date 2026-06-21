export interface Message {
  role: 'user' | 'agent'
  text: string
  messageId?: string
  timestamp?: string
  isError?: boolean
  artifactId?: string
}

export interface ExecutionMetadata {
  taskId: string
  started?: string
  duration?: number
  status?: string
}

export const STREAMING = 'streaming'
export const PLACEHOLDER_TEXT = 'Working on your request...'
