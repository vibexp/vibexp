import { ArrowRight, Calendar, History, Info, RotateCcw } from 'lucide-react'
import { type ComponentType, type ReactNode } from 'react'
import { Link } from 'react-router'

import { CopyChip } from '@/components/CopyChip'
import { RelativeTime } from '@/components/RelativeTime'
import {
  Panel,
  PanelHeader,
  PanelRow,
  PanelTitle,
  usePanelInset,
} from '@/components/ui/panel'
import { cn } from '@/lib/utils'

/* A right-rail metadata panel built entirely on the shared design system:
   the `ui/panel` primitives (so it is a card on a page and flat inside the
   reading page's details column, #890), lucide icons, and semantic token
   utilities (bg-secondary, text-muted-foreground, border, …) — no hardcoded
   colours, so it flips with `.dark` for free. Rows are a hairline-divided
   list: key on the left, value on the right. Created / Updated relative-time
   rows are rendered automatically; pass any leading rows (Type, Status, Slug,
   …) as `MetaRow` / `MetaSlugRow` children. Reusable across every resource
   detail view. */

type IconType = ComponentType<{ className?: string }>

/** A single key/value row. The value sits on the right, hairline above. */
export function MetaRow({
  label,
  children,
  className,
}: Readonly<{
  label: string
  children: ReactNode
  className?: string
}>) {
  return (
    <PanelRow as="li" className={className}>
      <span className="text-muted-foreground shrink-0">{label}</span>
      <span className="flex min-w-0 items-center justify-end gap-2 text-right font-medium">
        {children}
      </span>
    </PanelRow>
  )
}

/**
 * A key/value row whose value is the shared click-to-copy `CopyChip` — the one
 * field people actually grab. The chip is a real `<button>`, so it aligns flush
 * right with the other rows instead of reserving space for a separate copy
 * button, and long slugs truncate without losing the copied value.
 */
export function MetaSlugRow({
  label = 'Slug',
  value,
}: Readonly<{
  label?: string
  value: string
}>) {
  return (
    <PanelRow as="li">
      <span className="text-muted-foreground shrink-0">{label}</span>
      <CopyChip label={label} value={value} />
    </PanelRow>
  )
}

function TimeRow({
  label,
  value,
  icon: Icon,
}: Readonly<{
  label: string
  value: string
  icon: IconType
}>) {
  return (
    <MetaRow label={label}>
      <span className="inline-flex items-center gap-1.5">
        <Icon className="size-[13px] text-muted-foreground" />
        <RelativeTime value={value} />
      </span>
    </MetaRow>
  )
}

/**
 * Optional version-history affordance for resources that support versioning.
 * When supplied, the panel renders a "Version" row (when `currentVersion` is
 * given) plus a "View version history" footer link with a count chip.
 * Omit the whole object for resources without versioning — nothing renders.
 *
 * Designed to be resource-agnostic: artifacts pass it today; other resource
 * types can opt in later by passing the same shape.
 */
export interface VersionHistoryMeta {
  /** Number of entries on the linked history page — rendered as the footer chip. */
  count: number
  /** react-router target for the "View version history" footer link. */
  to: string
  /** Current version number — rendered as a "Version" row (vN). Omit to hide the row. */
  currentVersion?: number
  /** ISO timestamp of the latest edit — rendered beside the version number. */
  editedAt?: string
  /** Footer link label. Defaults to "View version history". */
  label?: string
}

interface MetadataPanelProps {
  /** Panel heading. Defaults to "Metadata". */
  title?: string
  /** ISO timestamp; renders a "Created" row when present. */
  createdAt?: string
  /** ISO timestamp; renders an "Updated" row when it differs from createdAt. */
  updatedAt?: string
  /** Leading rows (MetaRow / MetaSlugRow), rendered above Created / Updated. */
  children?: ReactNode
  /**
   * Opt-in version-history footer. When present, appends a "Version" row and a
   * "View version history" link with a count chip. Pass only for resources that
   * support versioning (e.g. artifacts).
   */
  versionHistory?: VersionHistoryMeta
  className?: string
}

/** The current-version row: a monospace `vN` chip, a separator, and the edit time. */
function VersionRow({
  currentVersion,
  editedAt,
}: Readonly<{
  currentVersion: number
  editedAt?: string
}>) {
  return (
    <MetaRow label="Version">
      <code className="rounded-sm bg-secondary px-[7px] py-[2px] font-mono text-xs text-secondary-foreground">
        v{currentVersion}
      </code>
      {editedAt && (
        <>
          <span aria-hidden="true" className="text-border">
            ·
          </span>
          <span className="text-muted-foreground">
            edited <RelativeTime value={editedAt} />
          </span>
        </>
      )}
    </MetaRow>
  )
}

/** The footer link that navigates to the full version-history view. */
function VersionHistoryLink({
  to,
  count,
  label = 'View version history',
}: Readonly<Pick<VersionHistoryMeta, 'to' | 'count' | 'label'>>) {
  const inset = usePanelInset()
  return (
    <Link
      to={to}
      data-testid="metadata-version-history-link"
      aria-label={`${label}, ${String(count)} ${count === 1 ? 'version' : 'versions'}`}
      className={cn(
        inset,
        'flex items-center gap-2 border-t border-border py-3 text-sm font-medium text-foreground transition-colors hover:bg-accent'
      )}
    >
      <History
        aria-hidden="true"
        className="size-[15px] shrink-0 text-muted-foreground"
      />
      {label}
      <span
        aria-hidden="true"
        className="rounded-full bg-secondary px-[7px] py-[3px] font-mono text-xs leading-none text-secondary-foreground"
      >
        {count}
      </span>
      <ArrowRight
        aria-hidden="true"
        className="ml-auto size-[13px] shrink-0 text-muted-foreground"
      />
    </Link>
  )
}

/**
 * The redesigned Metadata widget. Renders an info-led header followed by a
 * hairline-divided row list. Created / Updated rows (with relative-time hover
 * tooltips) are appended automatically after the caller's rows.
 */
export function MetadataPanel({
  title = 'Metadata',
  createdAt,
  updatedAt,
  children,
  versionHistory,
  className,
}: Readonly<MetadataPanelProps>) {
  const createdMs = createdAt ? new Date(createdAt).getTime() : 0
  const updatedMs = updatedAt ? new Date(updatedAt).getTime() : 0
  // The version row already surfaces the edit time ("edited 12m ago"), so a
  // standalone "Updated" row would be redundant when version history carries it.
  const showUpdated =
    updatedAt !== undefined &&
    versionHistory?.editedAt === undefined &&
    (createdAt === undefined || updatedMs !== createdMs)

  return (
    <Panel className={className} data-testid="metadata-panel">
      <PanelHeader>
        <div className="flex min-w-0 items-center gap-2.5">
          <Info className="size-4 shrink-0 text-muted-foreground" />
          <PanelTitle>{title}</PanelTitle>
        </div>
      </PanelHeader>
      <ul className="divide-y divide-border border-t border-border">
        {children}
        {createdAt && (
          <TimeRow label="Created" value={createdAt} icon={Calendar} />
        )}
        {showUpdated && updatedAt && (
          <TimeRow label="Updated" value={updatedAt} icon={RotateCcw} />
        )}
        {versionHistory?.currentVersion !== undefined && (
          <VersionRow
            currentVersion={versionHistory.currentVersion}
            editedAt={versionHistory.editedAt}
          />
        )}
      </ul>
      {versionHistory && (
        <VersionHistoryLink
          to={versionHistory.to}
          count={versionHistory.count}
          label={versionHistory.label}
        />
      )}
    </Panel>
  )
}
