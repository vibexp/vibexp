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
import type { TeamSortKey } from '@/pages/admin/teams/teamListParams'
import {
  buildTeamListParams,
  ownerEmailParam,
  TEAM_ADVANCED_FILTERS,
} from '@/pages/admin/teams/teamListParams'
import { TeamSetupIndicators } from '@/pages/admin/teams/TeamSetupIndicators'
import { useAdminListFilters } from '@/pages/admin/useAdminListFilters'
import type { AdminTeamListItem } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

const PAGE_SIZE = 20

// `satisfies` pins these to the published `sort_by` enum: a renamed value fails
// `tsc -b` rather than becoming a silent 400. Per-type resource counts are
// filterable but not displayed, so only the columns shown here are listed.
const SORTABLE_KEYS = [
  'name',
  'member_count',
  'owner_count',
  'admin_count',
  'project_count',
  'total_resource_count',
  'created_at',
] as const satisfies readonly TeamSortKey[]
type SortKey = (typeof SORTABLE_KEYS)[number]

/**
 * Filter defaults. Every value here is omitted from the URL, so an unfiltered
 * page has a clean address bar (see `useUrlFilters`).
 */
const FILTER_DEFAULTS = {
  page: '1',
  search: '',
  kind: 'all',
  owner_email: '',
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

/** A right-aligned count column; `id` doubles as the `sort_by` value sent. */
function countColumn(
  id: SortKey,
  header: string,
  get: (team: AdminTeamListItem) => number
): ColumnDef<AdminTeamListItem> {
  return {
    id,
    header,
    meta: { align: 'right' },
    cell: ({ row }) => (
      <span className="text-sm tabular-nums">{get(row.original)}</span>
    ),
  }
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
    advancedActiveCount,
    getRange,
    setRange,
    getTriState,
    setTriState,
  } = useAdminListFilters<SortKey>({
    defaults: FILTER_DEFAULTS,
    sortableKeys: SORTABLE_KEYS,
    defaultSort: 'created_at',
    filterKeys: ['kind', 'owner_email'],
    advanced: TEAM_ADVANCED_FILTERS,
  })
  const [state, setState] = useState<State>(INITIAL)
  const [clearCount, setClearCount] = useState(0)

  const clearAll = useCallback(() => {
    setClearCount(count => count + 1)
    handleClear()
  }, [handleClear])

  // One memoised request object, so the fetch effect depends on it alone rather
  // than on every one of the ~40 URL keys.
  const params = useMemo(
    () =>
      buildTeamListParams(filters, {
        page,
        limit: PAGE_SIZE,
        createdFrom,
        createdTo,
        sortBy,
        sortOrder,
      }),
    [filters, page, createdFrom, createdTo, sortBy, sortOrder]
  )

  useEffect(() => {
    let cancelled = false
    setState(prev => ({ ...prev, loading: true, error: null }))
    adminService
      .listTeams(params)
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
  }, [params])

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
      countColumn('member_count', 'Members', team => team.member_count),
      countColumn('owner_count', 'Owners', team => team.owner_count),
      countColumn('admin_count', 'Admins', team => team.admin_count),
      countColumn('project_count', 'Projects', team => team.project_count),
      countColumn(
        'total_resource_count',
        'Total resources',
        team => team.resource_counts.total
      ),
      {
        id: 'setup',
        header: 'Setup',
        cell: ({ row }) => (
          <TeamSetupIndicators configuration={row.original.configuration} />
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
            onClear={clearAll}
            hasActiveFilters={hasActiveFilters}
            getRange={getRange}
            onRangeChange={setRange}
            getTriState={getTriState}
            onTriStateChange={setTriState}
            ownerEmail={filters.owner_email}
            onOwnerEmailChange={value => {
              setFilters({ owner_email: value })
            }}
            ownerEmailResetKey={clearCount}
            // Owner email lives in the panel too, so it counts toward the badge
            // and opens the panel when a shared link carries it.
            advancedActiveCount={
              advancedActiveCount +
              (ownerEmailParam(filters.owner_email) === undefined ? 0 : 1)
            }
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
                description="Try a different search, team type, date range, or advanced filter."
                actions={
                  <Button variant="outline" onClick={clearAll}>
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
