import { Activity, AlertCircle, ArrowLeft } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { EmptyState } from '@/components/EmptyState'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useTeam } from '@/contexts/TeamContext'
import { toast } from '@/lib/toast'
import { agentService } from '@/services/agentService'
import type { Agent, AgentExecution } from '@/types'
import { getErrorMessage } from '@/utils/errorHandling'

import { ExecutionFilters } from './tasks/ExecutionFilters'
import { ExecutionStats } from './tasks/ExecutionStats'
import { ExecutionTable } from './tasks/ExecutionTable'

interface ExecutionsState {
  executions: AgentExecution[]
  loading: boolean
  error: string | null
  totalPages: number
  currentPage: number
  total: number
  stats: {
    totalExecutions: number
    successfulExecutions: number
    failedExecutions: number
    runningExecutions: number
    avgDuration: number
  } | null
}

interface ExecutionFiltersState {
  status?: string
  page: number
  limit: number
}

function calcStats(executions: AgentExecution[], total: number) {
  const completedExecutions = executions.filter(
    e => e.duration !== null && e.duration !== undefined
  )
  const avgDuration =
    completedExecutions.length > 0
      ? completedExecutions.reduce((sum, e) => sum + (e.duration ?? 0), 0) /
        completedExecutions.length /
        1000
      : 0

  return {
    totalExecutions: total,
    successfulExecutions: executions.filter(e => e.status === 'success').length,
    failedExecutions: executions.filter(e => e.status === 'error').length,
    runningExecutions: executions.filter(e => e.status === 'running').length,
    avgDuration,
  }
}

export function AgentTasks() {
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const { currentTeam } = useTeam()

  const [agent, setAgent] = useState<Agent | null>(null)
  const [loadingAgent, setLoadingAgent] = useState(true)

  const [state, setState] = useState<ExecutionsState>({
    executions: [],
    loading: true,
    error: null,
    totalPages: 0,
    currentPage: 1,
    total: 0,
    stats: null,
  })

  const [filters, setFilters] = useState<ExecutionFiltersState>({
    page: 1,
    limit: 20,
  })
  const [searchInput, setSearchInput] = useState('')

  const loadAgent = useCallback(async (agentId: string, teamId: string) => {
    try {
      setLoadingAgent(true)
      const response = await agentService.getAgent(teamId, agentId)
      const agentData = 'data' in response ? response.data : response
      setAgent(agentData)
    } catch (error) {
      toast.error(getErrorMessage(error, 'Failed to load agent'))
    } finally {
      setLoadingAgent(false)
    }
  }, [])

  const fetchExecutions = useCallback(
    async (agentId: string, currentFilters: ExecutionFiltersState) => {
      if (!currentTeam) return

      try {
        setState(prev => ({ ...prev, loading: true, error: null }))
        const response = await agentService.listAgentExecutions(
          currentTeam.id,
          agentId,
          {
            status: currentFilters.status,
            page: currentFilters.page,
            limit: currentFilters.limit,
          }
        )

        setState({
          executions: response.executions,
          totalPages: response.total_pages,
          currentPage: response.page,
          total: response.total_count,
          loading: false,
          error: null,
          stats: calcStats(response.executions, response.total_count),
        })
      } catch (error) {
        const errorMessage = getErrorMessage(
          error,
          'Failed to fetch executions'
        )
        setState(prev => ({ ...prev, loading: false, error: errorMessage }))
        toast.error(errorMessage)
      }
    },
    [currentTeam]
  )

  useEffect(() => {
    if (id && currentTeam) {
      void loadAgent(id, currentTeam.id)
    }
  }, [id, currentTeam, loadAgent])

  useEffect(() => {
    if (id) {
      void fetchExecutions(id, filters)
    }
  }, [id, filters, fetchExecutions])

  if (loadingAgent) {
    return (
      <div className="space-y-6">
        <PageHeader title="Loading agent…" description="Please wait." />
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

  const hasExecutions = state.executions.length > 0

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
        title={`Tasks: ${agent.name}`}
        description="View all task executions for this agent."
      />

      {state.stats && !state.loading && !state.error && (
        <ExecutionStats stats={state.stats} />
      )}

      <ExecutionFilters
        searchInput={searchInput}
        onSearchInputChange={setSearchInput}
        currentStatusFilter={filters.status ?? 'all'}
        onStatusFilterChange={status => {
          setFilters(prev => ({
            ...prev,
            status: status === 'all' ? undefined : status,
            page: 1,
          }))
        }}
      />

      {state.error && (
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Failed to load executions</AlertTitle>
          <AlertDescription className="space-y-2">
            <p>{state.error}</p>
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                if (id) void fetchExecutions(id, filters)
              }}
            >
              Retry
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {state.loading && (
        <Card>
          <CardContent className="space-y-3 p-6">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </CardContent>
        </Card>
      )}

      {!state.loading && !state.error && !hasExecutions && (
        <EmptyState
          icon={Activity}
          title="No executions found"
          description={
            filters.status
              ? 'Try adjusting your filters.'
              : 'No tasks have been executed for this agent yet.'
          }
        />
      )}

      {!state.loading && !state.error && hasExecutions && (
        <Card>
          <CardContent className="p-4">
            <ExecutionTable
              executions={state.executions}
              currentPage={state.currentPage}
              totalPages={state.totalPages}
              onPageChange={page => {
                setFilters(prev => ({ ...prev, page }))
              }}
            />
          </CardContent>
        </Card>
      )}
    </div>
  )
}
