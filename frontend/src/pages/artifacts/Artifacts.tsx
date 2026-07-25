import { Package, Plus } from 'lucide-react'
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
import { useTypes } from '@/hooks/useTypes'
import { useUrlFilters } from '@/hooks/useUrlFilters'
import { ArtifactFilters } from '@/pages/artifacts/ArtifactFilters'
import { buildArtifactsColumns } from '@/pages/artifacts/artifactsColumns'
import { ARTIFACT_STATUS_OPTIONS } from '@/pages/artifacts/artifactStatus'
import type { Artifact } from '@/services/artifactService'
import { artifactService } from '@/services/artifactService'
import {
  parseMetadataFilter,
  serializeMetadataFilter,
} from '@/services/metadataService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

type ArtifactSortKey = 'updated_at'

const ARTIFACT_SORTABLE_KEYS: readonly ArtifactSortKey[] = ['updated_at']

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
  status: 'all',
  metadata: '',
  sort_by: 'updated_at',
  sort_order: 'desc',
}

interface State {
  artifacts: Artifact[]
  loading: boolean
  error: string | null
  totalPages: number
  currentPage: number
  total: number
}

const INITIAL: State = {
  artifacts: [],
  loading: true,
  error: null,
  totalPages: 0,
  currentPage: 1,
  total: 0,
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
  const { trackEvent } = useAnalytics()

  const { filters, setFilters, resetFilters } = useUrlFilters(FILTER_DEFAULTS)

  const [state, setState] = useState<State>(INITIAL)
  // Uncommitted text in the search box, debounced into the URL below.
  const [searchInput, setSearchInput] = useState(filters.search)
  const [artifactToDelete, setArtifactToDelete] = useState<Artifact | null>(
    null
  )
  const [deleting, setDeleting] = useState(false)
  // Bumped after a delete to re-run the fetch effect without duplicating it.
  const [reloadToken, setReloadToken] = useState(0)

  const projectId = currentProject?.id

  const page = Number(filters.page) || 1
  const sortOrder: 'asc' | 'desc' =
    filters.sort_order === 'asc' ? 'asc' : 'desc'
  const type = filters.type === 'all' ? undefined : filters.type
  const status =
    filters.status === 'all' ? undefined : coerceStatus(filters.status)

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

    artifactService
      .getArtifacts(currentTeam.id, {
        page,
        limit: PAGE_SIZE,
        search: filters.search || undefined,
        type,
        status,
        metadata: metadataParam,
        project_id: projectId,
        sort_by: 'updated_at',
        sort_order: sortOrder,
      })
      .then(response => {
        if (cancelled) return
        setState({
          artifacts: response.artifacts,
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
          error: getErrorMessage(error, 'Failed to fetch artifacts'),
        }))
        handleError(error, 'Failed to load artifacts')
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
    status,
    metadataParam,
    sortOrder,
    reloadToken,
    handleError,
  ])

  useEffect(() => {
    trackEvent({
      event: ANALYTICS_EVENTS.ARTIFACTS_PAGE_VIEW,
      properties: { action_context: 'view' },
    })
  }, [trackEvent])

  const hasActiveFilters =
    filters.search !== '' ||
    filters.type !== FILTER_DEFAULTS.type ||
    filters.status !== FILTER_DEFAULTS.status ||
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

  const handleSortChange = useCallback(
    (key: ArtifactSortKey) => {
      // Only one sortable column today, so a click always flips direction.
      setFilters({
        sort_by: key,
        sort_order: sortOrder === 'asc' ? 'desc' : 'asc',
      })
    },
    [setFilters, sortOrder]
  )

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
            type={type}
            onTypeChange={value => {
              setFilters({ type: value ?? FILTER_DEFAULTS.type })
            }}
            status={status}
            onStatusChange={value => {
              setFilters({ status: value ?? FILTER_DEFAULTS.status })
            }}
            metadata={metadata}
            onMetadataChange={handleMetadataChange}
            projectId={projectId}
            onClear={handleClear}
            hasActiveFilters={hasActiveFilters}
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
            rows={state.artifacts}
            columns={columns}
            sortableKeys={ARTIFACT_SORTABLE_KEYS}
            sortKey="updated_at"
            sortDir={sortOrder}
            onSortChange={handleSortChange}
          />
        </ListPage.Body>

        <ListPage.Footer
          count={
            listStatus === 'loading' || listStatus === 'error'
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
            onPageChange: next => {
              setFilters({ page: String(next) })
            },
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
