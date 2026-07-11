import type { AgentCard } from '@/services/agentService'

export { formatDate, formatRelativeTime } from '@/lib/time'

// A2A v1.0 (protocol >= 1.0) moved transport/protocol off the top level of the
// agent card into a per-interface `supportedInterfaces[]` list (each entry has a
// url + protocolBinding + protocolVersion). The old top-level transport /
// protocol-version fields were dropped. Cards may expose zero, one, or many
// interfaces, so the UI renders a single "primary" interface (the first entry).
export type AgentInterface = NonNullable<
  AgentCard['supportedInterfaces']
>[number]

/**
 * The interface to surface for a card in single-value UI (transport / protocol):
 * the first entry of `supportedInterfaces`, or undefined when the list is
 * null/empty. Callers fall back to a placeholder (e.g. "Not specified").
 */
export function primaryInterface(
  card: AgentCard | null | undefined
): AgentInterface | undefined {
  return card?.supportedInterfaces?.[0] ?? undefined
}

export type AgentStatusVariant =
  | 'default'
  | 'secondary'
  | 'destructive'
  | 'outline'

export function agentStatusVariant(status: string): AgentStatusVariant {
  switch (status) {
    case 'active':
      return 'default'
    case 'paused':
      return 'secondary'
    case 'error':
      return 'destructive'
    default:
      return 'outline'
  }
}

export function agentStatusLabel(status: string): string {
  switch (status) {
    case 'active':
      return 'Active'
    case 'paused':
      return 'Paused'
    case 'error':
      return 'Error'
    default:
      return status.charAt(0).toUpperCase() + status.slice(1)
  }
}

export function successRateColor(percentage: number): string {
  if (percentage >= 80) return 'text-success'
  if (percentage >= 60) return 'text-warning'
  return 'text-destructive'
}

export function formatDuration(ms: number | null | undefined): string {
  if (ms == null || ms < 0) return '—'
  if (ms < 1000) return `${String(ms)}ms`
  const seconds = ms / 1000
  if (seconds < 60) return `${seconds.toFixed(1)}s`
  const minutes = Math.floor(seconds / 60)
  const remainingSeconds = Math.floor(seconds % 60)
  return `${String(minutes)}m ${String(remainingSeconds)}s`
}
