import { useEffect, useState } from 'react'
import { Link } from 'react-router'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import { formatDateTime } from '@/lib/time'
import { formatResourceType } from '@/pages/admin/users/detail/userInsightsChartData'
import type { AdminUserTimelineEvent } from '@/services/adminService'
import { adminService } from '@/services/adminService'
import { getErrorMessage } from '@/utils/errorHandling'

const TIMELINE_PAGE_SIZE = 50

/**
 * The user's create/update timeline, newest first, with cursor "Load more".
 *
 * Rows are opaque: each cell reads one allowlisted field, so a title added to the
 * wire type later can never render here.
 */
export function UserActivityTab({ userId }: Readonly<{ userId: string }>) {
  const [entries, setEntries] = useState<AdminUserTimelineEvent[]>([])
  const [nextCursor, setNextCursor] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [loadingMore, setLoadingMore] = useState(false)
  const [loadMoreError, setLoadMoreError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    adminService
      .getUserTimeline(userId, { limit: TIMELINE_PAGE_SIZE })
      .then(page => {
        if (cancelled) return
        setEntries(page.items)
        setNextCursor(page.next_cursor)
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setError(getErrorMessage(err, 'Failed to load activity'))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [userId])

  const loadMore = () => {
    if (!nextCursor) return
    setLoadingMore(true)
    setLoadMoreError(null)
    adminService
      .getUserTimeline(userId, {
        cursor: nextCursor,
        limit: TIMELINE_PAGE_SIZE,
      })
      .then(page => {
        setEntries(prev => [...prev, ...page.items])
        setNextCursor(page.next_cursor)
      })
      .catch((err: unknown) => {
        // The rows already loaded stay; only the next page failed.
        setLoadMoreError(getErrorMessage(err, 'Failed to load more activity'))
      })
      .finally(() => {
        setLoadingMore(false)
      })
  }

  if (loading) {
    return (
      <div className="space-y-2" data-testid="activity-loading">
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-9 w-full" />
      </div>
    )
  }

  if (error) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Failed to load activity</AlertTitle>
        <AlertDescription>{error}</AlertDescription>
      </Alert>
    )
  }

  if (entries.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        This user has not created or updated any resources.
      </p>
    )
  }

  return (
    <div className="space-y-3">
      <Card className="overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow className="bg-muted/40 hover:bg-muted/40">
              <TableHead className="h-9 text-xs font-medium">Type</TableHead>
              <TableHead className="h-9 text-xs font-medium">Action</TableHead>
              <TableHead className="h-9 text-xs font-medium">Team</TableHead>
              <TableHead className="h-9 text-xs font-medium">Project</TableHead>
              <TableHead className="h-9 text-xs font-medium">Id</TableHead>
              <TableHead className="h-9 text-xs font-medium">When</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {entries.map(entry => (
              <TableRow
                key={`${entry.resource_type}-${entry.resource_short_id}-${entry.action}-${entry.occurred_at}`}
              >
                <TableCell className="py-3">
                  <Badge variant="outline">
                    {formatResourceType(entry.resource_type)}
                  </Badge>
                </TableCell>
                <TableCell className="py-3 text-sm">{entry.action}</TableCell>
                <TableCell className="py-3 text-sm">
                  <Link
                    to={`/admin/teams/${entry.team_id}`}
                    className="hover:underline"
                  >
                    {entry.team_name}
                  </Link>
                </TableCell>
                <TableCell className="py-3 text-sm">
                  {entry.project_name ?? '—'}
                </TableCell>
                <TableCell className="py-3 font-mono text-xs">
                  {entry.resource_short_id}
                </TableCell>
                <TableCell className="text-muted-foreground py-3 text-sm">
                  {formatDateTime(entry.occurred_at)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>

      {loadMoreError && (
        <p className="text-destructive text-sm" role="alert">
          {loadMoreError}
        </p>
      )}

      {nextCursor && (
        <Button
          variant="outline"
          size="sm"
          disabled={loadingMore}
          onClick={loadMore}
        >
          {loadingMore ? 'Loading…' : 'Load more'}
        </Button>
      )}
    </div>
  )
}
