import type { ColumnDef } from '@tanstack/react-table'
import { Users } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { EmptyState } from '@/components/EmptyState'
import {
  ListPage,
  listPageStatus,
  ListTable,
} from '@/components/patterns/list-page'
import { formatAdminDate } from '@/pages/admin/formatAdminDate'
import type { AdminUserListItem } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

const PAGE_SIZE = 20

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

/** Instance-wide users list with server pagination (#316). */
export function AdminUsers() {
  const navigate = useNavigate()
  const [state, setState] = useState<State>(INITIAL)
  const [page, setPage] = useState(1)

  useEffect(() => {
    let cancelled = false
    setState(prev => ({ ...prev, loading: true, error: null }))
    adminService
      .listUsers(page, PAGE_SIZE)
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
  }, [page])

  const columns = useMemo<ColumnDef<AdminUserListItem>[]>(
    () => [
      {
        accessorKey: 'email',
        header: 'Email',
        cell: ({ row }) => (
          <span className="text-sm font-medium">{row.original.email}</span>
        ),
      },
      {
        accessorKey: 'name',
        header: 'Name',
        cell: ({ row }) => (
          <span className="text-sm">{row.original.name || '—'}</span>
        ),
      },
      {
        accessorKey: 'idp_provider',
        header: 'Provider',
        cell: ({ row }) => (
          <span className="text-muted-foreground text-sm">
            {row.original.idp_provider ?? '—'}
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
        header: 'Joined',
        cell: ({ row }) => (
          <span className="text-muted-foreground whitespace-nowrap text-xs">
            {formatAdminDate(row.original.created_at)}
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

  const status = listPageStatus(
    state.loading,
    state.error,
    state.users.length === 0
  )

  return (
    <ListPage>
      <ListPage.Container>
        <ListPage.Body
          status={status}
          errorTitle="Failed to load users"
          errorMessage={state.error}
          empty={
            <EmptyState
              icon={Users}
              title="No users yet"
              description="Users appear here once accounts exist on this instance."
            />
          }
        >
          <ListTable
            rows={state.users}
            columns={columns}
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
    </ListPage>
  )
}
