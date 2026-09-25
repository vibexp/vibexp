import { useCallback, useState } from 'react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatDateTime } from '@/lib/time'
import {
  carriedCredential,
  describeResource,
  surfaceLabel,
} from '@/pages/teams/settings/audit/settingsAuditFormat'
import type { AdminTeamSettingsAuditEntry } from '@/services/adminService'
import { adminService } from '@/services/adminService'

import { AdminConfigPanel } from './AdminConfigPanel'
import { shortId } from './teamConfigFormat'
import { useAdminTeamSection } from './useAdminTeamSection'

const PER_PAGE = 20

/** The source team by name, or the id once that team is gone. */
function sourceTeam(entry: AdminTeamSettingsAuditEntry): string {
  if (entry.source_team_name) return entry.source_team_name
  if (entry.source_team_id) {
    return `Deleted team ${shortId(entry.source_team_id)}`
  }
  return 'Unknown team'
}

function AuditRow({ entry }: Readonly<{ entry: AdminTeamSettingsAuditEntry }>) {
  return (
    <TableRow data-testid="settings-audit-row">
      <TableCell className="text-muted-foreground py-3 text-xs whitespace-nowrap">
        {formatDateTime(entry.created_at)}
      </TableCell>
      <TableCell className="py-3 text-sm">
        {surfaceLabel(entry.surface)}
      </TableCell>
      <TableCell className="py-3 text-sm">
        {entry.actor_name ?? (
          <span className="text-muted-foreground italic">Deleted user</span>
        )}
      </TableCell>
      <TableCell className="py-3 text-sm">{sourceTeam(entry)}</TableCell>
      <TableCell className="py-3 text-sm">
        <div className="flex flex-wrap items-center gap-2">
          <span>{describeResource(entry)}</span>
          {carriedCredential(entry) && (
            <Badge variant="secondary" className="font-normal">
              Included an API key
            </Badge>
          )}
        </div>
      </TableCell>
    </TableRow>
  )
}

/**
 * The team's settings audit log — configuration copied in from other teams —
 * newest first, 20 per page (read-only).
 */
export function TeamSettingsAuditTab({ teamId }: Readonly<{ teamId: string }>) {
  const [page, setPage] = useState(1)
  const load = useCallback(
    () => adminService.listTeamSettingsAudit(teamId, { page, limit: PER_PAGE }),
    [teamId, page]
  )
  const { data, loading, error } = useAdminTeamSection(
    load,
    'Failed to load the settings audit log'
  )
  const totalPages = data?.total_pages ?? 1

  return (
    <AdminConfigPanel
      loading={loading}
      error={error}
      errorTitle="Failed to load the settings audit log"
      empty={data?.entries.length === 0 && page === 1}
      emptyMessage="Nothing has been copied into this team from another one."
    >
      {data && (
        <>
          <Card className="overflow-hidden">
            <Table>
              <TableHeader>
                <TableRow className="bg-muted/40 hover:bg-muted/40">
                  <TableHead className="h-9 text-xs font-medium">
                    When
                  </TableHead>
                  <TableHead className="h-9 text-xs font-medium">
                    Surface
                  </TableHead>
                  <TableHead className="h-9 text-xs font-medium">
                    Actor
                  </TableHead>
                  <TableHead className="h-9 text-xs font-medium">
                    Source team
                  </TableHead>
                  <TableHead className="h-9 text-xs font-medium">
                    What
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.entries.map(entry => (
                  <AuditRow key={entry.id} entry={entry} />
                ))}
              </TableBody>
            </Table>
          </Card>
          <div className="flex items-center justify-between">
            <p className="text-muted-foreground text-sm">
              Page {page} of {Math.max(totalPages, 1)} · {data.total_count}{' '}
              entries
            </p>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={page <= 1}
                onClick={() => {
                  setPage(p => p - 1)
                }}
              >
                Previous
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={page >= totalPages}
                onClick={() => {
                  setPage(p => p + 1)
                }}
              >
                Next
              </Button>
            </div>
          </div>
        </>
      )}
    </AdminConfigPanel>
  )
}
