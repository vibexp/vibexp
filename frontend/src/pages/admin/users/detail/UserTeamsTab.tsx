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
import type { AdminTeamMembership } from '@/services/adminService'

/** The user's team memberships, from the already-loaded user (no fetch). */
export function UserTeamsTab({
  memberships,
}: Readonly<{ memberships: readonly AdminTeamMembership[] }>) {
  return (
    <div className="space-y-2">
      <h2 className="text-sm font-semibold">Team memberships</h2>
      {memberships.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          This user is not a member of any team.
        </p>
      ) : (
        <Card className="overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/40 hover:bg-muted/40">
                <TableHead className="h-9 text-xs font-medium">Team</TableHead>
                <TableHead className="h-9 text-xs font-medium">Role</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {memberships.map(m => (
                <TableRow key={m.team_id}>
                  <TableCell className="py-3 text-sm">{m.team_name}</TableCell>
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
  )
}
