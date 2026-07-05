import { BookOpen, Plus } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EmptyState } from '@/components/EmptyState'
import { ListPage, ListTable } from '@/components/patterns/list-page'
import { Button } from '@/components/ui/button'
import { useProject } from '@/contexts/ProjectContext'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { BlueprintFilters } from '@/pages/blueprints/BlueprintFilters'
import { buildBlueprintsColumns } from '@/pages/blueprints/blueprintsColumns'
import type {
  Blueprint,
  BlueprintFilters as BlueprintFiltersType,
} from '@/services/blueprintService'
import { blueprintService } from '@/services/blueprintService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

type BlueprintSortKey = 'title' | 'updated_at'

const BLUEPRINT_SORTABLE_KEYS: readonly BlueprintSortKey[] = [
  'title',
  'updated_at',
]

interface State {
  blueprints: Blueprint[]
  loading: boolean
  error: string | null
  totalPages: number
  currentPage: number
  total: number
}

export function Blueprints() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { currentProject, isLoading: isProjectLoading } = useProject()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [state, setState] = useState<State>({
    blueprints: [],
    loading: true,
    error: null,
    totalPages: 0,
    currentPage: 1,
    total: 0,
  })
  const [filters, setFilters] = useState<BlueprintFiltersType>(() => ({
    search: '',
    page: 1,
    limit: 20,
    sort_by: 'updated_at',
    sort_order: 'desc',
    project_id: currentProject?.id,
  }))
  const [searchInput, setSearchInput] = useState('')
  const [blueprintToDelete, setBlueprintToDelete] = useState<Blueprint | null>(
    null
  )
  const [deleting, setDeleting] = useState(false)

  const fetchBlueprints = useCallback(
    async (current: BlueprintFiltersType) => {
      // Wait for a persisted project selection to restore, so the first fetch
      // is already scoped instead of flashing unfiltered results.
      if (!currentTeam || isProjectLoading) return
      setState(prev => ({ ...prev, loading: true, error: null }))
      const response = await blueprintService.getBlueprints(
        currentTeam.id,
        current
      )
      setState({
        blueprints: response.spec_libraries,
        loading: false,
        error: null,
        totalPages: response.total_pages,
        currentPage: current.page ?? 1,
        total: response.total_count,
      })
    },
    [currentTeam, isProjectLoading]
  )

  useEffect(() => {
    fetchBlueprints(filters).catch((error: unknown) => {
      setState(prev => ({
        ...prev,
        loading: false,
        error: getErrorMessage(error, 'Failed to fetch blueprints'),
      }))
      handleError(error, 'Failed to load blueprints')
    })
  }, [fetchBlueprints, filters, handleError])

  useEffect(() => {
    const t = setTimeout(() => {
      setFilters(prev =>
        prev.search === searchInput
          ? prev
          : { ...prev, search: searchInput, page: 1 }
      )
    }, 500)
    return () => {
      clearTimeout(t)
    }
  }, [searchInput])

  // Keep the list scoped to the globally selected project (header selector).
  const projectId = currentProject?.id
  useEffect(() => {
    setFilters(prev =>
      prev.project_id === projectId
        ? prev
        : { ...prev, project_id: projectId, page: 1 }
    )
  }, [projectId])

  useEffect(() => {
    trackEvent({
      event: ANALYTICS_EVENTS.BLUEPRINT_PAGE_VIEW,
      properties: { action_context: 'view' },
    })
  }, [trackEvent])

  const handleDelete = async () => {
    if (!blueprintToDelete || !currentTeam) return
    try {
      setDeleting(true)
      await blueprintService.deleteBlueprint(
        currentTeam.id,
        blueprintToDelete.project_id,
        blueprintToDelete.slug
      )
      showSuccess('Blueprint deleted successfully', 'Success')
      void fetchBlueprints(filters)
    } catch (error) {
      handleError(error, 'Failed to delete blueprint')
    } finally {
      setDeleting(false)
      setBlueprintToDelete(null)
    }
  }

  const sortKey = (filters.sort_by ?? 'updated_at') as BlueprintSortKey
  const sortDir = filters.sort_order ?? 'desc'

  const handleSortChange = useCallback((key: BlueprintSortKey) => {
    setFilters(prev => {
      if (key === prev.sort_by) {
        return {
          ...prev,
          sort_order: prev.sort_order === 'asc' ? 'desc' : 'asc',
          page: 1,
        }
      }
      return {
        ...prev,
        sort_by: key,
        sort_order: key === 'title' ? 'asc' : 'desc',
        page: 1,
      }
    })
  }, [])

  const columns = useMemo(
    () => buildBlueprintsColumns({ navigate, onDelete: setBlueprintToDelete }),
    [navigate]
  )

  const status = state.loading
    ? 'loading'
    : state.error
      ? 'error'
      : state.blueprints.length === 0
        ? 'empty'
        : 'ready'

  return (
    <ListPage>
      <ListPage.Header
        title="Blueprints"
        description="Organize all AI-generated blueprints."
        actions={
          <Button
            onClick={() => {
              void navigate('/blueprints/new')
            }}
          >
            <Plus className="mr-2 size-4" />
            New blueprint
          </Button>
        }
      />

      <ListPage.Container>
        <ListPage.Filters>
          <BlueprintFilters
            searchInput={searchInput}
            onSearchInputChange={setSearchInput}
            type={filters.type}
            onTypeChange={value => {
              setFilters(prev => ({ ...prev, type: value, page: 1 }))
            }}
          />
        </ListPage.Filters>

        <ListPage.Body
          status={status}
          errorTitle="Failed to load blueprints"
          errorMessage={state.error}
          empty={
            <EmptyState
              icon={BookOpen}
              title={
                filters.search || filters.project_id
                  ? 'No blueprints match your filters'
                  : 'No blueprints yet'
              }
              description={
                filters.search || filters.project_id
                  ? 'Try different search or filter settings.'
                  : 'Create your first blueprint to save AI-generated content.'
              }
              actions={
                <Button
                  onClick={() => {
                    void navigate('/blueprints/new')
                  }}
                >
                  <Plus className="mr-2 size-4" />
                  New blueprint
                </Button>
              }
            />
          }
        >
          <ListTable
            rows={state.blueprints}
            columns={columns}
            sortableKeys={BLUEPRINT_SORTABLE_KEYS}
            sortKey={sortKey}
            sortDir={sortDir}
            onSortChange={handleSortChange}
          />
        </ListPage.Body>

        <ListPage.Footer
          count={
            status === 'loading' || status === 'error'
              ? undefined
              : {
                  visible: state.blueprints.length,
                  total: state.total,
                  noun: 'blueprint',
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
        open={!!blueprintToDelete}
        onOpenChange={open => {
          if (!open) setBlueprintToDelete(null)
        }}
        title="Delete blueprint?"
        description={
          <>
            This will permanently delete{' '}
            <span className="font-medium">
              {blueprintToDelete?.title ?? 'this blueprint'}
            </span>
            . This action cannot be undone.
          </>
        }
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDelete}
      />
    </ListPage>
  )
}
