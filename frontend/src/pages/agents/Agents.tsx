import { Bot, Plus } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EmptyState } from '@/components/EmptyState'
import {
  ListPage,
  listPageStatus,
  ListTable,
} from '@/components/patterns/list-page'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { useResourceListFilters } from '@/hooks/useResourceListFilters'
import { useResourceListQuery } from '@/hooks/useResourceListQuery'
import { toast } from '@/lib/toast'
import type {
  Agent,
  AgentFilters as AgentFiltersType,
} from '@/services/agentService'
import { agentService } from '@/services/agentService'

import { AgentFilters } from './AgentFilters'
import { buildAgentsColumns } from './agentsColumns'
import { AgentStats } from './AgentStats'

type AgentStatus = NonNullable<AgentFiltersType['status']>
type AgentSortKey = NonNullable<AgentFiltersType['sort_by']>

interface AgentStatsSummary {
  totalAgents: number
  activeAgents: number
  pausedAgents: number
  errorAgents: number
  totalRuns: number
  avgSuccessRate: number
}

const AGENT_STATUSES: ReadonlySet<AgentStatus> = new Set([
  'active',
  'paused',
  'error',
])

/**
 * `agentService.getAgents` does not forward `sort_by`/`sort_order` today and no
 * agent column is sortable, so these keys only pin the URL contract — validated
 * here so #908 can wire the sort up without a junk value reaching the API.
 */
const AGENT_SORT_KEYS: ReadonlySet<AgentSortKey> = new Set([
  'name',
  'status',
  'total_runs',
  'success_rate',
  'last_run',
  'created_at',
])

const PAGE_SIZE = 20

/**
 * Filter defaults. Every value here is omitted from the URL, so an unfiltered
 * page has a clean address bar (see `useUrlFilters`).
 *
 * Agents are not project-scoped and have no metadata filter, so `metadata` — a
 * base key of `useResourceListFilters` — stays at its default and is never sent
 * (#906; adding either is out of scope).
 */
const FILTER_DEFAULTS = {
  page: '1',
  search: '',
  metadata: '',
  status: 'all',
  sort_by: 'created_at',
  sort_order: 'desc',
}

function coerceStatus(value: string): AgentStatus | undefined {
  return AGENT_STATUSES.has(value as AgentStatus)
    ? (value as AgentStatus)
    : undefined
}

function coerceSortKey(value: string): AgentSortKey {
  return AGENT_SORT_KEYS.has(value as AgentSortKey)
    ? (value as AgentSortKey)
    : 'created_at'
}

/** Fallback stats derived from the loaded page when the stats call fails. */
function summarizeAgents(agents: Agent[]): AgentStatsSummary {
  return {
    totalAgents: agents.length,
    activeAgents: agents.filter(a => a.status === 'active').length,
    pausedAgents: agents.filter(a => a.status === 'paused').length,
    errorAgents: agents.filter(a => a.status === 'error').length,
    totalRuns: agents.reduce((sum, a) => sum + a.total_runs, 0),
    avgSuccessRate:
      agents.length > 0
        ? agents.reduce((sum, a) => sum + a.success_rate, 0) / agents.length
        : 0,
  }
}

export function Agents() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { canDeleteResource } = usePermissions()
  const { handleError } = useErrorHandler()
  const handleErrorRef = useCallback(
    (error: unknown) => {
      handleError(error, 'Failed to load agents')
    },
    [handleError]
  )

  const {
    filters,
    setFilters,
    searchInput,
    setSearchInput,
    page,
    setPage,
    sortOrder,
    hasActiveFilters,
    handleClear,
  } = useResourceListFilters({
    defaults: FILTER_DEFAULTS,
    filterKeys: ['status'],
    // Agents are team-scoped, not project-scoped, so the hook's project guard
    // is inert: with no project it never arms a reset.
    projectId: undefined,
    isProjectLoading: false,
  })

  const [stats, setStats] = useState<AgentStatsSummary | null>(null)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [selectedAgent, setSelectedAgent] = useState<Agent | null>(null)
  const [deleting, setDeleting] = useState(false)
  // Bumped after a delete to re-run the fetch effect without duplicating it.
  const [reloadToken, setReloadToken] = useState(0)

  const status =
    filters.status === 'all' ? undefined : coerceStatus(filters.status)
  const sortKey = coerceSortKey(filters.sort_by)

  const load = useCallback(async () => {
    const response = await agentService.getAgents(currentTeam?.id ?? '', {
      page,
      limit: PAGE_SIZE,
      search: filters.search || undefined,
      status,
      // Dropped by the service until #908 wires agent sorting; sent so the
      // request shape does not have to change when it does.
      sort_by: sortKey,
      sort_order: sortOrder,
    })
    const agents = Array.isArray(response.agents) ? response.agents : []
    return {
      items: agents,
      totalPages: response.total_pages,
      total: response.total_count || agents.length,
    }
  }, [currentTeam?.id, page, filters.search, status, sortKey, sortOrder])

  const state = useResourceListQuery({
    ready: !!currentTeam,
    load,
    reloadToken,
    errorFallback: 'Failed to fetch agents',
    onError: handleErrorRef,
  })

  const teamId = currentTeam?.id
  const agents = state.items
  useEffect(() => {
    if (!teamId || agents.length === 0) return
    let cancelled = false
    agentService
      .getAgentStats(teamId)
      .then(response => {
        if (cancelled) return
        setStats({
          totalAgents: response.total_agents || 0,
          activeAgents: response.active_agents || 0,
          pausedAgents: response.paused_agents || 0,
          errorAgents: response.error_agents || 0,
          totalRuns: response.total_runs || 0,
          avgSuccessRate: response.avg_success_rate || 0,
        })
      })
      .catch(() => {
        if (cancelled) return
        setStats(summarizeAgents(agents))
      })
    return () => {
      cancelled = true
    }
  }, [teamId, agents])

  const handleDeleteAgent = async () => {
    if (!selectedAgent || !currentTeam) return

    try {
      setDeleting(true)
      await agentService.deleteAgent(currentTeam.id, selectedAgent.id)
      setIsDeleteDialogOpen(false)
      setSelectedAgent(null)
      setReloadToken(token => token + 1)
      toast.success('Agent deleted successfully')
    } catch (error) {
      handleError(error, 'Failed to delete agent')
    } finally {
      setDeleting(false)
    }
  }

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

  const listStatus = listPageStatus(
    state.loading,
    state.error,
    state.items.length === 0
  )

  return (
    <ListPage>
      <ListPage.Header
        title="Agents"
        description="Manage AI agents and task automation."
        actions={
          <Button
            onClick={() => {
              void navigate('/agents/new')
            }}
          >
            <Plus className="mr-2 size-4" />
            Add agent
          </Button>
        }
      />

      {stats && !state.loading && !state.error && <AgentStats stats={stats} />}

      <ListPage.Container>
        <ListPage.Filters>
          <AgentFilters
            searchInput={searchInput}
            onSearchInputChange={setSearchInput}
            currentStatusFilter={status ?? 'all'}
            onStatusFilterChange={value => {
              setFilters({ status: value })
            }}
            onClear={handleClear}
            hasActiveFilters={hasActiveFilters}
          />
        </ListPage.Filters>

        <ListPage.Body
          status={listStatus}
          errorTitle="Failed to load agents"
          errorMessage={state.error}
          empty={
            // Two distinct empty states: "nothing exists" is a fact about the
            // team, "nothing matches" is a fact about the filters, and only the
            // second one has a way out.
            hasActiveFilters ? (
              <EmptyState
                icon={Bot}
                title="No agents match your filters"
                description="Try different search or status settings."
                actions={
                  <Button variant="outline" onClick={handleClear}>
                    Clear filters
                  </Button>
                }
              />
            ) : (
              <EmptyState
                icon={Bot}
                title="No agents yet"
                description="Create your first agent to start automating tasks."
                actions={
                  <Button
                    onClick={() => {
                      void navigate('/agents/new')
                    }}
                  >
                    <Plus className="mr-2 size-4" />
                    Add agent
                  </Button>
                }
              />
            )
          }
        >
          <ListTable
            rows={state.items}
            columns={columns}
            onRowClick={agent => {
              void navigate(`/agents/${agent.id}`)
            }}
          />
        </ListPage.Body>

        <ListPage.Footer
          count={
            listStatus === 'loading' || listStatus === 'error'
              ? undefined
              : {
                  visible: state.items.length,
                  total: state.total,
                  noun: 'agent',
                }
          }
          pagination={{
            page,
            totalPages: state.totalPages,
            onPageChange: setPage,
          }}
          hideCount={listStatus === 'loading'}
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
            {'? This action cannot be undone.'}
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
