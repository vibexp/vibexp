import { Package, Plus } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EmptyState } from '@/components/EmptyState'
import {
  ListPage,
  listPageStatus,
  ListTable,
} from '@/components/patterns/list-page'
import { Button } from '@/components/ui/button'
import { useProject } from '@/contexts/ProjectContext'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { useTypes } from '@/hooks/useTypes'
import { ArtifactFilters } from '@/pages/artifacts/ArtifactFilters'
import { buildArtifactsColumns } from '@/pages/artifacts/artifactsColumns'
import type {
  Artifact,
  ArtifactFilters as ArtifactFiltersType,
} from '@/services/artifactService'
import { artifactService } from '@/services/artifactService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

type ArtifactSortKey = 'updated_at'

const ARTIFACT_SORTABLE_KEYS: readonly ArtifactSortKey[] = ['updated_at']

interface State {
  artifacts: Artifact[]
  loading: boolean
  error: string | null
  totalPages: number
  currentPage: number
  total: number
}

export function Artifacts() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { canDeleteResource } = usePermissions()
  const { currentProject, isLoading: isProjectLoading } = useProject()
  const { types } = useTypes('artifacts')
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [state, setState] = useState<State>({
    artifacts: [],
    loading: true,
    error: null,
    totalPages: 0,
    currentPage: 1,
    total: 0,
  })
  const [filters, setFilters] = useState<ArtifactFiltersType>(() => ({
    search: '',
    page: 1,
    limit: 20,
    sort_by: 'updated_at',
    sort_order: 'desc',
    project_id: currentProject?.id,
  }))
  const [searchInput, setSearchInput] = useState('')
  const [artifactToDelete, setArtifactToDelete] = useState<Artifact | null>(
    null
  )
  const [deleting, setDeleting] = useState(false)

  const fetchArtifacts = useCallback(
    async (current: ArtifactFiltersType) => {
      // Wait for a persisted project selection to restore, so the first fetch
      // is already scoped instead of flashing unfiltered results.
      if (!currentTeam || isProjectLoading) return
      setState(prev => ({ ...prev, loading: true, error: null }))
      const response = await artifactService.getArtifacts(
        currentTeam.id,
        current
      )
      setState({
        artifacts: response.artifacts,
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
    fetchArtifacts(filters).catch((error: unknown) => {
      setState(prev => ({
        ...prev,
        loading: false,
        error: getErrorMessage(error, 'Failed to fetch artifacts'),
      }))
      handleError(error, 'Failed to load artifacts')
    })
  }, [fetchArtifacts, filters, handleError])

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
      event: ANALYTICS_EVENTS.ARTIFACTS_PAGE_VIEW,
      properties: { action_context: 'view' },
    })
  }, [trackEvent])

  const handleDelete = async () => {
    if (!artifactToDelete || !currentTeam) return
    try {
      setDeleting(true)
      await artifactService.deleteArtifact(
        currentTeam.id,
        artifactToDelete.project_id,
        artifactToDelete.slug
      )
      showSuccess('Artifact deleted successfully', 'Success')
      void fetchArtifacts(filters)
    } catch (error) {
      handleError(error, 'Failed to delete artifact')
    } finally {
      setDeleting(false)
      setArtifactToDelete(null)
    }
  }

  const sortKey = (filters.sort_by ?? 'updated_at') as ArtifactSortKey
  const sortDir = filters.sort_order ?? 'desc'

  const handleSortChange = useCallback((key: ArtifactSortKey) => {
    setFilters(prev => {
      if (key === prev.sort_by) {
        return {
          ...prev,
          sort_order: prev.sort_order === 'asc' ? 'desc' : 'asc',
          page: 1,
        }
      }
      return { ...prev, sort_by: key, sort_order: 'desc', page: 1 }
    })
  }, [])

  const typeNames = useMemo(
    () => new Map(types.map(t => [t.slug, t.name])),
    [types]
  )

  const columns = useMemo(
    () =>
      buildArtifactsColumns({
        navigate,
        onDelete: setArtifactToDelete,
        canDelete: artifact => canDeleteResource(artifact.user_id),
        typeNames,
      }),
    [navigate, typeNames, canDeleteResource]
  )

  const status = listPageStatus(
    state.loading,
    state.error,
    state.artifacts.length === 0
  )

  return (
    <ListPage>
      <ListPage.Header
        title="Artifacts"
        description="Organize all AI-generated artifacts."
        actions={
          <Button
            onClick={() => {
              void navigate('/artifacts/new')
            }}
          >
            <Plus className="mr-2 size-4" />
            New artifact
          </Button>
        }
      />

      <ListPage.Container>
        <ListPage.Filters>
          <ArtifactFilters
            searchInput={searchInput}
            onSearchInputChange={setSearchInput}
            type={filters.type}
            onTypeChange={value => {
              setFilters(prev => ({ ...prev, type: value, page: 1 }))
            }}
            status={filters.status}
            onStatusChange={value => {
              setFilters(prev => ({ ...prev, status: value, page: 1 }))
            }}
          />
        </ListPage.Filters>

        <ListPage.Body
          status={status}
          errorTitle="Failed to load artifacts"
          errorMessage={state.error}
          empty={
            <EmptyState
              icon={Package}
              title={
                filters.search || filters.project_id
                  ? 'No artifacts match your filters'
                  : 'No artifacts yet'
              }
              description={
                filters.search || filters.project_id
                  ? 'Try different search or filter settings.'
                  : 'Create your first artifact to save AI-generated content.'
              }
              actions={
                <Button
                  onClick={() => {
                    void navigate('/artifacts/new')
                  }}
                >
                  <Plus className="mr-2 size-4" />
                  New artifact
                </Button>
              }
            />
          }
        >
          <ListTable
            rows={state.artifacts}
            columns={columns}
            sortableKeys={ARTIFACT_SORTABLE_KEYS}
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
                  visible: state.artifacts.length,
                  total: state.total,
                  noun: 'artifact',
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
        open={!!artifactToDelete}
        onOpenChange={open => {
          if (!open) setArtifactToDelete(null)
        }}
        title="Delete artifact?"
        description={
          <>
            This will permanently delete{' '}
            <span className="font-medium">
              {artifactToDelete?.title ?? 'this artifact'}
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
