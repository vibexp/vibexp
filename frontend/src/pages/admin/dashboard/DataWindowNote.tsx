import { formatDate } from '@/lib/time'

/**
 * Explains a truncated history.
 *
 * Both event tables behind the activity charts are TTL-pruned (`retention.
 * activity_days` / `access_event_days`), so asking for a range older than the
 * retention window legitimately returns zeros. Without this caption that reads as
 * data loss, which is the wrong conclusion to invite an admin to draw.
 *
 * Rendered as one string rather than interpolated JSX so the sentence is a single
 * text node — split nodes are both harder to assert on and read less predictably
 * to a screen reader.
 */
export function DataWindowNote({
  earliestRetainedAt,
  label,
}: Readonly<{ earliestRetainedAt: string; label: string }>) {
  const text = `${label} are retained from ${formatDate(earliestRetainedAt)}. Earlier buckets report zero because the events have been pruned, not because nothing happened.`
  return <p className="text-muted-foreground text-xs">{text}</p>
}
