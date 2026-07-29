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
import { useAdminListFilters } from '@/pages/admin/useAdminListFilters'
import type { AdminProjectListItem } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

const PAGE_SIZE = 20

/** #453 allows sorting by these two only; anything else is a 400. */
const SORTABLE_KEYS = ['name', 'created_at'] as const
type SortKey = (typeof SORTABLE_KEYS)[number]

const FILTER_DEFAULTS = {
  page: '1',
  search: '',
  team_id: '',
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
  } = useAdminListFilters<SortKey>({
    defaults: FILTER_DEFAULTS,
    sortableKeys: SORTABLE_KEYS,
    defaultSort: 'created_at',
    filterKeys: ['team_id'],
  })
  const [state, setState] = useState<State>(INITIAL)

  useEffect(() => {
    let cancelled = false
    setState(prev => ({ ...prev, loading: true, error: null }))
    adminService
      .listProjects({
        page,
        limit: PAGE_SIZE,
        search: filters.search || undefined,
        team_id: filters.team_id || undefined,
        created_from: createdFrom,
        created_to: createdTo,
        sort_by: sortBy,
        sort_order: sortOrder,
      })
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
  }, [
    page,
    filters.search,
    filters.team_id,
    createdFrom,
    createdTo,
    sortBy,
    sortOrder,
  ])

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
            onClear={handleClear}
            hasActiveFilters={hasActiveFilters}
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
                description="Try a different search, team, or date range."
                actions={
                  <Button variant="outline" onClick={handleClear}>
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
