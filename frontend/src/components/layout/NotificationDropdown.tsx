import { Bell } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

import { NotificationItem } from '@/components/layout/NotificationItem'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { useNotifications } from '@/hooks/useNotifications'

interface NotificationDropdownProps {
  onClose: () => void
  onUnreadChange: (count: number) => void
}

export function NotificationDropdown({
  onClose,
  onUnreadChange,
}: NotificationDropdownProps) {
  const { notifications, loading, markAsRead, markAllAsRead } =
    useNotifications({ limit: 10 })
  const navigate = useNavigate()

  const handleMarkAllRead = () => {
    markAllAsRead()
    onUnreadChange(0)
  }

  const handleRead = (id: string) => {
    markAsRead(id)
    // Per-item reads do not update the badge here; the next focus poll will resync.
  }

  const handleSeeAll = () => {
    onClose()
    void navigate('/notifications')
  }

  const unreadCount = notifications.filter(n => !n.read_at).length

  return (
    <div className="flex flex-col">
      <div className="flex items-center justify-between px-4 py-3">
        <h3 className="text-sm font-semibold">Notifications</h3>
        {unreadCount > 0 && (
          <Button
            variant="ghost"
            size="sm"
            className="text-primary h-auto p-0 text-xs font-normal hover:text-primary/80"
            onClick={handleMarkAllRead}
          >
            Mark all read
          </Button>
        )}
      </div>
      <Separator />

      {loading && notifications.length === 0 ? (
        <div className="flex items-center justify-center py-8">
          <span className="text-muted-foreground text-sm">Loading…</span>
        </div>
      ) : notifications.length === 0 ? (
        <div className="flex flex-col items-center justify-center gap-2 py-10">
          <Bell className="text-muted-foreground size-8" />
          <p className="text-muted-foreground text-sm">
            You&apos;re all caught up
          </p>
        </div>
      ) : (
        <ScrollArea className="max-h-96">
          <div className="py-1">
            {notifications.map(notification => (
              <NotificationItem
                key={notification.id}
                notification={notification}
                onRead={handleRead}
              />
            ))}
          </div>
        </ScrollArea>
      )}

      <Separator />
      <div className="p-2">
        <Button
          variant="ghost"
          size="sm"
          className="w-full text-xs"
          onClick={handleSeeAll}
        >
          See all notifications
        </Button>
      </div>
    </div>
  )
}
