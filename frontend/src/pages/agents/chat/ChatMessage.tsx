import { Bot } from 'lucide-react'
import { useState } from 'react'

import { cn } from '@/lib/utils'
import type { Agent } from '@/services/agentService'

import { type Message, STREAMING } from './types'

interface ChatMessageProps {
  message: Message
  agent: Agent
}

function formatTime(dateString: string): string {
  if (dateString === STREAMING) return ''
  const date = new Date(dateString)
  if (isNaN(date.getTime())) return ''
  return date.toLocaleTimeString('en-US', {
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function ChatMessage({ message, agent }: ChatMessageProps) {
  const [iconLoadError, setIconLoadError] = useState(false)
  const isUser = message.role === 'user'
  const isStreaming = message.timestamp === STREAMING

  return (
    <div className={cn('flex', isUser ? 'justify-end' : 'justify-start')}>
      <div
        className={cn(
          'max-w-3xl rounded-lg px-4 py-3',
          isUser
            ? 'bg-primary text-primary-foreground'
            : message.isError
              ? 'border-destructive/30 bg-destructive/10 text-destructive border'
              : 'bg-muted'
        )}
      >
        <div className="flex items-start gap-3">
          {!isUser && (
            <div className="bg-background flex size-8 shrink-0 items-center justify-center overflow-hidden rounded-md border">
              {agent.agent_card?.iconUrl && !iconLoadError ? (
                <img
                  src={agent.agent_card.iconUrl}
                  alt={agent.name}
                  className="size-full object-cover"
                  onError={() => {
                    setIconLoadError(true)
                  }}
                />
              ) : (
                <Bot className="text-muted-foreground size-4" />
              )}
            </div>
          )}
          <div className="flex-1">
            <div className="whitespace-pre-wrap break-words text-sm">
              {message.text}
            </div>
            {message.timestamp && !isStreaming && (
              <div
                className={cn(
                  'mt-1.5 text-xs',
                  isUser
                    ? 'text-primary-foreground/80'
                    : 'text-muted-foreground'
                )}
              >
                {formatTime(message.timestamp)}
              </div>
            )}
            {isStreaming && !isUser && (
              <div className="text-muted-foreground mt-1.5 flex items-center gap-2 text-xs">
                <span className="inline-block size-2 animate-pulse rounded-full bg-current" />
                Streaming response…
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
