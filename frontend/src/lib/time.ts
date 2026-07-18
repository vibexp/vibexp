/**
 * Shared time-formatting helpers.
 * Centralises duplicated date logic from pages/agents/helpers.ts and
 * the various inline formatDate functions scattered across the codebase.
 */

/** Accepted input for the formatters: a Date, an ISO string, or nothing. */
type DateValue = Date | string | null | undefined

/**
 * Formats a date string or Date object as a human-readable date.
 * Returns "Never" when the value is null / undefined.
 */
export function formatDate(value: DateValue): string {
  if (!value) return 'Never'
  const d = value instanceof Date ? value : new Date(value)
  return d.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

/**
 * Formats a date string or Date object as a long date+time string.
 * Returns "Never" when the value is null / undefined.
 * Example output: "January 1, 2024, 12:00 AM"
 */
export function formatDateTime(value: DateValue): string {
  if (!value) return 'Never'
  const d = value instanceof Date ? value : new Date(value)
  return d.toLocaleString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

/**
 * Formats a date string or Date object as a relative time expression
 * (e.g. "just now", "3m ago", "2h ago", "5d ago").
 * Falls back to a formatted date for older values.
 * Returns "Never" when the value is null / undefined.
 */
export function formatRelativeTime(value: DateValue): string {
  if (!value) return 'Never'
  const date = value instanceof Date ? value : new Date(value)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const seconds = Math.floor(diffMs / 1000)
  if (seconds < 60) return 'just now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${String(minutes)}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${String(hours)}h ago`
  const days = Math.floor(hours / 24)
  if (days < 7) return `${String(days)}d ago`
  return formatDate(date)
}
