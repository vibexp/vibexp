import { Bot, Plus } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EmptyState } from '@/components/EmptyState'
import { ListPage, ListTable } from '@/components/patterns/list-page'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { toast } from '@/lib/toast'
import type {
  Agent,
  AgentFilters as AgentFiltersType,
} from '@/services/agentService'
import { agentService } from '@/services/agentService'
import { getErrorMessage } from '@/utils/errorHandling'

import { AgentFilters, type StatusFilter } from './AgentFilters'
import { buildAgentsColumns } from './agentsColumns'
import { AgentStats } from './AgentStats'

interface AgentsState {
  agents: Agent[]
  loading: boolean
  error: string | null
  totalPages: number
  currentPage: number
  total: number
  stats: {
    totalAgents: number
    activeAgents: number
    pausedAgents: number
    errorAgents: number
    totalRuns: number
    avgSuccessRate: number
  } | null
}

export function Agents() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { canDeleteResource } = usePermissions()
  const { handleError } = useErrorHandler()

  const [state, setState] = useState<AgentsState>({
    agents: [],
    loading: true,
    error: null,
    totalPages: 0,
    currentPage: 1,
    total: 0,
    stats: null,
  })

  const [filters, setFilters] = useState<AgentFiltersType>({
    status: undefined,
    search: '',
    page: 1,
    limit: 20,
    sort_by: 'created_at',
    sort_order: 'desc',
  })

  const [searchInput, setSearchInput] = useState('')

  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [selectedAgent, setSelectedAgent] = useState<Agent | null>(null)
  const [deleting, setDeleting] = useState(false)

  const fetchAgents = useCallback(
    async (currentFilters: AgentFiltersType) => {
      if (!currentTeam) return

      setState(prev => ({ ...prev, loading: true, error: null }))
      const response = await agentService.getAgents(
        currentTeam.id,
        currentFilters
      )

      const responseData = response
      const agents = Array.isArray(responseData.agents)
        ? responseData.agents
        : []

      setState(prev => ({
        ...prev,
        agents,
        totalPages: responseData.total_pages,
        currentPage: responseData.page,
        total: responseData.total_count || agents.length,
        loading: false,
      }))
    },
    [currentTeam]
  )

  const fetchAgentStats = useCallback(
    async (agents: Agent[]) => {
      if (!currentTeam) return

      try {
        const response = await agentService.getAgentStats(currentTeam.id)
        const statsData = response

        setState(prev => ({
          ...prev,
          stats: {
            totalAgents: statsData.total_agents || 0,
            activeAgents: statsData.active_agents || 0,
            pausedAgents: statsData.paused_agents || 0,
            errorAgents: statsData.error_agents || 0,
            totalRuns: statsData.total_runs || 0,
            avgSuccessRate: statsData.avg_success_rate || 0,
          },
        }))
      } catch {
        setState(prev => ({
          ...prev,
          stats: {
            totalAgents: agents.length,
            activeAgents: agents.filter(a => a.status === 'active').length,
            pausedAgents: agents.filter(a => a.status === 'paused').length,
            errorAgents: agents.filter(a => a.status === 'error').length,
            totalRuns: agents.reduce((sum, a) => sum + a.total_runs, 0),
            avgSuccessRate:
              agents.length > 0
                ? agents.reduce((sum, a) => sum + a.success_rate, 0) /
                  agents.length
                : 0,
          },
        }))
      }
    },
    [currentTeam]
  )

  useEffect(() => {
    fetchAgents(filters).catch((error: unknown) => {
      const errorMessage = getErrorMessage(error, 'Failed to fetch agents')
      setState(prev => ({ ...prev, loading: false, error: errorMessage }))
      handleError(error, 'Failed to load agents')
    })
  }, [fetchAgents, filters, handleError])

  useEffect(() => {
    if (state.agents.length > 0) {
      void fetchAgentStats(state.agents)
    }
  }, [state.agents, fetchAgentStats])

  useEffect(() => {
    const timeout = setTimeout(() => {
      setFilters(prev =>
        prev.search === searchInput
          ? prev
          : { ...prev, search: searchInput, page: 1 }
      )
    }, 500)
    return () => {
      clearTimeout(timeout)
    }
  }, [searchInput])

  const handleStatusFilter = (status: StatusFilter) => {
    setFilters(prev => ({
      ...prev,
      status: status === 'all' ? undefined : status,
      page: 1,
    }))
  }

  const handleDeleteAgent = async () => {
    if (!selectedAgent || !currentTeam) return

    try {
      setDeleting(true)
      await agentService.deleteAgent(currentTeam.id, selectedAgent.id)
      setIsDeleteDialogOpen(false)
      setSelectedAgent(null)
      void fetchAgents(filters)
      toast.success('Agent deleted successfully')
    } catch (error) {
      handleError(error, 'Failed to delete agent')
    } finally {
      setDeleting(false)
    }
  }

  const currentStatusFilter: StatusFilter = filters.status ?? 'all'

  const columns = useMemo(
    () =>
      buildAgentsColumns({
        navigate,
        onDelete: agent => {
          setSelectedAgent(agent)
          setIsDeleteDialogOpen(true)
        },
        canDelete: agent => canDeleteResource(agent.user_id),
      }),
    [navigate, canDeleteResource]
  )

  const status = state.loading
    ? 'loading'
    : state.error
      ? 'error'
      : state.agents.length === 0
        ? 'empty'
        : 'ready'

  return (
    <ListPage>
      <ListPage.Header
        title="Agents"
        description="Manage AI agents and task automation."
        actions={
          <Button
            onClick={() => {
              void navigate('/agents/add')
            }}
          >
            <Plus className="mr-2 size-4" />
            Add agent
          </Button>
        }
      />

      {state.stats && !state.loading && !state.error && (
        <AgentStats stats={state.stats} />
      )}

      <ListPage.Container>
        <ListPage.Filters>
          <AgentFilters
            searchInput={searchInput}
            onSearchInputChange={setSearchInput}
            currentStatusFilter={currentStatusFilter}
            onStatusFilterChange={handleStatusFilter}
          />
        </ListPage.Filters>

        <ListPage.Body
          status={status}
          errorTitle="Failed to load agents"
          errorMessage={state.error}
          empty={
            <EmptyState
              icon={Bot}
              title={
                filters.search
                  ? 'No agents match your filters'
                  : 'No agents yet'
              }
              description={
                filters.search
                  ? 'Try different search or filter settings.'
                  : 'Create your first agent to start automating tasks.'
              }
              actions={
                <Button
                  onClick={() => {
                    void navigate('/agents/add')
                  }}
                >
                  <Plus className="mr-2 size-4" />
                  Add agent
                </Button>
              }
            />
          }
        >
          <ListTable
            rows={state.agents}
            columns={columns}
            onRowClick={agent => {
              void navigate(`/agents/${agent.id}`)
            }}
          />
        </ListPage.Body>

        <ListPage.Footer
          count={
            status === 'loading' || status === 'error'
              ? undefined
              : {
                  visible: state.agents.length,
                  total: state.total,
                  noun: 'agent',
                }
          }
          pagination={{
            page: state.currentPage,
            totalPages: state.totalPages,
            onPageChange: page => {
              setFilters(prev => ({ ...prev, page }))
            },
          }}
          hideCount={status === 'loading'}
        />
      </ListPage.Container>

      <ConfirmDialog
        open={isDeleteDialogOpen}
        onOpenChange={open => {
          if (!open) {
            setIsDeleteDialogOpen(false)
            setSelectedAgent(null)
          }
        }}
        title="Delete agent?"
        description={
          <>
            Are you sure you want to delete{' '}
            <span className="font-medium">
              {selectedAgent?.name ?? 'this agent'}
            </span>
            ? This action cannot be undone.
          </>
        }
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDeleteAgent}
      />
    </ListPage>
  )
}
