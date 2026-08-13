import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { buildResourceUrl } from '@/lib/resourceUrl'
import { formatDateTime } from '@/lib/time'
import type {
  FreshnessAuditEntry,
  FreshnessAuditListResponse,
} from '@/services/freshnessService'
import { freshnessService } from '@/services/freshnessService'

import { RESOURCE_TYPE_OPTIONS } from './freshnessOptions'

const PER_PAGE = 20

/**
 * Human wording for each (action, reason) pair.
 *
 * `rule_run` is the only cause that can mark; `accessed` and `edited` only ever
 * clear, and only when the team has reversibility on.
 */
function describeEvent(entry: FreshnessAuditEntry): string {
  if (entry.action === 'marked') return 'Marked stale — a rule matched it'
  switch (entry.reason) {
    case 'accessed':
      return 'Cleared — someone opened it'
    case 'edited':
      return 'Cleared — someone edited it'
    default:
      return 'Cleared — no rule matches it any more'
  }
}

function typeLabel(type: FreshnessAuditEntry['resource_type']): string {
  // The options list is plural ("Artifacts"); a single row wants the singular.
  return (
    RESOURCE_TYPE_OPTIONS.find(o => o.value === type)?.label.replace(
      /s$/,
      ''
    ) ?? type
  )
}

/**
 * The resource cell.
 *
 * The audit entry carries only `resource_type` + `resource_id`, and
 * `buildResourceUrl` needs a slug (plus a project id for artifacts and
 * blueprints), so today only memories resolve to a URL. That is exactly the
 * case the helper's `null` return exists for — render plain text rather than a
 * link that 404s. Deep-linking the other three needs `slug`/`project_id` on the
 * audit payload; tracked as a follow-up.
 */
function ResourceCell({ entry }: Readonly<{ entry: FreshnessAuditEntry }>) {
  const href = buildResourceUrl({
    type: entry.resource_type,
    id: entry.resource_id,
  })
  const label = `${typeLabel(entry.resource_type)} ${entry.resource_id.slice(0, 8)}`

  if (!href) {
    return <span className="font-mono text-xs">{label}</span>
  }
  return (
    <Link to={href} className="font-mono text-xs underline underline-offset-2">
      {label}
    </Link>
  )
}

/**
 * The audit tab: a paginated, newest-first log of every mark and clear.
 *
 * Visible to every member by explicit product decision — the engine writes to
 * everyone's resources, so anyone can check what it did and why. There are no
 * actions here; it is a record.
 */
export function FreshnessAudit({ teamId }: Readonly<{ teamId: string }>) {
  const [page, setPage] = useState(1)
  const [result, setResult] = useState<FreshnessAuditListResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(
    (signal: AbortSignal) =>
      freshnessService.getAudit(teamId, page, PER_PAGE, signal),
    [teamId, page]
  )

  useEffect(() => {
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
  }, [load])

  if (loading && !result) {
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

  if (error) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Couldn&apos;t load the audit log</AlertTitle>
        <AlertDescription>{error}</AlertDescription>
      </Alert>
    )
  }

  const entries = result?.entries ?? []

  if (entries.length === 0) {
    return (
      <Card>
        <CardContent className="text-muted-foreground p-8 text-center text-sm">
          Nothing logged yet. Every time a rule marks a resource stale — or
          someone using it clears the flag — it will be recorded here.
        </CardContent>
      </Card>
    )
  }

  const totalPages = result?.total_pages ?? 1

  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Resource</TableHead>
                <TableHead>What happened</TableHead>
                <TableHead className="text-right">When</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {entries.map(entry => (
                <TableRow key={entry.id} data-testid="audit-row">
                  <TableCell>
                    <ResourceCell entry={entry} />
                  </TableCell>
                  <TableCell className="text-sm">
                    {describeEvent(entry)}
                  </TableCell>
                  <TableCell className="text-muted-foreground text-right text-sm whitespace-nowrap">
                    {formatDateTime(entry.created_at)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {totalPages > 1 && (
        <div className="flex items-center justify-between">
          <p className="text-muted-foreground text-sm">
            Page {page} of {totalPages} · {result?.total_count ?? 0} events
          </p>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1 || loading}
              onClick={() => {
                setPage(p => p - 1)
              }}
            >
              Previous
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages || loading}
              onClick={() => {
                setPage(p => p + 1)
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
