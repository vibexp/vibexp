export { formatDate, formatRelativeTime } from '@/lib/time'

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
