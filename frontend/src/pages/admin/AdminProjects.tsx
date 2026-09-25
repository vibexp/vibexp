import type { ColumnDef } from '@tanstack/react-table'
import { FolderKanban } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router'

import { EmptyState } from '@/components/EmptyState'
import {
  ListPage,
  listPageStatus,
  ListTable,
} from '@/components/patterns/list-page'
import { Button } from '@/components/ui/button'
import { formatDate } from '@/lib/time'
import { ProjectFilters } from '@/pages/admin/projects/ProjectFilters'
import type { ProjectSortKey } from '@/pages/admin/projects/projectListParams'
import {
  buildProjectListParams,
  PROJECT_ADVANCED_FILTERS,
} from '@/pages/admin/projects/projectListParams'
import { ProjectResourceSummary } from '@/pages/admin/projects/ProjectResourceSummary'
import { useAdminListFilters } from '@/pages/admin/useAdminListFilters'
import type { AdminProjectListItem } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

const PAGE_SIZE = 20

// `satisfies` pins these to the published `sort_by` enum: a renamed value fails
// `tsc -b` rather than becoming a silent 400. Per-type resource counts are
// filterable but not displayed, so only the columns shown here are listed.
const SORTABLE_KEYS = [
  'name',
  'total_resource_count',
  'last_resource_created_at',
  'created_at',
] as const satisfies readonly ProjectSortKey[]
type SortKey = (typeof SORTABLE_KEYS)[number]

const FILTER_DEFAULTS = {
  page: '1',
  search: '',
  team_id: '',
  owner_email: '',
  created_from: '',
  created_to: '',
  sort_by: 'created_at',
  sort_order: 'desc',
}

interface State {
  projects: AdminProjectListItem[]
  loading: boolean
  error: string | null
  page: number
  totalPages: number
  total: number
}

const INITIAL: State = {
  projects: [],
  loading: true,
  error: null,
  page: 1,
  totalPages: 0,
  total: 0,
}

/** Instance-wide projects list: server-side filtering, sorting, pagination (#461). */
export function AdminProjects() {
  const navigate = useNavigate()
  const {
    filters,
    setFilters,
    searchInput,
    setSearchInput,
    page,
    setPage,
    sortBy,
    sortOrder,
    created,
    setCreated,
    createdFrom,
    createdTo,
    hasActiveFilters,
    handleSortChange,
    handleClear,
    advancedParams,
    advancedActiveCount,
    getRange,
    setRange,
    getDateRange,
    setDateRange,
  } = useAdminListFilters<SortKey>({
    defaults: FILTER_DEFAULTS,
    sortableKeys: SORTABLE_KEYS,
    defaultSort: 'created_at',
    filterKeys: ['team_id', 'owner_email'],
    advanced: PROJECT_ADVANCED_FILTERS,
  })
  const [state, setState] = useState<State>(INITIAL)
  const [clearCount, setClearCount] = useState(0)

  const clearAll = useCallback(() => {
    setClearCount(count => count + 1)
    handleClear()
  }, [handleClear])

  // One memoised request object, so the fetch effect depends on it alone rather
  // than on every one of the URL keys.
  const params = useMemo(
    () =>
      buildProjectListParams(filters, {
        advanced: advancedParams,
        page,
        limit: PAGE_SIZE,
        createdFrom,
        createdTo,
        sortBy,
        sortOrder,
      }),
    [filters, advancedParams, page, createdFrom, createdTo, sortBy, sortOrder]
  )

  useEffect(() => {
    let cancelled = false
    setState(prev => ({ ...prev, loading: true, error: null }))
    adminService
      .listProjects(params)
      .then(response => {
        if (cancelled) return
        setState({
          projects: response.projects,
          loading: false,
          error: null,
          page: response.page,
          totalPages: response.total_pages,
          total: response.total_count,
        })
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setState(prev => ({
          ...prev,
          loading: false,
          error: getErrorMessage(err, 'Failed to load projects'),
        }))
      })
    return () => {
      // Guards the debounced-search race: a slow response for an earlier filter
      // must not overwrite the results of a newer one.
      cancelled = true
    }
  }, [params])

  const columns = useMemo<ColumnDef<AdminProjectListItem>[]>(
    () => [
      {
        accessorKey: 'name',
        header: 'Name',
        cell: ({ row }) => (
          <div className="flex flex-col gap-0.5">
            <span className="text-sm font-medium">{row.original.name}</span>
            <span className="text-muted-foreground text-xs">
              {row.original.slug}
            </span>
          </div>
        ),
      },
      {
        id: 'team',
        header: 'Team',
        cell: ({ row }) => (
          <span className="text-sm">{row.original.team.name}</span>
        ),
      },
      {
        // The project's creator (projects.user_id), NOT the owning team's owner.
        // The two can differ, which is why Team and Owner are separate columns.
        id: 'owner',
        header: 'Owner',
        cell: ({ row }) => (
          <span className="text-muted-foreground text-sm">
            {row.original.owner.email}
          </span>
        ),
      },
      {
        // `id` doubles as the `sort_by` value sent.
        id: 'total_resource_count',
        header: 'Resources',
        meta: { align: 'right' },
        cell: ({ row }) => (
          <ProjectResourceSummary counts={row.original.resource_counts} />
        ),
      },
      {
        id: 'last_resource_created_at',
        header: 'Last resource',
        cell: ({ row }) => {
          const at = row.original.last_resource_created_at
          return (
            <span className="text-muted-foreground whitespace-nowrap text-xs">
              {at ? formatDate(at) : '—'}
            </span>
          )
        },
      },
      {
        accessorKey: 'created_at',
        header: 'Created',
        cell: ({ row }) => (
          <span className="text-muted-foreground whitespace-nowrap text-xs">
            {formatDate(row.original.created_at)}
          </span>
        ),
      },
    ],
    []
  )

  const handleRowClick = useCallback(
    (row: AdminProjectListItem) => {
      void navigate(`/admin/projects/${row.id}`)
    },
    [navigate]
  )

  const status = listPageStatus(
    state.loading,
    state.error,
    state.projects.length === 0
  )

  return (
    <ListPage>
      <ListPage.Container>
        <ListPage.Filters>
          <ProjectFilters
            searchInput={searchInput}
            onSearchInputChange={setSearchInput}
            teamId={filters.team_id}
            onTeamIdChange={value => {
              setFilters({ team_id: value })
            }}
            created={created}
            onCreatedChange={setCreated}
            onClear={clearAll}
            hasActiveFilters={hasActiveFilters}
            getRange={getRange}
            onRangeChange={setRange}
            getDateRange={getDateRange}
            onDateRangeChange={setDateRange}
            ownerEmail={filters.owner_email}
            onOwnerEmailChange={value => {
              setFilters({ owner_email: value })
            }}
            ownerEmailResetKey={clearCount}
            // Creator email lives in the panel too, so it counts toward the
            // badge and opens the panel when a shared link carries it — a
            // malformed one included, so its invalid marker is visible.
            advancedActiveCount={
              advancedActiveCount + (filters.owner_email.trim() === '' ? 0 : 1)
            }
          />
        </ListPage.Filters>

        <ListPage.Body
          status={status}
          errorTitle="Failed to load projects"
          errorMessage={state.error}
          empty={
            hasActiveFilters ? (
              <EmptyState
                icon={FolderKanban}
                title="No projects match your filters"
                description="Try a different search, team, date range, or advanced filter."
                actions={
                  <Button variant="outline" onClick={clearAll}>
                    Clear filters
                  </Button>
                }
              />
            ) : (
              <EmptyState
                icon={FolderKanban}
                title="No projects yet"
                description="Projects appear here once teams create them."
              />
            )
          }
        >
          <ListTable
            rows={state.projects}
            columns={columns}
            sortableKeys={SORTABLE_KEYS}
            sortKey={sortBy}
            sortDir={sortOrder}
            onSortChange={handleSortChange}
            onRowClick={handleRowClick}
          />
        </ListPage.Body>
        <ListPage.Footer
          count={
            status === 'loading' || status === 'error'
              ? undefined
              : {
                  visible: state.projects.length,
                  total: state.total,
                  noun: 'project',
                }
          }
          pagination={{
            page: state.page,
            totalPages: state.totalPages,
            onPageChange: setPage,
          }}
          hideCount={status === 'loading'}
        />
      </ListPage.Container>
    </ListPage>
  )
}
