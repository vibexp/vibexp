import type { ColumnDef } from '@tanstack/react-table'
import { UsersRound } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { EmptyState } from '@/components/EmptyState'
import {
  ListPage,
  listPageStatus,
  ListTable,
} from '@/components/patterns/list-page'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { DateRangeValue } from '@/components/ui/date-range'
import {
  fromDateParam,
  rangeToInstants,
  toDateParam,
} from '@/components/ui/date-range'
import { useUrlFilters } from '@/hooks/useUrlFilters'
import { formatDate } from '@/lib/time'
import type { TeamKindFilter } from '@/pages/admin/teams/TeamFilters'
import { TeamFilters } from '@/pages/admin/teams/TeamFilters'
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

type Filters = typeof FILTER_DEFAULTS

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

function isSortKey(value: string): value is SortKey {
  return (SORTABLE_KEYS as readonly string[]).includes(value)
}

/** Instance-wide teams list: server-side filtering, sorting and pagination (#460). */
export function AdminTeams() {
  const navigate = useNavigate()
  const { filters, setFilters, resetFilters } = useUrlFilters<Filters>({
    ...FILTER_DEFAULTS,
  })
  const [state, setState] = useState<State>(INITIAL)
  // Uncommitted text in the search box, debounced into the URL below, so a
  // keystroke fires neither a request nor a history entry.
  const [searchInput, setSearchInput] = useState(filters.search)

  const created: DateRangeValue = useMemo(
    () => ({
      from: fromDateParam(filters.created_from),
      to: fromDateParam(filters.created_to),
    }),
    [filters.created_from, filters.created_to]
  )

  const page = Number(filters.page) || 1
  const sortBy = isSortKey(filters.sort_by) ? filters.sort_by : 'created_at'
  const sortOrder = filters.sort_order === 'asc' ? 'asc' : 'desc'

  const hasActiveFilters =
    filters.search !== '' ||
    filters.kind !== 'all' ||
    filters.created_from !== '' ||
    filters.created_to !== ''

  useEffect(() => {
    const t = setTimeout(() => {
      if (searchInput !== filters.search) {
        setFilters({ search: searchInput })
      }
    }, 400)
    return () => {
      clearTimeout(t)
    }
  }, [searchInput, filters.search, setFilters])

  // Instants, not calendar days: the upper bound has to be local end-of-day or a
  // single-day filter matches nothing.
  const { from: createdFrom, to: createdTo } = useMemo(
    () => rangeToInstants(created),
    [created]
  )

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

  const handleSortChange = useCallback(
    (key: SortKey) => {
      // Clicking the active column flips direction; a new column starts
      // descending, the useful default for both dates and counts.
      setFilters(
        key === sortBy
          ? { sort_order: sortOrder === 'asc' ? 'desc' : 'asc' }
          : { sort_by: key, sort_order: 'desc' }
      )
    },
    [setFilters, sortBy, sortOrder]
  )

  const handleClear = useCallback(() => {
    setSearchInput('')
    resetFilters()
  }, [resetFilters])

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
            onCreatedChange={value => {
              setFilters({
                created_from: value.from ? toDateParam(value.from) : '',
                created_to: value.to ? toDateParam(value.to) : '',
              })
            }}
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
            onPageChange: next => {
              setFilters({ page: String(next) })
            },
          }}
          hideCount={status === 'loading'}
        />
      </ListPage.Container>
    </ListPage>
  )
}
