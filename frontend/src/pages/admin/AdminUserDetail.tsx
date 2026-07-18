import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'

import { PageHeader } from '@/components/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { AdminDetailScaffold } from '@/pages/admin/AdminDetailScaffold'
import { formatAdminDate } from '@/pages/admin/formatAdminDate'
import type { AdminUserDetail as AdminUserDetailType } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

/** Instance user detail — profile + team memberships (#316). */
export function AdminUserDetail() {
  const { id } = useParams<{ id: string }>()
  const [user, setUser] = useState<AdminUserDetailType | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!id) return
    let cancelled = false
    setLoading(true)
    setError(null)
    adminService
      .getUser(id)
      .then(result => {
        if (!cancelled) setUser(result)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(getErrorMessage(err, 'Failed to load user'))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [id])

  return (
    <AdminDetailScaffold
      backTo="/admin/users"
      backLabel="Back to users"
      loading={loading}
      error={error}
      errorTitle="Failed to load user"
    >
      {user && (
        <>
          <PageHeader
            title={user.name || user.email}
            description={user.name ? user.email : undefined}
          />
          <Card>
            <CardContent className="grid grid-cols-1 gap-4 py-4 sm:grid-cols-3">
              <div>
                <p className="text-muted-foreground text-xs">Provider</p>
                <p className="text-sm">{user.idp_provider ?? '—'}</p>
              </div>
              <div>
                <p className="text-muted-foreground text-xs">Joined</p>
                <p className="text-sm">{formatAdminDate(user.created_at)}</p>
              </div>
              <div>
                <p className="text-muted-foreground text-xs">Teams</p>
                <p className="text-sm tabular-nums">
                  {user.memberships.length}
                </p>
              </div>
            </CardContent>
          </Card>

          <div className="space-y-2">
            <h2 className="text-sm font-semibold">Team memberships</h2>
            {user.memberships.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                This user is not a member of any team.
              </p>
            ) : (
              <Card className="overflow-hidden">
                <Table>
                  <TableHeader>
                    <TableRow className="bg-muted/40 hover:bg-muted/40">
                      <TableHead className="h-9 text-xs font-medium">
                        Team
                      </TableHead>
                      <TableHead className="h-9 text-xs font-medium">
                        Role
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {user.memberships.map(m => (
                      <TableRow key={m.team_id}>
                        <TableCell className="py-3 text-sm">
                          {m.team_name}
                        </TableCell>
                        <TableCell className="py-3">
                          <Badge variant="outline">{m.role}</Badge>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Card>
            )}
          </div>
        </>
      )}
    </AdminDetailScaffold>
  )
}
