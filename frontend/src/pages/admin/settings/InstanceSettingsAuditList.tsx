import { Loader2 } from 'lucide-react'
import { type ReactNode, useCallback, useEffect, useState } from 'react'

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
import type {
  AdminInstanceSettingsAuditEntry,
  AdminInstanceSettingsAuditPage,
} from '@/services/adminService'
import type { AdminInstanceSettingsAuditParams } from '@/services/adminSettingsService'

import {
  auditActionLabel,
  auditActorLabel,
  type AuditFieldChange,
} from './instanceSettingsAudit'

const AUDIT_PAGE_SIZE = 20

export interface InstanceSettingsAuditListProps {
  /** Fetches one page of this section's audit log. */
  fetchPage: (
    params: AdminInstanceSettingsAuditParams
  ) => Promise<AdminInstanceSettingsAuditPage>
  /** The section's allowlisted field diff of one entry. */
  describeChanges: (
    entry: AdminInstanceSettingsAuditEntry
  ) => AuditFieldChange[]
  /** Extra badge shown after the timestamp (e.g. the credential marker). */
  entryBadge?: (entry: AdminInstanceSettingsAuditEntry) => ReactNode
  actionLabel?: (action: AdminInstanceSettingsAuditEntry['action']) => string
  description: string
  /** `data-testid` of each entry row. */
  entryTestId: string
  /** Reloads from the first page when bumped (after a save or reset). */
  refreshKey: number
}

function AuditEntry({
  entry,
  changes,
  badge,
  actionLabel,
  testId,
}: Readonly<{
  entry: AdminInstanceSettingsAuditEntry
  changes: AuditFieldChange[]
  badge: ReactNode
  actionLabel: string
  testId: string
}>) {
  return (
    <li className="space-y-2 py-3" data-testid={testId}>
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <Badge variant={entry.action === 'delete' ? 'destructive' : 'outline'}>
          {actionLabel}
        </Badge>
        <span className="font-medium">{auditActorLabel(entry)}</span>
        <span className="text-muted-foreground">
          {formatDateTime(entry.created_at)}
        </span>
        {badge}
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
 * An instance settings section's change history, newest first,
 * cursor-paginated with "Load more" (#1191, generalized for #1202).
 * `refreshKey` reloads from the first page — the page bumps it after each
 * change so the new entry shows up.
 */
export function InstanceSettingsAuditList({
  fetchPage,
  describeChanges,
  entryBadge,
  actionLabel = auditActionLabel,
  description,
  entryTestId,
  refreshKey,
}: Readonly<InstanceSettingsAuditListProps>) {
  const [entries, setEntries] = useState<AdminInstanceSettingsAuditEntry[]>([])
  const [nextCursor, setNextCursor] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const loadPage = useCallback(
    (cursor?: string) =>
      fetchPage({ limit: AUDIT_PAGE_SIZE, ...(cursor ? { cursor } : {}) }),
    [fetchPage]
  )

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    loadPage()
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
  }, [loadPage, refreshKey])

  const loadMore = async () => {
    if (!nextCursor) return
    try {
      setLoadingMore(true)
      setError(null)
      const page = await loadPage(nextCursor)
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
        <CardDescription>{description}</CardDescription>
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
                  <AuditEntry
                    key={entry.id}
                    entry={entry}
                    changes={describeChanges(entry)}
                    badge={entryBadge?.(entry)}
                    actionLabel={actionLabel(entry.action)}
                    testId={entryTestId}
                  />
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
