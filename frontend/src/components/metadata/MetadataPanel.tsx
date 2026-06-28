import {
  ArrowRight,
  Calendar,
  Check,
  Copy,
  History,
  Info,
  RotateCcw,
} from 'lucide-react'
import { type ComponentType, type ReactNode } from 'react'
import { Link } from 'react-router-dom'

import { RelativeTime } from '@/components/RelativeTime'
import { Card } from '@/components/ui/card'
import { PanelTitle } from '@/components/ui/panel-title'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import { cn } from '@/lib/utils'

/* A right-rail metadata panel built entirely on the shared design system:
   shadcn Card, lucide icons, and semantic token utilities (bg-secondary,
   text-muted-foreground, border, …) — no hardcoded colours, so it flips with
   `.dark` for free. Rows are a hairline-divided list: key on the left, value on
   the right. Created / Updated relative-time rows are rendered automatically;
   pass any leading rows (Type, Status, Slug, …) as `MetaRow` / `MetaSlugRow`
   children. Reusable across every resource detail view. */

type IconType = ComponentType<{ className?: string }>

/** A single key/value row. The value sits on the right, hairline above. */
export function MetaRow({
  label,
  children,
  className,
}: {
  label: string
  children: ReactNode
  className?: string
}) {
  return (
    <li
      className={cn(
        'flex min-h-12 items-center justify-between gap-4 px-5 py-2.5',
        className
      )}
    >
      <span className="shrink-0 text-sm text-muted-foreground">{label}</span>
      <span className="flex min-w-0 items-center justify-end gap-2 text-right text-sm font-medium">
        {children}
      </span>
    </li>
  )
}

/**
 * A key/value row whose value is a monospace chip that is itself the
 * click-to-copy target — the one field people actually grab. The whole chip is a
 * real `<button>` (keyboard-activatable, with an `aria-label`), so it aligns
 * flush-right with the other rows instead of reserving space for a separate copy
 * button. On copy it swaps to a check affordance for ~1.5s. Long slugs truncate
 * to a single line with a trailing ellipsis — the full value is still copied on
 * click and announced via the `aria-label`/`title`, so nothing is lost.
 */
export function MetaSlugRow({
  label = 'Slug',
  value,
}: {
  label?: string
  value: string
}) {
  const { copied, copy } = useCopyToClipboard()
  const CopyIcon = copied ? Check : Copy
  const action = `Copy ${label.toLowerCase()}`

  return (
    <li className="flex min-h-12 items-center justify-between gap-4 px-5 py-2.5">
      <span className="shrink-0 text-sm text-muted-foreground">{label}</span>
      <button
        type="button"
        onClick={() => {
          copy(value)
        }}
        // Include the value: aria-label overrides the inner <code>, so without it
        // assistive tech would announce the action but not what gets copied.
        aria-label={`${action}: ${value}`}
        title={copied ? 'Copied!' : action}
        className="flex min-w-0 max-w-full cursor-pointer items-center justify-end gap-1.5 rounded-sm bg-secondary px-2 py-[3px] font-mono text-xs text-secondary-foreground transition-colors hover:bg-secondary/80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <code className="min-w-0 truncate font-mono">{value}</code>
        <CopyIcon
          aria-hidden="true"
          className="size-3 shrink-0 text-muted-foreground"
        />
      </button>
    </li>
  )
}

function TimeRow({
  label,
  value,
  icon: Icon,
}: {
  label: string
  value: string
  icon: IconType
}) {
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
}: {
  currentVersion: number
  editedAt?: string
}) {
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
}: Pick<VersionHistoryMeta, 'to' | 'count' | 'label'>) {
  return (
    <Link
      to={to}
      data-testid="metadata-version-history-link"
      aria-label={`${label}, ${String(count)} ${count === 1 ? 'version' : 'versions'}`}
      className="flex items-center gap-2 border-t border-border px-5 py-3 text-sm font-medium text-foreground transition-colors hover:bg-accent"
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
}: MetadataPanelProps) {
  const createdMs = createdAt ? new Date(createdAt).getTime() : 0
  const updatedMs = updatedAt ? new Date(updatedAt).getTime() : 0
  // The version row already surfaces the edit time ("edited 12m ago"), so a
  // standalone "Updated" row would be redundant when version history carries it.
  const showUpdated =
    updatedAt !== undefined &&
    versionHistory?.editedAt === undefined &&
    (createdAt === undefined || updatedMs !== createdMs)

  return (
    <Card
      className={cn('overflow-hidden', className)}
      data-testid="metadata-panel"
    >
      <div className="flex items-center gap-2.5 px-5 pb-4 pt-5">
        <Info className="size-4 shrink-0 text-muted-foreground" />
        <PanelTitle>{title}</PanelTitle>
      </div>
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
    </Card>
  )
}
