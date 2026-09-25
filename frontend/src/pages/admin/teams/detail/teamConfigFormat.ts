import type {
  AdminTeamConfigSource,
  AdminTeamEmailProviderConfig,
} from '@/services/adminService'

/**
 * Pure formatting for the admin team detail's configuration tabs (#1142).
 * Kept out of the `.tsx` files so they export components only and so the
 * logic is testable without rendering.
 */

/** The badge text for where a section's values came from. */
export function sourceLabel(source: AdminTeamConfigSource): string {
  return source === 'instance' ? 'Inherited from instance' : 'Team'
}

type EmailStatus = AdminTeamEmailProviderConfig['status']

/**
 * Badge label and variant for the email provider's derived health.
 * `unknown` (inherited, or never used) is neutral rather than alarming.
 */
export function emailStatusMeta(status: EmailStatus): {
  label: string
  variant: 'secondary' | 'destructive' | 'outline'
} {
  switch (status) {
    case 'healthy':
      return { label: 'Healthy', variant: 'secondary' }
    case 'failing':
      return { label: 'Failing', variant: 'destructive' }
    default:
      return { label: 'Unknown', variant: 'outline' }
  }
}

/** The first 8 characters of an id — enough to tell rows apart. */
export function shortId(id: string): string {
  return id.slice(0, 8)
}

/** A nullable value for a read-only field: `—` when there is nothing to show. */
export function displayValue(
  value: string | number | null | undefined
): string {
  if (value === null || value === undefined) return '—'
  const text = String(value)
  return text.trim() === '' ? '—' : text
}
