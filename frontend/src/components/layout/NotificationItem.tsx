import { formatRelativeTime } from '@/lib/time'
import { cn } from '@/lib/utils'
import type { Notification } from '@/services/notificationService'

/**
 * Returns a safe href string for use in anchor elements, or null if the URL
 * is not from an allowed scheme (relative paths, http, https).
 * Rejects javascript:, data:, vbscript:, and any other scheme.
 */
export function safeHref(url: string | null | undefined): string | null {
  if (!url) return null

  // Allow same-origin relative paths starting with /
  if (url.startsWith('/')) return url

  // Allow only http: and https: absolute URLs
  try {
    const parsed = new URL(url)
    if (parsed.protocol === 'http:' || parsed.protocol === 'https:') {
      return url
    }
    return null
  } catch {
    return null
  }
}

interface NotificationItemProps {
  notification: Notification
  onRead: (id: string) => void
}

export function NotificationItem({
  notification,
  onRead,
}: NotificationItemProps) {
  const isUnread = !notification.read_at

  const handleClick = () => {
    if (isUnread) {
      onRead(notification.id)
    }
  }

  const href = safeHref(notification.action_url)
  const isExternalLink =
    href !== null && (href.startsWith('http://') || href.startsWith('https://'))

  const sharedClassName = cn(
    'flex flex-col gap-1 px-4 py-3 text-sm transition-colors hover:bg-accent focus:bg-accent focus:outline-none',
    isUnread && 'bg-muted/50'
  )

  const content = (
    <>
      <div className="flex items-start justify-between gap-2">
        <span
          className={cn(
            'line-clamp-1 flex-1',
            isUnread ? 'font-semibold' : 'font-medium'
          )}
        >
          {isUnread && (
            <span className="mr-1.5 inline-block size-1.5 rounded-full bg-primary align-middle" />
          )}
          {notification.title}
        </span>
        <span className="text-muted-foreground shrink-0 text-xs">
          {formatRelativeTime(notification.created_at)}
        </span>
      </div>
      {notification.body && (
        <p className="text-muted-foreground line-clamp-2 text-xs">
          {notification.body}
        </p>
      )}
    </>
  )

  if (href !== null) {
    return (
      <a
        href={href}
        onClick={handleClick}
        target={isExternalLink ? '_blank' : undefined}
        rel={isExternalLink ? 'noopener noreferrer' : undefined}
        className={sharedClassName}
      >
        {content}
      </a>
    )
  }

  return (
    <button type="button" onClick={handleClick} className={sharedClassName}>
      {content}
    </button>
  )
}
