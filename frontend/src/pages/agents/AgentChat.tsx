import { ArrowLeft, Info } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'

import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useTeam } from '@/contexts/TeamContext'
import { toast } from '@/lib/toast'
import type { Agent } from '@/services/agentService'
import { agentService } from '@/services/agentService'
import { getErrorMessage } from '@/utils/errorHandling'

import { MessageInput } from './chat/MessageInput'
import { MessageList } from './chat/MessageList'
import { MetadataPanel } from './chat/MetadataPanel'
import { useChatMessages } from './chat/useChatMessages'

export function AgentChat() {
  const navigate = useNavigate()
  const location = useLocation()
  const { id } = useParams<{ id: string }>()
  const { currentTeam } = useTeam()

  const [loading, setLoading] = useState(true)
  const [agent, setAgent] = useState<Agent | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [inputText, setInputText] = useState('')
  const [showMetadata, setShowMetadata] = useState(false)

  const conversationId = useMemo(() => {
    const searchParams = new URLSearchParams(location.search)
    return searchParams.get('conversation')
  }, [location.search])

  const handleConversationCaptured = useCallback(
    (convId: string) => {
      if (agent) {
        void navigate(`/agents/${agent.id}/chat?conversation=${convId}`, {
          replace: true,
        })
      }
    },
    [agent, navigate]
  )

  const {
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
  } = useChatMessages({
    teamId: currentTeam?.id,
    agent,
    conversationId,
    onConversationCaptured: handleConversationCaptured,
  })

  const loadAgent = useCallback(async (agentId: string, teamId: string) => {
    try {
      setLoading(true)
      setError(null)
      const response = await agentService.getAgent(teamId, agentId)
      const responseData = (response as { data?: unknown }).data ?? response
      const agentData = responseData as Agent | Record<string, unknown>
      if (typeof agentData !== 'object' || !('id' in agentData)) {
        throw new Error('No agent data received')
      }
      setAgent(agentData as Agent)
    } catch (err) {
      const errorMessage = getErrorMessage(err, 'Failed to load agent')
      setError(errorMessage)
      toast.error(errorMessage)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (id && currentTeam) {
      void loadAgent(id, currentTeam.id)
    } else if (!id) {
      void navigate('/agents')
    }
  }, [id, currentTeam, loadAgent, navigate])

  const hasLoadedConversationRef = useRef(false)
  useEffect(() => {
    hasLoadedConversationRef.current = false
  }, [conversationId])
  useEffect(() => {
    if (
      conversationId &&
      agent &&
      messages.length === 0 &&
      !hasLoadedConversationRef.current
    ) {
      hasLoadedConversationRef.current = true
      void loadConversation(conversationId)
    }
  }, [conversationId, agent, messages.length, loadConversation])

  const handleSend = () => {
    const text = inputText
    setInputText('')
    void sendMessage(text)
  }

  const handleStartNewConversation = () => {
    reset()
    if (agent) {
      void navigate(`/agents/${agent.id}/chat`)
    }
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <PageHeader title="Loading agent…" description="Please wait." />
        <Skeleton className="h-96 w-full" />
      </div>
    )
  }

  if (error || !agent) {
    return (
      <div className="space-y-4">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => {
            void navigate('/agents')
          }}
        >
          <ArrowLeft className="mr-2 size-4" />
          Back to agents
        </Button>
        <Alert variant="destructive">
          <AlertTitle>Agent not found</AlertTitle>
          <AlertDescription>
            {error ??
              'The agent you are looking for does not exist or has been removed.'}
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  return (
    <div className="flex h-[calc(100vh-8rem)] flex-col space-y-4">
      <PageHeader
        title={`Chat with ${agent.name}`}
        description={agent.description}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                void navigate(`/agents/${agent.id}`)
              }}
            >
              <ArrowLeft className="mr-2 size-4" />
              Back to agent
            </Button>
            {executionMetadata && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setShowMetadata(v => !v)
                }}
              >
                <Info className="mr-2 size-4" />
                {showMetadata ? 'Hide' : 'Show'} metadata
              </Button>
            )}
          </div>
        }
      />

      {showMetadata && executionMetadata && (
        <MetadataPanel
          metadata={executionMetadata}
          onClose={() => {
            setShowMetadata(false)
            setExecutionMetadata(null)
          }}
        />
      )}

      {conversationId && (
        <Alert>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
            <span>
              <span className="font-medium">Continuing conversation.</span>
              {totalMessageCount > 0 && (
                <span className="text-muted-foreground ml-2 text-xs">
                  {messages.length} of {totalMessageCount} messages loaded
                </span>
              )}
            </span>
            <Button
              size="sm"
              variant="outline"
              onClick={handleStartNewConversation}
            >
              Start new chat
            </Button>
          </AlertDescription>
        </Alert>
      )}

      <Card className="flex min-h-0 flex-1 flex-col overflow-hidden p-0">
        <CardContent className="flex min-h-0 flex-1 flex-col p-0">
          <MessageList
            messages={messages}
            agent={agent}
            conversationId={conversationId}
            hasEarlierMessages={hasEarlierMessages}
            isLoadingEarlier={isLoadingEarlier}
            onLoadEarlier={() => {
              void loadEarlierMessages()
            }}
            isExecuting={isExecuting}
            currentState={currentState}
          />
          {currentExecutionId && (
            <div className="flex justify-end px-1 pb-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => {
                  void cancelExecution()
                }}
              >
                Cancel
              </Button>
            </div>
          )}
          <MessageInput
            value={inputText}
            onChange={setInputText}
            onSend={handleSend}
            disabled={isExecuting}
          />
        </CardContent>
      </Card>
    </div>
  )
}
