import { ArrowLeft, Clock, MessageSquare, Play } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { EmptyState } from '@/components/EmptyState'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useTeam } from '@/contexts/TeamContext'
import { toast } from '@/lib/toast'
import type { Agent, ConversationSummary } from '@/services/agentService'
import { agentService } from '@/services/agentService'
import { getErrorMessage } from '@/utils/errorHandling'

import { formatRelativeTime } from './helpers'

interface ConversationsState {
  conversations: ConversationSummary[]
  loading: boolean
  error: string | null
  totalPages: number
  currentPage: number
  total: number
}

function truncateText(text: string, maxLength: number): string {
  if (text.length <= maxLength) return text
  return `${text.substring(0, maxLength)}…`
}

function conversationStatusVariant(
  status: string
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'success':
    case 'completed':
      return 'default'
    case 'error':
    case 'failed':
      return 'destructive'
    case 'running':
    case 'working':
    case 'pending':
    case 'submitted':
      return 'secondary'
    default:
      return 'outline'
  }
}

export function AgentConversations() {
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const { currentTeam } = useTeam()

  const [agent, setAgent] = useState<Agent | null>(null)
  const [loadingAgent, setLoadingAgent] = useState(true)

  const [state, setState] = useState<ConversationsState>({
    conversations: [],
    loading: true,
    error: null,
    totalPages: 0,
    currentPage: 1,
    total: 0,
  })

  const [currentPage, setCurrentPage] = useState(1)
  const limit = 20

  const loadAgent = useCallback(async (agentId: string, teamId: string) => {
    try {
      setLoadingAgent(true)
      const response = await agentService.getAgent(teamId, agentId)
      const agentData = response
      setAgent(agentData)
    } catch (error) {
      toast.error(getErrorMessage(error, 'Failed to load agent'))
    } finally {
      setLoadingAgent(false)
    }
  }, [])

  const fetchConversations = useCallback(
    async (agentId: string, page: number) => {
      if (!currentTeam) return
      try {
        setState(prev => ({ ...prev, loading: true, error: null }))
        const response = await agentService.listAgentConversations(
          currentTeam.id,
          agentId,
          { page, limit }
        )
        setState({
          conversations: response.conversations,
          loading: false,
          error: null,
          totalPages: response.total_pages,
          currentPage: response.page,
          total: response.total_count,
        })
      } catch (error) {
        const errorMessage = getErrorMessage(
          error,
          'Failed to load conversations'
        )
        setState(prev => ({ ...prev, loading: false, error: errorMessage }))
      }
    },
    [currentTeam]
  )

  useEffect(() => {
    if (!id) {
      void navigate('/agents')
      return
    }
    if (!currentTeam) return
    void loadAgent(id, currentTeam.id)
    void fetchConversations(id, currentPage)
  }, [id, currentTeam, currentPage, loadAgent, fetchConversations, navigate])

  if (loadingAgent) {
    return (
      <div className="space-y-6">
        <PageHeader title="Loading…" description="Please wait." />
        <Skeleton className="h-64 w-full" />
      </div>
    )
  }

  if (!agent) {
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
            The agent you are looking for does not exist or has been removed.
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  const hasConversations = state.conversations.length > 0

  return (
    <div className="space-y-6">
      <Button
        variant="ghost"
        size="sm"
        onClick={() => {
          void navigate(`/agents/${agent.id}`)
        }}
      >
        <ArrowLeft className="mr-2 size-4" />
        Back to agent
      </Button>

      <PageHeader
        title={`${agent.name} — Conversations`}
        description={`View and resume conversations with ${agent.name}.`}
        actions={
          <Button
            onClick={() => {
              void navigate(`/agents/${agent.id}/chat`)
            }}
          >
            <MessageSquare className="mr-2 size-4" />
            New chat
          </Button>
        }
      />

      <Card>
        <CardContent className="flex items-center justify-between p-4">
          <div className="flex items-center gap-2">
            <MessageSquare className="text-muted-foreground size-4" />
            <span className="text-sm font-medium">
              Total conversations: {state.total}
            </span>
          </div>
        </CardContent>
      </Card>

      {state.error && (
        <Alert variant="destructive">
          <AlertTitle>Failed to load conversations</AlertTitle>
          <AlertDescription className="space-y-2">
            <p>{state.error}</p>
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                if (id) void fetchConversations(id, currentPage)
              }}
            >
              Retry
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {state.loading && !hasConversations && (
        <Card>
          <CardContent className="space-y-3 p-6">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-20 w-full" />
            ))}
          </CardContent>
        </Card>
      )}

      {!state.loading && !state.error && !hasConversations && (
        <EmptyState
          icon={MessageSquare}
          title="No conversations yet"
          description={`Start a conversation with ${agent.name} to see it here.`}
          actions={
            <Button
              onClick={() => {
                void navigate(`/agents/${agent.id}/chat`)
              }}
            >
              <MessageSquare className="mr-2 size-4" />
              Start new conversation
            </Button>
          }
        />
      )}

      {!state.error && hasConversations && (
        <div className="space-y-4">
          {state.conversations.map(conversation => (
            <Card key={conversation.conversation_id}>
              <CardHeader>
                <div className="flex items-start justify-between gap-4">
                  <CardTitle className="flex-1 text-base">
                    {truncateText(
                      conversation.first_message || 'Untitled conversation',
                      100
                    )}
                  </CardTitle>
                  <Button
                    size="sm"
                    onClick={() => {
                      void navigate(
                        `/agents/${agent.id}/chat?conversation=${conversation.conversation_id}`
                      )
                    }}
                  >
                    <Play className="mr-2 size-4" />
                    Resume
                  </Button>
                </div>
              </CardHeader>
              <CardContent className="space-y-3">
                <div className="text-muted-foreground flex flex-wrap items-center gap-4 text-sm">
                  <span className="flex items-center gap-1">
                    <MessageSquare className="size-4" />
                    {conversation.message_count} message
                    {conversation.message_count !== 1 ? 's' : ''}
                  </span>
                  <span className="flex items-center gap-1">
                    <Clock className="size-4" />
                    Started {formatRelativeTime(conversation.started_at)}
                  </span>
                  <span>
                    Last activity{' '}
                    {formatRelativeTime(conversation.last_activity_at)}
                  </span>
                </div>

                {conversation.last_message && (
                  <p className="text-muted-foreground text-sm">
                    <span className="font-medium">Last message:</span>{' '}
                    {truncateText(conversation.last_message, 150)}
                  </p>
                )}

                <Badge
                  variant={conversationStatusVariant(conversation.last_status)}
                >
                  {conversation.last_status}
                </Badge>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {state.totalPages > 1 && (
        <div className="flex items-center justify-between gap-2">
          <div className="text-muted-foreground text-sm">
            Page {state.currentPage} of {state.totalPages}
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setCurrentPage(p => Math.max(1, p - 1))
                window.scrollTo({ top: 0, behavior: 'smooth' })
              }}
              disabled={state.currentPage <= 1}
            >
              Previous
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setCurrentPage(p => Math.min(state.totalPages, p + 1))
                window.scrollTo({ top: 0, behavior: 'smooth' })
              }}
              disabled={state.currentPage >= state.totalPages}
            >
              Next
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
