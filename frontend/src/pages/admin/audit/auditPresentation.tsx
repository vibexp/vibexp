import type { LucideIcon } from 'lucide-react'
import {
  BookOpen,
  FileText,
  FolderKanban,
  HardDrive,
  KeyRound,
  MessageSquare,
  Package,
  Plug,
  Rss,
  User,
} from 'lucide-react'

import { RelativeTime } from '@/components/RelativeTime'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { AdminAuditEvent } from '@/services/adminAuditService'

import { AUDIT_RESOURCE_TYPES } from './auditListParams'

/**
 * The audit log's cell renderings (#1517), shared by the table, the stacked
 * cards at phone width and the detail drawer, so an event reads the same in all
 * three.
 */

const SOURCE_LABELS: Record<AdminAuditEvent['source'], string> = {
  web: 'Web',
  cli: 'CLI',
  mcp: 'MCP',
  api: 'API',
  system: 'System',
}

const OUTCOME_LABELS: Record<AdminAuditEvent['outcome'], string> = {
  success: 'Success',
  denied: 'Denied',
  failure: 'Failed',
}

export function sourceLabel(source: AdminAuditEvent['source']): string {
  return SOURCE_LABELS[source]
}

export function outcomeLabel(outcome: AdminAuditEvent['outcome']): string {
  return OUTCOME_LABELS[outcome]
}

/** The absolute instant, in UTC, e.g. "2026-10-16 14:03:22 UTC". */
export function formatUtc(at: string | Date): string {
  return `${new Date(at).toISOString().slice(0, 19).replace('T', ' ')} UTC`
}

/**
 * Relative time, with the absolute UTC instant on hover — the shared
 * `RelativeTime` (native `title`, as every timestamp in the app), given a UTC
 * formatter because audit evidence is compared across time zones.
 */
export function AuditTime({ at }: Readonly<{ at: string }>) {
  return (
    <RelativeTime
      value={at}
      formatTitle={formatUtc}
      className="text-muted-foreground whitespace-nowrap text-xs"
    />
  )
}

/**
 * Who acted. System and anonymous actors are a muted badge; the email of an
 * anonymous actor (the address a failed sign-in tried) is set in italics, since
 * no account may own it.
 */
export function AuditActor({ event }: Readonly<{ event: AdminAuditEvent }>) {
  if (event.actor_type === 'system') {
    return <MutedBadge>System</MutedBadge>
  }
  if (event.actor_type === 'anonymous') {
    return (
      <span className="flex min-w-0 flex-wrap items-center gap-1.5">
        <MutedBadge>Anonymous</MutedBadge>
        {event.actor_email && (
          <span className="text-muted-foreground truncate text-sm italic">
            {event.actor_email}
          </span>
        )}
      </span>
    )
  }
  return (
    <span className="flex min-w-0 flex-col">
      <span className="truncate text-sm">
        {event.actor_name || event.actor_email || 'Unknown user'}
      </span>
      {event.actor_name && event.actor_email && (
        <span className="text-muted-foreground truncate text-xs">
          {event.actor_email}
        </span>
      )}
    </span>
  )
}

function MutedBadge({ children }: Readonly<{ children: string }>) {
  return (
    <Badge variant="outline" className="text-muted-foreground font-normal">
      {children}
    </Badge>
  )
}

/** The channel; an MCP event also names its AI assistant when known. */
export function AuditSourceBadge({
  event,
}: Readonly<{ event: AdminAuditEvent }>) {
  const label =
    event.source === 'mcp' && event.assistant_name
      ? `MCP · ${event.assistant_name}`
      : sourceLabel(event.source)
  return (
    <Badge variant="outline" className="max-w-full truncate font-normal">
      {label}
    </Badge>
  )
}

/** Tinted, borderless tones of one badge family, per outcome. */
const OUTCOME_TONES: Record<AdminAuditEvent['outcome'], string> = {
  success: 'bg-muted text-muted-foreground',
  denied: 'bg-warning/10 text-warning',
  failure: 'bg-destructive/10 text-destructive',
}

/**
 * The outcome with its semantic tone: success stays neutral, a denial is a
 * warning and a failure is destructive. One style family — a subtle tint with
 * the matching text — so no outcome shouts louder than the others.
 * Design-system tokens only.
 */
export function AuditOutcomeBadge({
  outcome,
}: Readonly<{ outcome: AdminAuditEvent['outcome'] }>) {
  return (
    <Badge
      variant="outline"
      className={cn('border-transparent', OUTCOME_TONES[outcome])}
    >
      {outcomeLabel(outcome)}
    </Badge>
  )
}

/**
 * The team and project an event concerns, stacked: the team on top, the
 * project below in muted small text. "—" when it names neither.
 */
export function AuditScope({ event }: Readonly<{ event: AdminAuditEvent }>) {
  if (!event.team_name && !event.project_name) {
    return <span className="text-muted-foreground text-sm">—</span>
  }
  return (
    <span className="flex min-w-0 flex-col">
      {event.team_name && (
        <span className="truncate text-sm">{event.team_name}</span>
      )}
      {event.project_name && (
        <span className="text-muted-foreground truncate text-xs">
          {event.project_name}
        </span>
      )}
    </span>
  )
}

const RESOURCE_ICONS: Record<string, LucideIcon> = {
  prompt: FileText,
  blueprint: BookOpen,
  artifact: Package,
  memory: HardDrive,
  feed: Rss,
  feed_item: MessageSquare,
  project: FolderKanban,
  user: User,
  api_key: KeyRound,
  oauth_client: Plug,
}

const RESOURCE_LABELS: Record<string, string> = Object.fromEntries(
  AUDIT_RESOURCE_TYPES.map(type => [type.value, type.label])
)

/** A resource type as a label, falling back to the raw value. */
export function resourceTypeLabel(type: string): string {
  return RESOURCE_LABELS[type] ?? type
}

/** The resource acted on: its type's icon, then its name (or id). */
export function AuditResource({
  event,
  className,
}: Readonly<{ event: AdminAuditEvent; className?: string }>) {
  if (!event.resource_type) {
    return <span className="text-muted-foreground text-sm">—</span>
  }
  const Icon = RESOURCE_ICONS[event.resource_type] ?? FileText
  const type = resourceTypeLabel(event.resource_type)
  return (
    <span className={cn('flex min-w-0 items-center gap-1.5', className)}>
      <Icon
        className="text-muted-foreground size-4 shrink-0"
        aria-label={type}
      />
      <span
        className="truncate text-sm"
        title={event.resource_name ?? event.resource_id ?? undefined}
      >
        {event.resource_name ?? event.resource_id ?? type}
      </span>
    </span>
  )
}

/** "Team / Project", or whichever of the two the event names. */
export function auditScope(event: AdminAuditEvent): string {
  const parts = [event.team_name, event.project_name].filter(
    (part): part is string => Boolean(part)
  )
  return parts.length > 0 ? parts.join(' / ') : '—'
}
