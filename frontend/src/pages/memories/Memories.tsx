import { HardDrive, Plus } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EmptyState } from '@/components/EmptyState'
import {
  FILTER_ALL,
  ListPage,
  listPageStatus,
  ListTable,
  ResourceFilterBar,
  useResourceListSort,
} from '@/components/patterns/list-page'
import { getResourceDescriptor } from '@/components/patterns/resource'
import { Button } from '@/components/ui/button'
import { useProject } from '@/contexts/ProjectContext'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { useResourceListFilters } from '@/hooks/useResourceListFilters'
import { useResourceListQuery } from '@/hooks/useResourceListQuery'
import { buildMemoriesColumns } from '@/pages/memories/memoriesColumns'
import { MEMORY_STATUS_OPTIONS } from '@/pages/memories/memoryStatus'
import type {
  Memory,
  MemoryFilters,
  MemoryStatus,
} from '@/services/memoryService'
import { memoryService } from '@/services/memoryService'
import type { Project } from '@/services/projectService'
import { projectService } from '@/services/projectService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

const MEMORY = getResourceDescriptor('memory')

const PAGE_SIZE = 20

/**
 * Filter defaults. Every value here is omitted from the URL, so an unfiltered
 * page has a clean address bar (see `useUrlFilters`).
 *
 * `project_id` is deliberately absent: it comes from the global header project
 * selector, not this page's filter bar, so it is neither page-shareable nor
 * something `Clear filters` could clear.
 */
const FILTER_DEFAULTS = {
  page: '1',
  search: '',
  status: FILTER_ALL,
  freshness: FILTER_ALL,
  metadata: '',
  sort_by: 'updated_at',
  sort_order: 'desc',
}

/**
 * The API rejects a `status` outside its enum with a 400, so the page must not
 * forward whatever the URL happens to contain.
 */
function coerceStatus(value: string): MemoryStatus | undefined {
  return MEMORY_STATUS_OPTIONS.some(option => option.value === value)
    ? (value as MemoryStatus)
    : undefined
}

export function Memories() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { canDeleteResource } = usePermissions()
  const { currentProject, isLoading: isProjectLoading } = useProject()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const handleLoadError = useCallback(
    (error: unknown) => {
      handleError(error, 'Failed to load memories')
    },
    [handleError]
  )

  const projectId = currentProject?.id

  const listFilters = useResourceListFilters({
    defaults: FILTER_DEFAULTS,
    filterKeys: ['status', 'freshness'],
    projectId,
    isProjectLoading,
  })
  const {
    filters,
    setFilters,
    page,
    setPage,
    sortOrder,
    metadataParam,
    hasActiveFilters,
    handleClear,
  } = listFilters

  const [projects, setProjects] = useState<Project[]>([])
  const [memoryToDelete, setMemoryToDelete] = useState<Memory | null>(null)
  const [deleting, setDeleting] = useState(false)
  // Bumped after a delete to re-run the fetch without duplicating it.
  const [reloadToken, setReloadToken] = useState(0)

  const status =
    filters.status === FILTER_ALL ? undefined : coerceStatus(filters.status)
  // The API accepts only `stale` and 400s on anything else, so a junk URL value
  // must be dropped rather than forwarded.
  const freshness =
    filters.freshness === 'stale' ? ('stale' as const) : undefined

  const { sortableKeys, sortKey, onSortChange } = useResourceListSort({
    descriptor: MEMORY,
    sortBy: filters.sort_by,
    sortOrder,
    setFilters,
  })

  const load = useCallback(async () => {
    const response = await memoryService.getMemories(currentTeam?.id ?? '', {
      page,
      limit: PAGE_SIZE,
      search: filters.search || undefined,
      status,
      metadata: metadataParam,
      freshness,
      project_id: projectId,
      // Safe by construction: the descriptor's sortable keys are pinned to
      // this endpoint's `sort_by` enum by `resourceListSpec.test.ts`.
      sort_by: sortKey as MemoryFilters['sort_by'],
      sort_order: sortOrder,
    })
    return {
      items: Array.isArray(response.memories) ? response.memories : [],
      totalPages: response.total_pages,
      total: response.total_count,
    }
  }, [
    currentTeam?.id,
    page,
    filters.search,
    status,
    metadataParam,
    freshness,
    projectId,
    sortKey,
    sortOrder,
  ])

  const state = useResourceListQuery({
    // Wait for a persisted project selection to restore, so the first fetch is
    // already scoped instead of flashing unfiltered results.
    ready: !!currentTeam && !isProjectLoading,
    load,
    reloadToken,
    errorFallback: 'Failed to fetch memories',
    onError: handleLoadError,
  })

  useEffect(() => {
    const fetchProjects = async () => {
      if (!currentTeam) return
      try {
        const res = await projectService.getProjects(currentTeam.id, {
          limit: 100,
        })
        setProjects(res.projects)
      } catch (error) {
        handleError(error, 'Failed to load projects')
      }
    }
    void fetchProjects()
  }, [currentTeam, handleError])

  useEffect(() => {
    trackEvent({
      event: ANALYTICS_EVENTS.MEMORIES_PAGE_VIEW,
      properties: { action_context: 'view' },
    })
  }, [trackEvent])

  const handleDelete = async () => {
    if (!memoryToDelete || !currentTeam) return
    try {
      setDeleting(true)
      await memoryService.deleteMemory(currentTeam.id, memoryToDelete.id)
      setReloadToken(token => token + 1)
      showSuccess('Memory deleted successfully', 'Success')
    } catch (error) {
      handleError(error, 'Failed to delete memory')
    } finally {
      setDeleting(false)
      setMemoryToDelete(null)
    }
  }

  const columns = useMemo(
    () =>
      buildMemoriesColumns({
        navigate,
        onDelete: setMemoryToDelete,
        canDelete: memory => canDeleteResource(memory.user_id),
        // The tags column is no longer conditional on the loaded page: it used
        // to hide itself whenever the current page happened to carry no tags,
        // which made the column flicker while paginating (#518).
        includeTags: true,
        projects,
      }),
    [navigate, projects, canDeleteResource]
  )

  const listStatus = listPageStatus(
    state.loading,
    state.error,
    state.items.length === 0
  )

  return (
    <ListPage>
      <ListPage.Header
        title="Memories"
        description="Browse and manage AI memories."
        actions={
          <Button
            onClick={() => {
              void navigate('/memories/new')
            }}
          >
            <Plus className="mr-2 size-4" />
            New memory
          </Button>
        }
      />

      <ListPage.Container>
        <ListPage.Filters>
          <ResourceFilterBar
            descriptor={MEMORY}
            filters={listFilters}
            projectId={projectId}
          />
        </ListPage.Filters>

        <ListPage.Body
          status={listStatus}
          errorTitle="Failed to load memories"
          errorMessage={state.error}
          empty={
            // Two distinct empty states: "nothing exists" is a fact about the
            // team, "nothing matches" is a fact about the filters, and only the
            // second one has a way out.
            hasActiveFilters ? (
              <EmptyState
                icon={HardDrive}
                title="No memories match your filters"
                description="Try a different search, status or metadata setting."
                actions={
                  <Button variant="outline" onClick={handleClear}>
                    Clear filters
                  </Button>
                }
              />
            ) : (
              <EmptyState
                icon={HardDrive}
                title="No memories yet"
                description="Create your first memory to save insights, snippets, or notes."
                actions={
                  <Button
                    onClick={() => {
                      void navigate('/memories/new')
                    }}
                  >
                    <Plus className="mr-2 size-4" />
                    New memory
                  </Button>
                }
              />
            )
          }
        >
          <ListTable
            rows={state.items}
            columns={columns}
            sortableKeys={sortableKeys}
            sortKey={sortKey}
            sortDir={sortOrder}
            onSortChange={onSortChange}
          />
        </ListPage.Body>

        <ListPage.Footer
          count={
            listStatus === 'loading' || listStatus === 'error'
              ? undefined
              : {
                  // Filtering is server-side now, so the visible count and the
                  // server total finally describe the same result set (#518).
                  visible: state.items.length,
                  total: state.total,
                  noun: 'memory',
                  nounPlural: 'memories',
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
        open={!!memoryToDelete}
        onOpenChange={open => {
          if (!open) setMemoryToDelete(null)
        }}
        title="Delete memory?"
        description="This will permanently delete the memory. This action cannot be undone."
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDelete}
      />
    </ListPage>
  )
}
