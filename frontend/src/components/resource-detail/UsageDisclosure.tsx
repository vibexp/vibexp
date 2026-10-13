import { ChevronRight } from 'lucide-react'
import { type ReactNode, useId } from 'react'

import { STORAGE_KEYS } from '@/constants/storageKeys'
import { useLocalStorage } from '@/hooks/useLocalStorage'
import { cn } from '@/lib/utils'

/**
 * The reading rail's "Usage" section: the access-activity chart and channel
 * counts behind a disclosure, closed by default.
 *
 * Page views are mostly the reader's own, so the chart must not sit above the
 * parts of the rail that carry knowledge — but it is not removed either. The
 * open/closed choice is remembered per browser. While closed the content is
 * not mounted, so a reader who never opens it never pays for the request.
 */
export function UsageDisclosure({
  children,
}: Readonly<{ children: ReactNode }>) {
  const [open, setOpen] = useLocalStorage<boolean>(
    STORAGE_KEYS.USAGE_OPEN,
    false
  )
  const contentId = useId()

  return (
    <div data-testid="usage-disclosure" data-state={open ? 'open' : 'closed'}>
      <button
        type="button"
        aria-expanded={open}
        aria-controls={contentId}
        onClick={() => {
          setOpen(current => !current)
        }}
        className="hover:text-foreground flex w-full items-center gap-1.5 text-sm font-semibold"
      >
        <ChevronRight
          className={cn('size-4 transition-transform', open && 'rotate-90')}
          aria-hidden
        />
        Usage
      </button>
      {open && (
        <div id={contentId} className="mt-3">
          {children}
        </div>
      )}
    </div>
  )
}
