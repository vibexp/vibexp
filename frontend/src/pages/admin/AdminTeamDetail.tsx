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
import type { AdminTeamDetail as AdminTeamDetailType } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

/** Instance team detail — owner + member list (#316). */
export function AdminTeamDetail() {
  const { id } = useParams<{ id: string }>()
  const [team, setTeam] = useState<AdminTeamDetailType | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!id) return
    let cancelled = false
    setLoading(true)
    setError(null)
    adminService
      .getTeam(id)
      .then(result => {
        if (!cancelled) setTeam(result)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(getErrorMessage(err, 'Failed to load team'))
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
      backTo="/admin/teams"
      backLabel="Back to teams"
      loading={loading}
      error={error}
      errorTitle="Failed to load team"
    >
      {team && (
        <>
          <PageHeader title={team.name} />
          <Card>
            <CardContent className="grid grid-cols-1 gap-4 py-4 sm:grid-cols-3">
              <div>
                <p className="text-muted-foreground text-xs">Owner</p>
                <p className="text-sm">{team.owner.email}</p>
              </div>
              <div>
                <p className="text-muted-foreground text-xs">Created</p>
                <p className="text-sm">{formatAdminDate(team.created_at)}</p>
              </div>
              <div>
                <p className="text-muted-foreground text-xs">Members</p>
                <p className="text-sm tabular-nums">{team.members.length}</p>
              </div>
            </CardContent>
          </Card>

          <div className="space-y-2">
            <h2 className="text-sm font-semibold">Members</h2>
            {team.members.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                This team has no members.
              </p>
            ) : (
              <Card className="overflow-hidden">
                <Table>
                  <TableHeader>
                    <TableRow className="bg-muted/40 hover:bg-muted/40">
                      <TableHead className="h-9 text-xs font-medium">
                        Email
                      </TableHead>
                      <TableHead className="h-9 text-xs font-medium">
                        Name
                      </TableHead>
                      <TableHead className="h-9 text-xs font-medium">
                        Role
                      </TableHead>
                      <TableHead className="h-9 text-xs font-medium">
                        Joined
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {team.members.map(m => (
                      <TableRow key={m.user_id}>
                        <TableCell className="py-3 text-sm">
                          {m.email}
                        </TableCell>
                        <TableCell className="py-3 text-sm">
                          {m.name || '—'}
                        </TableCell>
                        <TableCell className="py-3">
                          <Badge variant="outline">{m.role}</Badge>
                        </TableCell>
                        <TableCell className="text-muted-foreground py-3 text-xs">
                          {formatAdminDate(m.joined_at)}
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
