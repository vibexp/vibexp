import { formatDateTime, formatRelativeTime } from '@/lib/time'
import { cn } from '@/lib/utils'

interface RelativeTimeProps {
  /** Date string or Date to display. */
  value: Date | string | null | undefined
  /** Extra classes applied to the visible compact label. */
  className?: string
}

/**
 * Renders a compact relative-time label (e.g. "3d ago", or a short date beyond
 * 7 days) and reveals the full date-time (e.g. "June 7, 2026, 09:14 AM") on
 * hover through the native `title`. Reuses the shared formatters in
 * `@/lib/time` — no new date logic.
 *
 * The full value used to come from a Radix tooltip. #907 needed it on `title`
 * as well (a list cell is read by screen readers, copied and asserted on in
 * e2e, none of which a Radix popper reaches), and a browser renders its own
 * `title` bubble regardless of any JS tooltip — so keeping both meant two
 * tooltips on every timestamp in the app. The native one is what survives: it
 * satisfies every consumer, and it drops three Radix nodes per rendered
 * timestamp, of which a list page has one per row.
 */
export function RelativeTime({
  value,
  className,
}: Readonly<RelativeTimeProps>) {
  // No value means both formatters return "Never", and a native bubble that
  // just repeats the cell is noise — so the attribute is omitted entirely.
  return (
    <span
      className={cn('cursor-default', className)}
      title={value ? formatDateTime(value) : undefined}
    >
      {formatRelativeTime(value)}
    </span>
  )
}
