import { Link } from 'react-router'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { Slot } from '@/pages/admin/detail/slot'
import { formatResourceType } from '@/pages/admin/users/detail/userInsightsChartData'
import type { AdminTopAccessedResource } from '@/services/adminService'

/**
 * The most-accessed resources, as opaque references (shared by the user and
 * project detail pages).
 *
 * Each cell reads one allowlisted field — never a spread of the row — so a
 * title can never leak into this table even if the wire type grows one.
 * `showLocation` hides the Team and Project columns where they would be
 * constant (every row of a project's list is in that project).
 */
export function TopAccessedTable({
  slot,
  showLocation = true,
}: Readonly<{
  slot: Slot<AdminTopAccessedResource[]>
  showLocation?: boolean
}>) {
  if (slot.loading) return <Skeleton className="h-32 w-full" />
  if (slot.error) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Failed to load the most accessed resources</AlertTitle>
        <AlertDescription>{slot.error}</AlertDescription>
      </Alert>
    )
  }
  const rows = slot.data ?? []
  if (rows.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        No resource access recorded in this range.
      </p>
    )
  }

  return (
    <Card className="overflow-hidden">
      <Table>
        <TableHeader>
          <TableRow className="bg-muted/40 hover:bg-muted/40">
            <TableHead className="h-9 text-xs font-medium">Type</TableHead>
            {showLocation && (
              <>
                <TableHead className="h-9 text-xs font-medium">Team</TableHead>
                <TableHead className="h-9 text-xs font-medium">
                  Project
                </TableHead>
              </>
            )}
            <TableHead className="h-9 text-xs font-medium">Id</TableHead>
            <TableHead className="h-9 text-right text-xs font-medium">
              Accesses
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map(row => (
            <TableRow key={`${row.resource_type}-${row.resource_short_id}`}>
              <TableCell className="py-3">
                <Badge variant="outline">
                  {formatResourceType(row.resource_type)}
                </Badge>
              </TableCell>
              {showLocation && (
                <>
                  <TableCell className="py-3 text-sm">
                    <Link
                      to={`/admin/teams/${row.team_id}`}
                      className="hover:underline"
                    >
                      {row.team_name}
                    </Link>
                  </TableCell>
                  <TableCell className="py-3 text-sm">
                    {row.project_name ?? '—'}
                  </TableCell>
                </>
              )}
              <TableCell className="py-3 font-mono text-xs">
                {row.resource_short_id}
                {row.resource_deleted && (
                  <span className="text-muted-foreground ml-2 font-sans">
                    (deleted)
                  </span>
                )}
              </TableCell>
              <TableCell className="py-3 text-right text-sm tabular-nums">
                {row.access_count.toLocaleString()}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  )
}
