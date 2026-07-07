import { ArrowLeft, Bot, Edit, MessageSquare, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { AccessActivityPanel } from '@/components/access-activity/AccessActivityPanel'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useTeam } from '@/contexts/TeamContext'
import { toast } from '@/lib/toast'
import type { Agent, AgentExecution } from '@/services/agentService'
import { agentService } from '@/services/agentService'
import { getErrorMessage } from '@/utils/errorHandling'

import { AgentBasicInfo } from './detail/AgentBasicInfo'
import { AgentCardDetails } from './detail/AgentCardDetails'
import { AgentStatsCards } from './detail/AgentStatsCards'
import { RecentExecutionsTable } from './detail/RecentExecutionsTable'

export function AgentDetails() {
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const { currentTeam } = useTeam()

  const [loading, setLoading] = useState(true)
  const [agent, setAgent] = useState<Agent | null>(null)
  const [error, setError] = useState<string | null>(null)

  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)

  const [recentExecutions, setRecentExecutions] = useState<AgentExecution[]>([])
  const [loadingExecutions, setLoadingExecutions] = useState(false)

  const loadAgent = useCallback(async (agentId: string, teamId: string) => {
    try {
      setLoading(true)
      setError(null)
      const response = await agentService.getAgent(teamId, agentId)
      setAgent(response)
    } catch (err) {
      const errorMessage = getErrorMessage(err, 'Failed to load agent')
      setError(errorMessage)
      toast.error(errorMessage)
    } finally {
      setLoading(false)
    }
  }, [])

  const loadRecentExecutions = useCallback(
    async (agentId: string) => {
      if (!currentTeam) return
      try {
        setLoadingExecutions(true)
        const response = await agentService.listAgentExecutions(
          currentTeam.id,
          agentId,
          { limit: 10, page: 1 }
        )
        setRecentExecutions(response.executions)
      } catch {
        setRecentExecutions([])
      } finally {
        setLoadingExecutions(false)
      }
    },
    [currentTeam]
  )

  useEffect(() => {
    if (id && currentTeam) {
      void loadAgent(id, currentTeam.id)
      void loadRecentExecutions(id)
    } else if (!id) {
      void navigate('/agents')
    }
  }, [id, currentTeam, loadAgent, loadRecentExecutions, navigate])

  const handleDelete = async () => {
    if (!agent || !currentTeam) return

    try {
      setDeleting(true)
      await agentService.deleteAgent(currentTeam.id, agent.id)
      setIsDeleteDialogOpen(false)
      toast.success('Agent deleted successfully')
      void navigate('/agents')
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to delete agent'))
    } finally {
      setDeleting(false)
    }
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <PageHeader title="Loading agent…" description="Please wait." />
        <Skeleton className="h-64 w-full" />
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
          <Bot className="size-4" />
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
    <div className="space-y-6">
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

      <PageHeader
        title={agent.name}
        description="View your agent details."
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                void navigate(`/agents/${agent.id}/chat`)
              }}
            >
              <Bot className="mr-2 size-4" />
              Chat
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                void navigate(`/agents/${agent.id}/conversations`)
              }}
            >
              <MessageSquare className="mr-2 size-4" />
              Conversations
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                void navigate(`/agents/${agent.id}/edit`)
              }}
            >
              <Edit className="mr-2 size-4" />
              Edit
            </Button>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => {
                setIsDeleteDialogOpen(true)
              }}
            >
              <Trash2 className="mr-2 size-4" />
              Delete
            </Button>
          </div>
        }
      />

      <AgentStatsCards agent={agent} />
      {currentTeam && (
        <AccessActivityPanel
          teamId={currentTeam.id}
          resourceType="agent"
          resourceId={agent.id}
        />
      )}
      <AgentBasicInfo agent={agent} />
      <AgentCardDetails agent={agent} />
      <RecentExecutionsTable
        recentExecutions={recentExecutions}
        loadingExecutions={loadingExecutions}
        agentId={agent.id}
      />

      <ConfirmDialog
        open={isDeleteDialogOpen}
        onOpenChange={open => {
          if (!open) setIsDeleteDialogOpen(false)
        }}
        title="Delete agent?"
        description={
          <>
            Are you sure you want to delete{' '}
            <span className="font-medium">{agent.name}</span>? This action
            cannot be undone.
          </>
        }
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDelete}
      />
    </div>
  )
}
