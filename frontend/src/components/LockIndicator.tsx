import { Lock } from 'lucide-react'

import { cn } from '@/lib/utils'
import type { ResourceLock } from '@/types/resourceLock'

/**
 * The lock marker on prompt, blueprint and artifact rows (#1285).
 *
 * One shared component for the three lockable lists, for the same reason
 * `FreshnessBadge` is shared: the surfaces cannot then drift apart in wording.
 *
 * **Renders nothing when `lock` is `null`**, which is how the API says
 * "unlocked", so callers pass the payload field straight through and an
 * unlocked row costs no markup.
 *
 * The icon is the whole message, so it carries the accessible name itself
 * (`role="img"` + `aria-label`) rather than being decorative beside a text
 * label. `shrink-0` keeps it whole when a long name truncates next to it.
 */
export function LockIndicator({
  lock,
  className,
}: Readonly<{
  lock?: ResourceLock | null
  className?: string
}>) {
  if (!lock) return null

  return (
    <Lock
      role="img"
      aria-label={`Locked by ${lock.locked_by.display_name}`}
      className={cn('text-muted-foreground size-3.5 shrink-0', className)}
      data-testid="lock-indicator"
    />
  )
}
