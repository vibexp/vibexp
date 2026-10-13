import type { ReactNode } from 'react'
import { Link } from 'react-router'

import { COPY_CHIP_CLASS, CopyChip } from '@/components/CopyChip'
import { READING_META_ROW_CLASS } from '@/components/patterns/reading-page'
import { RelativeTime } from '@/components/RelativeTime'
import { StatusBadge, type StatusTone } from '@/components/StatusBadge'
import { cn } from '@/lib/utils'

/** The resource's lifecycle state, rendered as the header's status badge. */
export interface ResourceHeaderStatus {
  /** Display text — already humanised by the caller (e.g. "Active"). */
  value: string
  tone?: StatusTone
}

/**
 * How the resource is addressed — the identifier a reader copies to use it
 * elsewhere (a prompt/artifact/blueprint slug). The EDIT page shows it under
 * its title; a reading page leaves it to the rail, where the other details
 * are, so it is not shown twice.
 */
export interface ResourceHeaderAddress {
  /** Names the identifier in the copy affordance. Defaults to "Slug". */
  label?: string
  value: string
  /** `false` renders a plain chip with no copy button. Defaults to `true`. */
  copyable?: boolean
}

/** The resource's version, shown once it has more than one. */
export interface ResourceHeaderVersion {
  /** Current version number — the chip reads "v3". */
  current: number
  /** How many versions the history holds; the chip appears above one. */
  count: number
  /** The version-history page the chip links to. */
  to: string
}

export interface ResourceHeaderMetaProps {
  status?: ResourceHeaderStatus
  /** The resource's type, where the kind has one ("Claude Code", "General"). */
  kindLabel?: string
  address?: ResourceHeaderAddress
  version?: ResourceHeaderVersion
  /** ISO timestamp of the last edit, rendered as "Updated <relative>". */
  updatedAt?: string
  /** Lead paragraph under the badge row (the resource's description). */
  summary?: ReactNode
  /** Kind-specific badges slotted after the status (the prompt's Shared badge). */
  extra?: ReactNode
}

/**
 * The one reading-page header treatment every resource uses (#902): one
 * compact meta line — status badge → kind-specific extras → type → address
 * chip → "Updated <relative>" → version chip — with the summary on its own
 * line beneath.
 *
 * A READING page passes no address: the slug is a detail a reader copies now
 * and then, and it lives with the other details in the rail rather than being
 * shown twice. The status, conversely, is shown here and not in the rail.
 *
 * Pages describe themselves with data (`ResourceReadingPage`'s props) instead
 * of hand-building a `description` node, which is what keeps the detail pages
 * identical — and `ReadingPage` itself domain-free.
 */
export function ResourceHeaderMeta({
  status,
  kindLabel,
  address,
  version,
  updatedAt,
  summary,
  extra,
}: Readonly<ResourceHeaderMetaProps>) {
  const showVersion = version !== undefined && version.count > 1
  // `.some(Boolean)`, not `a ?? b ?? …`: `??` stops at the first non-nullish
  // value, so one present-but-falsy prop would suppress the whole row.
  const hasBadgeRow = [
    status,
    kindLabel,
    address,
    updatedAt,
    extra,
    showVersion,
  ].some(Boolean)

  return (
    <div data-testid="resource-header-meta">
      {hasBadgeRow && (
        <div
          className={cn(READING_META_ROW_CLASS, 'gap-x-2.5 gap-y-1 text-xs')}
        >
          {status && (
            <StatusBadge tone={status.tone}>{status.value}</StatusBadge>
          )}
          {extra}
          {kindLabel && (
            <span className="text-muted-foreground">{kindLabel}</span>
          )}
          {address && <HeaderAddress address={address} />}
          {updatedAt && (
            <span className="text-muted-foreground inline-flex items-center gap-1">
              Updated <RelativeTime value={updatedAt} />
            </span>
          )}
          {showVersion && (
            <Link
              to={version.to}
              className="bg-muted text-muted-foreground hover:text-foreground rounded-sm px-1.5 py-0.5 font-mono"
              aria-label={`Version ${String(version.current)}, view version history`}
            >
              v{version.current}
            </Link>
          )}
        </div>
      )}
      {summary && (
        <p className="text-muted-foreground mt-2 text-sm">{summary}</p>
      )}
    </div>
  )
}

/** The address chip — click-to-copy by default, a plain code chip when not. */
function HeaderAddress({
  address,
}: Readonly<{ address: ResourceHeaderAddress }>) {
  if (address.copyable === false) {
    return (
      <code className={cn(COPY_CHIP_CLASS, 'truncate')}>{address.value}</code>
    )
  }
  return <CopyChip label={address.label} value={address.value} />
}
