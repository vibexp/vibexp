import type { ReactNode } from 'react'

import { Badge } from '@/components/ui/badge'
import type { AdminTeamConfigSource } from '@/services/adminService'

import { sourceLabel } from './teamConfigFormat'

/**
 * Read-only building blocks for the admin team configuration tabs (#1142).
 *
 * Every value on those tabs renders through these, never through a form
 * control, so a tab cannot put an editable input on the page by accident.
 */

/** A label/value grid; pair with `ConfigField` children. */
export function ConfigGrid({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <dl className="grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2 lg:grid-cols-3">
      {children}
    </dl>
  )
}

/** One read-only label + value pair. */
export function ConfigField({
  label,
  children,
}: Readonly<{ label: string; children: ReactNode }>) {
  return (
    <div className="min-w-0">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm break-words">{children}</dd>
    </div>
  )
}

/** "Inherited from instance" or "Team", so the two read differently. */
export function SourceBadge({
  source,
}: Readonly<{ source: AdminTeamConfigSource }>) {
  return (
    <Badge
      variant={source === 'instance' ? 'outline' : 'secondary'}
      className="font-normal"
      data-testid="config-source"
    >
      {sourceLabel(source)}
    </Badge>
  )
}

/** A stored credential's presence — never its value. */
export function SecretState({ configured }: Readonly<{ configured: boolean }>) {
  return configured ? (
    <span>Configured ✓</span>
  ) : (
    <span className="text-muted-foreground">Not set</span>
  )
}

/** A boolean as a small On/Off (or custom-labelled) badge. */
export function OnOffBadge({
  on,
  onLabel = 'On',
  offLabel = 'Off',
}: Readonly<{ on: boolean; onLabel?: string; offLabel?: string }>) {
  return (
    <Badge variant={on ? 'secondary' : 'outline'} className="font-normal">
      {on ? onLabel : offLabel}
    </Badge>
  )
}
