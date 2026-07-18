import { Bell } from 'lucide-react'
import { useCallback, useState } from 'react'

import { EmptyState } from '@/components/EmptyState'
import { NotificationItem } from '@/components/layout/NotificationItem'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useNotifications } from '@/hooks/useNotifications'

type Filter = 'all' | 'unread'

export function Notifications() {
  const [filter, setFilter] = useState<Filter>('all')

  const {
    notifications,
    loading,
    error,
    hasMore,
    fetchMore,
    markAsRead,
    markAllAsRead,
  } = useNotifications({
    limit: 20,
    unread: filter === 'unread' || undefined,
  })

  const handleFilterAll = useCallback(() => {
    setFilter('all')
  }, [])

  const handleFilterUnread = useCallback(() => {
    setFilter('unread')
  }, [])

  const handleMarkAllRead = useCallback(() => {
    markAllAsRead()
  }, [markAllAsRead])

  const handleRead = useCallback(
    (id: string) => {
      markAsRead(id)
    },
    [markAsRead]
  )

  const unreadCount = notifications.filter(n => !n.read_at).length

  const notificationsContent =
    notifications.length === 0 ? (
      <div className="p-6">
        <EmptyState
          icon={Bell}
          title="You're all caught up"
          description={
            filter === 'unread'
              ? 'No unread notifications.'
              : 'No notifications yet. Activity across your workspace will appear here.'
          }
        />
      </div>
    ) : (
      <div className="divide-y">
        {notifications.map(notification => (
          <NotificationItem
            key={notification.id}
            notification={notification}
            onRead={handleRead}
          />
        ))}
      </div>
    )

  return (
    <div className="space-y-6">
      <PageHeader
        title="Notifications"
        description="Stay up to date with activity across your workspace."
      />

      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-2">
          <Button
            variant={filter === 'all' ? 'default' : 'outline'}
            size="sm"
            onClick={handleFilterAll}
          >
            All
          </Button>
          <Button
            variant={filter === 'unread' ? 'default' : 'outline'}
            size="sm"
            onClick={handleFilterUnread}
          >
            Unread
          </Button>
        </div>

        {unreadCount > 0 && (
          <Button
            variant="ghost"
            size="sm"
            className="text-primary text-xs hover:text-primary/80"
            onClick={handleMarkAllRead}
          >
            Mark all read
          </Button>
        )}
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardContent className="p-0">
          {loading && notifications.length === 0 ? (
            <div className="space-y-0 divide-y">
              {Array.from({ length: 5 }).map((_, i) => (
                <div
                  key={`skeleton-${String(i)}`}
                  className="flex flex-col gap-2 px-4 py-3"
                >
                  <Skeleton className="h-4 w-3/4" />
                  <Skeleton className="h-3 w-1/2" />
                </div>
              ))}
            </div>
          ) : (
            notificationsContent
          )}
        </CardContent>
      </Card>

      {hasMore && (
        <div className="flex justify-center">
          <Button
            variant="outline"
            size="sm"
            onClick={fetchMore}
            disabled={loading}
          >
            {loading ? 'Loading…' : 'Load more'}
          </Button>
        </div>
      )}
    </div>
  )
}
