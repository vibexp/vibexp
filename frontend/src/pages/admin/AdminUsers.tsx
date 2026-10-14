import type { ColumnDef } from '@tanstack/react-table'
import { UserPlus, Users } from 'lucide-react'
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
import { AdminExportButton } from '@/pages/admin/AdminExportButton'
import { withoutPaging } from '@/pages/admin/exportParams'
import { useAdminListFilters } from '@/pages/admin/useAdminListFilters'
import { useCreateUser } from '@/pages/admin/users/useCreateUser'
import { USER_ADVANCED_FILTERS } from '@/pages/admin/users/userAdvancedFilters'
import { UserColumnChooser } from '@/pages/admin/users/UserColumnChooser'
import type { UserSortKey } from '@/pages/admin/users/userColumns'
import {
  userActivityColumns,
  useUserColumnVisibility,
} from '@/pages/admin/users/userColumns'
import type {
  UserSetupFilter,
  UserStatusFilter,
} from '@/pages/admin/users/UserFilters'
import { UserFilters } from '@/pages/admin/users/UserFilters'
import { UserFormDialog } from '@/pages/admin/users/UserFormDialog'
import { buildUserListParams } from '@/pages/admin/users/userListParams'
import type { AdminUserListItem } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

const PAGE_SIZE = 20

// `satisfies` pins these to the published `sort_by` enum: a renamed value fails
// `tsc -b` rather than becoming a silent 400.
const SORTABLE_KEYS = [
  'email',
  'name',
  'created_at',
  'team_count',
  'project_count',
  'total_resource_count',
  'prompt_count',
  'memory_count',
  'artifact_count',
  'blueprint_count',
  'agent_count',
  'feed_count',
  'feed_item_count',
  'comment_count',
  'attachment_count',
  'last_resource_created_at',
] as const satisfies readonly UserSortKey[]
type SortKey = (typeof SORTABLE_KEYS)[number]

const FILTER_DEFAULTS = {
  page: '1',
  search: '',
  status: 'all',
  idp_provider: '',
  setup: 'all',
  created_from: '',
  created_to: '',
  sort_by: 'created_at',
  sort_order: 'desc',
}

interface State {
  users: AdminUserListItem[]
  loading: boolean
  error: string | null
  page: number
  totalPages: number
  total: number
}

const INITIAL: State = {
  users: [],
  loading: true,
  error: null,
  page: 1,
  totalPages: 0,
  total: 0,
}

/**
 * Renders an unset value as an em dash.
 *
 * `idp_provider` is `string | null`, and an empty string means "unset" just as
 * much as null does — so neither `??` (which would render a blank cell for `''`)
 * nor the `x ? x : y` shape the lint rule rewrites into it will do.
 */
function orDash(value: string | null | undefined): string {
  return value && value !== '' ? value : '—'
}

/** Instance-wide users list: server-side filtering, sorting, pagination (#459). */
export function AdminUsers() {
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
    currentQuery,
    applyQuery,
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
    filterKeys: ['status', 'idp_provider', 'setup'],
    advanced: USER_ADVANCED_FILTERS,
  })
  const { visibility, setVisible } = useUserColumnVisibility(sortBy)
  const [state, setState] = useState<State>(INITIAL)

  // One memoised request object: the list call and the CSV export both derive
  // from it, so they cannot send different filters (#1150).
  const params = useMemo(
    () =>
      buildUserListParams(filters, {
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

  // Bumped after a user is created, so the list shows them.
  const [reloadKey, setReloadKey] = useState(0)
  const create = useCreateUser(() => {
    setReloadKey(key => key + 1)
  })

  useEffect(() => {
    let cancelled = false
    setState(prev => ({ ...prev, loading: true, error: null }))
    adminService
      .listUsers(params)
      .then(response => {
        if (cancelled) return
        setState({
          users: response.users,
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
          error: getErrorMessage(err, 'Failed to load users'),
        }))
      })
    return () => {
      cancelled = true
    }
  }, [params, reloadKey])

  const columns = useMemo<ColumnDef<AdminUserListItem>[]>(
    () => [
      {
        accessorKey: 'email',
        header: 'Email',
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">{row.original.email}</span>
            {/* A suspended account cannot sign in at all — exactly the fact an
                admin scanning this list needs to see. */}
            {row.original.status === 'suspended' && (
              <Badge variant="destructive" className="font-normal">
                Suspended
              </Badge>
            )}
            {/* No way to sign in yet: an admin-created account whose owner has
                not used the setup link (#1443). */}
            {row.original.pending_setup && (
              <Badge variant="outline" className="font-normal">
                Pending setup
              </Badge>
            )}
          </div>
        ),
      },
      {
        accessorKey: 'name',
        header: 'Name',
        cell: ({ row }) => (
          <span className="text-sm">{orDash(row.original.name)}</span>
        ),
      },
      {
        id: 'idp_provider',
        header: 'Provider',
        cell: ({ row }) => (
          <span className="text-muted-foreground text-sm">
            {orDash(row.original.idp_provider)}
          </span>
        ),
      },
      ...userActivityColumns(visibility),
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
    [visibility]
  )

  const handleRowClick = useCallback(
    (row: AdminUserListItem) => {
      void navigate(`/admin/users/${row.id}`)
    },
    [navigate]
  )

  const status = listPageStatus(
    state.loading,
    state.error,
    state.users.length === 0
  )

  return (
    <ListPage>
      <ListPage.Container>
        <ListPage.Filters>
          {/* The New user action sits with the filters rather than in a
              ListPage.Header: AdminShell already renders this section's <h1>, and
              a second one would be wrong for screen readers and duplicated on
              screen. */}
          <div className="flex flex-wrap items-start justify-between gap-2">
            <UserFilters
              searchInput={searchInput}
              onSearchInputChange={setSearchInput}
              status={filters.status as UserStatusFilter}
              onStatusChange={value => {
                setFilters({ status: value })
              }}
              provider={filters.idp_provider}
              onProviderChange={value => {
                setFilters({ idp_provider: value })
              }}
              setup={filters.setup as UserSetupFilter}
              onSetupChange={value => {
                setFilters({ setup: value })
              }}
              created={created}
              onCreatedChange={setCreated}
              onClear={handleClear}
              hasActiveFilters={hasActiveFilters}
              getRange={getRange}
              onRangeChange={setRange}
              getDateRange={getDateRange}
              onDateRangeChange={setDateRange}
              advancedActiveCount={advancedActiveCount}
              currentQuery={currentQuery}
              onApplyPreset={applyQuery}
              actions={
                <AdminExportButton
                  noun="users"
                  disabled={state.total === 0}
                  onExport={() =>
                    adminService.exportUsers(withoutPaging(params))
                  }
                />
              }
            />
            <div className="flex items-center gap-2">
              <UserColumnChooser
                visibility={visibility}
                onVisibleChange={setVisible}
                sortBy={sortBy}
              />
              <Button onClick={create.start}>
                <UserPlus className="mr-2 size-4" aria-hidden />
                New user
              </Button>
            </div>
          </div>
        </ListPage.Filters>

        <ListPage.Body
          status={status}
          errorTitle="Failed to load users"
          errorMessage={state.error}
          empty={
            hasActiveFilters ? (
              <EmptyState
                icon={Users}
                title="No users match your filters"
                description="Try a different search, status, provider, date range, or activity filter."
                actions={
                  <Button variant="outline" onClick={handleClear}>
                    Clear filters
                  </Button>
                }
              />
            ) : (
              <EmptyState
                icon={Users}
                title="No users yet"
                description="Users appear here once accounts exist on this instance."
              />
            )
          }
        >
          <ListTable
            rows={state.users}
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
                  visible: state.users.length,
                  total: state.total,
                  noun: 'user',
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

      <UserFormDialog
        open={create.open}
        onOpenChange={create.onOpenChange}
        mode="create"
        submitting={create.submitting}
        error={create.error}
        teamErrors={create.teamErrors}
        created={create.created}
        onOpenCreated={() => {
          const id = create.created?.id
          create.onOpenChange(false)
          if (id) void navigate(`/admin/users/${id}`)
        }}
        onSubmit={create.submit}
      />
    </ListPage>
  )
}
