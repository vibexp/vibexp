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
import { useAdminListFilters } from '@/pages/admin/useAdminListFilters'
import type { UserStatusFilter } from '@/pages/admin/users/UserFilters'
import { UserFilters } from '@/pages/admin/users/UserFilters'
import type { UserFormValues } from '@/pages/admin/users/UserFormDialog'
import { UserFormDialog } from '@/pages/admin/users/UserFormDialog'
import type { AdminUserListItem } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

const PAGE_SIZE = 20

const SORTABLE_KEYS = ['email', 'name', 'team_count', 'created_at'] as const
type SortKey = (typeof SORTABLE_KEYS)[number]

const FILTER_DEFAULTS = {
  page: '1',
  search: '',
  status: 'all',
  idp_provider: '',
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

/** `all` must send nothing; the other two are the API's enum values. */
function statusParam(value: string): 'active' | 'suspended' | undefined {
  return value === 'active' || value === 'suspended' ? value : undefined
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
  } = useAdminListFilters<SortKey>({
    defaults: FILTER_DEFAULTS,
    sortableKeys: SORTABLE_KEYS,
    defaultSort: 'created_at',
    filterKeys: ['status', 'idp_provider'],
  })
  const [state, setState] = useState<State>(INITIAL)
  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setState(prev => ({ ...prev, loading: true, error: null }))
    adminService
      .listUsers({
        page,
        limit: PAGE_SIZE,
        search: filters.search || undefined,
        status: statusParam(filters.status),
        idp_provider: filters.idp_provider || undefined,
        created_from: createdFrom,
        created_to: createdTo,
        sort_by: sortBy,
        sort_order: sortOrder,
      })
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
  }, [
    page,
    filters.search,
    filters.status,
    filters.idp_provider,
    createdFrom,
    createdTo,
    sortBy,
    sortOrder,
  ])

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
      {
        accessorKey: 'team_count',
        header: 'Teams',
        meta: { align: 'right' },
        cell: ({ row }) => (
          <span className="text-sm tabular-nums">
            {row.original.team_count}
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
    (row: AdminUserListItem) => {
      void navigate(`/admin/users/${row.id}`)
    },
    [navigate]
  )

  const handleCreate = (values: UserFormValues) => {
    setCreating(true)
    setCreateError(null)
    adminService
      .createUser({
        email: values.email,
        name: values.name,
        ...(values.idp_provider ? { idp_provider: values.idp_provider } : {}),
      })
      .then(user => {
        setCreateOpen(false)
        // Straight to the new user: creating an account is usually the first half
        // of doing something with it.
        void navigate(`/admin/users/${user.id}`)
      })
      .catch((err: unknown) => {
        // Inline rather than a toast: a duplicate email is a correction to make in
        // the form, which stays open.
        setCreateError(getErrorMessage(err, 'Failed to create the user'))
      })
      .finally(() => {
        setCreating(false)
      })
  }

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
              created={created}
              onCreatedChange={setCreated}
              onClear={handleClear}
              hasActiveFilters={hasActiveFilters}
            />
            <Button
              onClick={() => {
                setCreateError(null)
                setCreateOpen(true)
              }}
            >
              <UserPlus className="mr-2 size-4" aria-hidden />
              New user
            </Button>
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
                description="Try a different search, status, provider, or date range."
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
        open={createOpen}
        onOpenChange={setCreateOpen}
        mode="create"
        submitting={creating}
        error={createError}
        onSubmit={handleCreate}
      />
    </ListPage>
  )
}
