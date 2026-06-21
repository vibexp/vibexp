import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatDateTime, formatRelativeTime } from '@/lib/time'
import { cn } from '@/lib/utils'

interface RelativeTimeProps {
  /** Date string or Date to display. */
  value: Date | string | null | undefined
  /** Extra classes applied to the visible compact label. */
  className?: string
}

/**
 * Renders a compact relative-time label (e.g. "3d ago", or a short date
 * beyond 7 days) and reveals the full date-time (e.g. "June 7, 2026,
 * 09:14 AM") on hover via a tooltip. Reuses the shared formatters in
 * `@/lib/time` — no new date logic.
 */
export function RelativeTime({ value, className }: RelativeTimeProps) {
  const compact = formatRelativeTime(value)
  const full = formatDateTime(value)

  return (
    <TooltipProvider>
      <Tooltip delayDuration={0}>
        <TooltipTrigger asChild>
          <span className={cn('cursor-default', className)}>{compact}</span>
        </TooltipTrigger>
        <TooltipContent>{full}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
