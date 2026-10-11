import { Lock, Unlock } from 'lucide-react'

import { RelativeTime } from '@/components/RelativeTime'
import {
  Panel,
  PanelBody,
  PanelHeader,
  PanelTitle,
} from '@/components/ui/panel'
import type { ResourceLockEvent } from '@/types/resourceLock'

/** What the history panel renders: the events, or why there are none. */
export interface LockHistoryState {
  /** Newest first, as the server returns them. */
  events: readonly ResourceLockEvent[]
  /** Set when the last load failed. */
  error: string | null
}

/** Shown for an event whose actor's account has since been deleted. */
const DELETED_USER = 'Deleted user'

function LockHistoryEvent({ event }: Readonly<{ event: ResourceLockEvent }>) {
  const locked = event.action === 'lock'
  const Icon = locked ? Lock : Unlock
  return (
    <li
      className="flex gap-2 text-xs"
      data-testid="lock-history-event"
      data-action={event.action}
    >
      <Icon
        aria-hidden="true"
        className="mt-0.5 size-[13px] shrink-0 text-muted-foreground"
      />
      <div className="min-w-0 space-y-0.5">
        <p>
          <span className="font-medium">
            {event.actor?.display_name ?? DELETED_USER}
          </span>{' '}
          {locked ? 'locked' : 'unlocked'}
          <span className="text-muted-foreground">
            {' · '}
            <RelativeTime value={event.created_at} />
          </span>
        </p>
        {event.comment && (
          <p className="whitespace-pre-wrap break-words text-muted-foreground">
            {event.comment}
          </p>
        )}
      </div>
    </li>
  )
}

export function LockHistoryPanel({
  events,
  error,
}: Readonly<LockHistoryState>) {
  return (
    <Panel data-testid="lock-history">
      <PanelHeader>
        <PanelTitle>Lock history</PanelTitle>
      </PanelHeader>
      <PanelBody className="pb-4">
        {error !== null && events.length === 0 ? (
          <p className="text-xs text-muted-foreground">{error}</p>
        ) : (
          <ul className="space-y-3">
            {events.map(event => (
              <LockHistoryEvent key={event.id} event={event} />
            ))}
          </ul>
        )}
      </PanelBody>
    </Panel>
  )
}
