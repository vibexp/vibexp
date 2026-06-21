import { Bell } from 'lucide-react'
import { useCallback, useState } from 'react'

import { NotificationDropdown } from '@/components/layout/NotificationDropdown'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { useUnreadCount } from '@/hooks/useUnreadCount'
import { cn } from '@/lib/utils'

const MAX_BADGE_COUNT = 99

function formatBadgeCount(count: number): string {
  return count > MAX_BADGE_COUNT ? '99+' : String(count)
}

export function NotificationBell() {
  const [open, setOpen] = useState(false)
  const { unreadCount, resetUnread } = useUnreadCount()

  const handleClose = useCallback(() => {
    setOpen(false)
  }, [])

  const handleUnreadChange = useCallback(
    (count: number) => {
      if (count === 0) {
        resetUnread()
      }
    },
    [resetUnread]
  )

  const ariaLabel =
    unreadCount > 0
      ? `Notifications, ${formatBadgeCount(unreadCount)} unread`
      : 'Notifications'

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label={ariaLabel}
          className="relative"
        >
          <Bell
            className={cn('size-5', unreadCount > 0 && 'text-foreground')}
          />
          {unreadCount > 0 && (
            <span
              aria-hidden="true"
              className="bg-destructive text-destructive-foreground absolute right-1 top-1 flex size-4 items-center justify-center rounded-full text-xs font-bold"
            >
              {formatBadgeCount(unreadCount)}
            </span>
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        sideOffset={8}
        className="w-80 p-0"
        onOpenAutoFocus={e => {
          e.preventDefault()
        }}
      >
        <NotificationDropdown
          onClose={handleClose}
          onUnreadChange={handleUnreadChange}
        />
      </PopoverContent>
    </Popover>
  )
}
