import { BookOpen, Plus } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
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
import { useUrlFilters } from '@/hooks/useUrlFilters'
import { BlueprintFilters } from '@/pages/blueprints/BlueprintFilters'
import { buildBlueprintsColumns } from '@/pages/blueprints/blueprintsColumns'
import type { Blueprint } from '@/services/blueprintService'
import { blueprintService } from '@/services/blueprintService'
import {
  parseMetadataFilter,
  serializeMetadataFilter,
} from '@/services/metadataService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

type BlueprintSortKey = 'title' | 'updated_at'

const BLUEPRINT_SORTABLE_KEYS: readonly BlueprintSortKey[] = [
  'title',
  'updated_at',
]

const BLUEPRINT_TYPES: readonly string[] = [
  'general',
  'claude-code',
  'claude',
  'cursor',
  'codex',
]

const PAGE_SIZE = 20
const SEARCH_DEBOUNCE_MS = 500

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
  type: 'all',
  metadata: '',
  sort_by: 'updated_at',
  sort_order: 'desc',
}

interface State {
  blueprints: Blueprint[]
  loading: boolean
  error: string | null
  totalPages: number
  currentPage: number
  total: number
}

const INITIAL: State = {
  blueprints: [],
  loading: true,
  error: null,
  totalPages: 0,
  currentPage: 1,
  total: 0,
}

function isSortKey(value: string): value is BlueprintSortKey {
  return (BLUEPRINT_SORTABLE_KEYS as readonly string[]).includes(value)
}

/**
 * The API rejects a `type` outside its enum with a 400, so the page must not
 * forward whatever the URL happens to contain.
 */
function coerceType(value: string): Blueprint['type'] | undefined {
  return BLUEPRINT_TYPES.includes(value)
    ? (value as Blueprint['type'])
    : undefined
}

export function Blueprints() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { canDeleteResource } = usePermissions()
  const { currentProject, isLoading: isProjectLoading } = useProject()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const { filters, setFilters, resetFilters } = useUrlFilters(FILTER_DEFAULTS)

  const [state, setState] = useState<State>(INITIAL)
  // Uncommitted text in the search box, debounced into the URL below.
  const [searchInput, setSearchInput] = useState(filters.search)
  const [blueprintToDelete, setBlueprintToDelete] = useState<Blueprint | null>(
    null
  )
  const [deleting, setDeleting] = useState(false)
  // Bumped after a delete to re-run the fetch effect without duplicating it.
  const [reloadToken, setReloadToken] = useState(0)

  const projectId = currentProject?.id

  const page = Number(filters.page) || 1
  const sortBy: BlueprintSortKey = isSortKey(filters.sort_by)
    ? filters.sort_by
    : 'updated_at'
  const sortOrder: 'asc' | 'desc' =
    filters.sort_order === 'asc' ? 'asc' : 'desc'
  const type = filters.type === 'all' ? undefined : coerceType(filters.type)

  const metadata = useMemo(
    () => parseMetadataFilter(filters.metadata),
    [filters.metadata]
  )
  // The re-serialized canonical string is both the fetch dep and the request
  // value, so a malformed URL param is never forwarded and the effect does not
  // re-run merely because the parsed object is referentially new.
  const metadataParam = useMemo(
    () => serializeMetadataFilter(metadata),
    [metadata]
  )

  useEffect(() => {
    const timer = setTimeout(() => {
      if (searchInput !== filters.search) {
        setFilters({ search: searchInput })
      }
    }, SEARCH_DEBOUNCE_MS)
    return () => {
      clearTimeout(timer)
    }
  }, [searchInput, filters.search, setFilters])

  // Changing the globally selected project returns to page 1 — but a persisted
  // project RESTORING is not a change, and treating it as one would clobber the
  // `?page=3` of a shared link. So the guard is only armed once the restore has
  // finished; seeding it on first render is not enough, because at that point
  // the project is still undefined and the restore itself then looks like a
  // change.
  const previousProjectRef = useRef<string | undefined>(undefined)
  const projectSyncArmedRef = useRef(false)
  useEffect(() => {
    if (isProjectLoading) return
    if (!projectSyncArmedRef.current) {
      projectSyncArmedRef.current = true
      previousProjectRef.current = projectId
      return
    }
    if (previousProjectRef.current === projectId) return
    previousProjectRef.current = projectId
    setFilters({ page: FILTER_DEFAULTS.page })
  }, [projectId, isProjectLoading, setFilters])

  useEffect(() => {
    // Wait for a persisted project selection to restore, so the first fetch is
    // already scoped instead of flashing unfiltered results.
    if (!currentTeam || isProjectLoading) return

    let cancelled = false
    setState(prev => ({ ...prev, loading: true, error: null }))

    blueprintService
      .getBlueprints(currentTeam.id, {
        page,
        limit: PAGE_SIZE,
        search: filters.search || undefined,
        type,
        metadata: metadataParam,
        project_id: projectId,
        sort_by: sortBy,
        sort_order: sortOrder,
      })
      .then(response => {
        if (cancelled) return
        setState({
          blueprints: response.blueprints,
          loading: false,
          error: null,
          totalPages: response.total_pages,
          currentPage: page,
          total: response.total_count,
        })
      })
      .catch((error: unknown) => {
        if (cancelled) return
        setState(prev => ({
          ...prev,
          loading: false,
          error: getErrorMessage(error, 'Failed to fetch blueprints'),
        }))
        handleError(error, 'Failed to load blueprints')
      })

    return () => {
      // Guards the debounced-search race: a slow response for an earlier filter
      // must not overwrite the results of a newer one.
      cancelled = true
    }
  }, [
    currentTeam,
    isProjectLoading,
    projectId,
    page,
    filters.search,
    type,
    metadataParam,
    sortBy,
    sortOrder,
    reloadToken,
    handleError,
  ])

  useEffect(() => {
    trackEvent({
      event: ANALYTICS_EVENTS.BLUEPRINT_PAGE_VIEW,
      properties: { action_context: 'view' },
    })
  }, [trackEvent])

  const hasActiveFilters =
    filters.search !== '' ||
    filters.type !== FILTER_DEFAULTS.type ||
    Object.keys(metadata).length > 0

  const handleClear = useCallback(() => {
    // Stale text left in the box would be re-committed on the next debounce
    // tick and undo the clear.
    setSearchInput('')
    resetFilters()
  }, [resetFilters])

  const handleMetadataChange = useCallback(
    (next: Record<string, string[]>) => {
      setFilters({ metadata: serializeMetadataFilter(next) ?? '' })
    },
    [setFilters]
  )

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

  const handleSortChange = useCallback(
    (key: BlueprintSortKey) => {
      // Clicking the active column flips direction; a new column starts in the
      // direction that reads naturally for it — A-Z for titles, newest first
      // for dates.
      setFilters(
        key === sortBy
          ? { sort_order: sortOrder === 'asc' ? 'desc' : 'asc' }
          : { sort_by: key, sort_order: key === 'title' ? 'asc' : 'desc' }
      )
    },
    [setFilters, sortBy, sortOrder]
  )

  const columns = useMemo(
    () =>
      buildBlueprintsColumns({
        navigate,
        onDelete: setBlueprintToDelete,
        canDelete: blueprint => canDeleteResource(blueprint.user_id),
      }),
    [navigate, canDeleteResource]
  )

  const status = listPageStatus(
    state.loading,
    state.error,
    state.blueprints.length === 0
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
          <BlueprintFilters
            searchInput={searchInput}
            onSearchInputChange={setSearchInput}
            type={type}
            onTypeChange={value => {
              setFilters({ type: value ?? FILTER_DEFAULTS.type })
            }}
            metadata={metadata}
            onMetadataChange={handleMetadataChange}
            projectId={projectId}
            onClear={handleClear}
            hasActiveFilters={hasActiveFilters}
          />
        </ListPage.Filters>

        <ListPage.Body
          status={status}
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
                description="Try different search, type or metadata settings."
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
            rows={state.blueprints}
            columns={columns}
            sortableKeys={BLUEPRINT_SORTABLE_KEYS}
            sortKey={sortBy}
            sortDir={sortOrder}
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
            onPageChange: next => {
              setFilters({ page: String(next) })
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
