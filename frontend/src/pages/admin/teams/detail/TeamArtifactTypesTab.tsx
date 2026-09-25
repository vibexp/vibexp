import { useCallback } from 'react'

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
import { adminService } from '@/services/adminService'

import { AdminConfigPanel } from './AdminConfigPanel'
import { useAdminTeamSection } from './useAdminTeamSection'

/** The artifact types the team sees — system defaults and its own (read-only). */
export function TeamArtifactTypesTab({ teamId }: Readonly<{ teamId: string }>) {
  const load = useCallback(
    () => adminService.getTeamArtifactTypes(teamId),
    [teamId]
  )
  const { data, loading, error } = useAdminTeamSection(
    load,
    'Failed to load artifact types'
  )

  return (
    <AdminConfigPanel
      loading={loading}
      error={error}
      errorTitle="Failed to load artifact types"
      empty={data?.types.length === 0}
      emptyMessage="This team sees no artifact types."
    >
      {data && (
        <Card className="overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/40 hover:bg-muted/40">
                <TableHead className="h-9 text-xs font-medium">Name</TableHead>
                <TableHead className="h-9 text-xs font-medium">Slug</TableHead>
                <TableHead className="h-9 text-xs font-medium">Kind</TableHead>
                <TableHead className="h-9 text-xs font-medium">
                  Created
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.types.map(type => (
                <TableRow key={type.id} data-testid="artifact-type-row">
                  <TableCell className="py-3 text-sm">{type.name}</TableCell>
                  <TableCell className="py-3 font-mono text-xs">
                    {type.slug}
                  </TableCell>
                  <TableCell className="py-3">
                    <Badge
                      variant={type.is_system ? 'outline' : 'secondary'}
                      className="font-normal"
                    >
                      {type.is_system ? 'System' : 'Team'}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground py-3 text-xs">
                    {formatDate(type.created_at)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
    </AdminConfigPanel>
  )
}
