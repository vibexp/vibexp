import { BookOpen, Plus } from 'lucide-react'
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
import {
  getResourceDescriptor,
  roleValues,
} from '@/components/patterns/resource'
import { Button } from '@/components/ui/button'
import { useProject } from '@/contexts/ProjectContext'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { useResourceListFilters } from '@/hooks/useResourceListFilters'
import { useResourceListQuery } from '@/hooks/useResourceListQuery'
import { buildBlueprintsColumns } from '@/pages/blueprints/blueprintsColumns'
import type { Blueprint, BlueprintFilters } from '@/services/blueprintService'
import { blueprintService } from '@/services/blueprintService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

const BLUEPRINT = getResourceDescriptor('blueprint')

// Both guards read the descriptor's EXHAUSTIVE value lists, which is also what
// the filter bar enumerates — so the page can never drop a value the control
// just offered (`active | expired`, and the five blueprint types).
const BLUEPRINT_TYPES = roleValues(BLUEPRINT, 'type')
const BLUEPRINT_STATUSES = roleValues(BLUEPRINT, 'status')

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
 * The API rejects a `type` or `status` outside its enum with a 400, so the page
 * must not forward whatever the URL happens to contain.
 */
function coerceType(value: string): Blueprint['type'] | undefined {
  return BLUEPRINT_TYPES.has(value) ? (value as Blueprint['type']) : undefined
}

function coerceStatus(value: string): Blueprint['status'] | undefined {
  return BLUEPRINT_STATUSES.has(value)
    ? (value as Blueprint['status'])
    : undefined
}

export function Blueprints() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { canDeleteResource } = usePermissions()
  const { currentProject, isLoading: isProjectLoading } = useProject()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const handleErrorRef = useCallback(
    (error: unknown) => {
      handleError(error, 'Failed to load blueprints')
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

  const [blueprintToDelete, setBlueprintToDelete] = useState<Blueprint | null>(
    null
  )
  const [deleting, setDeleting] = useState(false)
  // Bumped after a delete to re-run the fetch effect without duplicating it.
  const [reloadToken, setReloadToken] = useState(0)

  const type =
    filters.type === FILTER_ALL ? undefined : coerceType(filters.type)
  const status =
    filters.status === FILTER_ALL ? undefined : coerceStatus(filters.status)
  // The API accepts only `stale` and 400s on anything else, so a junk URL value
  // must be dropped rather than forwarded.
  const freshness =
    filters.freshness === 'stale' ? ('stale' as const) : undefined

  const { sortableKeys, sortKey, onSortChange } = useResourceListSort({
    descriptor: BLUEPRINT,
    sortBy: filters.sort_by,
    sortOrder,
    setFilters,
    fallback: FILTER_DEFAULTS.sort_by,
  })

  const load = useCallback(async () => {
    const response = await blueprintService.getBlueprints(
      currentTeam?.id ?? '',
      {
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
        sort_by: sortKey as BlueprintFilters['sort_by'],
        sort_order: sortOrder,
      }
    )
    return {
      items: response.blueprints,
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
    errorFallback: 'Failed to fetch blueprints',
    onError: handleErrorRef,
  })

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
      setReloadToken(token => token + 1)
    } catch (error) {
      handleError(error, 'Failed to delete blueprint')
    } finally {
      setDeleting(false)
      setBlueprintToDelete(null)
    }
  }

  const columns = useMemo(
    () =>
      buildBlueprintsColumns({
        navigate,
        onDelete: setBlueprintToDelete,
        canDelete: blueprint => canDeleteResource(blueprint.user_id),
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
          <ResourceFilterBar
            descriptor={BLUEPRINT}
            filters={listFilters}
            projectId={projectId}
          />
        </ListPage.Filters>

        <ListPage.Body
          status={listStatus}
          errorTitle="Failed to load blueprints"
          errorMessage={state.error}
          empty={
            // Two distinct empty states: "nothing exists" is a fact about the
            // team, "nothing matches" is a fact about the filters, and only the
            // second one has a way out.
            hasActiveFilters ? (
              <EmptyState
                icon={BookOpen}
                title="No blueprints match your filters"
                description="Try different search, type, status or metadata settings."
                actions={
                  <Button variant="outline" onClick={handleClear}>
                    Clear filters
                  </Button>
                }
              />
            ) : (
              <EmptyState
                icon={BookOpen}
                title="No blueprints yet"
                description="Create your first blueprint to save AI-generated content."
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
                  noun: 'blueprint',
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
