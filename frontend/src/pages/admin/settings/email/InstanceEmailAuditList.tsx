import { Loader2 } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { formatDateTime } from '@/lib/time'
import {
  type AdminInstanceSettingsAuditEntry,
  adminService,
} from '@/services/adminService'

import {
  auditActionLabel,
  auditActorLabel,
  auditFieldChanges,
  credentialChange,
} from './instanceEmailAudit'

const AUDIT_PAGE_SIZE = 20

function AuditEntry({
  entry,
}: Readonly<{ entry: AdminInstanceSettingsAuditEntry }>) {
  const changes = auditFieldChanges(entry)
  const credential = credentialChange(entry)

  return (
    <li className="space-y-2 py-3" data-testid="instance-email-audit-entry">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <Badge variant={entry.action === 'delete' ? 'destructive' : 'outline'}>
          {auditActionLabel(entry.action)}
        </Badge>
        <span className="font-medium">{auditActorLabel(entry)}</span>
        <span className="text-muted-foreground">
          {formatDateTime(entry.created_at)}
        </span>
        {credential && (
          <Badge variant="secondary" className="font-normal">
            {credential === 'changed'
              ? 'Credential changed'
              : 'Credential unchanged'}
          </Badge>
        )}
      </div>
      {changes.length > 0 ? (
        <dl className="grid gap-x-4 gap-y-1 text-sm sm:grid-cols-[max-content_1fr]">
          {changes.map(change => (
            <div key={change.key} className="contents">
              <dt className="text-muted-foreground">{change.label}</dt>
              <dd className="break-all">
                <span className="text-muted-foreground line-through">
                  {change.before}
                </span>{' '}
                → {change.after}
              </dd>
            </div>
          ))}
        </dl>
      ) : (
        <p className="text-muted-foreground text-sm">
          No configuration field changed.
        </p>
      )}
    </li>
  )
}

/**
 * The instance email provider's change history, newest first, cursor-paginated
 * with "Load more". `refreshKey` reloads from the first page — the page bumps
 * it after each save or removal so the new entry shows up.
 */
export function InstanceEmailAuditList({
  refreshKey,
}: Readonly<{ refreshKey: number }>) {
  const [entries, setEntries] = useState<AdminInstanceSettingsAuditEntry[]>([])
  const [nextCursor, setNextCursor] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const fetchPage = useCallback(
    (cursor?: string) =>
      adminService.listInstanceEmailSettingsAudit({
        limit: AUDIT_PAGE_SIZE,
        ...(cursor ? { cursor } : {}),
      }),
    []
  )

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    fetchPage()
      .then(page => {
        if (cancelled) return
        setEntries(page.entries)
        setNextCursor(page.next_cursor)
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setError(
          err instanceof Error ? err.message : 'Failed to load the history'
        )
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [fetchPage, refreshKey])

  const loadMore = async () => {
    if (!nextCursor) return
    try {
      setLoadingMore(true)
      setError(null)
      const page = await fetchPage(nextCursor)
      setEntries(current => [...current, ...page.entries])
      setNextCursor(page.next_cursor)
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to load the history'
      )
    } finally {
      setLoadingMore(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Change history</CardTitle>
        <CardDescription>
          Every change to the instance email provider. Credentials are never
          recorded — only whether one changed.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {loading ? (
          <div className="flex justify-center py-6">
            <Loader2
              className="text-muted-foreground size-5 animate-spin"
              aria-label="Loading history"
            />
          </div>
        ) : (
          <>
            {entries.length === 0 && !error && (
              <p className="text-muted-foreground text-sm">
                No changes have been recorded yet.
              </p>
            )}
            {entries.length > 0 && (
              <ul className="divide-y">
                {entries.map(entry => (
                  <AuditEntry key={entry.id} entry={entry} />
                ))}
              </ul>
            )}
            {error && (
              <Alert variant="destructive">
                <AlertTitle>Failed to load the history</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            {nextCursor && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={loadingMore}
                onClick={() => {
                  void loadMore()
                }}
              >
                {loadingMore ? 'Loading…' : 'Load more'}
              </Button>
            )}
          </>
        )}
      </CardContent>
    </Card>
  )
}
