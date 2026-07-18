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
import { formatDate } from '@/lib/time'
import type { AdminTeamListItem } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

const PAGE_SIZE = 20

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

/** Instance-wide teams list with server pagination (#316). */
export function AdminTeams() {
  const navigate = useNavigate()
  const [state, setState] = useState<State>(INITIAL)
  const [page, setPage] = useState(1)

  useEffect(() => {
    let cancelled = false
    setState(prev => ({ ...prev, loading: true, error: null }))
    adminService
      .listTeams(page, PAGE_SIZE)
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
      cancelled = true
    }
  }, [page])

  const columns = useMemo<ColumnDef<AdminTeamListItem>[]>(
    () => [
      {
        accessorKey: 'name',
        header: 'Name',
        cell: ({ row }) => (
          <span className="text-sm font-medium">{row.original.name}</span>
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
        <ListPage.Body
          status={status}
          errorTitle="Failed to load teams"
          errorMessage={state.error}
          empty={
            <EmptyState
              icon={UsersRound}
              title="No teams yet"
              description="Teams appear here once they are created on this instance."
            />
          }
        >
          <ListTable
            rows={state.teams}
            columns={columns}
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
