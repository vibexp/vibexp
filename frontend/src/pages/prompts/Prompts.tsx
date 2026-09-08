import { FileText, Plus } from 'lucide-react'
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
import { buildPromptsColumns } from '@/pages/prompts/promptsColumns'
import {
  PromptSharedFilter,
  type SharedFilter,
} from '@/pages/prompts/PromptSharedFilter'
import type {
  Prompt,
  PromptFilters as PromptFiltersType,
} from '@/services/promptService'
import { promptService } from '@/services/promptService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

type PromptStatus = NonNullable<PromptFiltersType['status']>

const PROMPT = getResourceDescriptor('prompt')

const PROMPT_STATUSES: ReadonlySet<string> = new Set(
  PROMPT.fields.find(field => field.role === 'status')?.statusValues ?? []
)

const PAGE_SIZE = 20

/**
 * Filter defaults. Every value here is omitted from the URL, so an unfiltered
 * page has a clean address bar (see `useUrlFilters`).
 *
 * `project_id` is deliberately absent: it comes from the global header project
 * selector, not this page's filter bar, so it is neither page-shareable nor
 * something `Clear filters` could clear.
 *
 * `metadata` is a base key of `useResourceListFilters` that prompts do not
 * filter on — the list endpoint has no such parameter — so it stays at its
 * default and is never sent (#906; adding the control is out of scope).
 *
 * `labels` is a comma-separated list rather than an enum, so its cleared value
 * is the empty string, not `all` (#908).
 */
const FILTER_DEFAULTS = {
  page: '1',
  search: '',
  metadata: '',
  status: FILTER_ALL,
  labels: '',
  shared: FILTER_ALL,
  freshness: FILTER_ALL,
  sort_by: 'updated_at',
  sort_order: 'desc',
}

/**
 * `status` is an enum the API 400s on, so whatever the URL happens to contain
 * must be validated rather than forwarded. `labels` needs no such guard: it is
 * an open list matched against the team's own labels, and an unknown label
 * simply matches nothing.
 */
function coerceStatus(value: string): PromptStatus | undefined {
  return PROMPT_STATUSES.has(value) ? (value as PromptStatus) : undefined
}

/** The tri-state stays a string in the URL; only the request sees a boolean. */
function coerceShared(value: string): boolean | undefined {
  if (value === 'shared') return true
  if (value === 'not_shared') return false
  return undefined
}

function toSharedFilter(shared: boolean | undefined): SharedFilter {
  if (shared === undefined) return 'all'
  return shared ? 'shared' : 'not_shared'
}

export function Prompts() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { canDeleteResource } = usePermissions()
  const { currentProject, isLoading: isProjectLoading } = useProject()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const handleErrorRef = useCallback(
    (error: unknown) => {
      handleError(error, 'Failed to load prompts')
    },
    [handleError]
  )
  const { trackEvent } = useAnalytics()

  const projectId = currentProject?.id

  const listFilters = useResourceListFilters({
    defaults: FILTER_DEFAULTS,
    filterKeys: ['status', 'labels', 'shared', 'freshness'],
    projectId,
    isProjectLoading,
  })
  const {
    filters,
    setFilters,
    page,
    setPage,
    sortOrder,
    hasActiveFilters,
    handleClear,
  } = listFilters

  const [promptToDelete, setPromptToDelete] = useState<Prompt | null>(null)
  const [deleting, setDeleting] = useState(false)
  // Bumped after a delete to re-run the fetch effect without duplicating it.
  const [reloadToken, setReloadToken] = useState(0)

  const status =
    filters.status === FILTER_ALL ? undefined : coerceStatus(filters.status)
  const labels = filters.labels || undefined
  const shared =
    filters.shared === FILTER_ALL ? undefined : coerceShared(filters.shared)
  // The API accepts only `stale` and 400s on anything else, so a junk URL value
  // must be dropped rather than forwarded.
  const freshness =
    filters.freshness === 'stale' ? ('stale' as const) : undefined

  const { sortableKeys, sortKey, onSortChange } = useResourceListSort({
    descriptor: PROMPT,
    sortBy: filters.sort_by,
    sortOrder,
    setFilters,
  })

  const load = useCallback(async () => {
    const response = await promptService.getPrompts(currentTeam?.id ?? '', {
      page,
      limit: PAGE_SIZE,
      search: filters.search || undefined,
      status,
      labels,
      shared,
      freshness,
      project_id: projectId,
      // Safe by construction: the descriptor's sortable keys are pinned to
      // this endpoint's `sort_by` enum by `resourceListSpec.test.ts`.
      sort_by: sortKey as PromptFiltersType['sort_by'],
      sort_order: sortOrder,
    })
    return {
      items: response.prompts,
      totalPages: response.total_pages,
      total: response.total_count,
    }
  }, [
    currentTeam?.id,
    page,
    filters.search,
    status,
    labels,
    shared,
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
    errorFallback: 'Failed to fetch prompts',
    onError: handleErrorRef,
  })

  useEffect(() => {
    trackEvent({
      event: ANALYTICS_EVENTS.PROMPTS_PAGE_VIEW,
      properties: { action_context: 'view' },
    })
  }, [trackEvent])

  const handleDelete = async () => {
    if (!promptToDelete || !currentTeam) return
    try {
      setDeleting(true)
      await promptService.deletePrompt(currentTeam.id, promptToDelete.slug)
      showSuccess('Prompt deleted successfully', 'Success')
      setReloadToken(token => token + 1)
    } catch (error) {
      handleError(error, 'Failed to delete prompt')
    } finally {
      setDeleting(false)
      setPromptToDelete(null)
    }
  }

  const columns = useMemo(
    () =>
      buildPromptsColumns({
        navigate,
        onDelete: setPromptToDelete,
        canDelete: prompt => canDeleteResource(prompt.user_id),
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
        title="Prompts"
        description="Organize and manage your AI prompts."
        actions={
          <Button
            onClick={() => {
              // Editor still lives in v1 until Slice 5b lands
              void navigate('/prompts/new')
            }}
          >
            <Plus className="mr-2 size-4" />
            New prompt
          </Button>
        }
      />

      <ListPage.Container>
        <ListPage.Filters>
          <ResourceFilterBar
            descriptor={PROMPT}
            filters={listFilters}
            extras={
              <PromptSharedFilter
                value={toSharedFilter(shared)}
                onChange={value => {
                  setFilters({ shared: value })
                }}
              />
            }
          />
        </ListPage.Filters>

        <ListPage.Body
          status={listStatus}
          errorTitle="Failed to load prompts"
          errorMessage={state.error}
          empty={
            // Two distinct empty states: "nothing exists" is a fact about the
            // team, "nothing matches" is a fact about the filters, and only the
            // second one has a way out.
            hasActiveFilters ? (
              <EmptyState
                icon={FileText}
                title="No prompts match your filters"
                description="Try different search, status, label, shared or freshness settings."
                actions={
                  <Button variant="outline" onClick={handleClear}>
                    Clear filters
                  </Button>
                }
              />
            ) : (
              <EmptyState
                icon={FileText}
                title="No prompts yet"
                description="Create your first prompt to build a reusable AI workflow."
                actions={
                  <Button
                    onClick={() => {
                      void navigate('/prompts/new')
                    }}
                  >
                    <Plus className="mr-2 size-4" />
                    New prompt
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
                  noun: 'prompt',
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
        open={!!promptToDelete}
        onOpenChange={open => {
          if (!open) setPromptToDelete(null)
        }}
        title="Delete prompt?"
        description={
          <>
            This will permanently delete{' '}
            <span className="font-medium">
              {promptToDelete?.name ?? 'this prompt'}
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
