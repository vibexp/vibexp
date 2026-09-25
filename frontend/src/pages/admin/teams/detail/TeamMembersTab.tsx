import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatDate } from '@/lib/time'
import type { AdminTeamMember } from '@/services/adminService'

/** The team's member list — already loaded with the team, so no fetch. */
export function TeamMembersTab({
  members,
}: Readonly<{ members: AdminTeamMember[] }>) {
  return (
    <div className="space-y-2">
      <h2 className="text-sm font-semibold">Members</h2>
      {members.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          This team has no members.
        </p>
      ) : (
        <Card className="overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/40 hover:bg-muted/40">
                <TableHead className="h-9 text-xs font-medium">Email</TableHead>
                <TableHead className="h-9 text-xs font-medium">Name</TableHead>
                <TableHead className="h-9 text-xs font-medium">Role</TableHead>
                <TableHead className="h-9 text-xs font-medium">
                  Joined
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {members.map(m => (
                <TableRow key={m.user_id}>
                  <TableCell className="py-3 text-sm">{m.email}</TableCell>
                  <TableCell className="py-3 text-sm">
                    {m.name || '—'}
                  </TableCell>
                  <TableCell className="py-3">
                    <Badge variant="outline">{m.role}</Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground py-3 text-xs">
                    {formatDate(m.joined_at)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
    </div>
  )
}
