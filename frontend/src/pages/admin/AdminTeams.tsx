import type { ColumnDef } from '@tanstack/react-table'
import { UsersRound } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router'

import { EmptyState } from '@/components/EmptyState'
import {
  ListPage,
  listPageStatus,
  ListTable,
} from '@/components/patterns/list-page'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatDate } from '@/lib/time'
import type { TeamKindFilter } from '@/pages/admin/teams/TeamFilters'
import { TeamFilters } from '@/pages/admin/teams/TeamFilters'
import { useAdminListFilters } from '@/pages/admin/useAdminListFilters'
import type { AdminTeamListItem } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

const PAGE_SIZE = 20

const SORTABLE_KEYS = ['name', 'member_count', 'created_at'] as const
type SortKey = (typeof SORTABLE_KEYS)[number]

/**
 * Filter defaults. Every value here is omitted from the URL, so an unfiltered
 * page has a clean address bar (see `useUrlFilters`).
 */
const FILTER_DEFAULTS = {
  page: '1',
  search: '',
  kind: 'all',
  created_from: '',
  created_to: '',
  sort_by: 'created_at',
  sort_order: 'desc',
}

interface State {
  teams: AdminTeamListItem[]
  loading: boolean
  error: string | null
  page: number
  totalPages: number
  total: number
}

const INITIAL: State = {
  teams: [],
  loading: true,
  error: null,
  page: 1,
  totalPages: 0,
  total: 0,
}

/**
 * Maps the tri-state UI filter onto the optional `is_personal` boolean.
 *
 * `undefined` for "all" is the whole point: sending `is_personal=false` would
 * mean "shared only" and silently hide every personal workspace.
 */
function isPersonalParam(kind: string): boolean | undefined {
  if (kind === 'personal') return true
  if (kind === 'shared') return false
  return undefined
}

/** Instance-wide teams list: server-side filtering, sorting and pagination (#460). */
export function AdminTeams() {
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
    filterKeys: ['kind'],
  })
  const [state, setState] = useState<State>(INITIAL)

  useEffect(() => {
    let cancelled = false
    setState(prev => ({ ...prev, loading: true, error: null }))
    adminService
      .listTeams({
        page,
        limit: PAGE_SIZE,
        search: filters.search || undefined,
        is_personal: isPersonalParam(filters.kind),
        created_from: createdFrom,
        created_to: createdTo,
        sort_by: sortBy,
        sort_order: sortOrder,
      })
      .then(response => {
        if (cancelled) return
        setState({
          teams: response.teams,
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
          error: getErrorMessage(err, 'Failed to load teams'),
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
    filters.kind,
    createdFrom,
    createdTo,
    sortBy,
    sortOrder,
  ])

  const columns = useMemo<ColumnDef<AdminTeamListItem>[]>(
    () => [
      {
        accessorKey: 'name',
        header: 'Name',
        cell: ({ row }) => (
          <div className="flex flex-col gap-0.5">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium">{row.original.name}</span>
              {/* Without this the personal/shared filter is unverifiable — the
                  admin cannot see which rows are which. */}
              {row.original.is_personal && (
                <Badge variant="secondary" className="font-normal">
                  Personal
                </Badge>
              )}
            </div>
            <span className="text-muted-foreground text-xs">
              {row.original.slug}
            </span>
          </div>
        ),
      },
      {
        id: 'owner',
        header: 'Owner',
        cell: ({ row }) => (
          <span className="text-muted-foreground text-sm">
            {row.original.owner.email}
          </span>
        ),
      },
      {
        accessorKey: 'member_count',
        header: 'Members',
        meta: { align: 'right' },
        cell: ({ row }) => (
          <span className="text-sm tabular-nums">
            {row.original.member_count}
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
    (row: AdminTeamListItem) => {
      void navigate(`/admin/teams/${row.id}`)
    },
    [navigate]
  )

  const status = listPageStatus(
    state.loading,
    state.error,
    state.teams.length === 0
  )

  return (
    <ListPage>
      <ListPage.Container>
        <ListPage.Filters>
          <TeamFilters
            searchInput={searchInput}
            onSearchInputChange={setSearchInput}
            kind={filters.kind as TeamKindFilter}
            onKindChange={value => {
              setFilters({ kind: value })
            }}
            created={created}
            onCreatedChange={setCreated}
            onClear={handleClear}
            hasActiveFilters={hasActiveFilters}
          />
        </ListPage.Filters>

        <ListPage.Body
          status={status}
          errorTitle="Failed to load teams"
          errorMessage={state.error}
          empty={
            // Two distinct empty states: "nothing exists" is a fact about the
            // instance, "nothing matches" is a fact about the filters, and only
            // the second one has a way out.
            hasActiveFilters ? (
              <EmptyState
                icon={UsersRound}
                title="No teams match your filters"
                description="Try a different search, team type, or date range."
                actions={
                  <Button variant="outline" onClick={handleClear}>
                    Clear filters
                  </Button>
                }
              />
            ) : (
              <EmptyState
                icon={UsersRound}
                title="No teams yet"
                description="Teams appear here once they are created on this instance."
              />
            )
          }
        >
          <ListTable
            rows={state.teams}
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
                  visible: state.teams.length,
                  total: state.total,
                  noun: 'team',
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
