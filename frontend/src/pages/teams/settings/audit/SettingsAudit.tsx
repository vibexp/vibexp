import { useCallback, useEffect, useState } from 'react'

import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { usePermissions } from '@/hooks/usePermissions'
import { formatDateTime } from '@/lib/time'
import type { Team } from '@/services/teamService'
import type {
  TeamSettingsAuditEntry,
  TeamSettingsAuditListResponse,
  TeamSettingsAuditSurface,
} from '@/services/teamSettingsAuditService'
import { teamSettingsAuditService } from '@/services/teamSettingsAuditService'

const PER_PAGE = 20

const SURFACE_LABELS: Record<TeamSettingsAuditSurface, string> = {
  model_provider: 'Model provider',
  embedding_provider: 'Embedding provider',
  custom_types: 'Artifact types',
}

type Detail = TeamSettingsAuditEntry['detail']

function detailString(detail: Detail, key: string): string | null {
  const value = detail[key]
  return typeof value === 'string' && value.trim() !== '' ? value : null
}

function detailStrings(detail: Detail, key: string): string[] {
  const value = detail[key]
  if (!Array.isArray(value)) return []
  return value.filter((item): item is string => typeof item === 'string')
}

/**
 * What arrived, in the entry's own words.
 *
 * The entry is a SNAPSHOT: the copy services write the resource names into
 * `detail` at write time precisely because the rows they name are polymorphic
 * and may be deleted afterwards (#832). So the name is read from `detail`, never
 * resolved live — a live lookup is the thing that breaks for a deleted resource.
 *
 * `custom_types` is the one surface where a single action copies a whole set, so
 * it has no `source_resource_id`/`created_resource_id` and its names live in
 * `detail.added_slugs` instead.
 */
function describeResource(entry: TeamSettingsAuditEntry): string {
  if (entry.surface === 'custom_types') {
    const slugs = detailStrings(entry.detail, 'added_slugs')
    if (slugs.length === 0) return 'No new types — every one already existed'
    return `${String(slugs.length)} type${slugs.length === 1 ? '' : 's'}: ${slugs.join(', ')}`
  }
  return (
    detailString(entry.detail, 'created_name') ??
    detailString(entry.detail, 'source_name') ??
    'Unnamed'
  )
}

/**
 * The source team, by the name the server resolved.
 *
 * `source_team_name` is null once that team is deleted — the column carries no
 * foreign key by design, so the id survives the team. The row must still read as
 * a record of something that happened rather than as a broken row, so it falls
 * back to naming the team as deleted and keeps the id as the remaining handle.
 */
function SourceTeamCell({
  entry,
}: Readonly<{ entry: TeamSettingsAuditEntry }>) {
  if (entry.source_team_name) {
    return <>from {entry.source_team_name}</>
  }
  if (entry.source_team_id) {
    return (
      <>
        from a deleted team{' '}
        <span className="font-mono">{entry.source_team_id.slice(0, 8)}</span>
      </>
    )
  }
  return <>from an unknown team</>
}

function AuditRow({ entry }: Readonly<{ entry: TeamSettingsAuditEntry }>) {
  const carriedCredential = entry.detail.has_api_key === true

  return (
    <TableRow data-testid="settings-audit-row">
      <TableCell className="text-muted-foreground text-sm whitespace-nowrap">
        {formatDateTime(entry.created_at)}
      </TableCell>
      <TableCell className="text-sm">
        {entry.actor_name ?? (
          <span className="text-muted-foreground italic">Deleted user</span>
        )}
      </TableCell>
      <TableCell className="text-sm">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium">{SURFACE_LABELS[entry.surface]}</span>
          <span>{describeResource(entry)}</span>
          {carriedCredential && (
            <Badge variant="secondary" data-testid="carried-credential">
              Included an API key
            </Badge>
          )}
        </div>
        <p className="text-muted-foreground">
          <SourceTeamCell entry={entry} />
        </p>
      </TableCell>
    </TableRow>
  )
}

function LoadingSkeleton() {
  return (
    <Card>
      <CardContent className="space-y-3 p-6">
        {['r1', 'r2', 'r3', 'r4', 'r5'].map(key => (
          <Skeleton key={key} className="h-10 w-full" />
        ))}
      </CardContent>
    </Card>
  )
}

function EmptyLog() {
  return (
    <Card>
      <CardContent
        className="text-muted-foreground p-8 text-center text-sm"
        data-testid="settings-audit-empty"
      >
        Nothing copied in yet. Whenever configuration is copied into this team
        from another one, it will be recorded here.
      </CardContent>
    </Card>
  )
}

/**
 * Settings › Audit — the copy history for one team (#836, epic #827).
 *
 * This is the epic's compensating control, not a reporting nicety: cross-team
 * copy lets configuration — including a credential's *use* — arrive in a team
 * from outside it, so a team's owners need a first-class answer to "where did
 * this provider come from, and who brought it here?".
 *
 * `team` is the team `TeamScopeLayout` resolved from the URL (#584), and the
 * permission gate reads off *it*. `RequirePermission` would read the ambient
 * team, which inside `/teams/:id/**` is a different team on a cold deep-link —
 * gating the wrong team's settings. That is a correctness bug in both
 * directions, so the explicit `usePermissions(team)` form is load-bearing here.
 */
export function SettingsAudit({ team }: Readonly<{ team: Team }>) {
  const { can } = usePermissions(team)
  const allowed = can('team.settings.update')

  const [page, setPage] = useState(1)
  const [result, setResult] = useState<TeamSettingsAuditListResponse | null>(
    null
  )
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const teamId = team.id
  const load = useCallback(
    (signal: AbortSignal) =>
      teamSettingsAuditService.getAudit(teamId, page, PER_PAGE, signal),
    [teamId, page]
  )

  useEffect(() => {
    if (!allowed) return
    const controller = new AbortController()
    setLoading(true)
    load(controller.signal)
      .then(response => {
        if (controller.signal.aborted) return
        setResult(response)
        setError(null)
      })
      .catch((err: unknown) => {
        if (controller.signal.aborted) return
        setError(
          err instanceof Error ? err.message : 'Failed to load the audit log'
        )
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => {
      controller.abort()
    }
  }, [load, allowed])

  const header = (
    <PageHeader
      title="Audit"
      description="Configuration copied into this team from another one, newest first."
    />
  )

  // Gate by hiding the surface, not by disabling it. The hub already omits the
  // card, so this only fires on a deep link — but a blank page would read as a
  // bug, and the server refuses the request anyway (the gate here is
  // convenience; `team.settings.update` is enforced in the service).
  if (!allowed) {
    return (
      <div className="space-y-8">
        {header}
        <Alert data-testid="settings-audit-forbidden">
          <AlertTitle>Not available for your role</AlertTitle>
          <AlertDescription>
            Only a team&apos;s owners and admins can see where its configuration
            came from.
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  return (
    <div className="space-y-8">
      {header}
      <AuditLog
        loading={loading}
        error={error}
        result={result}
        page={page}
        onPageChange={setPage}
      />
    </div>
  )
}

function AuditLog({
  loading,
  error,
  result,
  page,
  onPageChange,
}: Readonly<{
  loading: boolean
  error: string | null
  result: TeamSettingsAuditListResponse | null
  page: number
  onPageChange: (next: number) => void
}>) {
  if (loading && !result) return <LoadingSkeleton />

  const errorAlert = error && (
    <Alert variant="destructive">
      <AlertTitle>Couldn&apos;t load the audit log</AlertTitle>
      <AlertDescription>{error}</AlertDescription>
    </Alert>
  )

  // Only replace the whole view when there is nothing to keep. Once a page has
  // loaded, a later failure (usually paging past the end) is shown ABOVE the
  // retained table — returning just the alert would take the pagination
  // controls with it and strand the user with no way back to a page that works.
  if (error && !result) return errorAlert

  const entries = result?.entries ?? []
  if (entries.length === 0) {
    return (
      <div className="space-y-4">
        {errorAlert}
        <EmptyLog />
      </div>
    )
  }

  const totalPages = result?.total_pages ?? 1

  return (
    <div className="space-y-4">
      {errorAlert}
      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>When</TableHead>
                <TableHead>Who</TableHead>
                <TableHead>What</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {entries.map(entry => (
                <AuditRow key={entry.id} entry={entry} />
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {totalPages > 1 && (
        <div className="flex items-center justify-between">
          <p className="text-muted-foreground text-sm">
            Page {page} of {totalPages} · {result?.total_count ?? 0} entries
          </p>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1 || loading}
              onClick={() => {
                onPageChange(page - 1)
              }}
            >
              Previous
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages || loading}
              onClick={() => {
                onPageChange(page + 1)
              }}
            >
              Next
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
