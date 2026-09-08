import { Package, Plus } from 'lucide-react'
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
import { useTypes } from '@/hooks/useTypes'
import { buildArtifactsColumns } from '@/pages/artifacts/artifactsColumns'
import { ARTIFACT_STATUS_OPTIONS } from '@/pages/artifacts/artifactStatus'
import type { Artifact, ArtifactFilters } from '@/services/artifactService'
import { artifactService } from '@/services/artifactService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

const ARTIFACT = getResourceDescriptor('artifact')

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
  type: FILTER_ALL,
  status: FILTER_ALL,
  freshness: FILTER_ALL,
  metadata: '',
  sort_by: 'updated_at',
  sort_order: 'desc',
}

/**
 * The API rejects a `status` outside its enum with a 400, so the page must not
 * forward whatever the URL happens to contain. `type` needs no such guard: it
 * is an open string matched against the team's registered types, not an enum.
 */
function coerceStatus(value: string): Artifact['status'] | undefined {
  return ARTIFACT_STATUS_OPTIONS.some(option => option.value === value)
    ? (value as Artifact['status'])
    : undefined
}

export function Artifacts() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { canDeleteResource } = usePermissions()
  const { currentProject, isLoading: isProjectLoading } = useProject()
  const { types } = useTypes('artifacts')
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const handleErrorRef = useCallback(
    (error: unknown) => {
      handleError(error, 'Failed to load artifacts')
    },
    [handleError]
  )
  const { trackEvent } = useAnalytics()

  const projectId = currentProject?.id

  const listFilters = useResourceListFilters({
    defaults: FILTER_DEFAULTS,
    filterKeys: ['type', 'status', 'freshness'],
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

  const [artifactToDelete, setArtifactToDelete] = useState<Artifact | null>(
    null
  )
  const [deleting, setDeleting] = useState(false)
  // Bumped after a delete to re-run the fetch effect without duplicating it.
  const [reloadToken, setReloadToken] = useState(0)

  // `type` is an open string (the team's registered types), so it has no enum
  // coercion to absorb a junk value the way `status` does — an explicit `?type=`
  // in the URL would otherwise be forwarded as an empty string.
  const type =
    filters.type === FILTER_ALL || filters.type === ''
      ? undefined
      : filters.type
  const status =
    filters.status === FILTER_ALL ? undefined : coerceStatus(filters.status)
  // The API accepts only `stale` and 400s on anything else, so a junk URL value
  // must be dropped rather than forwarded.
  const freshness =
    filters.freshness === 'stale' ? ('stale' as const) : undefined

  const { sortableKeys, sortKey, onSortChange } = useResourceListSort({
    descriptor: ARTIFACT,
    sortBy: filters.sort_by,
    sortOrder,
    setFilters,
  })

  const load = useCallback(async () => {
    const response = await artifactService.getArtifacts(currentTeam?.id ?? '', {
      page,
      limit: PAGE_SIZE,
      search: filters.search || undefined,
      type,
      status,
      metadata: metadataParam,
      freshness,
      project_id: projectId,
      // Safe by construction: the descriptor's sortable keys are pinned to
      // this endpoint's `sort_by` enum by `resourceListSpec.test.ts`.
      sort_by: sortKey as ArtifactFilters['sort_by'],
      sort_order: sortOrder,
    })
    return {
      items: response.artifacts,
      totalPages: response.total_pages,
      total: response.total_count,
    }
  }, [
    currentTeam?.id,
    page,
    filters.search,
    type,
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
    errorFallback: 'Failed to fetch artifacts',
    onError: handleErrorRef,
  })

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
      setReloadToken(token => token + 1)
    } catch (error) {
      handleError(error, 'Failed to delete artifact')
    } finally {
      setDeleting(false)
      setArtifactToDelete(null)
    }
  }

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

  const listStatus = listPageStatus(
    state.loading,
    state.error,
    state.items.length === 0
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
          <ResourceFilterBar
            descriptor={ARTIFACT}
            filters={listFilters}
            projectId={projectId}
          />
        </ListPage.Filters>

        <ListPage.Body
          status={listStatus}
          errorTitle="Failed to load artifacts"
          errorMessage={state.error}
          empty={
            // Two distinct empty states: "nothing exists" is a fact about the
            // team, "nothing matches" is a fact about the filters, and only the
            // second one has a way out.
            hasActiveFilters ? (
              <EmptyState
                icon={Package}
                title="No artifacts match your filters"
                description="Try different search, type, status or metadata settings."
                actions={
                  <Button variant="outline" onClick={handleClear}>
                    Clear filters
                  </Button>
                }
              />
            ) : (
              <EmptyState
                icon={Package}
                title="No artifacts yet"
                description="Create your first artifact to save AI-generated content."
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
                  visible: state.items.length,
                  total: state.total,
                  noun: 'artifact',
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
            {'. This action cannot be undone.'}
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
