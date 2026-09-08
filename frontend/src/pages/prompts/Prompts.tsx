import { FileText, Plus } from 'lucide-react'
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
import { useProject } from '@/contexts/ProjectContext'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { usePermissions } from '@/hooks/usePermissions'
import { useResourceListFilters } from '@/hooks/useResourceListFilters'
import { useResourceListQuery } from '@/hooks/useResourceListQuery'
import { PromptFilters, type SharedFilter } from '@/pages/prompts/PromptFilters'
import { buildPromptsColumns } from '@/pages/prompts/promptsColumns'
import type {
  Prompt,
  PromptFilters as PromptFiltersType,
} from '@/services/promptService'
import { promptService } from '@/services/promptService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

type PromptSortKey = NonNullable<PromptFiltersType['sort_by']>
type PromptStatus = NonNullable<PromptFiltersType['status']>

// Backend also accepts 'created_at' as a sort field, but the UI only exposes
// the three columns rendered with sortable headers (name, status, updated_at).
const PROMPT_SORTABLE_KEYS: readonly PromptSortKey[] = [
  'name',
  'status',
  'updated_at',
]

const PROMPT_STATUSES: readonly PromptStatus[] = ['draft', 'published']

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
 */
const FILTER_DEFAULTS = {
  page: '1',
  search: '',
  metadata: '',
  status: 'all',
  shared: 'all',
  freshness: 'all',
  sort_by: 'updated_at',
  sort_order: 'desc',
}

/**
 * `status` and `sort_by` are enums the API 400s on, so whatever the URL happens
 * to contain must be validated rather than forwarded.
 */
function coerceStatus(value: string): PromptStatus | undefined {
  return PROMPT_STATUSES.includes(value as PromptStatus)
    ? (value as PromptStatus)
    : undefined
}

function coerceSortKey(value: string): PromptSortKey {
  return PROMPT_SORTABLE_KEYS.includes(value as PromptSortKey)
    ? (value as PromptSortKey)
    : 'updated_at'
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
    filterKeys: ['status', 'shared', 'freshness'],
    projectId,
    isProjectLoading,
  })

  const [promptToDelete, setPromptToDelete] = useState<Prompt | null>(null)
  const [deleting, setDeleting] = useState(false)
  // Bumped after a delete to re-run the fetch effect without duplicating it.
  const [reloadToken, setReloadToken] = useState(0)

  const status =
    filters.status === 'all' ? undefined : coerceStatus(filters.status)
  const shared =
    filters.shared === 'all' ? undefined : coerceShared(filters.shared)
  // The API accepts only `stale` and 400s on anything else, so a junk URL value
  // must be dropped rather than forwarded.
  const freshness =
    filters.freshness === 'stale' ? ('stale' as const) : undefined
  const sortKey = coerceSortKey(filters.sort_by)

  const load = useCallback(async () => {
    const response = await promptService.getPrompts(currentTeam?.id ?? '', {
      page,
      limit: PAGE_SIZE,
      search: filters.search || undefined,
      status,
      shared,
      freshness,
      project_id: projectId,
      sort_by: sortKey,
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

  // Toggle direction when re-clicking the active column; otherwise switch
  // column and pick a sensible default direction (asc for name, desc otherwise).
  const handleSortChange = useCallback(
    (key: PromptSortKey) => {
      setFilters({
        sort_by: key,
        sort_order:
          key === sortKey
            ? sortOrder === 'asc'
              ? 'desc'
              : 'asc'
            : key === 'name'
              ? 'asc'
              : 'desc',
      })
    },
    [setFilters, sortKey, sortOrder]
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
          <PromptFilters
            searchInput={searchInput}
            onSearchInputChange={setSearchInput}
            statusFilter={status ?? 'all'}
            onStatusChange={value => {
              setFilters({ status: value })
            }}
            sharedFilter={toSharedFilter(shared)}
            onSharedChange={value => {
              setFilters({ shared: value })
            }}
            freshness={freshness}
            onFreshnessChange={value => {
              setFilters({ freshness: value ?? FILTER_DEFAULTS.freshness })
            }}
            onClear={handleClear}
            hasActiveFilters={hasActiveFilters}
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
                description="Try different search, status, shared or freshness settings."
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
            sortableKeys={PROMPT_SORTABLE_KEYS}
            sortKey={sortKey}
            sortDir={sortOrder}
            onSortChange={handleSortChange}
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
