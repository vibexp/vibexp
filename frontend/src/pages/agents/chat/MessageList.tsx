import { ArrowUp, Bot } from 'lucide-react'
import { useEffect, useRef } from 'react'

import { Button } from '@/components/ui/button'
import type { Agent } from '@/services/agentService'

import { ChatMessage } from './ChatMessage'
import type { Message } from './types'

interface MessageListProps {
  messages: Message[]
  agent: Agent
  conversationId: string | null
  hasEarlierMessages: boolean
  isLoadingEarlier: boolean
  onLoadEarlier: () => void
  isExecuting: boolean
  currentState: string | null
}

export function MessageList({
  messages,
  agent,
  conversationId,
  hasEarlierMessages,
  isLoadingEarlier,
  onLoadEarlier,
  isExecuting,
  currentState,
}: Readonly<MessageListProps>) {
  const messagesEndRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  return (
    <div className="flex-1 space-y-4 overflow-y-auto p-6">
      {hasEarlierMessages && conversationId && (
        <div className="flex justify-center">
          <Button
            variant="outline"
            size="sm"
            onClick={onLoadEarlier}
            disabled={isLoadingEarlier}
          >
            <ArrowUp className="mr-2 size-4" />
            {isLoadingEarlier ? 'Loading…' : 'Load earlier messages'}
          </Button>
        </div>
      )}

      {messages.length === 0 && !conversationId && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <Bot className="text-muted-foreground mb-3 size-12" />
          <h3 className="text-lg font-medium">Start a conversation</h3>
          <p className="text-muted-foreground mt-1 text-sm">
            Send a message to {agent.name} to begin
          </p>
        </div>
      )}

      {messages.map((message, index) => (
        <ChatMessage key={index} message={message} agent={agent} />
      ))}

      {isExecuting && (
        <div className="text-muted-foreground flex items-center gap-2 pl-4 text-sm">
          <span className="inline-block size-2 animate-pulse rounded-full bg-current" />
          {currentState ? `Agent is ${currentState}…` : 'Thinking…'}
        </div>
      )}

      <div ref={messagesEndRef} />
    </div>
  )
}
