import {
  Activity,
  AlertCircle,
  ArrowLeft,
  BarChart3,
  Bot,
  MessageSquare,
  Pencil,
  Trash2,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router'

import { AccessActivityPanel } from '@/components/access-activity/AccessActivityPanel'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import type {
  ReadingAction,
  ReadingSection,
} from '@/components/patterns/reading-page'
import {
  fieldLabel,
  fieldTone,
  getResourceDescriptor,
  ResourceMetadataSection,
} from '@/components/patterns/resource'
import { ResourceReadingPage } from '@/components/resource-detail/ResourceReadingPage'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { useTeam } from '@/contexts/TeamContext'
import { usePermissions } from '@/hooks/usePermissions'
import { toast } from '@/lib/toast'
import type { Agent, AgentExecution } from '@/services/agentService'
import { agentService } from '@/services/agentService'
import { getErrorMessage } from '@/utils/errorHandling'

import { agentStatusField } from './agentStatus'
import { AgentBasicInfo } from './detail/AgentBasicInfo'
import { AgentCardDetails } from './detail/AgentCardDetails'
import { AgentStatsPanel } from './detail/AgentStatsPanel'
import { RecentExecutionsTable } from './detail/RecentExecutionsTable'

const AGENT = getResourceDescriptor('agent')

export function AgentDetails() {
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const { currentTeam } = useTeam()
  const { canDeleteResource } = usePermissions()

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

  const backAction: ReadingAction = useMemo(
    () => ({
      id: 'back',
      label: 'Back',
      icon: ArrowLeft,
      onClick: () => {
        void navigate('/agents')
      },
    }),
    [navigate]
  )

  if (loading) {
    return (
      <ResourceReadingPage title="Loading agent…">
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </ResourceReadingPage>
    )
  }

  if (error || !agent) {
    return (
      <ResourceReadingPage title="Agent not found" actions={[backAction]}>
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Agent not found</AlertTitle>
          <AlertDescription>
            {error ??
              'The agent you are looking for does not exist or has been removed.'}
          </AlertDescription>
        </Alert>
      </ResourceReadingPage>
    )
  }

  const actions: ReadingAction[] = [
    backAction,
    {
      id: 'chat',
      label: 'Chat',
      icon: Bot,
      testId: 'chat-agent-button',
      onClick: () => {
        void navigate(`/agents/${agent.id}/chat`)
      },
    },
    {
      id: 'conversations',
      label: 'Conversations',
      icon: MessageSquare,
      testId: 'agent-conversations-button',
      // The longest label in the rail; at half width it clips.
      span: 'full',
      onClick: () => {
        void navigate(`/agents/${agent.id}/conversations`)
      },
    },
    {
      id: 'edit',
      label: 'Edit',
      icon: Pencil,
      testId: 'edit-agent-button',
      onClick: () => {
        void navigate(`/agents/${agent.id}/edit`)
      },
    },
  ]
  if (canDeleteResource(agent.user_id)) {
    actions.push({
      id: 'delete',
      label: 'Delete',
      icon: Trash2,
      // Outlined, like every other reading action — not the solid red button
      // the pre-#890 page used.
      tone: 'destructive',
      testId: 'delete-agent-button',
      onClick: () => {
        setIsDeleteDialogOpen(true)
      },
    })
  }

  // Stats and Access activity are details sections rather than standard panels:
  // an agent has no team-scoped resource id, so this page passes no `resource`
  // and every shared panel (attachments, comments, relations) drops out on its
  // own. `ResourceAccessType` does cover `agent`, so the activity chart is
  // still available — it just arrives here instead.
  const extraSections: ReadingSection[] = [
    {
      id: 'stats',
      label: 'Stats',
      icon: BarChart3,
      content: <AgentStatsPanel agent={agent} />,
    },
  ]
  if (currentTeam) {
    extraSections.push({
      id: 'access-activity',
      label: 'Access activity',
      icon: Activity,
      content: (
        <AccessActivityPanel
          teamId={currentTeam.id}
          resourceType="agent"
          resourceId={agent.id}
        />
      ),
    })
  }

  return (
    <>
      <ResourceReadingPage
        title={agent.name}
        status={{
          value: fieldLabel(agentStatusField, agent.status),
          tone: fieldTone(agentStatusField, agent.status),
        }}
        updatedAt={agent.updated_at}
        summary={agent.description}
        actions={actions}
        metadata={
          <ResourceMetadataSection descriptor={AGENT} resource={agent} />
        }
        extraSections={extraSections}
      >
        <div className="space-y-8">
          <AgentBasicInfo agent={agent} />
          <AgentCardDetails agent={agent} />
          <RecentExecutionsTable
            recentExecutions={recentExecutions}
            loadingExecutions={loadingExecutions}
            agentId={agent.id}
          />
        </div>
      </ResourceReadingPage>

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
    </>
  )
}
