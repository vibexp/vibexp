import { ChevronDown } from 'lucide-react'
import type { ReactNode } from 'react'

import { RelativeTime } from '@/components/RelativeTime'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import type { AdminAuditEvent } from '@/services/adminAuditService'

import { describeAuditMetadata } from './auditMetadata'
import {
  AuditOutcomeBadge,
  formatUtc,
  resourceTypeLabel,
  sourceLabel,
} from './auditPresentation'
import { operationLabel } from './operationLabels'

/** Shown for client info the instance did not capture (#1512). */
export const NOT_CAPTURED = 'Not captured'

export interface AuditEventDrawerProps {
  /** The event to show; `null` closes the drawer. */
  event: AdminAuditEvent | null
  onClose: () => void
}

/**
 * Every field of one audit event (#1517): who, through which channel and
 * client, what it touched and what changed. Changes read old → new whatever
 * shape the recorder used; the metadata is also there as raw JSON.
 */
export function AuditEventDrawer({
  event,
  onClose,
}: Readonly<AuditEventDrawerProps>) {
  return (
    <Sheet
      open={event !== null}
      onOpenChange={open => {
        if (!open) onClose()
      }}
    >
      <SheetContent
        className="w-full overflow-y-auto focus:outline-none sm:max-w-lg"
        // Radix would focus the first control, the "Raw metadata" toggle, and
        // draw a focus ring on it the moment the drawer opens. Focus the sheet
        // itself instead; Tab still reaches every control.
        onOpenAutoFocus={event => {
          event.preventDefault()
          if (event.currentTarget instanceof HTMLElement) {
            event.currentTarget.focus()
          }
        }}
      >
        {event && <AuditEventDetail event={event} />}
      </SheetContent>
    </Sheet>
  )
}

function AuditEventDetail({ event }: Readonly<{ event: AdminAuditEvent }>) {
  const { changes, details } = describeAuditMetadata(event.metadata)
  const channel =
    event.source === 'mcp' && event.assistant_name
      ? `MCP · ${event.assistant_name}`
      : sourceLabel(event.source)

  return (
    <div className="space-y-6">
      <SheetHeader className="pr-6 text-left">
        <div className="flex flex-wrap items-center gap-2">
          <SheetTitle>{operationLabel(event.operation)}</SheetTitle>
          <AuditOutcomeBadge outcome={event.outcome} />
        </div>
        <SheetDescription>
          {formatUtc(event.created_at)} ·{' '}
          <RelativeTime value={event.created_at} />
        </SheetDescription>
      </SheetHeader>

      <Section title="Who">
        <Field label="Actor">{actorText(event)}</Field>
        <Field label="Actor type">{event.actor_type}</Field>
        <Field label="Email">{event.actor_email || '—'}</Field>
        <Field label="User ID" mono>
          {event.actor_user_id ?? '—'}
        </Field>
      </Section>

      <Section title="How">
        <Field label="Channel">{channel}</Field>
        <Field label="MCP client">{event.assistant_name ?? '—'}</Field>
        <Field label="API key">
          {event.api_key_name ?? event.api_key_id ?? '—'}
        </Field>
        <Field label="IP address" mono={event.ip !== null}>
          {event.ip ?? NOT_CAPTURED}
        </Field>
        <Field label="User agent">{event.user_agent ?? NOT_CAPTURED}</Field>
        <Field label="Request ID" mono>
          {event.request_id ?? '—'}
        </Field>
      </Section>

      <Section title="What">
        <Field label="Operation" mono>
          {event.operation}
        </Field>
        <Field label="Resource">
          {event.resource_type
            ? `${resourceTypeLabel(event.resource_type)} · ${event.resource_name ?? event.resource_id ?? ''}`
            : '—'}
        </Field>
        <Field label="Resource ID" mono>
          {event.resource_id ?? '—'}
        </Field>
        <Field label="Team">{event.team_name ?? event.team_id ?? '—'}</Field>
        <Field label="Project">
          {event.project_name ?? event.project_id ?? '—'}
        </Field>
      </Section>

      {changes.length > 0 && (
        <Section title="Changes">
          <ul className="space-y-1.5" aria-label="Changes">
            {changes.map(change => (
              <li key={change.field} className="text-sm">
                <span className="font-medium">{change.field}</span>
                <span className="text-muted-foreground">: </span>
                {change.valuesWithheld ? (
                  <span className="text-muted-foreground">
                    changed (content is not recorded)
                  </span>
                ) : (
                  <>
                    <span className="text-muted-foreground line-through decoration-1">
                      {change.old ?? '—'}
                    </span>
                    <span aria-hidden> → </span>
                    <span className="sr-only"> changed to </span>
                    <span>{change.new ?? '—'}</span>
                  </>
                )}
              </li>
            ))}
          </ul>
        </Section>
      )}

      {details.length > 0 && (
        <Section title="Details">
          {details.map(detail => (
            <Field key={detail.key} label={detail.key}>
              {detail.value}
            </Field>
          ))}
        </Section>
      )}

      <Collapsible>
        <CollapsibleTrigger asChild>
          <Button variant="ghost" size="sm" className="-ml-2">
            <ChevronDown className="size-4" aria-hidden />
            Raw metadata
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <pre className="bg-muted mt-2 overflow-x-auto rounded-md p-3 text-xs">
            {JSON.stringify(event.metadata, null, 2)}
          </pre>
        </CollapsibleContent>
      </Collapsible>
    </div>
  )
}

function actorText(event: AdminAuditEvent): string {
  if (event.actor_type === 'system') return 'System'
  if (event.actor_type === 'anonymous') return 'Anonymous'
  return event.actor_name || event.actor_email || 'Unknown user'
}

function Section({
  title,
  children,
}: Readonly<{ title: string; children: ReactNode }>) {
  return (
    <section className="space-y-2">
      <h3 className="text-muted-foreground text-xs font-semibold uppercase tracking-wide">
        {title}
      </h3>
      <div className="space-y-1.5">{children}</div>
    </section>
  )
}

function Field({
  label,
  mono = false,
  children,
}: Readonly<{ label: string; mono?: boolean; children: ReactNode }>) {
  return (
    <dl className="grid grid-cols-[7.5rem_1fr] gap-2 text-sm">
      <dt className="text-muted-foreground">{label}</dt>
      <dd
        className={
          mono
            ? 'break-all font-mono text-xs'
            : 'min-w-0 [overflow-wrap:anywhere]'
        }
      >
        {children}
      </dd>
    </dl>
  )
}
